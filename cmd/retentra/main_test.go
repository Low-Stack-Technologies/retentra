package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRunCLIHelpLong(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout = %q, want help text", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIHelpShort(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"-h"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Options:") {
		t.Fatalf("stdout = %q, want help text", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLINoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI(nil, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: retentra [--no-parallel] config.yaml [config2.yaml ...]") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCLIUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"--unknown"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown flag "--unknown"`) {
		t.Fatalf("stderr = %q, want unknown flag error", stderr.String())
	}
}

func TestRunCLIAcceptsMultipleConfigs(t *testing.T) {
	restore := stubRunConfig(t, func(_ context.Context, configPath string, out io.Writer) error {
		fmt.Fprintf(out, "ran %s\n", configPath)
		return nil
	})
	defer restore()
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"one.yaml", "two.yaml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, want := range []string{"[one.yaml] ran one.yaml", "[two.yaml] ran two.yaml"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIExpandsGlobs(t *testing.T) {
	restore := stubRunConfig(t, func(_ context.Context, configPath string, out io.Writer) error {
		fmt.Fprintf(out, "ran %s\n", filepath.Base(configPath))
		return nil
	})
	defer restore()
	dir := t.TempDir()
	first := filepath.Join(dir, "first-retentra.yaml")
	second := filepath.Join(dir, "second-retentra.yaml")
	writeFile(t, first)
	writeFile(t, second)
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{filepath.Join(dir, "*-retentra.yaml")}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"first-retentra.yaml", "second-retentra.yaml"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunCLIRejectsUnmatchedGlob(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{filepath.Join(t.TempDir(), "*-retentra.yaml")}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "matched no files") {
		t.Fatalf("stderr = %q, want unmatched glob error", stderr.String())
	}
}

func TestRunCLIRejectsDuplicateConfigPath(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "app-retentra.yaml")
	writeFile(t, config)
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{config, filepath.Join(dir, "*-retentra.yaml")}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "duplicate config path") {
		t.Fatalf("stderr = %q, want duplicate path error", stderr.String())
	}
}

func TestRunCLINoParallelRunsInOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	restore := stubRunConfig(t, func(_ context.Context, configPath string, out io.Writer) error {
		mu.Lock()
		order = append(order, configPath)
		mu.Unlock()
		fmt.Fprintf(out, "ran %s\n", configPath)
		return nil
	})
	defer restore()
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"--no-parallel", "one.yaml", "two.yaml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := strings.Join(order, ","); got != "one.yaml,two.yaml" {
		t.Fatalf("run order = %q, want one.yaml,two.yaml", got)
	}
}

func TestRunCLINoParallelFlagCanFollowConfig(t *testing.T) {
	var mu sync.Mutex
	var order []string
	restore := stubRunConfig(t, func(_ context.Context, configPath string, out io.Writer) error {
		mu.Lock()
		order = append(order, configPath)
		mu.Unlock()
		fmt.Fprintf(out, "ran %s\n", configPath)
		return nil
	})
	defer restore()
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"one.yaml", "--no-parallel", "two.yaml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := strings.Join(order, ","); got != "one.yaml,two.yaml" {
		t.Fatalf("run order = %q, want one.yaml,two.yaml", got)
	}
}

func TestRunCLIParallelRunsAllConfigsAndAggregatesFailures(t *testing.T) {
	var mu sync.Mutex
	var ran []string
	restore := stubRunConfig(t, func(_ context.Context, configPath string, out io.Writer) error {
		mu.Lock()
		ran = append(ran, configPath)
		mu.Unlock()
		fmt.Fprintf(out, "ran %s\n", configPath)
		if configPath == "bad.yaml" {
			return errors.New("backup failed")
		}
		return nil
	})
	defer restore()
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"good.yaml", "bad.yaml", "other.yaml"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	for _, want := range []string{"good.yaml", "bad.yaml", "other.yaml"} {
		if !containsString(ran, want) {
			t.Fatalf("ran = %#v, want %q", ran, want)
		}
	}
	if !strings.Contains(stderr.String(), "[bad.yaml] retentra: backup failed") {
		t.Fatalf("stderr = %q, want prefixed run error", stderr.String())
	}
}

func TestPrefixWriterPrefixesEveryLine(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	writer := newPrefixWriter(&out, "config.yaml", &mu)

	if _, err := writer.Write([]byte("one\ntwo\n\nthree")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	want := "[config.yaml] one\n[config.yaml] two\n[config.yaml] \n[config.yaml] three"
	if out.String() != want {
		t.Fatalf("prefix writer output = %q, want %q", out.String(), want)
	}
}

func TestRunCLITooManyArgsNoLongerErrors(t *testing.T) {
	restore := stubRunConfig(t, func(_ context.Context, _ string, _ io.Writer) error {
		return nil
	})
	defer restore()
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"one.yaml", "two.yaml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
}

func stubRunConfig(t *testing.T, fn func(context.Context, string, io.Writer) error) func() {
	t.Helper()
	previous := runConfig
	runConfig = fn
	return func() {
		runConfig = previous
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("config"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
