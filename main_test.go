package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSingleBinaryFallsBackToPrefixedBinaryName(t *testing.T) {
	payload := []byte("fake-binary")
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	header := &tar.Header{Name: "codex-x86_64-unknown-linux-musl", Mode: 0o755, Size: int64(len(payload))}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatalf("write tar payload: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	got, err := extractSingleBinary("tar.gz", buf.Bytes(), "codex")
	if err != nil {
		t.Fatalf("extractSingleBinary() unexpected error: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("extractSingleBinary() payload mismatch: got %q want %q", got, payload)
	}
}

func TestSyncFileWithBackupCreatesBackupForChangedContent(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "service.conf")
	oldContent := []byte("old-content")
	newContent := []byte("new-content")

	if err := os.WriteFile(targetPath, oldContent, 0o600); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	if err := syncFileWithBackup(targetPath, newContent); err != nil {
		t.Fatalf("syncFileWithBackup returned error: %v", err)
	}

	backupPath := targetPath + "." + md5Hex(oldContent)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file to exist: %v", err)
	}

	updated, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if string(updated) != string(newContent) {
		t.Fatalf("expected updated content %q, got %q", newContent, updated)
	}
}

func TestSyncFileWithBackupAddsNumericSuffixWhenBackupAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "service.conf")
	oldContent := []byte("old-content")

	if err := os.WriteFile(targetPath, oldContent, 0o600); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	backupPath := targetPath + "." + md5Hex(oldContent)
	if err := os.WriteFile(backupPath, []byte("existing-backup"), 0o600); err != nil {
		t.Fatalf("write existing backup file: %v", err)
	}

	if err := syncFileWithBackup(targetPath, []byte("new-content")); err != nil {
		t.Fatalf("syncFileWithBackup returned error: %v", err)
	}

	backupPathWithSuffix := backupPath + ".1"
	if _, err := os.Stat(backupPathWithSuffix); err != nil {
		t.Fatalf("expected backup file with suffix to exist: %v", err)
	}
}

func TestEnsurePathExportInFileAddsEntryOnce(t *testing.T) {
	tmpDir := t.TempDir()
	rcPath := filepath.Join(tmpDir, ".bashrc")
	installDir := filepath.Join(tmpDir, "bin")

	if err := ensurePathExportInFile(rcPath, installDir, "bash"); err != nil {
		t.Fatalf("ensurePathExportInFile returned error: %v", err)
	}

	content, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read shell rc file: %v", err)
	}

	line := `export PATH="` + installDir + `:$PATH"`
	if !strings.Contains(string(content), line) {
		t.Fatalf("expected shell rc content to contain %q, got %q", line, string(content))
	}

	if err := ensurePathExportInFile(rcPath, installDir, "bash"); err != nil {
		t.Fatalf("second ensurePathExportInFile returned error: %v", err)
	}

	contentAgain, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read shell rc file again: %v", err)
	}
	if strings.Count(string(contentAgain), line) != 1 {
		t.Fatalf("expected PATH export to be written once, got %d occurrences", strings.Count(string(contentAgain), line))
	}
}

func TestParseInstallOptionsDefaultsToFalse(t *testing.T) {
	opts, err := parseInstallOptions([]string{})
	if err != nil {
		t.Fatalf("parseInstallOptions returned error: %v", err)
	}
	if opts.InstallCodex || opts.InstallClaude {
		t.Fatalf("expected install flags to default to false, got %+v", opts)
	}
}

func TestParseInstallOptionsEnablesRequestedTools(t *testing.T) {
	opts, err := parseInstallOptions([]string{"--install-codex", "--install-claude"})
	if err != nil {
		t.Fatalf("parseInstallOptions returned error: %v", err)
	}
	if !opts.InstallCodex || !opts.InstallClaude {
		t.Fatalf("expected requested tools to be enabled, got %+v", opts)
	}
}

func TestLoadVarsPrefersExternalOverrideFile(t *testing.T) {
	tmpDir := t.TempDir()
	overridePath := filepath.Join(tmpDir, "vars.json")
	overrideContent := []byte(`{"api_key":"override-key","base_url":"https://override.test/v1","model":"override-model"}`)
	if err := os.WriteFile(overridePath, overrideContent, 0o600); err != nil {
		t.Fatalf("write override vars: %v", err)
	}

	t.Setenv("SETUP_LOCALAI_VARS_JSON", overridePath)

	vars := loadVars()
	if vars.ApiKey != "override-key" {
		t.Fatalf("expected api_key override, got %q", vars.ApiKey)
	}
	if vars.BaseUrl != "https://override.test/v1" {
		t.Fatalf("expected base_url override, got %q", vars.BaseUrl)
	}
	if vars.Model != "override-model" {
		t.Fatalf("expected model override, got %q", vars.Model)
	}
	if vars.Codex["linux"].Version != "latest" {
		t.Fatalf("expected embedded codex defaults to remain available, got %q", vars.Codex["linux"].Version)
	}
}
