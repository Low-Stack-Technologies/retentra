package retentra

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeliverFilesystemOutputCopiesArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "backup.tar.gz")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(dir, "out")
	desc, err := deliverOutput(context.Background(), OutputConfig{Type: "filesystem", Path: outputDir}, archive, "backup.tar.gz")
	if err != nil {
		t.Fatalf("deliverOutput() error = %v", err)
	}
	if desc != filepath.Join(outputDir, "backup.tar.gz") {
		t.Fatalf("desc = %q", desc)
	}
	got, err := os.ReadFile(filepath.Join(outputDir, "backup.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "archive" {
		t.Fatalf("copied archive = %q", got)
	}
}

func TestCopyFileFailureLeavesExistingDestinationUntouched(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "source-dir")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "backup.tar.gz")
	if err := os.WriteFile(destination, []byte("previous archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyFile(sourceDir, destination)
	if err == nil {
		t.Fatal("copyFile() error = nil, want read error")
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "previous archive" {
		t.Fatalf("destination = %q, want previous archive", got)
	}
}

func TestExpandUserPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	got, err := expandUserPath("~/.ssh/id_ed25519")
	if err != nil {
		t.Fatalf("expandUserPath() error = %v", err)
	}
	want := filepath.Join(home, ".ssh", "id_ed25519")
	if got != want {
		t.Fatalf("expandUserPath() = %q, want %q", got, want)
	}
}
