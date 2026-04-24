package retentra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	googleDriveStateFileName  = "drive_state.json"
	googleDriveRootFolderName = "retentra"
)

func googleDriveStatePath(settings googleSettings) (string, error) {
	if settings.configDir == "" {
		return "", fmt.Errorf("google token directory is empty")
	}
	if err := os.MkdirAll(settings.configDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(settings.configDir, googleDriveStateFileName), nil
}

func loadGoogleDriveState(settings googleSettings) (string, googleDriveState, error) {
	path, err := googleDriveStatePath(settings)
	if err != nil {
		return "", googleDriveState{}, err
	}
	state, err := readGoogleDriveState(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, googleDriveState{Folders: map[string]string{}}, nil
		}
		return "", googleDriveState{}, err
	}
	if state.Folders == nil {
		state.Folders = map[string]string{}
	}
	return path, state, nil
}

func saveGoogleDriveState(path string, state googleDriveState) error {
	return writeGoogleDriveState(path, state)
}

func readGoogleDriveState(path string) (googleDriveState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return googleDriveState{}, err
	}
	var state googleDriveState
	if err := json.Unmarshal(data, &state); err != nil {
		return googleDriveState{}, err
	}
	return state, nil
}

func writeGoogleDriveState(path string, state googleDriveState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if state.Folders == nil {
		state.Folders = map[string]string{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
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
