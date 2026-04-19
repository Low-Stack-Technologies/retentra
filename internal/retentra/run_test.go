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
