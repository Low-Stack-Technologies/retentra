package retentra

import "testing"

func TestConfigValidateAcceptsDocumentedShape(t *testing.T) {
	cfg := validConfig()
	cfg.Sources = append(cfg.Sources, SourceConfig{
		Type:     "command",
		Workdir:  "{tmpdir}",
		Commands: []string{"true"},
		Collect:  []CollectConfig{{Label: "DB Dump: app", Path: "{tmpdir}/db", Target: "db"}},
	})
	cfg.Outputs = append(cfg.Outputs, OutputConfig{Type: "sftp", Label: "Upload (example.com)", Host: "example.com", Port: 22, Username: "backup", RemotePath: "/backups", Password: "secret"})
	cfg.Notifications = []NotificationConfig{
		{Type: "discord", WebhookURL: "https://example.com/discord"},
		{Type: "ntfy", URL: "https://example.com/topic", Username: "user", Password: "pass"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAcceptsGoogleDriveOutput(t *testing.T) {
	cfg := validConfig()
	cfg.Outputs = []OutputConfig{{Type: "gdrive", Label: "Google Drive", Path: "Backups/App"}}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsUnsupportedArchive(t *testing.T) {
	cfg := validConfig()
	cfg.Archive = ArchiveConfig{Name: "backup.7z", Format: "7z", Compression: "none"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported archive error")
	}
}

func TestConfigValidateRejectsArchiveNamePath(t *testing.T) {
	for _, name := range []string{"../backup.tar.gz", "nested/backup.tar.gz", `nested\backup.tar.gz`, ".", ".."} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Archive.Name = name

			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want unsafe archive name error")
			}
		})
	}
}

func TestConfigValidateRejectsAmbiguousSFTPAuth(t *testing.T) {
	cfg := validConfig()
	cfg.Outputs = []OutputConfig{{Type: "sftp", Label: "Upload (example.com)", Host: "example.com", Username: "backup", RemotePath: "/backups"}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want sftp auth error")
	}
}

func TestConfigValidateRejectsUnsafeArchiveTarget(t *testing.T) {
	cfg := validConfig()
	cfg.Sources = []SourceConfig{{Type: "filesystem", Label: "Site files", Path: "/data", Target: "../data"}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe target error")
	}
}

func TestConfigValidateRejectsPartialNTFYAuth(t *testing.T) {
	cfg := validConfig()
	cfg.Notifications = []NotificationConfig{{Type: "ntfy", URL: "https://ntfy.sh/topic", Username: "user"}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want partial ntfy auth error")
	}
}

func TestConfigValidateRejectsMissingReportTitle(t *testing.T) {
	cfg := validConfig()
	cfg.Report.Title = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing report title error")
	}
}

func TestConfigValidateRejectsMissingSourceLabel(t *testing.T) {
	cfg := validConfig()
	cfg.Sources[0].Label = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing source label error")
	}
}

func TestConfigValidateRejectsMissingCollectLabel(t *testing.T) {
	cfg := validConfig()
	cfg.Sources = []SourceConfig{{
		Type:     "command",
		Workdir:  "{tmpdir}",
		Commands: []string{"true"},
		Collect:  []CollectConfig{{Path: "{tmpdir}/db", Target: "db"}},
	}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing collect label error")
	}
}

func TestConfigValidateRejectsMissingOutputLabel(t *testing.T) {
	cfg := validConfig()
	cfg.Outputs[0].Label = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing output label error")
	}
}

func TestConfigValidateRejectsMissingGoogleDrivePath(t *testing.T) {
	cfg := validConfig()
	cfg.Outputs = []OutputConfig{{Type: "gdrive", Label: "Google Drive"}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing Google Drive path error")
	}
}

func TestConfigValidateRejectsNegativeRetention(t *testing.T) {
	cfg := validConfig()
	cfg.Outputs[0].Retention.KeepLast = -1

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want negative retention error")
	}
}

func validConfig() Config {
	return Config{
		Report:  ReportConfig{Title: "Backup Report"},
		Sources: []SourceConfig{{Type: "filesystem", Label: "Site files", Path: "/data", Target: "data"}},
		Archive: ArchiveConfig{Name: "backup.tar", Format: "tar", Compression: "none"},
		Outputs: []OutputConfig{{Type: "filesystem", Label: "Local copy", Path: "/backups"}},
	}
}
