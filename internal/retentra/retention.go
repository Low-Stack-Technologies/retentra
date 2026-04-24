package retentra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

type retentionEntry struct {
	name    string
	modTime time.Time
}

func applyOutputRetention(ctx context.Context, output OutputConfig, archiveNameTemplate string) error {
	if output.Retention.KeepLast == 0 {
		return nil
	}
	matcher, err := archiveNameMatcher(archiveNameTemplate)
	if err != nil {
		return err
	}
	switch output.Type {
	case "filesystem":
		return pruneFilesystemOutput(output.Path, matcher, output.Retention.KeepLast)
	case "sftp":
		return pruneSFTPOutput(output, matcher, output.Retention.KeepLast)
	case "gdrive":
		return pruneGoogleDriveOutput(ctx, output, matcher, output.Retention.KeepLast)
	default:
		return fmt.Errorf("output type %q is unsupported", output.Type)
	}
}

func archiveNameMatcher(template string) (*regexp.Regexp, error) {
	pattern := regexp.QuoteMeta(template)
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{date}"), `\d{4}-\d{2}-\d{2}`)
	return regexp.Compile("^" + pattern + "$")
}

func pruneFilesystemOutput(dir string, matcher *regexp.Regexp, keepLast int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var matches []retentionEntry
	for _, entry := range entries {
		if entry.IsDir() || !matcher.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		matches = append(matches, retentionEntry{name: entry.Name(), modTime: info.ModTime()})
	}
	sortRetentionEntries(matches)
	for _, entry := range matches[retentionDeleteStart(len(matches), keepLast):] {
		if err := os.Remove(filepath.Join(dir, entry.name)); err != nil {
			return err
		}
	}
	return nil
}

func pruneSFTPOutput(output OutputConfig, matcher *regexp.Regexp, keepLast int) error {
	return withSFTPClient(output, func(client *sftp.Client) error {
		entries, err := client.ReadDir(output.RemotePath)
		if err != nil {
			return err
		}
		var matches []retentionEntry
		for _, entry := range entries {
			if entry.IsDir() || !matcher.MatchString(entry.Name()) {
				continue
			}
			matches = append(matches, retentionEntry{name: entry.Name(), modTime: entry.ModTime()})
		}
		sortRetentionEntries(matches)
		for _, entry := range matches[retentionDeleteStart(len(matches), keepLast):] {
			if err := client.Remove(filepath.ToSlash(filepath.Join(output.RemotePath, entry.name))); err != nil {
				return err
			}
		}
		return nil
	})
}

func retentionDeleteStart(total, keepLast int) int {
	if keepLast >= total {
		return total
	}
	return keepLast
}

func sortRetentionEntries(entries []retentionEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].name > entries[j].name
		}
		return entries[i].modTime.After(entries[j].modTime)
	})
}
