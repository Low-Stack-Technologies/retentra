package retentra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func uploadGoogleDrive(ctx context.Context, output OutputConfig, archivePath, archiveName string) (string, error) {
	settings := loadGoogleSettings()
	token, err := googleAccessToken(ctx)
	if err != nil {
		return "", err
	}

	folderID, err := resolveGoogleDriveFolder(ctx, settings, token, output.Path)
	if err != nil {
		return "", err
	}
	file, err := uploadGoogleDriveFile(ctx, settings, token, folderID, archivePath, archiveName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("gdrive://%s/%s", normalizeGoogleDrivePath(output.Path), file.Name), nil
}

func pruneGoogleDriveOutput(ctx context.Context, output OutputConfig, matcher *regexp.Regexp, keepLast int) error {
	settings := loadGoogleSettings()
	token, err := googleAccessToken(ctx)
	if err != nil {
		return err
	}
	folderID, err := resolveGoogleDriveFolder(ctx, settings, token, output.Path)
	if err != nil {
		return err
	}
	items, err := listGoogleDriveFiles(ctx, settings, token, folderID)
	if err != nil {
		return err
	}
	var matches []retentionEntry
	for _, item := range items {
		if item.MimeType == googleFolderMimeType || !matcher.MatchString(item.Name) {
			continue
		}
		modTime, err := time.Parse(time.RFC3339, item.ModifiedTime)
		if err != nil {
			return fmt.Errorf("parse Drive modifiedTime for %q: %w", item.Name, err)
		}
		matches = append(matches, retentionEntry{name: item.Name, modTime: modTime})
	}
	sortRetentionEntries(matches)
	for _, entry := range matches[retentionDeleteStart(len(matches), keepLast):] {
		id, err := findGoogleDriveFileID(ctx, settings, token, folderID, entry.name)
		if err != nil {
			return err
		}
		if err := deleteGoogleDriveFile(ctx, settings, token, id); err != nil {
			return err
		}
	}
	return nil
}

type googleDriveUploadMetadata struct {
	Name    string   `json:"name"`
	Parents []string `json:"parents,omitempty"`
}

func uploadGoogleDriveFile(ctx context.Context, settings googleSettings, token, folderID, archivePath, archiveName string) (googleDriveFile, error) {
	info, err := os.Stat(archivePath)
	if err != nil {
		return googleDriveFile{}, err
	}

	initURL := settings.uploadURL + "/files?uploadType=resumable&fields=id,name,mimeType,modifiedTime"
	meta := googleDriveUploadMetadata{Name: archiveName, Parents: []string{folderID}}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return googleDriveFile{}, err
	}

	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, bytes.NewReader(metaJSON))
	if err != nil {
		return googleDriveFile{}, err
	}
	initReq.ContentLength = int64(len(metaJSON))
	initReq.Header.Set("Authorization", "Bearer "+token)
	initReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	initReq.Header.Set("X-Upload-Content-Type", "application/octet-stream")
	initReq.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", info.Size()))

	initResp, err := http.DefaultClient.Do(initReq)
	if err != nil {
		return googleDriveFile{}, err
	}
	defer initResp.Body.Close()
	if initResp.StatusCode < 200 || initResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(initResp.Body, 1<<20))
		return googleDriveFile{}, fmt.Errorf("init resumable upload: %s: %s", initResp.Status, strings.TrimSpace(string(body)))
	}
	sessionURL := initResp.Header.Get("Location")
	if sessionURL == "" {
		return googleDriveFile{}, fmt.Errorf("init resumable upload: missing Location header")
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return googleDriveFile{}, err
	}
	defer archive.Close()

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURL, archive)
	if err != nil {
		return googleDriveFile{}, err
	}
	uploadReq.ContentLength = info.Size()
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadReq.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", info.Size()-1, info.Size()))

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return googleDriveFile{}, err
	}
	defer uploadResp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(uploadResp.Body, 1<<20))
	if err != nil {
		return googleDriveFile{}, err
	}
	if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
		return googleDriveFile{}, fmt.Errorf("upload file: %s: %s", uploadResp.Status, strings.TrimSpace(string(data)))
	}
	var file googleDriveFile
	if err := json.Unmarshal(data, &file); err != nil {
		return googleDriveFile{}, err
	}
	return file, nil
}

func resolveGoogleDriveFolder(ctx context.Context, settings googleSettings, token, drivePath string) (string, error) {
	cleaned := normalizeGoogleDrivePath(drivePath)
	if cleaned == "" {
		return "", fmt.Errorf("drive path is empty")
	}
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return "", err
	}
	if state.ClientID != settings.clientID {
		state = googleDriveState{
			ClientID:          settings.clientID,
			CredentialStorage: state.CredentialStorage,
			Folders:           map[string]string{},
		}
	}
	rootID, err := ensureGoogleDriveRootFolder(ctx, settings, token, statePath, &state)
	if err != nil {
		return "", err
	}
	parentID := rootID
	currentPath := ""
	for _, part := range strings.Split(cleaned, "/") {
		currentPath = filepath.ToSlash(filepath.Join(currentPath, part))
		if id, ok := state.Folders[currentPath]; ok {
			exists, err := googleDriveFileExists(ctx, settings, token, id)
			if err != nil {
				return "", err
			}
			if exists {
				parentID = id
				continue
			}
			delete(state.Folders, currentPath)
		}
		childID, err := createGoogleDriveFolder(ctx, settings, token, parentID, part)
		if err != nil {
			return "", err
		}
		state.Folders[currentPath] = childID
		parentID = childID
	}
	if err := writeGoogleDriveState(statePath, state); err != nil {
		return "", err
	}
	return parentID, nil
}

func normalizeGoogleDrivePath(p string) string {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
	if cleaned == "." {
		return ""
	}
	return strings.Trim(cleaned, "/")
}

func createGoogleDriveFolder(ctx context.Context, settings googleSettings, token, parentID, name string) (string, error) {
	payload := map[string]any{
		"name":     name,
		"mimeType": googleFolderMimeType,
		"parents":  []string{parentID},
	}
	rawURL := settings.apiBaseURL + "/files?fields=id,name"
	var file googleDriveFile
	if err := googleDriveRequestJSON(ctx, http.MethodPost, rawURL, token, payload, &file); err != nil {
		return "", err
	}
	if file.ID == "" {
		return "", fmt.Errorf("create Drive folder %q: missing file id", name)
	}
	return file.ID, nil
}

func ensureGoogleDriveRootFolder(ctx context.Context, settings googleSettings, token, statePath string, state *googleDriveState) (string, error) {
	if state.RootFolderID != "" {
		exists, err := googleDriveFileExists(ctx, settings, token, state.RootFolderID)
		if err != nil {
			return "", err
		}
		if exists {
			return state.RootFolderID, nil
		}
	}
	id, err := createGoogleDriveFolder(ctx, settings, token, "root", googleDriveRootFolderName)
	if err != nil {
		return "", err
	}
	state.RootFolderID = id
	state.Folders = map[string]string{}
	if err := writeGoogleDriveState(statePath, *state); err != nil {
		return "", err
	}
	return id, nil
}

func googleDriveFileExists(ctx context.Context, settings googleSettings, token, id string) (bool, error) {
	rawURL := settings.apiBaseURL + "/files/" + url.PathEscape(id) + "?fields=id"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return false, fmt.Errorf("get Drive file %s: %s: %s", id, resp.Status, strings.TrimSpace(string(body)))
	}
	return true, nil
}

func listGoogleDriveFiles(ctx context.Context, settings googleSettings, token, parentID string) ([]googleDriveFile, error) {
	var all []googleDriveFile
	pageToken := ""
	for {
		values := url.Values{}
		values.Set("q", fmt.Sprintf("trashed=false and '%s' in parents", parentID))
		values.Set("fields", "nextPageToken,files(id,name,mimeType,modifiedTime)")
		values.Set("spaces", "drive")
		values.Set("pageSize", "1000")
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		rawURL := settings.apiBaseURL + "/files?" + values.Encode()
		var list googleDriveFileList
		if err := googleDriveRequestJSON(ctx, http.MethodGet, rawURL, token, nil, &list); err != nil {
			return nil, err
		}
		all = append(all, list.Files...)
		if list.NextPageToken == "" {
			return all, nil
		}
		pageToken = list.NextPageToken
	}
}

func findGoogleDriveFileID(ctx context.Context, settings googleSettings, token, parentID, name string) (string, error) {
	values := url.Values{}
	values.Set("q", fmt.Sprintf("trashed=false and '%s' in parents and name='%s'", parentID, escapeDriveQueryString(name)))
	values.Set("fields", "files(id,name)")
	values.Set("spaces", "drive")
	values.Set("pageSize", "10")
	rawURL := settings.apiBaseURL + "/files?" + values.Encode()
	var list googleDriveFileList
	if err := googleDriveRequestJSON(ctx, http.MethodGet, rawURL, token, nil, &list); err != nil {
		return "", err
	}
	if len(list.Files) == 0 {
		return "", fmt.Errorf("Drive file %q not found for retention", name)
	}
	return list.Files[0].ID, nil
}

func deleteGoogleDriveFile(ctx context.Context, settings googleSettings, token, id string) error {
	rawURL := settings.apiBaseURL + "/files/" + url.PathEscape(id)
	return googleDriveRequestJSON(ctx, http.MethodDelete, rawURL, token, nil, nil)
}
