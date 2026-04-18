package retentra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type archiveItem struct {
	Path   string
	Target string
}

func collectSources(ctx context.Context, sources []SourceConfig, p placeholders, status *Status) ([]archiveItem, error) {
	var items []archiveItem
	for i, source := range sources {
		switch source.Type {
		case "filesystem":
			path := p.expand(source.Path)
			if _, err := os.Stat(path); err != nil {
				appendSourceResult(status, source.Label, err)
				return nil, fmt.Errorf("sources[%d].path: %w", i, err)
			}
			items = append(items, archiveItem{Path: path, Target: source.Target})
			appendSourceResult(status, source.Label, nil)
		case "command":
			if err := runCommandSource(ctx, source, p); err != nil {
				return nil, fmt.Errorf("sources[%d]: %w", i, err)
			}
			for j, collect := range source.Collect {
				path := p.expand(collect.Path)
				if _, err := os.Stat(path); err != nil {
					appendSourceResult(status, collect.Label, err)
					return nil, fmt.Errorf("sources[%d].collect[%d].path: %w", i, j, err)
				}
				items = append(items, archiveItem{Path: path, Target: collect.Target})
				appendSourceResult(status, collect.Label, nil)
			}
		}
	}
	return items, nil
}

func appendSourceResult(status *Status, label string, err error) {
	if status == nil {
		return
	}
	status.SourceResults = append(status.SourceResults, ReportResult{Label: label, Error: err})
}

func runCommandSource(ctx context.Context, source SourceConfig, p placeholders) error {
	workdir := p.expand(source.Workdir)
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}

	for i, command := range source.Commands {
		expanded := p.expand(command)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", expanded)
		cmd.Dir = workdir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("commands[%d] failed: %w: %s", i, err, string(output))
		}
	}
	return nil
}
