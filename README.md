# retentra

`retentra` is a small backup archiving tool intended to run from a YAML
configuration file:

```sh
retentra config.yaml
```

The configuration describes where backup contents come from, how the archive is
named and encoded, and where the finished archive should be written or uploaded.

## Install

Install the latest Linux amd64 release:

```sh
curl -fsSL https://raw.githubusercontent.com/Low-Stack-Technologies/retentra/main/install.sh | bash
```

The installer resolves the newest published release asset through the GitHub
Releases API and downloads `retentra-linux-amd64`. The release workflow attaches
that asset after a release is published.

The installer writes `retentra` to `$HOME/.local/bin` by default. To choose a
different directory:

```sh
curl -fsSL https://raw.githubusercontent.com/Low-Stack-Technologies/retentra/main/install.sh | INSTALL_DIR=/usr/local/bin bash
```

## Build And Test

Prerequisites:

- Go
- Docker, only for SFTP integration tests

Run the CLI directly during development:

```sh
go run ./cmd/retentra config.yaml
```

Show the help menu:

```sh
retentra --help
retentra -h
```

Build a local binary:

```sh
make build
./bin/retentra config.yaml
```

Run tests:

```sh
make test
make test-integration
```

The integration test starts a temporary Docker SFTP server and is not included
in the default test target.

## Backup Flow

`retentra` is designed around three steps:

1. Collect files from one or more configured sources.
2. Package those files into an archive such as `backup-2026-04-17.tar.gz`.
3. Copy or upload that archive to one or more configured outputs.

## Configuration

Start from [config.example.yaml](config.example.yaml) and adjust paths,
commands, archive settings, outputs, and notifications for your environment.
See [CONFIGURATION.md](CONFIGURATION.md) for the full configuration reference.

```yaml
sources:
  - type: filesystem
    path: /srv/app/data
    target: app/data

archive:
  name: backup-{date}.tar.gz
  format: tar
  compression: gzip

outputs:
  - type: filesystem
    path: /var/backups
```
