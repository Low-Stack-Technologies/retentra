package retentra

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendDiscordNotification(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := sendNotification(context.Background(), NotificationConfig{Type: "discord", WebhookURL: server.URL}, Status{Success: true, ArchiveName: "backup.tar.gz", Outputs: []string{"/out/backup.tar.gz"}})
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if !strings.Contains(body, "retentra backup succeeded") {
		t.Fatalf("discord body = %q", body)
	}
}

func TestSendNTFYNotificationWithoutAuth(t *testing.T) {
	var auth, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := sendNotification(context.Background(), NotificationConfig{Type: "ntfy", URL: server.URL}, Status{Success: false})
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if auth != "" {
		t.Fatalf("Authorization = %q", auth)
	}
	if !strings.Contains(body, "retentra backup failed") {
		t.Fatalf("ntfy body = %q", body)
	}
}

func TestSendNTFYNotificationWithBasicAuth(t *testing.T) {
	var username, password, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, _ = r.BasicAuth()
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := sendNotification(context.Background(), NotificationConfig{Type: "ntfy", URL: server.URL, Username: "user", Password: "secret"}, Status{Success: false})
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if username != "user" || password != "secret" {
		t.Fatalf("BasicAuth = %q/%q", username, password)
	}
	if !strings.Contains(body, "retentra backup failed") {
		t.Fatalf("ntfy body = %q", body)
	}
}
