package retentra

import "testing"

func TestConfigValidateAcceptsDocumentedShape(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{
			{Type: "filesystem", Path: "/data", Target: "data"},
			{Type: "command", Workdir: "{tmpdir}", Commands: []string{"true"}, Collect: []CollectConfig{{Path: "{tmpdir}/db", Target: "db"}}},
		},
		Archive: ArchiveConfig{Name: "backup-{date}.tar.gz", Format: "tar", Compression: "gzip"},
		Outputs: []OutputConfig{
			{Type: "filesystem", Path: "/backups"},
			{Type: "sftp", Host: "example.com", Port: 22, Username: "backup", RemotePath: "/backups", Password: "secret"},
		},
		Notifications: []NotificationConfig{
			{Type: "discord", WebhookURL: "https://example.com/discord"},
			{Type: "ntfy", URL: "https://example.com/topic", Username: "user", Password: "pass"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsUnsupportedArchive(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{{Type: "filesystem", Path: "/data", Target: "data"}},
		Archive: ArchiveConfig{Name: "backup.7z", Format: "7z", Compression: "none"},
		Outputs: []OutputConfig{{Type: "filesystem", Path: "/backups"}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported archive error")
	}
}

func TestConfigValidateRejectsAmbiguousSFTPAuth(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{{Type: "filesystem", Path: "/data", Target: "data"}},
		Archive: ArchiveConfig{Name: "backup.tar", Format: "tar", Compression: "none"},
		Outputs: []OutputConfig{{Type: "sftp", Host: "example.com", Username: "backup", RemotePath: "/backups"}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want sftp auth error")
	}
}

func TestConfigValidateRejectsUnsafeArchiveTarget(t *testing.T) {
	cfg := Config{
		Sources: []SourceConfig{{Type: "filesystem", Path: "/data", Target: "../data"}},
		Archive: ArchiveConfig{Name: "backup.tar", Format: "tar", Compression: "none"},
		Outputs: []OutputConfig{{Type: "filesystem", Path: "/backups"}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe target error")
	}
}

func TestConfigValidateRejectsPartialNTFYAuth(t *testing.T) {
	cfg := Config{
		Sources:       []SourceConfig{{Type: "filesystem", Path: "/data", Target: "data"}},
		Archive:       ArchiveConfig{Name: "backup.tar", Format: "tar", Compression: "none"},
		Outputs:       []OutputConfig{{Type: "filesystem", Path: "/backups"}},
		Notifications: []NotificationConfig{{Type: "ntfy", URL: "https://ntfy.sh/topic", Username: "user"}},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want partial ntfy auth error")
	}
}
