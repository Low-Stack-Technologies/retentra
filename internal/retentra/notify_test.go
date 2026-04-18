package retentra

import (
	"context"
	"errors"
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

	err := sendNotification(context.Background(), NotificationConfig{Type: "discord", WebhookURL: server.URL}, successfulStatus())
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if !strings.Contains(body, "DB Backup Report") || !strings.Contains(body, "Upload (127.0.0.1): Success") {
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

	err := sendNotification(context.Background(), NotificationConfig{Type: "ntfy", URL: server.URL}, failedStatus())
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if auth != "" {
		t.Fatalf("Authorization = %q", auth)
	}
	if !strings.Contains(body, "Backup Report") || !strings.Contains(body, "Remote directory check/create failed") {
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

	err := sendNotification(context.Background(), NotificationConfig{Type: "ntfy", URL: server.URL, Username: "user", Password: "secret"}, failedStatus())
	if err != nil {
		t.Fatalf("sendNotification() error = %v", err)
	}
	if username != "user" || password != "secret" {
		t.Fatalf("BasicAuth = %q/%q", username, password)
	}
	if !strings.Contains(body, "Remote directory check/create failed") {
		t.Fatalf("ntfy body = %q", body)
	}
}

func TestStatusMessageSuccess(t *testing.T) {
	got := statusMessage(successfulStatus())
	want := strings.Join([]string{
		"DB Backup Report",
		"✅ Dump: wopl",
		"✅ Dump: wp_centrumkyrkangrabo",
		"📦 Archive: Created successfully",
		"📁 Included: wopl.sql, wp_centrumkyrkangrabo.sql",
		"🚀 Upload (127.0.0.1): Success",
	}, "\n")
	if got != want {
		t.Fatalf("statusMessage() = %q, want %q", got, want)
	}
}

func TestStatusMessageOutputFailure(t *testing.T) {
	got := statusMessage(failedStatus())
	want := strings.Join([]string{
		"Backup Report",
		"✅ DB Dump: wordpress",
		"📦 Archive: Created successfully",
		"📁 Included: wordpress.sql, site",
		"❌ Upload (127.0.0.1): Remote directory check/create failed",
	}, "\n")
	if got != want {
		t.Fatalf("statusMessage() = %q, want %q", got, want)
	}
}

func TestStatusMessageUnattributedFailure(t *testing.T) {
	got := statusMessage(Status{
		Success:     false,
		ReportTitle: "Backup Report",
		Error:       errors.New("sources[0]: commands[0] failed"),
	})
	want := strings.Join([]string{
		"Backup Report",
		"❌ Error: sources[0]: commands[0] failed",
	}, "\n")
	if got != want {
		t.Fatalf("statusMessage() = %q, want %q", got, want)
	}
}

func TestStatusMessageArchiveFailure(t *testing.T) {
	got := statusMessage(Status{
		Success:       false,
		ReportTitle:   "Backup Report",
		SourceResults: []ReportResult{{Label: "DB Dump: wordpress"}},
		Included:      []string{"wordpress.sql"},
		ArchiveError:  errors.New("permission denied"),
		Error:         errors.New("permission denied"),
	})
	want := strings.Join([]string{
		"Backup Report",
		"✅ DB Dump: wordpress",
		"❌ Archive: permission denied",
		"📁 Included: wordpress.sql",
	}, "\n")
	if got != want {
		t.Fatalf("statusMessage() = %q, want %q", got, want)
	}
}

func successfulStatus() Status {
	return Status{
		Success:        true,
		ReportTitle:    "DB Backup Report",
		ArchiveName:    "backup.tar.gz",
		SourceResults:  []ReportResult{{Label: "Dump: wopl"}, {Label: "Dump: wp_centrumkyrkangrabo"}},
		ArchiveCreated: true,
		Included:       []string{"wopl.sql", "wp_centrumkyrkangrabo.sql"},
		OutputResults:  []ReportResult{{Label: "Upload (127.0.0.1)"}},
	}
}

func failedStatus() Status {
	err := errors.New("Remote directory check/create failed")
	return Status{
		Success:        false,
		ReportTitle:    "Backup Report",
		SourceResults:  []ReportResult{{Label: "DB Dump: wordpress"}},
		ArchiveCreated: true,
		Included:       []string{"wordpress.sql", "site"},
		OutputResults:  []ReportResult{{Label: "Upload (127.0.0.1)", Error: err}},
		Error:          err,
	}
}
