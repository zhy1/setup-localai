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
	"flag"
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

type installOptions struct {
	InstallCodex  bool
	InstallClaude bool
}

func parseInstallOptions(args []string) (installOptions, error) {
	var opts installOptions
	fs := flag.NewFlagSet("setup-localai", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.InstallCodex, "install-codex", false, "install the codex binary")
	fs.BoolVar(&opts.InstallClaude, "install-claude", false, "install the claude binary")
	if err := fs.Parse(args); err != nil {
		return installOptions{}, err
	}
	return opts, nil
}

func loadVarsFromFile(path string) (toolkitVars, error) {
	var v toolkitVars
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return toolkitVars{}, readErr
	}
	if unmarshalErr := json.Unmarshal(content, &v); unmarshalErr != nil {
		return toolkitVars{}, fmt.Errorf("vars.json parse failed for %s: %w", path, unmarshalErr)
	}
	return v, nil
}

func mergeVars(base toolkitVars, override toolkitVars) toolkitVars {
	if override.ApiKey != "" {
		base.ApiKey = override.ApiKey
	}
	if override.BaseUrl != "" {
		base.BaseUrl = override.BaseUrl
	}
	if override.Model != "" {
		base.Model = override.Model
	}
	if override.Codex != nil {
		if base.Codex == nil {
			base.Codex = map[string]releaseAsset{}
		}
		for platform, asset := range override.Codex {
			if existing, ok := base.Codex[platform]; ok {
				if asset.Url != "" {
					existing.Url = asset.Url
				}
				if asset.Sha256 != "" {
					existing.Sha256 = asset.Sha256
				}
				if asset.Version != "" {
					existing.Version = asset.Version
				}
				if asset.BinName != "" {
					existing.BinName = asset.BinName
				}
				if asset.Archive != "" {
					existing.Archive = asset.Archive
				}
				base.Codex[platform] = existing
			} else {
				base.Codex[platform] = asset
			}
		}
	}
	if override.Claude != nil {
		if base.Claude == nil {
			base.Claude = map[string]releaseAsset{}
		}
		for platform, asset := range override.Claude {
			if existing, ok := base.Claude[platform]; ok {
				if asset.Url != "" {
					existing.Url = asset.Url
				}
				if asset.Sha256 != "" {
					existing.Sha256 = asset.Sha256
				}
				if asset.Version != "" {
					existing.Version = asset.Version
				}
				if asset.BinName != "" {
					existing.BinName = asset.BinName
				}
				if asset.Archive != "" {
					existing.Archive = asset.Archive
				}
				base.Claude[platform] = existing
			} else {
				base.Claude[platform] = asset
			}
		}
	}
	return base
}

func loadVars() toolkitVars {
	var v toolkitVars
	if unmarshalErr := json.Unmarshal(embeddedVars, &v); unmarshalErr != nil {
		fmt.Fprintln(os.Stderr, "embedded vars.json parse failed:", unmarshalErr)
		os.Exit(1)
	}

	candidatePaths := []string{}
	if envPath := strings.TrimSpace(os.Getenv("SETUP_LOCALAI_VARS_JSON")); envPath != "" {
		candidatePaths = append(candidatePaths, envPath)
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		candidatePaths = append(candidatePaths, filepath.Join(cwd, "vars.json"), filepath.Join(cwd, "assets", "vars.json"))
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr == nil {
		candidatePaths = append(candidatePaths, filepath.Join(homeDir, "vars.json"))
	}

	for _, candidatePath := range candidatePaths {
		if candidatePath == "" {
			continue
		}
		overrideVars, readErr := loadVarsFromFile(candidatePath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			fmt.Fprintln(os.Stderr, "override vars.json load failed:", readErr)
			os.Exit(1)
		}
		v = mergeVars(v, overrideVars)
		break
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

func ensurePathExportInFile(rcPath string, installDir string, shellName string) error {
	if rcPath == "" {
		return nil
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(rcPath), 0o755); mkdirErr != nil {
		return mkdirErr
	}

	exportLine := fmt.Sprintf("export PATH=\"%s:$PATH\"", installDir)
	if _, statErr := os.Stat(rcPath); statErr == nil {
		content, readErr := os.ReadFile(rcPath)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), exportLine) {
			return nil
		}
	}

	comment := fmt.Sprintf("# Added by setup-localai for %s", shellName)
	var builder strings.Builder
	if _, statErr := os.Stat(rcPath); statErr == nil {
		content, readErr := os.ReadFile(rcPath)
		if readErr != nil {
			return readErr
		}
		builder.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			builder.WriteByte('\n')
		}
	} else {
		builder.WriteString("")
	}
	builder.WriteByte('\n')
	builder.WriteString(comment)
	builder.WriteByte('\n')
	builder.WriteString(exportLine)
	builder.WriteByte('\n')
	return atomicWrite(rcPath, []byte(builder.String()))
}

func syncFileWithBackup(targetPath string, newContent []byte) error {
	oldContent, readErr := os.ReadFile(targetPath)
	if readErr == nil {
		if md5Hex(oldContent) == md5Hex(newContent) {
			return nil
		}

		backupPath := targetPath + "." + md5Hex(oldContent)
		if _, statErr := os.Stat(backupPath); statErr == nil {
			candidate := backupPath
			for idx := 1; ; idx++ {
				candidate = fmt.Sprintf("%s.%d", backupPath, idx)
				if _, statErr := os.Stat(candidate); os.IsNotExist(statErr) {
					backupPath = candidate
					break
				}
			}
		}
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
		var fallback []byte
		for _, entry := range reader.File {
			name := filepath.Base(entry.Name)
			if strings.EqualFold(name, wantedBinName) {
				fh, openErr := entry.Open()
				if openErr != nil {
					return nil, openErr
				}
				defer fh.Close()
				return io.ReadAll(fh)
			}
			if fallback == nil && isLikelyExecutable(name) {
				fh, openErr := entry.Open()
				if openErr != nil {
					return nil, openErr
				}
				defer fh.Close()
				content, readErr := io.ReadAll(fh)
				if readErr == nil {
					fallback = content
				}
			}
		}
		if fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("binary %s not found in zip", wantedBinName)
	case "tar.gz":
		gzReader, gzErr := gzip.NewReader(bytes.NewReader(rawData))
		if gzErr != nil {
			return nil, gzErr
		}
		defer gzReader.Close()
		tarReader := tar.NewReader(gzReader)
		var fallback []byte
		for {
			header, nextErr := tarReader.Next()
			if nextErr == io.EOF {
				break
			}
			if nextErr != nil {
				return nil, nextErr
			}
			name := filepath.Base(header.Name)
			if strings.EqualFold(name, wantedBinName) {
				return io.ReadAll(tarReader)
			}
			if fallback == nil && isLikelyExecutable(name) && header.Typeflag == tar.TypeReg {
				content, readErr := io.ReadAll(tarReader)
				if readErr == nil {
					fallback = content
				}
			}
		}
		if fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("binary %s not found in tar.gz", wantedBinName)
	default:
		return rawData, nil
	}
}

func isLikelyExecutable(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if base == "" {
		return false
	}
	if strings.Contains(base, "claude") || strings.Contains(base, "codex") {
		return true
	}
	return strings.HasSuffix(base, ".exe") || strings.HasSuffix(base, ".bin")
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
	opts, err := parseInstallOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse install options failed:", err)
		os.Exit(1)
	}

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

	if opts.InstallCodex {
		if codexAsset, ok := vars.Codex[currentOs]; ok {
			codexPath, installErr := ensureBinaryInstalled("codex", codexAsset, installDir)
			if installErr != nil {
				fmt.Fprintln(os.Stderr, "codex install failed:", installErr)
			} else {
				fmt.Println("codex ready ->", codexPath, "version", codexAsset.Version)
			}
		}
	} else {
		fmt.Println("codex install skipped; pass --install-codex to enable")
	}

	if opts.InstallClaude {
		if claudeAsset, ok := vars.Claude[currentOs]; ok {
			claudePath, installErr := ensureBinaryInstalled("claude-code", claudeAsset, installDir)
			if installErr != nil {
				fmt.Fprintln(os.Stderr, "claude-code install failed:", installErr)
			} else {
				fmt.Println("claude-code ready ->", claudePath, "version", claudeAsset.Version)
			}
		}
	} else {
		fmt.Println("claude install skipped; pass --install-claude to enable")
	}

	if mkdirErr := os.MkdirAll(installDir, 0o755); mkdirErr != nil {
		fmt.Fprintln(os.Stderr, "install dir prepare failed:", mkdirErr)
		os.Exit(1)
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	shellName := "shell"
	rcCandidates := []string{}
	if shell != "" {
		base := filepath.Base(shell)
		switch base {
		case "bash":
			rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".bashrc"))
			shellName = "bash"
		case "zsh":
			rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".zshrc"))
			shellName = "zsh"
		case "sh":
			rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".profile"))
			shellName = "sh"
		default:
			rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".profile"))
		}
	} else {
		rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".profile"))
	}
	if len(rcCandidates) == 0 || rcCandidates[0] != filepath.Join(homeDir, ".profile") {
		rcCandidates = append(rcCandidates, filepath.Join(homeDir, ".profile"))
	}

	for _, rcPath := range rcCandidates {
		if updateErr := ensurePathExportInFile(rcPath, installDir, shellName); updateErr != nil {
			fmt.Fprintln(os.Stderr, "path export update failed for", rcPath, ":", updateErr)
		} else {
			fmt.Println("shell config updated ->", rcPath)
		}
	}

	fmt.Println("installation complete; reload your shell or run: source ~/.bashrc")
}
