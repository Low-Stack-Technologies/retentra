package retentra

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateTarGzipArchiveWithTargets(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(sourceDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "nested", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "backup.tar.gz")
	err := createArchive(archivePath, ArchiveConfig{Format: "tar", Compression: "gzip"}, []archiveItem{{Path: sourceDir, Target: "app/data"}})
	if err != nil {
		t.Fatalf("createArchive() error = %v", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "app/data/nested/file.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("archive did not contain app/data/nested/file.txt")
	}
}

func TestCreateZipArchiveWithTargets(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "db.sqlite")
	if err := os.WriteFile(source, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "backup.zip")
	err := createArchive(archivePath, ArchiveConfig{Format: "zip", Compression: "none"}, []archiveItem{{Path: source, Target: "app/db.sqlite"}})
	if err != nil {
		t.Fatalf("createArchive() error = %v", err)
	}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "app/db.sqlite" {
		t.Fatalf("zip entries = %#v, want app/db.sqlite", zr.File)
	}
}

func TestCreateArchiveRejectsSymlinkedFile(t *testing.T) {
	for _, archive := range []ArchiveConfig{
		{Format: "tar", Compression: "gzip"},
		{Format: "zip", Compression: "none"},
	} {
		t.Run(archive.Format, func(t *testing.T) {
			dir := t.TempDir()
			sourceDir := filepath.Join(dir, "source")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "secret.txt")
			if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(sourceDir, "linked-secret.txt")); err != nil {
				if errors.Is(err, os.ErrPermission) {
					t.Skipf("symlink not permitted: %v", err)
				}
				t.Fatal(err)
			}

			err := createArchive(filepath.Join(dir, "backup.out"), archive, []archiveItem{{Path: sourceDir, Target: "data"}})
			if err == nil || !strings.Contains(err.Error(), "symlinks are not supported") {
				t.Fatalf("createArchive() error = %v, want symlink error", err)
			}
		})
	}
}

func TestCreateArchiveRejectsSymlinkedDirectory(t *testing.T) {
	for _, archive := range []ArchiveConfig{
		{Format: "tar", Compression: "gzip"},
		{Format: "zip", Compression: "none"},
	} {
		t.Run(archive.Format, func(t *testing.T) {
			dir := t.TempDir()
			sourceDir := filepath.Join(dir, "source")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			targetDir := filepath.Join(dir, "external")
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(targetDir, filepath.Join(sourceDir, "linked-dir")); err != nil {
				if errors.Is(err, os.ErrPermission) {
					t.Skipf("symlink not permitted: %v", err)
				}
				t.Fatal(err)
			}

			err := createArchive(filepath.Join(dir, "backup.out"), archive, []archiveItem{{Path: sourceDir, Target: "data"}})
			if err == nil || !strings.Contains(err.Error(), "symlinks are not supported") {
				t.Fatalf("createArchive() error = %v, want symlink error", err)
			}
		})
	}
}

func TestCreateArchiveRejectsRootSymlinkSource(t *testing.T) {
	for _, archive := range []ArchiveConfig{
		{Format: "tar", Compression: "gzip"},
		{Format: "zip", Compression: "none"},
	} {
		t.Run(archive.Format, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target.txt")
			if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "source-link")
			if err := os.Symlink(target, source); err != nil {
				if errors.Is(err, os.ErrPermission) {
					t.Skipf("symlink not permitted: %v", err)
				}
				t.Fatal(err)
			}

			err := createArchive(filepath.Join(dir, "backup.out"), archive, []archiveItem{{Path: source, Target: "data.txt"}})
			if err == nil || !strings.Contains(err.Error(), "symlinks are not supported") {
				t.Fatalf("createArchive() error = %v, want symlink error", err)
			}
		})
	}
}
