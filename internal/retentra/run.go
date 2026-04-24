package retentra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Status struct {
	Success        bool
	ReportTitle    string
	ArchiveName    string
	ArchivePath    string
	SourceResults  []ReportResult
	Included       []string
	ArchiveCreated bool
	ArchiveError   error
	OutputResults  []ReportResult
	Error          error
}

type ReportResult struct {
	Label string
	Error error
}

func (result ReportResult) Success() bool {
	return result.Error == nil
}

func Run(ctx context.Context, configPath string, out io.Writer) error {
	fmt.Fprintln(out, "Loading config")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	tmpdir, err := os.MkdirTemp("", "retentra-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	p := placeholders{tmpdir: tmpdir, now: time.Now()}
	archiveName := p.expand(cfg.Archive.Name)
	if err := validateArchiveName(archiveName); err != nil {
		return fmt.Errorf("archive.name: %w", err)
	}
	archivePath := filepath.Join(tmpdir, archiveName)
	status := Status{ReportTitle: cfg.Report.Title, ArchiveName: archiveName, ArchivePath: archivePath}

	runErr := runBackup(ctx, cfg, p, archivePath, archiveName, out, &status)
	status.Success = runErr == nil
	status.Error = runErr

	fmt.Fprintln(out)
	fmt.Fprintln(out, statusMessage(status))

	notifyErr := sendNotifications(ctx, cfg.Notifications, status)
	if runErr != nil {
		if notifyErr != nil {
			return fmt.Errorf("%w; additionally failed to send notification: %v", runErr, notifyErr)
		}
		return runErr
	}
	if notifyErr != nil {
		return notifyErr
	}
	return nil
}

func runBackup(ctx context.Context, cfg Config, p placeholders, archivePath, archiveName string, out io.Writer, status *Status) error {
	fmt.Fprintln(out, "Collecting sources")
	items, err := collectSources(ctx, cfg.Sources, p, status)
	if err != nil {
		return err
	}
	for _, item := range items {
		status.Included = append(status.Included, item.Target)
	}

	fmt.Fprintln(out, "Creating archive")
	if err := createArchive(archivePath, cfg.Archive, items); err != nil {
		status.ArchiveError = err
		return err
	}
	status.ArchiveCreated = true

	fmt.Fprintln(out, "Delivering outputs")
	var outputErrs []error
	for i, output := range cfg.Outputs {
		if _, err := deliverOutput(ctx, output, archivePath, archiveName); err != nil {
			status.OutputResults = append(status.OutputResults, ReportResult{Label: output.Label, Error: err})
			outputErrs = append(outputErrs, fmt.Errorf("outputs[%d]: %w", i, err))
			continue
		}
		if err := applyOutputRetention(ctx, output, cfg.Archive.Name); err != nil {
			status.OutputResults = append(status.OutputResults, ReportResult{Label: output.Label, Error: err})
			outputErrs = append(outputErrs, fmt.Errorf("outputs[%d].retention: %w", i, err))
			continue
		}
		status.OutputResults = append(status.OutputResults, ReportResult{Label: output.Label})
	}
	return joinErrors(outputErrs)
}

func joinErrors(errs []error) error {
	return errors.Join(errs...)
}
