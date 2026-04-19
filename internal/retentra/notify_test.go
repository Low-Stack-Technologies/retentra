package retentra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("discord body is not json: %v", err)
	}
	if _, ok := payload["content"]; ok {
		t.Fatalf("discord body contains content field: %q", body)
	}
	embeds, ok := payload["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("discord embeds = %#v, want one embed", payload["embeds"])
	}
	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatalf("discord embed = %#v, want object", embeds[0])
	}
	if embed["title"] != "DB Backup Report" {
		t.Fatalf("discord embed title = %#v, want DB Backup Report", embed["title"])
	}
	if embed["color"] != float64(discordSuccessColor) {
		t.Fatalf("discord embed color = %#v, want %d", embed["color"], discordSuccessColor)
	}
	if !strings.Contains(body, `"name":"Sources"`) || !strings.Contains(body, "Dump: wopl") || !strings.Contains(body, `"name":"Outputs"`) {
		t.Fatalf("discord body = %q, want sources and outputs fields", body)
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

func TestDiscordMessageSuccess(t *testing.T) {
	payload := discordMessage(successfulStatus())

	if len(payload.Embeds) != 1 {
		t.Fatalf("len(Embeds) = %d, want 1", len(payload.Embeds))
	}
	embed := payload.Embeds[0]
	if embed.Title != "DB Backup Report" {
		t.Fatalf("Title = %q, want DB Backup Report", embed.Title)
	}
	if embed.Color != discordSuccessColor {
		t.Fatalf("Color = %d, want %d", embed.Color, discordSuccessColor)
	}
	assertDiscordField(t, embed.Fields, "Sources", "✅ Dump: wopl\n✅ Dump: wp_centrumkyrkangrabo", false)
	assertDiscordField(t, embed.Fields, "Archive", "📦 Created successfully", true)
	assertDiscordField(t, embed.Fields, "Included", "wopl.sql, wp_centrumkyrkangrabo.sql", false)
	assertDiscordField(t, embed.Fields, "Outputs", "🚀 Upload (127.0.0.1): Success", true)
}

func TestDiscordMessageOutputFailure(t *testing.T) {
	payload := discordMessage(failedStatus())

	embed := payload.Embeds[0]
	if embed.Color != discordFailureColor {
		t.Fatalf("Color = %d, want %d", embed.Color, discordFailureColor)
	}
	assertDiscordField(t, embed.Fields, "Sources", "✅ DB Dump: wordpress", false)
	assertDiscordField(t, embed.Fields, "Archive", "📦 Created successfully", true)
	assertDiscordField(t, embed.Fields, "Included", "wordpress.sql, site", false)
	assertDiscordField(t, embed.Fields, "Outputs", "❌ Upload (127.0.0.1): Remote directory check/create failed", true)
}

func TestDiscordMessageUnattributedFailure(t *testing.T) {
	payload := discordMessage(Status{
		Success:     false,
		ReportTitle: "Backup Report",
		Error:       errors.New("sources[0]: commands[0] failed"),
	})

	embed := payload.Embeds[0]
	if embed.Color != discordFailureColor {
		t.Fatalf("Color = %d, want %d", embed.Color, discordFailureColor)
	}
	assertDiscordField(t, embed.Fields, "Error", "❌ sources[0]: commands[0] failed", false)
}

func TestDiscordMessageArchiveFailure(t *testing.T) {
	payload := discordMessage(Status{
		Success:       false,
		ReportTitle:   "Backup Report",
		SourceResults: []ReportResult{{Label: "DB Dump: wordpress"}},
		Included:      []string{"wordpress.sql"},
		ArchiveError:  errors.New("permission denied"),
		Error:         errors.New("permission denied"),
	})

	embed := payload.Embeds[0]
	assertDiscordField(t, embed.Fields, "Sources", "✅ DB Dump: wordpress", false)
	assertDiscordField(t, embed.Fields, "Archive", "❌ permission denied", true)
	assertDiscordField(t, embed.Fields, "Included", "wordpress.sql", false)
	if field := findDiscordField(embed.Fields, "Error"); field != nil {
		t.Fatalf("unexpected Error field: %#v", *field)
	}
}

func TestStatusMessageSummarizesLargeIncludedList(t *testing.T) {
	status := successfulStatus()
	status.Included = make([]string, 25)
	for i := range status.Included {
		status.Included[i] = fmt.Sprintf("file-%02d.sql", i+1)
	}

	got := statusMessage(status)
	if !strings.Contains(got, "... and 5 more") {
		t.Fatalf("statusMessage() = %q, want omitted included count", got)
	}
	if strings.Contains(got, "file-25.sql") {
		t.Fatalf("statusMessage() = %q, want truncated included list", got)
	}
}

func TestDiscordMessageBoundsLargeFields(t *testing.T) {
	status := successfulStatus()
	status.Included = make([]string, 60)
	for i := range status.Included {
		status.Included[i] = strings.Repeat("very-long-target-name-", 20) + fmt.Sprint(i)
	}

	payload := discordMessage(status)
	field := findDiscordField(payload.Embeds[0].Fields, "Included")
	if field == nil {
		t.Fatal("missing Included field")
	}
	if len([]rune(field.Value)) > maxReportValueRunes {
		t.Fatalf("Included field length = %d, want <= %d", len([]rune(field.Value)), maxReportValueRunes)
	}
	if !strings.HasSuffix(field.Value, "...") {
		t.Fatalf("Included field = %q, want truncated suffix", field.Value)
	}
}

func TestDiscordMessageBoundsLargeSourceAndOutputFields(t *testing.T) {
	status := successfulStatus()
	status.SourceResults = make([]ReportResult, 20)
	status.OutputResults = make([]ReportResult, 20)
	for i := range status.SourceResults {
		label := strings.Repeat("source-with-very-long-name-", 20) + fmt.Sprint(i)
		status.SourceResults[i] = ReportResult{Label: label}
		status.OutputResults[i] = ReportResult{Label: strings.Replace(label, "source", "output", 1)}
	}

	payload := discordMessage(status)
	for _, name := range []string{"Sources", "Outputs"} {
		field := findDiscordField(payload.Embeds[0].Fields, name)
		if field == nil {
			t.Fatalf("missing %s field", name)
		}
		if len([]rune(field.Value)) > maxReportValueRunes {
			t.Fatalf("%s field length = %d, want <= %d", name, len([]rune(field.Value)), maxReportValueRunes)
		}
		if !strings.HasSuffix(field.Value, "...") {
			t.Fatalf("%s field = %q, want truncated suffix", name, field.Value)
		}
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

func assertDiscordField(t *testing.T, fields []discordEmbedField, name, value string, inline bool) {
	t.Helper()
	field := findDiscordField(fields, name)
	if field == nil {
		t.Fatalf("missing discord field %q in %#v", name, fields)
	}
	if field.Value != value {
		t.Fatalf("field %q value = %q, want %q", name, field.Value, value)
	}
	if field.Inline != inline {
		t.Fatalf("field %q inline = %t, want %t", name, field.Inline, inline)
	}
}

func findDiscordField(fields []discordEmbedField, name string) *discordEmbedField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
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
