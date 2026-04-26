package retentra

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultGoogleDeviceCodeURL    = "https://oauth2.googleapis.com/device/code"
	defaultGoogleTokenURL         = "https://oauth2.googleapis.com/token"
	defaultGoogleRevokeURL        = "https://oauth2.googleapis.com/revoke"
	defaultGoogleAPIBaseURL       = "https://www.googleapis.com/drive/v3"
	defaultGoogleUploadURL        = "https://www.googleapis.com/upload/drive/v3"
	googleDriveScope              = "https://www.googleapis.com/auth/drive.file"
	googleCredentialStorageSecret = "secret_store"
	googleCredentialStorageFile   = "file"
	googleTokenFileName           = "token.json"
	googleFolderMimeType          = "application/vnd.google-apps.folder"
)

var (
	googleClientID      = ""
	googleClientSecret  = ""
	googleDeviceCodeURL = defaultGoogleDeviceCodeURL
	googleTokenURL      = defaultGoogleTokenURL
	googleRevokeURL     = defaultGoogleRevokeURL
	googleAPIBaseURL    = defaultGoogleAPIBaseURL
	googleUploadURL     = defaultGoogleUploadURL
	openBrowser         = openBrowserURL
)

type googleSettings struct {
	clientID      string
	clientSecret  string
	configDir     string
	deviceCodeURL string
	tokenURL      string
	revokeURL     string
	apiBaseURL    string
	uploadURL     string
}

type googleTokenRecord struct {
	ClientID string      `json:"client_id"`
	Scopes   []string    `json:"scopes,omitempty"`
	Token    googleToken `json:"token"`
}

type googleToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
	Scope        string    `json:"scope,omitempty"`
}

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type googleOAuthError struct {
	Code        string
	Description string
}

func (e *googleOAuthError) Error() string {
	if e == nil {
		return ""
	}
	if e.Description != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Description)
	}
	return e.Code
}

type googleDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURL         string `json:"verification_url"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURLComplete string `json:"verification_url_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

type googleDriveState struct {
	ClientID          string            `json:"client_id"`
	CredentialStorage string            `json:"credential_storage,omitempty"`
	RootFolderID      string            `json:"root_folder_id"`
	Folders           map[string]string `json:"folders,omitempty"`
}

type googleDriveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	ModifiedTime string `json:"modifiedTime"`
}

type googleDriveFileList struct {
	NextPageToken string            `json:"nextPageToken"`
	Files         []googleDriveFile `json:"files"`
}

func googleStatus(out io.Writer) error {
	settings := loadGoogleSettings()
	_, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return err
	}
	store, mode, err := resolveGoogleCredentialStoreForStatus(settings, state)
	switch {
	case err == nil && settings.configured():
		token, err := store.Load(context.Background())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(out, "Google auth: configured\n")
				fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
				fmt.Fprintf(out, "State: no cached credentials\n")
				return nil
			}
			return err
		}
		if token.ClientID != "" && token.ClientID != settings.clientID {
			fmt.Fprintf(out, "Google auth: configured\n")
			fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
			fmt.Fprintf(out, "State: cached credentials were created for a different client ID\n")
			return nil
		}
		if token.Token.RefreshToken == "" && token.Token.AccessToken == "" {
			fmt.Fprintf(out, "Google auth: configured\n")
			fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
			fmt.Fprintf(out, "State: credential cache is present but incomplete\n")
			return nil
		}
		if token.Token.Expiry.IsZero() {
			fmt.Fprintf(out, "Google auth: configured\n")
			fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
			fmt.Fprintf(out, "State: authenticated, expiry unknown\n")
			return nil
		}
		fmt.Fprintf(out, "Google auth: configured\n")
		fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
		if time.Now().After(token.Token.Expiry) {
			fmt.Fprintf(out, "State: token expired at %s\n", token.Token.Expiry.Format(time.RFC3339))
		} else {
			fmt.Fprintf(out, "State: authenticated until %s\n", token.Token.Expiry.Format(time.RFC3339))
		}
		return nil
	case err == nil:
		fmt.Fprintf(out, "Google auth: not configured\n")
		fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
		fmt.Fprintf(out, "State: cached credentials are present, but Google client settings are incomplete\n")
		return nil
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(out, "Google auth: %s\n", configuredText(settings.configured()))
		fmt.Fprintf(out, "Token storage: %s\n", googleCredentialStorageLabel(mode))
		fmt.Fprintf(out, "State: no cached credentials\n")
		return nil
	default:
		return err
	}
}

func GoogleStatus(out io.Writer) error {
	return googleStatus(out)
}

func googleLogin(ctx context.Context, out io.Writer, allowFileTokenStorage bool) error {
	settings := loadGoogleSettings()
	if !settings.configured() {
		return fmt.Errorf("google auth is not configured; set RETENTRA_GOOGLE_CLIENT_ID and RETENTRA_GOOGLE_CLIENT_SECRET")
	}

	token, err := performGoogleLogin(ctx, settings, out)
	if err != nil {
		return err
	}
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return err
	}
	store, mode, err := resolveGoogleCredentialStoreForLogin(settings, allowFileTokenStorage)
	if err != nil {
		return err
	}
	record := googleTokenRecord{
		ClientID: settings.clientID,
		Scopes:   []string{googleDriveScope},
		Token:    token,
	}
	if err := store.Save(ctx, record); err != nil {
		return err
	}
	state.ClientID = settings.clientID
	state.CredentialStorage = mode
	if err := saveGoogleDriveState(statePath, state); err != nil {
		return err
	}
	if mode == googleCredentialStorageSecret {
		_ = removeGoogleTokenFile(settings)
	} else {
		_ = newGoogleOSSecretStore(settings).Delete(ctx)
	}
	fmt.Fprintf(out, "Google credentials saved to %s\n", googleCredentialStorageLabel(mode))
	return nil
}

func GoogleLogin(ctx context.Context, out io.Writer) error {
	return googleLogin(ctx, out, false)
}

func GoogleLoginWithOptions(ctx context.Context, out io.Writer, allowFileTokenStorage bool) error {
	return googleLogin(ctx, out, allowFileTokenStorage)
}

func googleRefresh(ctx context.Context, out io.Writer) error {
	settings := loadGoogleSettings()
	if !settings.configured() {
		return fmt.Errorf("google auth is not configured; set RETENTRA_GOOGLE_CLIENT_ID and RETENTRA_GOOGLE_CLIENT_SECRET")
	}
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return err
	}
	store, mode, err := resolveGoogleCredentialStoreForAccess(settings, state)
	if err != nil {
		return err
	}
	record, err := store.Load(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("google auth credentials not found; run `retentra auth google login`")
		}
		return err
	}
	if record.ClientID != "" && record.ClientID != settings.clientID {
		return fmt.Errorf("cached credentials were created for a different client ID")
	}
	updated, err := refreshGoogleToken(ctx, settings, record.Token)
	if err != nil {
		return err
	}
	record.ClientID = settings.clientID
	record.Scopes = []string{googleDriveScope}
	record.Token = updated
	if err := store.Save(ctx, record); err != nil {
		return err
	}
	state.ClientID = settings.clientID
	state.CredentialStorage = mode
	if err := saveGoogleDriveState(statePath, state); err != nil {
		return err
	}
	if mode == googleCredentialStorageSecret {
		_ = removeGoogleTokenFile(settings)
	} else {
		_ = newGoogleOSSecretStore(settings).Delete(ctx)
	}
	fmt.Fprintf(out, "Google credentials refreshed at %s\n", updated.Expiry.Format(time.RFC3339))
	return nil
}

func GoogleRefresh(ctx context.Context, out io.Writer) error {
	return googleRefresh(ctx, out)
}

func googleLogout(out io.Writer) error {
	settings := loadGoogleSettings()
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return err
	}
	storageLabel := googleCredentialStorageLabel(state.CredentialStorage)
	if secretStore := newGoogleOSSecretStore(settings); secretStore.Available() == nil {
		if record, err := secretStore.Load(context.Background()); err == nil {
			token := record.Token.RefreshToken
			if token == "" {
				token = record.Token.AccessToken
			}
			if token != "" {
				_ = revokeGoogleToken(context.Background(), settings, token)
			}
		}
		_ = secretStore.Delete(context.Background())
	}
	_ = removeGoogleTokenFile(settings)
	state.CredentialStorage = ""
	if err := saveGoogleDriveState(statePath, state); err != nil {
		return err
	}
	fmt.Fprintf(out, "Google credentials removed from %s\n", storageLabel)
	return nil
}

func GoogleLogout(out io.Writer) error {
	return googleLogout(out)
}

func loadGoogleSettings() googleSettings {
	return googleSettings{
		clientID:      firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_CLIENT_ID"), googleClientID),
		clientSecret:  firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_CLIENT_SECRET"), googleClientSecret),
		configDir:     firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_CONFIG_DIR"), defaultGoogleConfigDir()),
		deviceCodeURL: firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_DEVICE_CODE_URL"), googleDeviceCodeURL),
		tokenURL:      firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_TOKEN_URL"), googleTokenURL),
		revokeURL:     firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_REVOKE_URL"), googleRevokeURL),
		apiBaseURL:    firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_API_BASE_URL"), googleAPIBaseURL),
		uploadURL:     firstNonEmpty(os.Getenv("RETENTRA_GOOGLE_UPLOAD_BASE_URL"), googleUploadURL),
	}
}

func (s googleSettings) configured() bool {
	return s.clientID != "" && s.clientSecret != ""
}

func googleCredentialStorageLabel(mode string) string {
	switch mode {
	case googleCredentialStorageFile:
		return "file"
	case googleCredentialStorageSecret:
		return "OS secret store"
	case "":
		return "unknown"
	default:
		return mode
	}
}

func removeGoogleTokenFile(settings googleSettings) error {
	cachePath, err := googleTokenCachePath(settings)
	if err != nil {
		return err
	}
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func defaultGoogleConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || home == "" {
			return filepath.Join(".", ".config", "retentra", "google")
		}
		return filepath.Join(home, ".config", "retentra", "google")
	}
	return filepath.Join(dir, "retentra", "google")
}

func googleTokenCachePath(settings googleSettings) (string, error) {
	if settings.configDir == "" {
		return "", fmt.Errorf("google token directory is empty")
	}
	if err := os.MkdirAll(settings.configDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(settings.configDir, googleTokenFileName), nil
}

func readGoogleTokenRecord(path string) (string, googleTokenRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", googleTokenRecord{}, err
	}
	var record googleTokenRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return "", googleTokenRecord{}, err
	}
	return string(data), record, nil
}

func writeGoogleTokenRecord(path string, record googleTokenRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
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

func performGoogleLogin(ctx context.Context, settings googleSettings, out io.Writer) (googleToken, error) {
	deviceCode, err := requestGoogleDeviceCode(ctx, settings)
	if err != nil {
		return googleToken{}, err
	}
	verificationURL := deviceCode.VerificationURL
	if verificationURL == "" {
		verificationURL = deviceCode.VerificationURI
	}
	if verificationURL == "" {
		return googleToken{}, fmt.Errorf("device code response missing verification URL")
	}
	if deviceCode.Interval <= 0 {
		deviceCode.Interval = 5
	}
	fmt.Fprintln(out, "Open this URL on another device to authenticate:")
	fmt.Fprintln(out, verificationURL)
	fmt.Fprintf(out, "Enter this code: %s\n", deviceCode.UserCode)
	fmt.Fprintln(out, "Waiting for Google approval...")
	return pollGoogleDeviceToken(ctx, settings, deviceCode)
}

func requestGoogleDeviceCode(ctx context.Context, settings googleSettings) (googleDeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", settings.clientID)
	form.Set("scope", googleDriveScope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleDeviceCodeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var resp googleDeviceCodeResponse
	if err := doGoogleJSONRequest(req, &resp); err != nil {
		return googleDeviceCodeResponse{}, err
	}
	if resp.DeviceCode == "" {
		return googleDeviceCodeResponse{}, fmt.Errorf("device code response missing device code")
	}
	if resp.UserCode == "" {
		return googleDeviceCodeResponse{}, fmt.Errorf("device code response missing user code")
	}
	return resp, nil
}

func pollGoogleDeviceToken(ctx context.Context, settings googleSettings, deviceCode googleDeviceCodeResponse) (googleToken, error) {
	interval := time.Duration(deviceCode.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(deviceCode.ExpiresIn) * time.Second)
	if deviceCode.ExpiresIn <= 0 {
		deadline = time.Now().Add(30 * time.Minute)
	}

	for {
		if time.Now().After(deadline) {
			return googleToken{}, fmt.Errorf("Google authorization expired before completion")
		}
		if err := waitForGoogleDevicePoll(ctx, interval); err != nil {
			return googleToken{}, err
		}
		token, err := exchangeGoogleDeviceToken(ctx, settings, deviceCode.DeviceCode)
		if err == nil {
			return token, nil
		}
		var oauthErr *googleOAuthError
		if !errors.As(err, &oauthErr) {
			return googleToken{}, err
		}
		switch oauthErr.Code {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return googleToken{}, fmt.Errorf("Google authorization was denied")
		case "expired_token":
			return googleToken{}, fmt.Errorf("Google authorization expired before completion")
		default:
			return googleToken{}, err
		}
	}
}

func exchangeGoogleDeviceToken(ctx context.Context, settings googleSettings, deviceCode string) (googleToken, error) {
	values := url.Values{}
	values.Set("client_id", settings.clientID)
	values.Set("client_secret", settings.clientSecret)
	values.Set("device_code", deviceCode)
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return googleToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResp, err := doGoogleTokenRequest(req)
	if err != nil {
		return googleToken{}, err
	}
	return tokenResponseToToken(tokenResp)
}

func exchangeGoogleToken(ctx context.Context, settings googleSettings, code, redirectURL, verifier string) (googleToken, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", settings.clientID)
	form.Set("client_secret", settings.clientSecret)
	form.Set("redirect_uri", redirectURL)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResp, err := doGoogleTokenRequest(req)
	if err != nil {
		return googleToken{}, err
	}
	return tokenResponseToToken(tokenResp)
}

func refreshGoogleToken(ctx context.Context, settings googleSettings, token googleToken) (googleToken, error) {
	if token.RefreshToken == "" {
		return googleToken{}, fmt.Errorf("cached credentials are missing a refresh token")
	}
	form := url.Values{}
	form.Set("client_id", settings.clientID)
	form.Set("client_secret", settings.clientSecret)
	form.Set("refresh_token", token.RefreshToken)
	form.Set("grant_type", "refresh_token")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResp, err := doGoogleTokenRequest(req)
	if err != nil {
		return googleToken{}, err
	}
	refreshed, err := tokenResponseToToken(tokenResp)
	if err != nil {
		return googleToken{}, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	return refreshed, nil
}

func revokeGoogleToken(ctx context.Context, settings googleSettings, token string) error {
	if settings.revokeURL == "" || token == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("revoke token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func doGoogleTokenRequest(req *http.Request) (googleTokenResponse, error) {
	var tokenResp googleTokenResponse
	if err := doGoogleJSONRequest(req, &tokenResp); err != nil {
		return googleTokenResponse{}, err
	}
	if tokenResp.Error != "" {
		return googleTokenResponse{}, &googleOAuthError{Code: tokenResp.Error, Description: tokenResp.ErrorDesc}
	}
	return tokenResp, nil
}

func tokenResponseToToken(resp googleTokenResponse) (googleToken, error) {
	if resp.AccessToken == "" {
		return googleToken{}, fmt.Errorf("token response missing access token")
	}
	token := googleToken{
		AccessToken: resp.AccessToken,
		TokenType:   resp.TokenType,
		Scope:       resp.Scope,
		Expiry:      time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if resp.RefreshToken != "" {
		token.RefreshToken = resp.RefreshToken
	}
	return token, nil
}

func randomState() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

func configuredText(configured bool) string {
	if configured {
		return "configured"
	}
	return "not configured"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func openBrowserURL(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}

func doGoogleJSONRequest(req *http.Request, out any) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var oauthErr struct {
			Error     string `json:"error"`
			ErrorDesc string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		if oauthErr.Error != "" {
			return &googleOAuthError{Code: oauthErr.Error, Description: oauthErr.ErrorDesc}
		}
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

var waitForGoogleDevicePoll = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func googleClientCredentialsConfigured() bool {
	settings := loadGoogleSettings()
	return settings.configured()
}

func googleAccessToken(ctx context.Context) (string, error) {
	settings := loadGoogleSettings()
	if !settings.configured() {
		return "", fmt.Errorf("google auth is not configured; set RETENTRA_GOOGLE_CLIENT_ID and RETENTRA_GOOGLE_CLIENT_SECRET")
	}
	statePath, state, err := loadGoogleDriveState(settings)
	if err != nil {
		return "", err
	}
	store, mode, err := resolveGoogleCredentialStoreForAccess(settings, state)
	if err != nil {
		return "", err
	}
	record, err := store.Load(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("google auth credentials not found; run `retentra auth google login`")
		}
		return "", err
	}
	if record.ClientID != "" && record.ClientID != settings.clientID {
		return "", fmt.Errorf("cached credentials were created for a different client ID")
	}
	token := record.Token
	if token.RefreshToken == "" && token.AccessToken == "" {
		return "", fmt.Errorf("cached credentials are incomplete")
	}
	if token.AccessToken != "" && !token.Expiry.IsZero() && time.Until(token.Expiry) > time.Minute {
		state.ClientID = settings.clientID
		state.CredentialStorage = mode
		if err := saveGoogleDriveState(statePath, state); err != nil {
			return "", err
		}
		return token.AccessToken, nil
	}
	refreshed, err := refreshGoogleToken(ctx, settings, token)
	if err != nil {
		return "", err
	}
	record.ClientID = settings.clientID
	record.Scopes = []string{googleDriveScope}
	record.Token = refreshed
	if err := store.Save(ctx, record); err != nil {
		return "", err
	}
	state.ClientID = settings.clientID
	state.CredentialStorage = mode
	if err := saveGoogleDriveState(statePath, state); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func googleDriveBaseQuery(settings googleSettings, query string, extra url.Values) string {
	values := url.Values{}
	values.Set("fields", "nextPageToken,files(id,name,mimeType,modifiedTime)")
	values.Set("spaces", "drive")
	if query != "" {
		values.Set("q", query)
	}
	for key, vals := range extra {
		if len(vals) > 0 {
			values.Set(key, vals[0])
		}
	}
	return settings.apiBaseURL + "/files?" + values.Encode()
}

func googleDriveUploadBaseURL(settings googleSettings) string {
	return settings.uploadURL + "/files"
}

func escapeDriveQueryString(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

func googleDriveFolderQuery(name, parentID string) string {
	return fmt.Sprintf("mimeType='%s' and trashed=false and name='%s' and '%s' in parents", googleFolderMimeType, escapeDriveQueryString(name), parentID)
}

func googleDriveFileQuery(parentID, matcher string) string {
	return fmt.Sprintf("trashed=false and '%s' in parents", parentID)
}

func googleDriveRequest(ctx context.Context, method, rawURL, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return http.DefaultClient.Do(req)
}

func googleDriveRequestJSON(ctx context.Context, method, rawURL, token string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	resp, err := googleDriveRequest(ctx, method, rawURL, token, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
