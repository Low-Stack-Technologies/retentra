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
	Success     bool
	ArchiveName string
	ArchivePath string
	Outputs     []string
	Error       error
}

func Run(ctx context.Context, configPath string, out io.Writer) error {
	fmt.Fprintln(out, "loading config")
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
	archivePath := filepath.Join(tmpdir, archiveName)
	status := Status{ArchiveName: archiveName, ArchivePath: archivePath}

	runErr := runBackup(ctx, cfg, p, archivePath, archiveName, out, &status)
	status.Success = runErr == nil
	status.Error = runErr

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
	fmt.Fprintln(out, "collecting sources")
	items, err := collectSources(ctx, cfg.Sources, p)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "creating archive")
	if err := createArchive(archivePath, cfg.Archive, items); err != nil {
		return err
	}

	fmt.Fprintln(out, "delivering outputs")
	for i, output := range cfg.Outputs {
		desc, err := deliverOutput(output, archivePath, archiveName)
		if err != nil {
			return fmt.Errorf("outputs[%d]: %w", i, err)
		}
		status.Outputs = append(status.Outputs, desc)
	}
	return nil
}

func joinErrors(errs []error) error {
	return errors.Join(errs...)
}
