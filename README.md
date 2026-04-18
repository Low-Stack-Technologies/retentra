# retentra

`retentra` is a small backup archiving tool intended to run from a YAML
configuration file:

```sh
retentra config.yaml
```

The configuration describes where backup contents come from, how the archive is
named and encoded, and where the finished archive should be written or uploaded.

## Build And Test

Prerequisites:

- Go
- Docker, only for SFTP integration tests

Run the CLI directly during development:

```sh
go run ./cmd/retentra config.yaml
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
commands, archive settings, and output details for your environment.

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

### Sources

The `sources` list defines the content that should be included in the backup
archive.

#### Filesystem Source

Use a filesystem source when the backup input already exists as a file or
directory.

```yaml
sources:
  - type: filesystem
    path: /srv/app/data
    target: app/data
```

The configured `path` may point to a file or directory.

Fields:

- `path`: File or directory to include in the archive.
- `target`: Path where the file or directory should be placed inside the
  archive.

#### Command Source

Use a command source when backup input must be generated before archiving, such
as exporting a database or rendering application state to files.

```yaml
sources:
  - type: command
    workdir: "{tmpdir}"
    commands:
      - mkdir -p "{tmpdir}/export"
      - sqlite3 /srv/app/app.db ".backup '{tmpdir}/export/app.db'"
    collect:
      - path: "{tmpdir}/export/app.db"
        target: app/app.db
```

Fields:

- `workdir`: Directory where commands run.
- `commands`: Shell commands to run in order.
- `collect`: Files or directories that should be added to the archive after the
  commands complete.
- `collect[].path`: Generated file or directory to include in the archive.
- `collect[].target`: Path where the generated file or directory should be
  placed inside the archive.

Command sources should write generated backup content into temporary or
dedicated output paths and list only those paths in `collect`.

### Temporary Directory

`retentra` creates one temporary directory for each run. Use the `{tmpdir}`
placeholder to refer to that directory in command sources.

Supported locations:

- Command source `workdir`.
- Command source `commands`.
- Command source `collect[].path`.

The temporary directory is intended for generated backup content and is cleaned
up after the run completes.

### Archive

The `archive` section controls the archive file that `retentra` creates.

```yaml
archive:
  name: backup-{date}.tar.gz
  format: tar
  compression: gzip
```

Fields:

- `name`: Archive filename template. `{date}` is replaced with the run date.
- `format`: Archive container format. Supported values are `tar` and `zip`.
- `compression`: Compression algorithm. Supported values are `gzip` or `none`
  for `tar`, and `none` for `zip`.

### Outputs

The `outputs` section controls where the finished archive is stored. `retentra`
creates the archive once, then copies or uploads that same archive to each
configured output in order.

#### Filesystem Output

Use filesystem output to write the archive to a local directory.

```yaml
outputs:
  - type: filesystem
    path: /var/backups
```

The archive is written inside the configured `path` using the rendered archive
name.

#### SFTP Output

Use SFTP output to upload the archive to a remote server.

```yaml
outputs:
  - type: sftp
    host: backup.example.com
    port: 22
    username: backup
    remote_path: /backups
    identity_file: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts
```

Fields:

- `host`: SFTP server hostname.
- `port`: SFTP server port. Defaults to `22` when omitted.
- `username`: Remote username.
- `remote_path`: Remote directory where the archive should be uploaded.
- `identity_file`: SSH private key used for authentication.
- `password`: Password used for authentication when an SSH private key is not
  used.
- `known_hosts`: Known hosts file used for host key verification. Defaults to
  `~/.ssh/known_hosts`.
- `insecure_ignore_host_key`: Set to `true` to skip host key verification for
  local testing or controlled environments.

Use either `identity_file` or `password` for SFTP authentication. Avoid storing
passwords or other secrets directly in the config file when possible.

### Notifications

The `notifications` section sends process status messages after a run succeeds
or fails. Notifications do not send the archive file.

#### Discord Notification

```yaml
notifications:
  - type: discord
    webhook_url: https://discord.com/api/webhooks/...
```

#### NTFY Notification

```yaml
notifications:
  - type: ntfy
    url: https://ntfy.sh/my-topic
    username: optional-username
    password: optional-password
```

Fields:

- `webhook_url`: Discord webhook URL.
- `url`: NTFY topic URL.
- `username`: Optional NTFY username.
- `password`: Optional NTFY password. Set both `username` and `password` when
  using NTFY authentication.
