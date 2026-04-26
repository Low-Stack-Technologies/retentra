package retentra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGoogleAccessTokenRefreshesExpiredToken(t *testing.T) {
	dir := t.TempDir()
	tokenServer := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("client_secret"); got != "secret" {
			t.Fatalf("client_secret = %q, want secret", got)
		}
		if got := r.Form.Get("client_id"); got != "client" {
			t.Fatalf("client_id = %q, want client", got)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-me" {
			t.Fatalf("refresh_token = %q, want refresh-me", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        googleDriveScope,
		})
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_CONFIG_DIR", dir)
	t.Setenv("RETENTRA_GOOGLE_TOKEN_URL", tokenServer.URL)

	cachePath, err := googleTokenCachePath(loadGoogleSettings())
	if err != nil {
		t.Fatal(err)
	}
	statePath, state, err := loadGoogleDriveState(loadGoogleSettings())
	if err != nil {
		t.Fatal(err)
	}
	state.ClientID = "client"
	state.CredentialStorage = googleCredentialStorageFile
	if err := saveGoogleDriveState(statePath, state); err != nil {
		t.Fatal(err)
	}
	if err := writeGoogleTokenRecord(cachePath, googleTokenRecord{
		ClientID: "client",
		Scopes:   []string{googleDriveScope},
		Token: googleToken{
			AccessToken:  "old-access",
			RefreshToken: "refresh-me",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(-time.Hour),
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := googleAccessToken(context.Background())
	if err != nil {
		t.Fatalf("googleAccessToken() error = %v", err)
	}
	if got != "new-access" {
		t.Fatalf("googleAccessToken() = %q, want new-access", got)
	}

	_, record, err := readGoogleTokenRecord(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if record.Token.AccessToken != "new-access" {
		t.Fatalf("cached access token = %q, want new-access", record.Token.AccessToken)
	}
	if time.Until(record.Token.Expiry) <= 0 {
		t.Fatalf("cached token expiry = %s, want future expiry", record.Token.Expiry)
	}
}

func TestRequestGoogleDeviceCodeUsesClientIDAndDriveScope(t *testing.T) {
	server := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("client_id"); got != "client" {
			t.Fatalf("client_id = %q, want client", got)
		}
		if got := r.Form.Get("scope"); got != googleDriveScope {
			t.Fatalf("scope = %q, want %q", got, googleDriveScope)
		}
		_ = json.NewEncoder(w).Encode(googleDeviceCodeResponse{
			DeviceCode:      "device",
			UserCode:        "ABCD-EFGH",
			VerificationURL: "https://www.google.com/device",
			ExpiresIn:       1800,
			Interval:        5,
		})
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_DEVICE_CODE_URL", server.URL)

	resp, err := requestGoogleDeviceCode(context.Background(), loadGoogleSettings())
	if err != nil {
		t.Fatalf("requestGoogleDeviceCode() error = %v", err)
	}
	if resp.DeviceCode != "device" {
		t.Fatalf("DeviceCode = %q, want device", resp.DeviceCode)
	}
	if resp.UserCode != "ABCD-EFGH" {
		t.Fatalf("UserCode = %q, want ABCD-EFGH", resp.UserCode)
	}
	if resp.VerificationURL != "https://www.google.com/device" {
		t.Fatalf("VerificationURL = %q, want verification URL", resp.VerificationURL)
	}
}

func TestPerformGoogleLoginPollsUntilSuccess(t *testing.T) {
	tokenRequests := 0
	tokenServer := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("client_secret"); got != "secret" {
			t.Fatalf("client_secret = %q, want secret", got)
		}
		if got := r.Form.Get("client_id"); got != "client" {
			t.Fatalf("client_id = %q, want client", got)
		}
		if got := r.Form.Get("device_code"); got != "device" {
			t.Fatalf("device_code = %q, want device", got)
		}
		if got := r.Form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Fatalf("grant_type = %q, want device_code grant", got)
		}
		tokenRequests++
		switch tokenRequests {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "authorization_pending",
				"error_description": "pending",
			})
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "slow_down",
				"error_description": "slow down",
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access",
				"refresh_token": "refresh",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         googleDriveScope,
			})
		}
	}))

	deviceServer := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(googleDeviceCodeResponse{
			DeviceCode:      "device",
			UserCode:        "ABCD-EFGH",
			VerificationURL: "https://www.google.com/device",
			ExpiresIn:       1800,
			Interval:        0,
		})
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_DEVICE_CODE_URL", deviceServer.URL)
	t.Setenv("RETENTRA_GOOGLE_TOKEN_URL", tokenServer.URL)

	originalWait := waitForGoogleDevicePoll
	waitForGoogleDevicePoll = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { waitForGoogleDevicePoll = originalWait })

	var stdout bytes.Buffer
	got, err := performGoogleLogin(context.Background(), loadGoogleSettings(), &stdout)
	if err != nil {
		t.Fatalf("performGoogleLogin() error = %v", err)
	}
	if got.AccessToken != "access" {
		t.Fatalf("AccessToken = %q, want access", got.AccessToken)
	}
	if got.RefreshToken != "refresh" {
		t.Fatalf("RefreshToken = %q, want refresh", got.RefreshToken)
	}
	if tokenRequests != 3 {
		t.Fatalf("tokenRequests = %d, want 3", tokenRequests)
	}
	if !strings.Contains(stdout.String(), "ABCD-EFGH") {
		t.Fatalf("stdout = %q, want user code", stdout.String())
	}
}

func TestPollGoogleDeviceTokenReturnsDeniedError(t *testing.T) {
	tokenServer := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "access_denied",
			"error_description": "denied",
		})
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_TOKEN_URL", tokenServer.URL)

	originalWait := waitForGoogleDevicePoll
	waitForGoogleDevicePoll = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { waitForGoogleDevicePoll = originalWait })

	_, err := pollGoogleDeviceToken(context.Background(), loadGoogleSettings(), googleDeviceCodeResponse{
		DeviceCode: "device",
		ExpiresIn:  1800,
		Interval:   1,
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("pollGoogleDeviceToken() error = %v, want denied error", err)
	}
}

func TestPollGoogleDeviceTokenReturnsExpiredError(t *testing.T) {
	tokenServer := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "expired_token",
			"error_description": "expired",
		})
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_TOKEN_URL", tokenServer.URL)

	originalWait := waitForGoogleDevicePoll
	waitForGoogleDevicePoll = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { waitForGoogleDevicePoll = originalWait })

	_, err := pollGoogleDeviceToken(context.Background(), loadGoogleSettings(), googleDeviceCodeResponse{
		DeviceCode: "device",
		ExpiresIn:  1800,
		Interval:   1,
	})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("pollGoogleDeviceToken() error = %v, want expired error", err)
	}
}

func TestUploadGoogleDriveCreatesNestedFoldersAndUploadsArchive(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "backup.tar.gz")
	archiveBytes := []byte("archive bytes")
	if err := os.WriteFile(archivePath, archiveBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	type folderKey struct {
		parent string
		name   string
	}

	folders := map[folderKey]string{}
	var folderCreates []folderKey
	var uploaded []byte
	server := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/drive/v3/files"):
			q := r.URL.Query().Get("q")
			if strings.Contains(q, "mimeType='"+googleFolderMimeType+"'") {
				name := extractDriveQueryValue(t, q, "name")
				parent := extractDriveParent(t, q)
				if id := folders[folderKey{parent: parent, name: name}]; id != "" {
					_ = json.NewEncoder(w).Encode(googleDriveFileList{Files: []googleDriveFile{{ID: id, Name: name, MimeType: googleFolderMimeType}}})
					return
				}
				_ = json.NewEncoder(w).Encode(googleDriveFileList{})
				return
			}
			if strings.Contains(q, "trashed=false") {
				parent := extractDriveParent(t, q)
				files := []googleDriveFile{}
				for key, id := range folders {
					if key.parent == parent {
						files = append(files, googleDriveFile{ID: id, Name: key.name, MimeType: googleFolderMimeType, ModifiedTime: time.Now().Format(time.RFC3339)})
					}
				}
				_ = json.NewEncoder(w).Encode(googleDriveFileList{Files: files})
				return
			}
			t.Fatalf("unexpected GET query: %s", q)
		case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files":
			if strings.Contains(r.URL.RawQuery, "fields=id,name") {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				name, _ := payload["name"].(string)
				parents, _ := payload["parents"].([]any)
				parent := ""
				if len(parents) > 0 {
					parent, _ = parents[0].(string)
				}
				id := "folder-" + name
				folders[folderKey{parent: parent, name: name}] = id
				folderCreates = append(folderCreates, folderKey{parent: parent, name: name})
				_ = json.NewEncoder(w).Encode(googleDriveFile{ID: id, Name: name, MimeType: googleFolderMimeType})
				return
			}
			t.Fatalf("unexpected POST query: %s", r.URL.RawQuery)
		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			if got := r.URL.Query().Get("uploadType"); got != "resumable" {
				t.Fatalf("uploadType = %q, want resumable", got)
			}
			w.Header().Set("Location", "http://"+r.Host+"/upload/session/1")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/upload/session/1":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			uploaded = body
			_ = json.NewEncoder(w).Encode(googleDriveFile{
				ID:           "file-1",
				Name:         "backup.tar.gz",
				MimeType:     "application/octet-stream",
				ModifiedTime: time.Now().Format(time.RFC3339),
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))

	t.Setenv("RETENTRA_GOOGLE_CLIENT_ID", "client")
	t.Setenv("RETENTRA_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("RETENTRA_GOOGLE_CONFIG_DIR", dir)
	t.Setenv("RETENTRA_GOOGLE_API_BASE_URL", server.URL+"/drive/v3")
	t.Setenv("RETENTRA_GOOGLE_UPLOAD_BASE_URL", server.URL+"/upload/drive/v3")

	cachePath, err := googleTokenCachePath(loadGoogleSettings())
	if err != nil {
		t.Fatal(err)
	}
	statePath, state, err := loadGoogleDriveState(loadGoogleSettings())
	if err != nil {
		t.Fatal(err)
	}
	state.ClientID = "client"
	state.CredentialStorage = googleCredentialStorageFile
	if err := saveGoogleDriveState(statePath, state); err != nil {
		t.Fatal(err)
	}
	if err := writeGoogleTokenRecord(cachePath, googleTokenRecord{
		ClientID: "client",
		Scopes:   []string{googleDriveScope},
		Token: googleToken{
			AccessToken:  "access",
			RefreshToken: "refresh",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(time.Hour),
		},
	}); err != nil {
		t.Fatal(err)
	}

	desc, err := uploadGoogleDrive(context.Background(), OutputConfig{Type: "gdrive", Path: "Backups/App"}, archivePath, "backup.tar.gz")
	if err != nil {
		t.Fatalf("uploadGoogleDrive() error = %v", err)
	}
	if desc != "gdrive://Backups/App/backup.tar.gz" {
		t.Fatalf("desc = %q", desc)
	}
	if len(folderCreates) != 3 {
		t.Fatalf("folderCreates = %#v, want root plus two nested folders", folderCreates)
	}
	if folderCreates[0].parent != "root" || folderCreates[0].name != googleDriveRootFolderName {
		t.Fatalf("folderCreates[0] = %#v, want app root folder", folderCreates[0])
	}
	if folderCreates[1].name != "Backups" || folderCreates[2].name != "App" {
		t.Fatalf("folderCreates = %#v, want nested Backups/App folders", folderCreates)
	}
	if !bytes.Equal(uploaded, archiveBytes) {
		t.Fatalf("uploaded bytes = %q, want %q", uploaded, archiveBytes)
	}
}

func TestGoogleOSSecretStoreUsesSecretTool(t *testing.T) {
	originalRunner := runGoogleCommand
	t.Cleanup(func() { runGoogleCommand = originalRunner })

	var stored string
	var lastLookup bool
	runGoogleCommand = func(ctx context.Context, name string, args []string, stdin string) (string, error) {
		lastArg := ""
		if len(args) > 0 {
			lastArg = args[0]
		}
		switch {
		case name == "secret-tool" && lastArg == "store":
			stored = stdin
			return "", nil
		case name == "secret-tool" && lastArg == "lookup":
			lastLookup = true
			return stored, nil
		case name == "secret-tool" && lastArg == "clear":
			stored = ""
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	store := googleOSSecretStore{settings: googleSettings{clientID: "client"}}
	record := googleTokenRecord{
		ClientID: "client",
		Scopes:   []string{googleDriveScope},
		Token: googleToken{
			AccessToken:  "access",
			RefreshToken: "refresh",
			TokenType:    "Bearer",
		},
	}
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.Contains(stored, `"access_token":"access"`) {
		t.Fatalf("stored secret = %q, want token JSON", stored)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !lastLookup {
		t.Fatal("Load() did not issue a lookup command")
	}
	if got.Token.AccessToken != "access" || got.Token.RefreshToken != "refresh" {
		t.Fatalf("loaded token = %#v, want original token", got.Token)
	}

	if err := store.Delete(context.Background()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if stored != "" {
		t.Fatalf("stored secret after delete = %q, want empty", stored)
	}
}

func extractDriveQueryValue(t *testing.T, q, field string) string {
	t.Helper()
	prefix := field + "='"
	start := strings.Index(q, prefix)
	if start == -1 {
		t.Fatalf("query %q missing %s", q, field)
	}
	start += len(prefix)
	end := strings.Index(q[start:], "'")
	if end == -1 {
		t.Fatalf("query %q missing closing quote for %s", q, field)
	}
	return strings.ReplaceAll(q[start:start+end], "\\'", "'")
}

func extractDriveParent(t *testing.T, q string) string {
	t.Helper()
	suffix := "' in parents"
	end := strings.Index(q, suffix)
	if end == -1 {
		t.Fatalf("query %q missing parent suffix", q)
	}
	start := strings.LastIndex(q[:end], "'")
	if start == -1 {
		t.Fatalf("query %q missing parent start", q)
	}
	return strings.ReplaceAll(q[start+1:end], "\\'", "'")
}
