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

func collectSources(ctx context.Context, sources []SourceConfig, p placeholders) ([]archiveItem, error) {
	var items []archiveItem
	for i, source := range sources {
		switch source.Type {
		case "filesystem":
			path := p.expand(source.Path)
			if _, err := os.Stat(path); err != nil {
				return nil, fmt.Errorf("sources[%d].path: %w", i, err)
			}
			items = append(items, archiveItem{Path: path, Target: source.Target})
		case "command":
			if err := runCommandSource(ctx, source, p); err != nil {
				return nil, fmt.Errorf("sources[%d]: %w", i, err)
			}
			for j, collect := range source.Collect {
				path := p.expand(collect.Path)
				if _, err := os.Stat(path); err != nil {
					return nil, fmt.Errorf("sources[%d].collect[%d].path: %w", i, j, err)
				}
				items = append(items, archiveItem{Path: path, Target: collect.Target})
			}
		}
	}
	return items, nil
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
