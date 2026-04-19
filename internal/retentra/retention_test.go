package retentra

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyFilesystemRetentionKeepsNewestMatchingArchives(t *testing.T) {
	dir := t.TempDir()
	writeRetentionFile(t, dir, "backup-2026-04-17.tar.gz", time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC))
	writeRetentionFile(t, dir, "backup-2026-04-18.tar.gz", time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC))
	writeRetentionFile(t, dir, "backup-2026-04-19.tar.gz", time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC))
	writeRetentionFile(t, dir, "notes.txt", time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC))

	output := OutputConfig{
		Type:      "filesystem",
		Path:      dir,
		Retention: RetentionConfig{KeepLast: 2},
	}
	if err := applyOutputRetention(output, "backup-{date}.tar.gz"); err != nil {
		t.Fatalf("applyOutputRetention() error = %v", err)
	}

	assertRetentionFileMissing(t, dir, "backup-2026-04-17.tar.gz")
	assertRetentionFileExists(t, dir, "backup-2026-04-18.tar.gz")
	assertRetentionFileExists(t, dir, "backup-2026-04-19.tar.gz")
	assertRetentionFileExists(t, dir, "notes.txt")
}

func TestApplyFilesystemRetentionDisabled(t *testing.T) {
	dir := t.TempDir()
	writeRetentionFile(t, dir, "backup-2026-04-17.tar.gz", time.Now())

	output := OutputConfig{Type: "filesystem", Path: dir}
	if err := applyOutputRetention(output, "backup-{date}.tar.gz"); err != nil {
		t.Fatalf("applyOutputRetention() error = %v", err)
	}

	assertRetentionFileExists(t, dir, "backup-2026-04-17.tar.gz")
}

func writeRetentionFile(t *testing.T, dir, name string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func assertRetentionFileExists(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("%s should exist: %v", name, err)
	}
}

func assertRetentionFileMissing(t *testing.T, dir, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("%s should be removed, stat error = %v", name, err)
	}
}
