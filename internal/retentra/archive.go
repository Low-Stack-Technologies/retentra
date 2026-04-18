package retentra

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func createArchive(path string, cfg ArchiveConfig, items []archiveItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	switch cfg.Format {
	case "tar":
		var writer io.WriteCloser = file
		if cfg.Compression == "gzip" {
			gzipWriter := gzip.NewWriter(file)
			defer gzipWriter.Close()
			writer = gzipWriter
		}
		tarWriter := tar.NewWriter(writer)
		defer tarWriter.Close()
		return writeTarItems(tarWriter, items)
	case "zip":
		zipWriter := zip.NewWriter(file)
		defer zipWriter.Close()
		return writeZipItems(zipWriter, cfg.Compression, items)
	default:
		return fmt.Errorf("archive format %q is unsupported", cfg.Format)
	}
}

func writeTarItems(writer *tar.Writer, items []archiveItem) error {
	for _, item := range items {
		if err := walkArchiveItem(item, func(path, target string, info os.FileInfo) error {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = target
			if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return writeFileBody(writer, path)
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeZipItems(writer *zip.Writer, compression string, items []archiveItem) error {
	for _, item := range items {
		if err := walkArchiveItem(item, func(path, target string, info os.FileInfo) error {
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = target
			if info.IsDir() {
				if !strings.HasSuffix(header.Name, "/") {
					header.Name += "/"
				}
			} else if compression == "none" {
				header.Method = zip.Store
			} else {
				header.Method = zip.Deflate
			}
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return writeFileBody(entry, path)
		}); err != nil {
			return err
		}
	}
	return nil
}

func walkArchiveItem(item archiveItem, visit func(path, target string, info os.FileInfo) error) error {
	target := cleanArchiveTarget(item.Target)
	info, err := os.Stat(item.Path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return visit(item.Path, target, info)
	}

	return filepath.Walk(item.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(item.Path, path)
		if err != nil {
			return err
		}
		entryTarget := target
		if rel != "." {
			entryTarget = filepath.ToSlash(filepath.Join(target, rel))
		}
		return visit(path, entryTarget, info)
	})
}

func cleanArchiveTarget(target string) string {
	return path.Clean(strings.ReplaceAll(target, "\\", "/"))
}

func writeFileBody(writer io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}
