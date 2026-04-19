package retentra

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsExpandedArchiveNamePath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	config := `
report:
  title: Backup Report
sources:
  - type: filesystem
    label: Site files
    path: /does/not/matter
    target: data
archive:
  name: "{tmpdir}.tar"
  format: tar
  compression: none
outputs:
  - type: filesystem
    label: Local copy
    path: /does/not/matter
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run(context.Background(), configPath, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() error = nil, want unsafe expanded archive name error")
	}
	if !strings.Contains(err.Error(), "archive.name") {
		t.Fatalf("Run() error = %v, want archive.name error", err)
	}
}

func TestRunBackupAttemptsLaterOutputsAfterFailure(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("backup data"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	successOutput := filepath.Join(dir, "successful-output")
	cfg := Config{
		Report:  ReportConfig{Title: "Backup Report"},
		Sources: []SourceConfig{{Type: "filesystem", Label: "Source", Path: source, Target: "source.txt"}},
		Archive: ArchiveConfig{Name: "backup.tar", Format: "tar", Compression: "none"},
		Outputs: []OutputConfig{
			{Type: "filesystem", Label: "Broken output", Path: filepath.Join(blocker, "child")},
			{Type: "filesystem", Label: "Successful output", Path: successOutput},
		},
	}
	status := Status{}
	archivePath := filepath.Join(dir, "backup.tar")

	err := runBackup(context.Background(), cfg, placeholders{}, archivePath, "backup.tar", &bytes.Buffer{}, &status)
	if err == nil {
		t.Fatal("runBackup() error = nil, want first output failure")
	}
	if len(status.OutputResults) != 2 {
		t.Fatalf("OutputResults = %#v, want both outputs recorded", status.OutputResults)
	}
	if status.OutputResults[0].Success() {
		t.Fatalf("first output result = %#v, want failure", status.OutputResults[0])
	}
	if !status.OutputResults[1].Success() {
		t.Fatalf("second output result = %#v, want success", status.OutputResults[1])
	}
	if _, err := os.Stat(filepath.Join(successOutput, "backup.tar")); err != nil {
		t.Fatalf("successful output was not delivered: %v", err)
	}
}
