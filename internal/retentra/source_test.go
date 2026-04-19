package retentra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectSourcesRunsCommandWithTmpdir(t *testing.T) {
	tmpdir := t.TempDir()
	source := SourceConfig{
		Type:     "command",
		Workdir:  "{tmpdir}",
		Commands: []string{`mkdir -p "{tmpdir}/export" && printf data > "{tmpdir}/export/file.txt"`},
		Collect:  []CollectConfig{{Label: "Generated file", Path: "{tmpdir}/export/file.txt", Target: "generated/file.txt"}},
	}
	status := Status{}

	items, err := collectSources(context.Background(), []SourceConfig{source}, placeholders{tmpdir: tmpdir, now: time.Now()}, &status)
	if err != nil {
		t.Fatalf("collectSources() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Path != filepath.Join(tmpdir, "export", "file.txt") || items[0].Target != "generated/file.txt" {
		t.Fatalf("items[0] = %#v", items[0])
	}
	if _, err := os.Stat(items[0].Path); err != nil {
		t.Fatal(err)
	}
	if len(status.SourceResults) != 1 || status.SourceResults[0].Label != "Generated file" || !status.SourceResults[0].Success() {
		t.Fatalf("SourceResults = %#v, want generated file success", status.SourceResults)
	}
}

func TestCommandSourceFailureDoesNotLeakOutput(t *testing.T) {
	tmpdir := t.TempDir()
	source := SourceConfig{
		Type:     "command",
		Workdir:  "{tmpdir}",
		Commands: []string{`printf 'SECRET_TOKEN=abc123\n' >&2; exit 1`},
		Collect:  []CollectConfig{{Label: "Generated file", Path: "{tmpdir}/export/file.txt", Target: "generated/file.txt"}},
	}

	err := runCommandSource(context.Background(), source, placeholders{tmpdir: tmpdir, now: time.Now()})
	if err == nil {
		t.Fatal("runCommandSource() error = nil, want command failure")
	}
	if strings.Contains(err.Error(), "SECRET_TOKEN") || strings.Contains(err.Error(), "abc123") {
		t.Fatalf("runCommandSource() error leaked command output: %v", err)
	}
	if !strings.Contains(err.Error(), "commands[0] failed") {
		t.Fatalf("runCommandSource() error = %v, want command index", err)
	}
}
