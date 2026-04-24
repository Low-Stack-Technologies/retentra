package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"retentra/internal/retentra"
)

var runConfig = retentra.Run
var loadConfig = retentra.LoadConfig

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

type cliOptions struct {
	validate    bool
	parallel    bool
	configPaths []string
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "auth" {
		if err := runAuthCLI(args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "retentra: %v\n", err)
			return 1
		}
		return 0
	}
	opts, help, err := parseCLI(args)
	if help {
		printHelp(stdout)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "retentra: %v\n", err)
		fmt.Fprintln(stderr, "run 'retentra --help' for more information")
		return 2
	}

	if opts.validate {
		if !validateConfigs(opts.configPaths, stdout, stderr) {
			return 1
		}
		return 0
	}

	if !runConfigs(opts, stdout, stderr) {
		return 1
	}
	return 0
}

func parseCLI(args []string) (cliOptions, bool, error) {
	if len(args) > 0 && args[0] == "validate" {
		return parseValidateCLI(args[1:])
	}

	var noParallel bool
	var help bool
	var configArgs []string
	for _, arg := range args {
		switch arg {
		case "--no-parallel":
			noParallel = true
		case "--help", "-h":
			help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return cliOptions{}, false, fmt.Errorf("unknown flag %q", arg)
			}
			configArgs = append(configArgs, arg)
		}
	}
	if help {
		return cliOptions{}, true, nil
	}
	configPaths, err := expandConfigArgs(configArgs)
	if err != nil {
		return cliOptions{}, false, err
	}
	if len(configPaths) == 0 {
		return cliOptions{}, false, fmt.Errorf("usage: retentra [--no-parallel] config.yaml [config2.yaml ...]")
	}
	return cliOptions{parallel: !noParallel, configPaths: configPaths}, false, nil
}

func runAuthCLI(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAuthHelp(stdout)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		printAuthHelp(stdout)
		return nil
	}
	if args[0] != "google" {
		return fmt.Errorf("usage: retentra auth google [login|status|refresh|logout]")
	}
	return runGoogleAuthCLI(args[1:], stdout)
}

func runGoogleAuthCLI(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printGoogleAuthHelp(stdout)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		printGoogleAuthHelp(stdout)
		return nil
	}
	switch args[0] {
	case "login":
		allowFileTokenStorage := false
		for _, arg := range args[1:] {
			switch arg {
			case "--allow-file-token-storage":
				allowFileTokenStorage = true
			case "--help", "-h":
				printGoogleAuthHelp(stdout)
				return nil
			default:
				return fmt.Errorf("usage: retentra auth google login [--allow-file-token-storage]")
			}
		}
		return retentra.GoogleLoginWithOptions(context.Background(), stdout, allowFileTokenStorage)
	case "status":
		return retentra.GoogleStatus(stdout)
	case "refresh":
		return retentra.GoogleRefresh(context.Background(), stdout)
	case "logout":
		return retentra.GoogleLogout(stdout)
	default:
		return fmt.Errorf("usage: retentra auth google [login|status|refresh|logout]")
	}
}

func parseValidateCLI(args []string) (cliOptions, bool, error) {
	var help bool
	var configArgs []string
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return cliOptions{}, false, fmt.Errorf("unknown flag %q", arg)
			}
			configArgs = append(configArgs, arg)
		}
	}
	if help {
		return cliOptions{}, true, nil
	}
	configPaths, err := expandConfigArgs(configArgs)
	if err != nil {
		return cliOptions{}, false, err
	}
	if len(configPaths) == 0 {
		return cliOptions{}, false, fmt.Errorf("usage: retentra validate config.yaml [config2.yaml ...]")
	}
	return cliOptions{validate: true, configPaths: configPaths}, false, nil
}

func expandConfigArgs(args []string) ([]string, error) {
	var paths []string
	seen := make(map[string]string)
	for _, arg := range args {
		matches := []string{arg}
		if hasGlobMeta(arg) {
			var err error
			matches, err = filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("glob pattern %q matched no files", arg)
			}
		}
		for _, match := range matches {
			cleaned := filepath.Clean(match)
			key, err := filepath.Abs(cleaned)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", match, err)
			}
			if previous, ok := seen[key]; ok {
				return nil, fmt.Errorf("duplicate config path %q also provided as %q", cleaned, previous)
			}
			seen[key] = cleaned
			paths = append(paths, cleaned)
		}
	}
	return paths, nil
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func validateConfigs(configPaths []string, stdout, stderr io.Writer) bool {
	var mu sync.Mutex
	ok := true
	for _, configPath := range configPaths {
		if _, err := loadConfig(configPath); err != nil {
			writeRunError(stderr, &mu, configPath, err)
			ok = false
			continue
		}
		fmt.Fprintln(newPrefixWriter(stdout, configPath, &mu), "Config is valid")
	}
	return ok
}

func runConfigs(opts cliOptions, stdout, stderr io.Writer) bool {
	var mu sync.Mutex
	if !opts.parallel {
		ok := true
		for _, configPath := range opts.configPaths {
			if err := runConfig(context.Background(), configPath, newPrefixWriter(stdout, configPath, &mu)); err != nil {
				writeRunError(stderr, &mu, configPath, err)
				ok = false
			}
		}
		return ok
	}

	var wg sync.WaitGroup
	errs := make(chan configRunError, len(opts.configPaths))
	for _, configPath := range opts.configPaths {
		wg.Add(1)
		go func(configPath string) {
			defer wg.Done()
			if err := runConfig(context.Background(), configPath, newPrefixWriter(stdout, configPath, &mu)); err != nil {
				errs <- configRunError{configPath: configPath, err: err}
			}
		}(configPath)
	}
	wg.Wait()
	close(errs)

	ok := true
	for runErr := range errs {
		writeRunError(stderr, &mu, runErr.configPath, runErr.err)
		ok = false
	}
	return ok
}

type configRunError struct {
	configPath string
	err        error
}

func writeRunError(stderr io.Writer, mu *sync.Mutex, configPath string, err error) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(stderr, "[%s] retentra: %v\n", configPath, err)
}

type prefixWriter struct {
	out         io.Writer
	prefix      string
	mu          *sync.Mutex
	atLineStart bool
}

func newPrefixWriter(out io.Writer, prefix string, mu *sync.Mutex) io.Writer {
	return &prefixWriter{out: out, prefix: prefix, mu: mu, atLineStart: true}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		if w.atLineStart {
			if _, err := fmt.Fprintf(w.out, "[%s] ", w.prefix); err != nil {
				return 0, err
			}
			w.atLineStart = false
		}
		if _, err := w.out.Write([]byte{b}); err != nil {
			return 0, err
		}
		if b == '\n' {
			w.atLineStart = true
		}
	}
	return len(p), nil
}

func printHelp(out io.Writer) {
	fmt.Fprint(out, `retentra creates backup archives from a YAML configuration.

Usage:
  retentra [--no-parallel] config.yaml [config2.yaml ...]
  retentra validate config.yaml [config2.yaml ...]
  retentra auth google [login|status|refresh|logout]

Arguments:
  config.yaml    Path or glob pattern for a retentra YAML configuration file.

Options:
  --no-parallel  Run configs sequentially instead of in parallel.
  -h, --help     Show this help message.

Examples:
  retentra config.yaml
  retentra config1.yaml config2.yaml
  retentra *-retentra.yaml
  retentra --no-parallel *-retentra.yaml
  retentra /etc/retentra/nightly.yaml
  retentra validate /etc/retentra/nightly.yaml
  retentra auth google status
`)
}

func printAuthHelp(out io.Writer) {
	fmt.Fprint(out, `retentra auth manages external integrations.

Usage:
  retentra auth google [login|status|refresh|logout]

See also:
  retentra auth google --help
`)
}

func printGoogleAuthHelp(out io.Writer) {
	fmt.Fprint(out, `retentra auth google manages Google Drive credentials.

Usage:
  retentra auth google login [--allow-file-token-storage]
  retentra auth google status
  retentra auth google refresh
  retentra auth google logout

Environment:
  RETENTRA_GOOGLE_CLIENT_ID (build-time input; loaded by the Makefile or release workflow)
  RETENTRA_GOOGLE_CLIENT_SECRET (build-time input; loaded by the Makefile or release workflow)
  RETENTRA_GOOGLE_CONFIG_DIR
  RETENTRA_GOOGLE_AUTH_URL
  RETENTRA_GOOGLE_TOKEN_URL
  RETENTRA_GOOGLE_REVOKE_URL
  RETENTRA_GOOGLE_API_BASE_URL
  RETENTRA_GOOGLE_UPLOAD_BASE_URL

The secret store is used by default. Use --allow-file-token-storage only when
the OS secret store is unavailable and you explicitly accept weaker storage.
`)
}
