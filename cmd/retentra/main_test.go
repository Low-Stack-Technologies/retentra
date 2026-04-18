package main

import (
	"bytes"
	"strings"
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
	if !strings.Contains(stderr.String(), "usage: retentra config.yaml") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunCLITooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runCLI([]string{"one.yaml", "two.yaml"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: retentra config.yaml") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}
