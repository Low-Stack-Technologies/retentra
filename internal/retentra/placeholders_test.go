package retentra

import (
	"testing"
	"time"
)

func TestPlaceholdersExpandDateAndTmpdir(t *testing.T) {
	p := placeholders{tmpdir: "/tmp/retentra-123", now: time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)}

	got := p.expand("{tmpdir}/backup-{date}.tar.gz")
	want := "/tmp/retentra-123/backup-2026-04-17.tar.gz"
	if got != want {
		t.Fatalf("expand() = %q, want %q", got, want)
	}
}
