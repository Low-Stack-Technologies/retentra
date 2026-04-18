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

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
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
	remoteFile, err := client.Create(filepath.ToSlash(filepath.Join(output.RemotePath, archiveName)))
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer localFile.Close()
	_, err = io.Copy(remoteFile, localFile)
	return err
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
