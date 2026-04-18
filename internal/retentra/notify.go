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
	message := statusMessage(status)
	switch notification.Type {
	case "discord":
		return sendDiscord(ctx, notification.WebhookURL, message)
	case "ntfy":
		return sendNTFY(ctx, notification, message)
	default:
		return fmt.Errorf("notification type %q is unsupported", notification.Type)
	}
}

func statusMessage(status Status) string {
	if status.Success {
		return fmt.Sprintf("retentra backup succeeded: %s delivered to %s", status.ArchiveName, strings.Join(status.Outputs, ", "))
	}
	if status.Error != nil {
		return fmt.Sprintf("retentra backup failed: %v", status.Error)
	}
	return "retentra backup failed"
}

func sendDiscord(ctx context.Context, webhookURL, message string) error {
	body, err := json.Marshal(map[string]string{"content": message})
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
