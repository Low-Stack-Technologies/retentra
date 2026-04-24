package retentra

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Sources       []SourceConfig       `yaml:"sources"`
	Report        ReportConfig         `yaml:"report"`
	Archive       ArchiveConfig        `yaml:"archive"`
	Outputs       []OutputConfig       `yaml:"outputs"`
	Notifications []NotificationConfig `yaml:"notifications"`
}

type ReportConfig struct {
	Title string `yaml:"title"`
}

type SourceConfig struct {
	Type     string          `yaml:"type"`
	Label    string          `yaml:"label"`
	Path     string          `yaml:"path"`
	Target   string          `yaml:"target"`
	Workdir  string          `yaml:"workdir"`
	Commands []string        `yaml:"commands"`
	Collect  []CollectConfig `yaml:"collect"`
}

type CollectConfig struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target"`
	Label  string `yaml:"label"`
}

type ArchiveConfig struct {
	Name        string `yaml:"name"`
	Format      string `yaml:"format"`
	Compression string `yaml:"compression"`
}

type OutputConfig struct {
	Type         string          `yaml:"type"`
	Label        string          `yaml:"label"`
	Path         string          `yaml:"path"`
	Host         string          `yaml:"host"`
	Port         int             `yaml:"port"`
	Username     string          `yaml:"username"`
	RemotePath   string          `yaml:"remote_path"`
	IdentityFile string          `yaml:"identity_file"`
	Password     string          `yaml:"password"`
	KnownHosts   string          `yaml:"known_hosts"`
	Insecure     bool            `yaml:"insecure_ignore_host_key"`
	Retention    RetentionConfig `yaml:"retention"`
}

type RetentionConfig struct {
	KeepLast int `yaml:"keep_last"`
}

type NotificationConfig struct {
	Type       string `yaml:"type"`
	WebhookURL string `yaml:"webhook_url"`
	URL        string `yaml:"url"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	normalizeConfig(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalizeConfig(cfg *Config) {
	cfg.Archive.Format = strings.ToLower(strings.TrimSpace(cfg.Archive.Format))
	cfg.Archive.Compression = strings.ToLower(strings.TrimSpace(cfg.Archive.Compression))
	if cfg.Archive.Compression == "" {
		cfg.Archive.Compression = "none"
	}
	for i := range cfg.Sources {
		cfg.Sources[i].Type = strings.ToLower(strings.TrimSpace(cfg.Sources[i].Type))
	}
	for i := range cfg.Outputs {
		cfg.Outputs[i].Type = strings.ToLower(strings.TrimSpace(cfg.Outputs[i].Type))
		if cfg.Outputs[i].Port == 0 {
			cfg.Outputs[i].Port = 22
		}
	}
	for i := range cfg.Notifications {
		cfg.Notifications[i].Type = strings.ToLower(strings.TrimSpace(cfg.Notifications[i].Type))
	}
}

func (cfg Config) Validate() error {
	var errs []error

	if cfg.Report.Title == "" {
		errs = append(errs, errors.New("report.title is required"))
	}

	if len(cfg.Sources) == 0 {
		errs = append(errs, errors.New("sources must contain at least one source"))
	}
	for i, source := range cfg.Sources {
		errs = append(errs, validateSource(i, source)...)
	}

	if cfg.Archive.Name == "" {
		errs = append(errs, errors.New("archive.name is required"))
	} else if err := validateArchiveName(cfg.Archive.Name); err != nil {
		errs = append(errs, fmt.Errorf("archive.name: %w", err))
	}
	switch cfg.Archive.Format {
	case "tar":
		if cfg.Archive.Compression != "none" && cfg.Archive.Compression != "gzip" {
			errs = append(errs, fmt.Errorf("archive.compression %q is unsupported for tar", cfg.Archive.Compression))
		}
	case "zip":
		if cfg.Archive.Compression != "none" {
			errs = append(errs, fmt.Errorf("archive.compression %q is unsupported for zip", cfg.Archive.Compression))
		}
	default:
		errs = append(errs, fmt.Errorf("archive.format %q is unsupported", cfg.Archive.Format))
	}

	if len(cfg.Outputs) == 0 {
		errs = append(errs, errors.New("outputs must contain at least one output"))
	}
	for i, output := range cfg.Outputs {
		errs = append(errs, validateOutput(i, output)...)
	}
	for i, notification := range cfg.Notifications {
		errs = append(errs, validateNotification(i, notification)...)
	}

	return errors.Join(errs...)
}

func validateSource(i int, source SourceConfig) []error {
	prefix := fmt.Sprintf("sources[%d]", i)
	var errs []error
	switch source.Type {
	case "filesystem":
		if source.Label == "" {
			errs = append(errs, fmt.Errorf("%s.label is required", prefix))
		}
		if source.Path == "" {
			errs = append(errs, fmt.Errorf("%s.path is required", prefix))
		}
		if source.Target == "" {
			errs = append(errs, fmt.Errorf("%s.target is required", prefix))
		} else if err := validateArchiveTarget(source.Target); err != nil {
			errs = append(errs, fmt.Errorf("%s.target: %w", prefix, err))
		}
	case "command":
		if source.Workdir == "" {
			errs = append(errs, fmt.Errorf("%s.workdir is required", prefix))
		}
		if len(source.Commands) == 0 {
			errs = append(errs, fmt.Errorf("%s.commands must contain at least one command", prefix))
		}
		if len(source.Collect) == 0 {
			errs = append(errs, fmt.Errorf("%s.collect must contain at least one entry", prefix))
		}
		for j, collect := range source.Collect {
			if collect.Label == "" {
				errs = append(errs, fmt.Errorf("%s.collect[%d].label is required", prefix, j))
			}
			if collect.Path == "" {
				errs = append(errs, fmt.Errorf("%s.collect[%d].path is required", prefix, j))
			}
			if collect.Target == "" {
				errs = append(errs, fmt.Errorf("%s.collect[%d].target is required", prefix, j))
			} else if err := validateArchiveTarget(collect.Target); err != nil {
				errs = append(errs, fmt.Errorf("%s.collect[%d].target: %w", prefix, j, err))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("%s.type %q is unsupported", prefix, source.Type))
	}
	return errs
}

func validateArchiveName(name string) error {
	if name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("%q must be a filename without path separators", name)
	}
	return nil
}

func validateArchiveTarget(target string) error {
	cleaned := path.Clean(strings.ReplaceAll(target, "\\", "/"))
	if cleaned == "." || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("%q must be a relative archive path without .. segments", target)
	}
	return nil
}

func validateOutput(i int, output OutputConfig) []error {
	prefix := fmt.Sprintf("outputs[%d]", i)
	var errs []error
	switch output.Type {
	case "filesystem":
		if output.Label == "" {
			errs = append(errs, fmt.Errorf("%s.label is required", prefix))
		}
		if output.Path == "" {
			errs = append(errs, fmt.Errorf("%s.path is required", prefix))
		}
	case "gdrive":
		if output.Label == "" {
			errs = append(errs, fmt.Errorf("%s.label is required", prefix))
		}
		if output.Path == "" {
			errs = append(errs, fmt.Errorf("%s.path is required", prefix))
		} else if err := validateDrivePath(output.Path); err != nil {
			errs = append(errs, fmt.Errorf("%s.path: %w", prefix, err))
		}
	case "sftp":
		if output.Label == "" {
			errs = append(errs, fmt.Errorf("%s.label is required", prefix))
		}
		if output.Host == "" {
			errs = append(errs, fmt.Errorf("%s.host is required", prefix))
		}
		if output.Username == "" {
			errs = append(errs, fmt.Errorf("%s.username is required", prefix))
		}
		if output.RemotePath == "" {
			errs = append(errs, fmt.Errorf("%s.remote_path is required", prefix))
		}
		if (output.IdentityFile == "") == (output.Password == "") {
			errs = append(errs, fmt.Errorf("%s must set exactly one of identity_file or password", prefix))
		}
	default:
		errs = append(errs, fmt.Errorf("%s.type %q is unsupported", prefix, output.Type))
	}
	if output.Retention.KeepLast < 0 {
		errs = append(errs, fmt.Errorf("%s.retention.keep_last must be greater than or equal to 0", prefix))
	}
	return errs
}

func validateDrivePath(target string) error {
	cleaned := path.Clean(strings.ReplaceAll(target, "\\", "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("%q must be a relative Drive folder path without .. segments", target)
	}
	return nil
}

func validateNotification(i int, notification NotificationConfig) []error {
	prefix := fmt.Sprintf("notifications[%d]", i)
	switch notification.Type {
	case "discord":
		if notification.WebhookURL == "" {
			return []error{fmt.Errorf("%s.webhook_url is required", prefix)}
		}
	case "ntfy":
		if notification.URL == "" {
			return []error{fmt.Errorf("%s.url is required", prefix)}
		}
		if (notification.Username == "") != (notification.Password == "") {
			return []error{fmt.Errorf("%s must set both username and password for ntfy authentication", prefix)}
		}
	default:
		return []error{fmt.Errorf("%s.type %q is unsupported", prefix, notification.Type)}
	}
	return nil
}
