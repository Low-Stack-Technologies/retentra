package retentra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type googleCredentialStore interface {
	Mode() string
	Available() error
	Exists() (bool, error)
	Load(ctx context.Context) (googleTokenRecord, error)
	Save(ctx context.Context, record googleTokenRecord) error
	Delete(ctx context.Context) error
}

type googleTokenFileStore struct {
	path string
}

type googleOSSecretStore struct {
	settings googleSettings
}

const (
	googleSecretStoreAppName     = "retentra"
	googleSecretStoreKind        = "google-oauth"
	googleSecretStoreServiceName = "com.retentra.google.oauth"
	googleSecretStoreLabel       = "retentra Google OAuth"
)

var runGoogleCommand = execGoogleCommand

func newGoogleTokenFileStore(settings googleSettings) (googleCredentialStore, error) {
	path, err := googleTokenCachePath(settings)
	if err != nil {
		return nil, err
	}
	return googleTokenFileStore{path: path}, nil
}

func newGoogleOSSecretStore(settings googleSettings) googleCredentialStore {
	return googleOSSecretStore{settings: settings}
}

func resolveGoogleCredentialStoreForLogin(settings googleSettings, allowFileTokenStorage bool) (googleCredentialStore, string, error) {
	if allowFileTokenStorage {
		store, err := newGoogleTokenFileStore(settings)
		if err != nil {
			return nil, "", err
		}
		return store, googleCredentialStorageFile, nil
	}
	store := newGoogleOSSecretStore(settings)
	if err := store.Available(); err != nil {
		return nil, "", fmt.Errorf("google auth requires access to an OS secret store: %w (use --allow-file-token-storage to fall back to files)", err)
	}
	return store, googleCredentialStorageSecret, nil
}

func resolveGoogleCredentialStoreForStatus(settings googleSettings, state googleDriveState) (googleCredentialStore, string, error) {
	return resolveGoogleCredentialStoreForAccess(settings, state)
}

func resolveGoogleCredentialStoreForAccess(settings googleSettings, state googleDriveState) (googleCredentialStore, string, error) {
	if state.CredentialStorage == googleCredentialStorageFile {
		store, err := newGoogleTokenFileStore(settings)
		if err != nil {
			return nil, "", err
		}
		return store, googleCredentialStorageFile, nil
	}
	if state.CredentialStorage == googleCredentialStorageSecret {
		store := newGoogleOSSecretStore(settings)
		if err := store.Available(); err != nil {
			return nil, "", err
		}
		return store, googleCredentialStorageSecret, nil
	}

	fileStore, err := newGoogleTokenFileStore(settings)
	if err != nil {
		return nil, "", err
	}
	if ok, err := fileStore.Exists(); err != nil {
		return nil, "", err
	} else if ok {
		if secretStore := newGoogleOSSecretStore(settings); secretStore.Available() == nil {
			return migrateLegacyGoogleTokenStore(context.Background(), settings, fileStore, secretStore)
		}
		return fileStore, googleCredentialStorageFile, nil
	}

	secretStore := newGoogleOSSecretStore(settings)
	if err := secretStore.Available(); err != nil {
		return fileStore, googleCredentialStorageFile, nil
	}
	return secretStore, googleCredentialStorageSecret, nil
}

func migrateLegacyGoogleTokenStore(ctx context.Context, settings googleSettings, fileStore googleCredentialStore, secretStore googleCredentialStore) (googleCredentialStore, string, error) {
	record, err := fileStore.Load(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := secretStore.Save(ctx, record); err != nil {
		return nil, "", err
	}
	if err := fileStore.Delete(ctx); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return nil, "", err
	}
	state.ClientID = settings.clientID
	state.CredentialStorage = googleCredentialStorageSecret
	if err := saveGoogleDriveState(statePath, state); err != nil {
		return nil, "", err
	}
	return secretStore, googleCredentialStorageSecret, nil
}

func (s googleTokenFileStore) Mode() string { return googleCredentialStorageFile }

func (s googleTokenFileStore) Available() error { return nil }

func (s googleTokenFileStore) Exists() (bool, error) {
	_, err := os.Stat(s.path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s googleTokenFileStore) Load(ctx context.Context) (googleTokenRecord, error) {
	_, record, err := readGoogleTokenRecord(s.path)
	return record, err
}

func (s googleTokenFileStore) Save(ctx context.Context, record googleTokenRecord) error {
	return writeGoogleTokenRecord(s.path, record)
}

func (s googleTokenFileStore) Delete(ctx context.Context) error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s googleOSSecretStore) Mode() string { return googleCredentialStorageSecret }

func (s googleOSSecretStore) Available() error {
	switch runtime.GOOS {
	case "linux":
		_, err := exec.LookPath("secret-tool")
		return err
	case "darwin":
		_, err := exec.LookPath("security")
		return err
	case "windows":
		return fmt.Errorf("Windows secret store support is not implemented")
	default:
		return fmt.Errorf("OS secret store is not supported on %s", runtime.GOOS)
	}
}

func (s googleOSSecretStore) Exists() (bool, error) {
	_, err := s.lookup(context.Background())
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s googleOSSecretStore) Load(ctx context.Context) (googleTokenRecord, error) {
	raw, err := s.lookup(ctx)
	if err != nil {
		if googleSecretStoreNotFound(err) {
			return googleTokenRecord{}, os.ErrNotExist
		}
		return googleTokenRecord{}, err
	}
	var record googleTokenRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return googleTokenRecord{}, err
	}
	return record, nil
}

func (s googleOSSecretStore) Save(ctx context.Context, record googleTokenRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.store(ctx, string(data))
}

func (s googleOSSecretStore) Delete(ctx context.Context) error {
	switch runtime.GOOS {
	case "linux":
		_, err := runGoogleCommand(ctx, "secret-tool", []string{
			"clear",
			"app", googleSecretStoreAppName,
			"kind", googleSecretStoreKind,
			"client_id", s.settings.clientID,
		}, "")
		if googleSecretStoreNotFound(err) {
			return nil
		}
		return err
	case "darwin":
		_, err := runGoogleCommand(ctx, "security", []string{
			"delete-generic-password",
			"-s", googleSecretStoreServiceName,
			"-a", s.settings.clientID,
		}, "")
		if googleSecretStoreNotFound(err) {
			return nil
		}
		return err
	case "windows":
		return fmt.Errorf("Windows secret store support is not implemented")
	default:
		return fmt.Errorf("OS secret store is not supported on %s", runtime.GOOS)
	}
}

func (s googleOSSecretStore) lookup(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "linux":
		return runGoogleCommand(ctx, "secret-tool", []string{
			"lookup",
			"app", googleSecretStoreAppName,
			"kind", googleSecretStoreKind,
			"client_id", s.settings.clientID,
		}, "")
	case "darwin":
		return runGoogleCommand(ctx, "security", []string{
			"find-generic-password",
			"-s", googleSecretStoreServiceName,
			"-a", s.settings.clientID,
			"-w",
		}, "")
	case "windows":
		return "", fmt.Errorf("Windows secret store support is not implemented")
	default:
		return "", fmt.Errorf("OS secret store is not supported on %s", runtime.GOOS)
	}
}

func (s googleOSSecretStore) store(ctx context.Context, secret string) error {
	switch runtime.GOOS {
	case "linux":
		_, err := runGoogleCommand(ctx, "secret-tool", []string{
			"store",
			"--label=" + googleSecretStoreLabel,
			"app", googleSecretStoreAppName,
			"kind", googleSecretStoreKind,
			"client_id", s.settings.clientID,
		}, secret)
		return err
	case "darwin":
		_, err := runGoogleCommand(ctx, "security", []string{
			"add-generic-password",
			"-U",
			"-s", googleSecretStoreServiceName,
			"-a", s.settings.clientID,
			"-l", googleSecretStoreLabel,
			"-w", secret,
		}, "")
		return err
	case "windows":
		return fmt.Errorf("Windows secret store support is not implemented")
	default:
		return fmt.Errorf("OS secret store is not supported on %s", runtime.GOOS)
	}
}

func execGoogleCommand(ctx context.Context, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func googleSecretStoreNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not be found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such secret") ||
		strings.Contains(msg, "item not found")
}
