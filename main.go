package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"
)

//go:embed assets/vars.json
var embeddedVars []byte

//go:embed assets/codex_config.toml.tmpl
var codexTemplateRaw string

//go:embed assets/claude_json.tmpl
var claudeTemplateRaw string

type releaseAsset struct {
	Url     string `json:"url"`
	Sha256  string `json:"sha256"`
	Version string `json:"version"`
	BinName string `json:"bin_name"`
	Archive string `json:"archive"`
}

type toolkitVars struct {
	ApiKey  string                  `json:"api_key"`
	BaseUrl string                  `json:"base_url"`
	Model   string                  `json:"model"`
	Codex   map[string]releaseAsset `json:"codex"`
	Claude  map[string]releaseAsset `json:"claude"`
}

func loadVars() toolkitVars {
	var v toolkitVars
	if unmarshalErr := json.Unmarshal(embeddedVars, &v); unmarshalErr != nil {
		fmt.Fprintln(os.Stderr, "embedded vars.json parse failed:", unmarshalErr)
		os.Exit(1)
	}
	return v
}

func renderTemplate(name string, raw string, data toolkitVars) []byte {
	tmpl, parseErr := template.New(name).Parse(raw)
	if parseErr != nil {
		fmt.Fprintln(os.Stderr, "template parse failed:", parseErr)
		os.Exit(1)
	}
	buf := &bytes.Buffer{}
	if execErr := tmpl.Execute(buf, data); execErr != nil {
		fmt.Fprintln(os.Stderr, "template exec failed:", execErr)
		os.Exit(1)
	}
	return buf.Bytes()
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func atomicWrite(targetPath string, content []byte) error {
	dir := filepath.Dir(targetPath)
	if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
		return mkdirErr
	}
	tempFile := filepath.Join(dir, fmt.Sprintf(".%s.tmp-%d", filepath.Base(targetPath), time.Now().UnixNano()))
	if writeErr := os.WriteFile(tempFile, content, 0o600); writeErr != nil {
		return writeErr
	}
	return os.Rename(tempFile, targetPath)
}

func syncFileWithBackup(targetPath string, newContent []byte) error {
	oldContent, readErr := os.ReadFile(targetPath)
	if readErr == nil {
		if md5Hex(oldContent) == md5Hex(newContent) {
			return nil
		}
		backupPath := targetPath + "." + md5Hex(oldContent)
		if renameErr := os.Rename(targetPath, backupPath); renameErr != nil {
			return renameErr
		}
	}
	return atomicWrite(targetPath, newContent)
}

func osKey() string {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return runtime.GOOS
	default:
		return runtime.GOOS
	}
}

func downloadBytes(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, getErr := client.Get(url)
	if getErr != nil {
		return nil, getErr
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed status=%d url=%s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func extractSingleBinary(archiveType string, rawData []byte, wantedBinName string) ([]byte, error) {
	switch archiveType {
	case "zip":
		reader, zipErr := zip.NewReader(bytes.NewReader(rawData), int64(len(rawData)))
		if zipErr != nil {
			return nil, zipErr
		}
		for _, entry := range reader.File {
			if strings.EqualFold(filepath.Base(entry.Name), wantedBinName) {
				fh, openErr := entry.Open()
				if openErr != nil {
					return nil, openErr
				}
				defer fh.Close()
				return io.ReadAll(fh)
			}
		}
		return nil, fmt.Errorf("binary %s not found in zip", wantedBinName)
	case "tar.gz":
		gzReader, gzErr := gzip.NewReader(bytes.NewReader(rawData))
		if gzErr != nil {
			return nil, gzErr
		}
		defer gzReader.Close()
		tarReader := tar.NewReader(gzReader)
		for {
			header, nextErr := tarReader.Next()
			if nextErr == io.EOF {
				break
			}
			if nextErr != nil {
				return nil, nextErr
			}
			if strings.EqualFold(filepath.Base(header.Name), wantedBinName) {
				return io.ReadAll(tarReader)
			}
		}
		return nil, fmt.Errorf("binary %s not found in tar.gz", wantedBinName)
	default:
		return rawData, nil
	}
}

func ensureBinaryInstalled(toolLabel string, asset releaseAsset, installDir string) (string, error) {
	binaryName := asset.BinName
	if runtime.GOOS == "windows" && !strings.HasSuffix(binaryName, ".exe") {
		binaryName += ".exe"
	}
	pointerPath := filepath.Join(installDir, binaryName)

	if existing, statErr := os.ReadFile(pointerPath); statErr == nil {
		if sha256Hex(existing) == asset.Sha256 {
			return pointerPath, nil
		}
	}

	rawData, downloadErr := downloadBytes(asset.Url)
	if downloadErr != nil {
		return "", fmt.Errorf("%s download error: %w", toolLabel, downloadErr)
	}

	binData, extractErr := extractSingleBinary(asset.Archive, rawData, asset.BinName)
	if extractErr != nil {
		return "", fmt.Errorf("%s extract error: %w", toolLabel, extractErr)
	}

	actualSum := sha256Hex(binData)
	if asset.Sha256 != "" && actualSum != asset.Sha256 {
		return "", fmt.Errorf("%s sha256 mismatch expected=%s actual=%s", toolLabel, asset.Sha256, actualSum)
	}

	contentAddressedPath := filepath.Join(installDir, fmt.Sprintf("%s.%s", binaryName, actualSum[:12]))
	if mkdirErr := os.MkdirAll(installDir, 0o755); mkdirErr != nil {
		return "", mkdirErr
	}
	if writeErr := atomicWrite(contentAddressedPath, binData); writeErr != nil {
		return "", writeErr
	}
	if chmodErr := os.Chmod(contentAddressedPath, 0o755); chmodErr != nil && runtime.GOOS != "windows" {
		return "", chmodErr
	}

	_ = os.Remove(pointerPath)
	linkErr := os.Symlink(contentAddressedPath, pointerPath)
	if linkErr != nil {
		copyData, readErr := os.ReadFile(contentAddressedPath)
		if readErr != nil {
			return "", readErr
		}
		if writeErr := atomicWrite(pointerPath, copyData); writeErr != nil {
			return "", writeErr
		}
		_ = os.Chmod(pointerPath, 0o755)
	}
	return pointerPath, nil
}

func main() {
	vars := loadVars()
	currentOs := osKey()

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		fmt.Fprintln(os.Stderr, "cannot resolve home dir:", homeErr)
		os.Exit(1)
	}

	installDir := filepath.Join(homeDir, ".kimi-toolkit", "bin")

	codexConfigPath := filepath.Join(homeDir, ".codex", "config.toml")
	claudeConfigPath := filepath.Join(homeDir, ".claude.json")

	renderedCodexToml := renderTemplate("codex", codexTemplateRaw, vars)
	renderedClaudeJson := renderTemplate("claude", claudeTemplateRaw, vars)

	if syncErr := syncFileWithBackup(codexConfigPath, renderedCodexToml); syncErr != nil {
		fmt.Fprintln(os.Stderr, "codex config sync failed:", syncErr)
		os.Exit(1)
	}
	fmt.Println("codex config synced ->", codexConfigPath)

	if syncErr := syncFileWithBackup(claudeConfigPath, renderedClaudeJson); syncErr != nil {
		fmt.Fprintln(os.Stderr, "claude config sync failed:", syncErr)
		os.Exit(1)
	}
	fmt.Println("claude config synced ->", claudeConfigPath)

	if codexAsset, ok := vars.Codex[currentOs]; ok {
		codexPath, installErr := ensureBinaryInstalled("codex", codexAsset, installDir)
		if installErr != nil {
			fmt.Fprintln(os.Stderr, "codex install failed:", installErr)
		} else {
			fmt.Println("codex ready ->", codexPath, "version", codexAsset.Version)
		}
	}

	if claudeAsset, ok := vars.Claude[currentOs]; ok {
		claudePath, installErr := ensureBinaryInstalled("claude-code", claudeAsset, installDir)
		if installErr != nil {
			fmt.Fprintln(os.Stderr, "claude-code install failed:", installErr)
		} else {
			fmt.Println("claude-code ready ->", claudePath, "version", claudeAsset.Version)
		}
	}

	fmt.Println("add to PATH manually if not already present ->", installDir)
}
