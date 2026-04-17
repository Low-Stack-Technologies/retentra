# retentra

`retentra` is a small backup archiving tool intended to run from a YAML
configuration file:

```sh
retentra config.yaml
```

The configuration describes where backup contents come from, how the archive is
named and encoded, and where the finished archive should be written or uploaded.

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
- `format`: Archive container format, such as `tar`, `zip`, or `7z`.
- `compression`: Compression algorithm, such as `gzip`. Use `none` when the
  selected format should not be compressed.

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
```

Fields:

- `host`: SFTP server hostname.
- `port`: SFTP server port. Defaults to `22` when omitted.
- `username`: Remote username.
- `remote_path`: Remote directory where the archive should be uploaded.
- `identity_file`: SSH private key used for authentication.
- `password`: Password used for authentication when an SSH private key is not
  used.

Use either `identity_file` or `password` for SFTP authentication. Avoid storing
passwords or other secrets directly in the config file when possible.
