package retentra

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	googleFileKeyFileName     = "file_key"
	googleFileCipherMagic     = "RETENTRA-GOOGLE-FILE-ENC\n"
	googleFileCipherVersion   = byte(1)
	googleFileKeySize         = 32
	googleFileCipherNonceSize = 12
)

func googleFileKeyPath(settings googleSettings) (string, error) {
	if settings.configDir == "" {
		return "", fmt.Errorf("google token directory is empty")
	}
	if err := os.MkdirAll(settings.configDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(settings.configDir, googleFileKeyFileName), nil
}

func loadGoogleFileKey(settings googleSettings) ([]byte, error) {
	path, err := googleFileKeyPath(settings)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(data)))
	if err != nil {
		return nil, fmt.Errorf("read google file key: %w", err)
	}
	if len(key) != googleFileKeySize {
		return nil, fmt.Errorf("google file key has invalid length")
	}
	return key, nil
}

func ensureGoogleFileKey(settings googleSettings) ([]byte, error) {
	key, err := loadGoogleFileKey(settings)
	if err == nil {
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	path, err := googleFileKeyPath(settings)
	if err != nil {
		return nil, err
	}
	key = make([]byte, googleFileKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := []byte(base64.StdEncoding.EncodeToString(key))
	if err := writeGoogleAtomicFile(path, encoded); err != nil {
		return nil, err
	}
	return key, nil
}

func readGoogleEncryptedJSONFile(path string, settings googleSettings, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if isGoogleEncryptedPayload(data) {
		plain, err := decryptGooglePayload(settings, data)
		if err != nil {
			return false, err
		}
		return false, json.Unmarshal(plain, out)
	}
	return true, json.Unmarshal(data, out)
}

func writeGoogleEncryptedJSONFile(path string, settings googleSettings, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encrypted, err := encryptGooglePayload(settings, data)
	if err != nil {
		return err
	}
	return writeGoogleAtomicFile(path, encrypted)
}

func isGoogleEncryptedPayload(data []byte) bool {
	return len(data) > len(googleFileCipherMagic)+1 && bytes.HasPrefix(data, []byte(googleFileCipherMagic))
}

func encryptGooglePayload(settings googleSettings, plain []byte) ([]byte, error) {
	key, err := ensureGoogleFileKey(settings)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	cipherText := aead.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(googleFileCipherMagic)+1+len(nonce)+len(cipherText))
	out = append(out, []byte(googleFileCipherMagic)...)
	out = append(out, googleFileCipherVersion)
	out = append(out, nonce...)
	out = append(out, cipherText...)
	return out, nil
}

func decryptGooglePayload(settings googleSettings, data []byte) ([]byte, error) {
	key, err := loadGoogleFileKey(settings)
	if err != nil {
		return nil, err
	}
	prefixLen := len(googleFileCipherMagic)
	if len(data) < prefixLen+1+googleFileCipherNonceSize {
		return nil, fmt.Errorf("google encrypted payload is truncated")
	}
	if !bytes.HasPrefix(data, []byte(googleFileCipherMagic)) {
		return nil, fmt.Errorf("google encrypted payload has invalid header")
	}
	if data[prefixLen] != googleFileCipherVersion {
		return nil, fmt.Errorf("google encrypted payload has unsupported version")
	}
	nonce := data[prefixLen+1 : prefixLen+1+googleFileCipherNonceSize]
	cipherText := data[prefixLen+1+googleFileCipherNonceSize:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, cipherText, nil)
}

func writeGoogleAtomicFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
