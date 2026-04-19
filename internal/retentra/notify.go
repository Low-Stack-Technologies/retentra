package retentra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	discordSuccessColor = 5763719
	discordFailureColor = 15548997
)

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title  string              `json:"title"`
	Color  int                 `json:"color"`
	Fields []discordEmbedField `json:"fields"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func sendNotifications(ctx context.Context, notifications []NotificationConfig, status Status) error {
	var errs []error
	for i, notification := range notifications {
		if err := sendNotification(ctx, notification, status); err != nil {
			errs = append(errs, fmt.Errorf("notifications[%d]: %w", i, err))
		}
	}
	return joinErrors(errs)
}

func sendNotification(ctx context.Context, notification NotificationConfig, status Status) error {
	switch notification.Type {
	case "discord":
		return sendDiscord(ctx, notification.WebhookURL, status)
	case "ntfy":
		return sendNTFY(ctx, notification, statusMessage(status))
	default:
		return fmt.Errorf("notification type %q is unsupported", notification.Type)
	}
}

func statusMessage(status Status) string {
	var b strings.Builder
	title := status.ReportTitle
	if title == "" {
		title = "Backup Report"
	}
	b.WriteString(title)

	for _, result := range status.SourceResults {
		writeSourceResult(&b, result)
	}

	if status.ArchiveCreated {
		writeLine(&b, "📦 Archive: Created successfully")
	} else if status.ArchiveError != nil {
		writeLine(&b, fmt.Sprintf("❌ Archive: %s", status.ArchiveError))
	}

	if len(status.Included) > 0 {
		writeLine(&b, fmt.Sprintf("📁 Included: %s", strings.Join(status.Included, ", ")))
	}

	for _, result := range status.OutputResults {
		writeOutputResult(&b, result)
	}

	if !status.Success && !reportHasFailure(status) && status.Error != nil {
		writeLine(&b, fmt.Sprintf("❌ Error: %s", status.Error))
	}

	return b.String()
}

func reportHasFailure(status Status) bool {
	for _, result := range status.SourceResults {
		if !result.Success() {
			return true
		}
	}
	if status.ArchiveError != nil {
		return true
	}
	for _, result := range status.OutputResults {
		if !result.Success() {
			return true
		}
	}
	return false
}

func writeSourceResult(b *strings.Builder, result ReportResult) {
	if result.Success() {
		writeLine(b, fmt.Sprintf("✅ %s", result.Label))
		return
	}
	writeLine(b, fmt.Sprintf("❌ %s: %s", result.Label, result.Error))
}

func writeOutputResult(b *strings.Builder, result ReportResult) {
	if result.Success() {
		writeLine(b, fmt.Sprintf("🚀 %s: Success", result.Label))
		return
	}
	writeLine(b, fmt.Sprintf("❌ %s: %s", result.Label, result.Error))
}

func writeLine(b *strings.Builder, line string) {
	b.WriteString("\n")
	b.WriteString(line)
}

func discordMessage(status Status) discordPayload {
	color := discordSuccessColor
	if !status.Success {
		color = discordFailureColor
	}
	embed := discordEmbed{
		Title:  statusTitle(status),
		Color:  color,
		Fields: discordFields(status),
	}
	return discordPayload{Embeds: []discordEmbed{embed}}
}

func statusTitle(status Status) string {
	if status.ReportTitle != "" {
		return status.ReportTitle
	}
	return "Backup Report"
}

func discordFields(status Status) []discordEmbedField {
	var fields []discordEmbedField
	if len(status.SourceResults) > 0 {
		fields = append(fields, discordEmbedField{Name: "Sources", Value: sourceResultsValue(status.SourceResults)})
	}
	if status.ArchiveCreated {
		fields = append(fields, discordEmbedField{Name: "Archive", Value: "📦 Created successfully", Inline: true})
	} else if status.ArchiveError != nil {
		fields = append(fields, discordEmbedField{Name: "Archive", Value: fmt.Sprintf("❌ %s", status.ArchiveError), Inline: true})
	}
	if len(status.Included) > 0 {
		fields = append(fields, discordEmbedField{Name: "Included", Value: strings.Join(status.Included, ", ")})
	}
	if len(status.OutputResults) > 0 {
		fields = append(fields, discordEmbedField{Name: "Outputs", Value: outputResultsValue(status.OutputResults), Inline: true})
	}
	if !status.Success && !reportHasFailure(status) && status.Error != nil {
		fields = append(fields, discordEmbedField{Name: "Error", Value: fmt.Sprintf("❌ %s", status.Error)})
	}
	return fields
}

func sourceResultsValue(results []ReportResult) string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		if result.Success() {
			lines = append(lines, fmt.Sprintf("✅ %s", result.Label))
			continue
		}
		lines = append(lines, fmt.Sprintf("❌ %s: %s", result.Label, result.Error))
	}
	return strings.Join(lines, "\n")
}

func outputResultsValue(results []ReportResult) string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		if result.Success() {
			lines = append(lines, fmt.Sprintf("🚀 %s: Success", result.Label))
			continue
		}
		lines = append(lines, fmt.Sprintf("❌ %s: %s", result.Label, result.Error))
	}
	return strings.Join(lines, "\n")
}

func sendDiscord(ctx context.Context, webhookURL string, status Status) error {
	body, err := json.Marshal(discordMessage(status))
	if err != nil {
		return err
	}
	return post(ctx, webhookURL, "application/json", bytes.NewReader(body), nil, nil)
}

func sendNTFY(ctx context.Context, notification NotificationConfig, message string) error {
	return post(ctx, notification.URL, "text/plain; charset=utf-8", strings.NewReader(message), nil, func(req *http.Request) {
		if notification.Username != "" && notification.Password != "" {
			req.SetBasicAuth(notification.Username, notification.Password)
		}
	})
}

func post(ctx context.Context, url, contentType string, body io.Reader, headers map[string]string, configure func(*http.Request)) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if configure != nil {
		configure(req)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	return nil
}
