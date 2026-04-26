# retentra

`retentra` is a small backup archiving tool intended to run from one or more
YAML configuration files:

```sh
retentra config.yaml [config2.yaml ...]
```

The configuration describes where backup contents come from, how the archive is
named and encoded, and where the finished archive should be written or uploaded.

## Install

Install the latest Linux amd64 or arm64 release:

```sh
curl -fsSL https://raw.githubusercontent.com/Low-Stack-Technologies/retentra/main/install.sh | bash
```

The installer resolves the newest published release asset through the GitHub
Releases API and downloads the asset matching your machine:
`retentra-linux-amd64` or `retentra-linux-arm64`. It also downloads the matching
`.sha256` asset and verifies the binary before installing it. Published releases
must include these assets:

- `retentra-linux-amd64`
- `retentra-linux-amd64.sha256`
- `retentra-linux-arm64`
- `retentra-linux-arm64.sha256`

The installer writes `retentra` to `$HOME/.local/bin` by default. To choose a
different directory:

```sh
curl -fsSL https://raw.githubusercontent.com/Low-Stack-Technologies/retentra/main/install.sh | INSTALL_DIR=/usr/local/bin bash
```

## Build And Test

Prerequisites:

- Go
- Docker, only for SFTP integration tests
- Google OAuth client ID and secret only if you want to use the Google Drive
  output or `retentra auth google`

Google auth stores refresh credentials in the OS secret store by default. If
the secret store is unavailable, `retentra auth google login
--allow-file-token-storage` falls back to encrypted file-backed storage in the
local config directory.
Login uses Google's limited-input device flow, so it can be completed on a
separate machine over SSH without running a browser on the Retentra host.
The file-backed mode also creates a local key file in that directory, so keep
the whole config directory intact if you move or back up the auth state.

Run the CLI directly during development:

```sh
go run ./cmd/retentra config.yaml
go run ./cmd/retentra *-retentra.yaml
```

Show the help menu:

```sh
retentra --help
retentra -h
```

Validate one or more configuration files without running backups:

```sh
retentra validate config.yaml
retentra validate *-retentra.yaml
```

Google Drive auth commands:

```sh
retentra auth google login
retentra auth google status
retentra auth google refresh
retentra auth google logout
```

Build a local binary:

```sh
make build
./bin/retentra config.yaml
./bin/retentra --no-parallel *-retentra.yaml
```

`make build` reads `.env` if one exists and uses `RETENTRA_GOOGLE_CLIENT_ID`
and `RETENTRA_GOOGLE_CLIENT_SECRET` as build-time inputs. The release workflow
loads the same values from the matching GitHub secrets.

Run tests:

```sh
make test
make test-integration
```

The integration test starts a temporary Docker SFTP server and is not included
in the default test target.

## Backup Flow

When multiple configs are provided, `retentra` runs them in parallel by default.
Use `--no-parallel` to run them sequentially. Output is prefixed with the config
path so concurrent runs can be distinguished.

`retentra` is designed around three steps:

1. Collect files from one or more configured sources.
2. Package those files into an archive such as `backup-2026-04-17.tar.gz`.
3. Copy or upload that archive to one or more configured outputs.

## Configuration

Start from [config.example.yaml](config.example.yaml) and adjust paths,
commands, archive settings, outputs, and notifications for your environment.
See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

```yaml
report:
  title: App Backup Report

sources:
  - type: filesystem
    label: Site files
    path: /srv/app/data
    target: app/data

archive:
  name: backup-{date}.tar.gz
  format: tar
  compression: gzip

outputs:
  - type: filesystem
    label: Local copy
    path: /var/backups

  - type: gdrive
    label: Google Drive backup
    path: Backups/App
```
