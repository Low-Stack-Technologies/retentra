package retentra

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func deliverOutput(output OutputConfig, archivePath, archiveName string) (string, error) {
	switch output.Type {
	case "filesystem":
		if err := os.MkdirAll(output.Path, 0o755); err != nil {
			return "", err
		}
		destination := filepath.Join(output.Path, archiveName)
		if err := copyFile(archivePath, destination); err != nil {
			return "", err
		}
		return destination, nil
	case "sftp":
		if err := uploadSFTP(output, archivePath, archiveName); err != nil {
			return "", err
		}
		return fmt.Sprintf("sftp://%s/%s", output.Host, filepath.ToSlash(filepath.Join(output.RemotePath, archiveName))), nil
	default:
		return "", fmt.Errorf("output type %q is unsupported", output.Type)
	}
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dir := filepath.Dir(destination)
	dst, err := os.CreateTemp(dir, "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := dst.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func uploadSFTP(output OutputConfig, archivePath, archiveName string) error {
	auth, err := sftpAuth(output)
	if err != nil {
		return err
	}
	hostKeyCallback, err := sftpHostKeyCallback(output)
	if err != nil {
		return err
	}
	sshConfig := &ssh.ClientConfig{
		User:            output.Username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}
	address := net.JoinHostPort(output.Host, fmt.Sprintf("%d", output.Port))
	conn, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.MkdirAll(output.RemotePath); err != nil {
		return err
	}
	remotePath := filepath.ToSlash(filepath.Join(output.RemotePath, archiveName))
	tempRemotePath := filepath.ToSlash(filepath.Join(output.RemotePath, fmt.Sprintf(".%s.tmp-%d", archiveName, time.Now().UnixNano())))
	remoteFile, err := client.Create(tempRemotePath)
	if err != nil {
		return err
	}
	cleanupRemote := true
	defer func() {
		if cleanupRemote {
			_ = client.Remove(tempRemotePath)
		}
	}()

	localFile, err := os.Open(archivePath)
	if err != nil {
		_ = remoteFile.Close()
		return err
	}
	defer localFile.Close()
	if _, err := io.Copy(remoteFile, localFile); err != nil {
		_ = remoteFile.Close()
		return err
	}
	if err := remoteFile.Close(); err != nil {
		return err
	}
	if err := client.Rename(tempRemotePath, remotePath); err != nil {
		return err
	}
	cleanupRemote = false
	return nil
}

func sftpAuth(output OutputConfig) (ssh.AuthMethod, error) {
	if output.Password != "" {
		return ssh.Password(output.Password), nil
	}
	identityFile, err := expandUserPath(output.IdentityFile)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(identityFile)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func sftpHostKeyCallback(output OutputConfig) (ssh.HostKeyCallback, error) {
	if output.Insecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	knownHosts := output.KnownHosts
	if knownHosts == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		knownHosts = filepath.Join(home, ".ssh", "known_hosts")
	}
	knownHosts, err := expandUserPath(knownHosts)
	if err != nil {
		return nil, err
	}
	callback, err := knownhosts.New(knownHosts)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	return callback, nil
}

func expandUserPath(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}
