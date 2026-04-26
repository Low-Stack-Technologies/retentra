package retentra

import (
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
	state, plaintext, err := readGoogleDriveState(path, settings)
	if err != nil {
		if os.IsNotExist(err) {
			return path, googleDriveState{Folders: map[string]string{}}, nil
		}
		return "", googleDriveState{}, err
	}
	if state.Folders == nil {
		state.Folders = map[string]string{}
	}
	if plaintext {
		if err := writeGoogleDriveState(path, settings, state); err != nil {
			return "", googleDriveState{}, err
		}
	}
	return path, state, nil
}

func saveGoogleDriveState(path string, settings googleSettings, state googleDriveState) error {
	return writeGoogleDriveState(path, settings, state)
}

func readGoogleDriveState(path string, settings googleSettings) (googleDriveState, bool, error) {
	var state googleDriveState
	plaintext, err := readGoogleEncryptedJSONFile(path, settings, &state)
	if err != nil {
		return googleDriveState{}, false, err
	}
	return state, plaintext, nil
}

func writeGoogleDriveState(path string, settings googleSettings, state googleDriveState) error {
	if state.Folders == nil {
		state.Folders = map[string]string{}
	}
	return writeGoogleEncryptedJSONFile(path, settings, state)
}
