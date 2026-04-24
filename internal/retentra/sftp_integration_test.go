//go:build integration

package retentra

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSFTPOutputUploadsArchive(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}

	dir := t.TempDir()
	uploadDir := filepath.Join(dir, "upload")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(dir, "backup.tar.gz")
	archiveBytes := []byte("archive bytes")
	if err := os.WriteFile(archivePath, archiveBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	containerID := dockerOutput(t,
		"run", "-d",
		"-p", "127.0.0.1::22",
		"-v", uploadDir+":/home/retentra/upload",
		"atmoz/sftp:latest",
		"retentra:secret:::upload",
	)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	})

	host, port := dockerSFTPAddress(t, containerID)
	waitForSFTP(t, host, port)

	output := OutputConfig{
		Type:     "sftp",
		Host:     host,
		Port:     port,
		Username: "retentra",
		Password: "secret",
		// This integration test uses a short-lived throwaway container.
		Insecure:   true,
		RemotePath: "/upload",
	}
	if err := deliverOutputEventually(output, archivePath, "backup.tar.gz"); err != nil {
		t.Fatalf("deliverOutput() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(uploadDir, "backup.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, archiveBytes) {
		t.Fatalf("uploaded bytes = %q, want %q", got, archiveBytes)
	}
}

func dockerOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

func dockerSFTPAddress(t *testing.T, containerID string) (string, int) {
	t.Helper()
	mapping := dockerOutput(t, "port", containerID, "22/tcp")
	parts := strings.Split(strings.TrimSpace(mapping), ":")
	if len(parts) < 2 {
		t.Fatalf("unexpected docker port output: %q", mapping)
	}
	var port int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &port); err != nil {
		t.Fatalf("parse docker port from %q: %v", mapping, err)
	}
	host := strings.Join(parts[:len(parts)-1], ":")
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return host, port
}

func waitForSFTP(t *testing.T, host string, port int) {
	t.Helper()
	address := fmt.Sprintf("%s:%d", host, port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("sftp container did not open %s", address)
}

func deliverOutputEventually(output OutputConfig, archivePath, archiveName string) error {
	var lastErr error
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, err := deliverOutput(context.Background(), output, archivePath, archiveName)
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return lastErr
}
