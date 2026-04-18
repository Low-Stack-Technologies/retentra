# Configuration

`retentra` reads one YAML configuration file:

```sh
retentra config.yaml
```

The file describes what to collect, how to archive it, where to deliver the
archive, and which status notifications to send.

## Top-Level Structure

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

notifications:
  - type: ntfy
    url: https://ntfy.sh/my-topic
```

Required top-level sections:

- `report`: display title for stdout and notification reports.
- `sources`: one or more backup inputs.
- `archive`: archive filename, format, and compression settings.
- `outputs`: one or more archive delivery targets.

Optional top-level sections:

- `notifications`: zero or more status notification targets.

## Placeholders

`retentra` supports these placeholders:

- `{tmpdir}`: one automatically-created temporary directory for the run.
- `{date}`: the local run date formatted as `YYYY-MM-DD`.

`{tmpdir}` is cleaned up after the run completes, including failed runs.

Supported placeholder locations:

- `archive.name`
- Filesystem source `path`
- Command source `workdir`
- Command source `commands`
- Command source `collect[].path`

Archive `target` values do not expand placeholders.

## Sources

The `sources` list defines files and directories that should be added to the
archive. At least one source is required.

Each source must have a `type`.

Supported source types:

- `filesystem`
- `command`

### Filesystem Source

Use a filesystem source when the backup input already exists as a file or
directory.

```yaml
sources:
  - type: filesystem
    label: Site files
    path: /srv/app/data
    target: app/data
```

Fields:

- `type`: must be `filesystem`.
- `label`: required display label for stdout and notification reports.
- `path`: required file or directory path to include in the archive.
- `target`: required path where the file or directory should be placed inside
  the archive.

If `path` is a file, the file is written to exactly `target` inside the archive.

If `path` is a directory, the directory and its contents are written under
`target`. For example, `/srv/app/data/users.json` with `target: app/data` becomes
`app/data/users.json` in the archive.

`target` must be a relative archive path. It cannot be empty, absolute, `.`, or
contain `..` path segments.

### Command Source

Use a command source when backup input must be generated before archiving, such
as exporting a database.

```yaml
sources:
  - type: command
    workdir: "{tmpdir}"
    commands:
      - mkdir -p "{tmpdir}/export"
      - sqlite3 /srv/app/app.db ".backup '{tmpdir}/export/app.db'"
    collect:
      - label: DB Dump: app
        path: "{tmpdir}/export/app.db"
        target: app/app.db
```

Fields:

- `type`: must be `command`.
- `workdir`: required directory where commands run.
- `commands`: required non-empty list of shell commands.
- `collect`: required non-empty list of generated files or directories to add
  to the archive.
- `collect[].label`: required display label for stdout and notification reports.
- `collect[].path`: required generated file or directory path.
- `collect[].target`: required archive-internal target path.

Commands run in order through:

```sh
/bin/sh -c 'command'
```

If any command exits non-zero, the command source fails and later commands are
not run.

`retentra` creates `workdir` when it does not already exist. Command output is
captured and included in the error message when a command fails.

Command sources should write generated backup content into `{tmpdir}` or another
dedicated output path, then list only the generated backup artifacts in
`collect`.

`collect[].target` follows the same archive target rules as filesystem source
`target`: it must be relative and cannot contain `..` path segments.

## Archive

The `archive` section controls the archive file that `retentra` creates.

```yaml
archive:
  name: backup-{date}.tar.gz
  format: tar
  compression: gzip
```

Fields:

- `name`: required archive filename template.
- `format`: required archive container format.
- `compression`: optional compression algorithm. Defaults to `none`.

Supported archive combinations:

| Format | Compression | Result |
| --- | --- | --- |
| `tar` | `none` | uncompressed tar archive |
| `tar` | `gzip` | gzip-compressed tar archive |
| `zip` | `none` | zip archive |

Unsupported formats or format/compression combinations fail validation. For
example, `format: 7z` and `format: zip` with `compression: gzip` are not
currently supported.

The archive is created once in the run temporary directory, then delivered to
each configured output.

## Outputs

The `outputs` list controls where the finished archive is delivered. At least
one output is required.

The same archive is copied or uploaded to each output in order.

Supported output types:

- `filesystem`
- `sftp`

### Filesystem Output

Use filesystem output to copy the archive to a local directory.

```yaml
outputs:
  - type: filesystem
    label: Local copy
    path: /var/backups
```

Fields:

- `type`: must be `filesystem`.
- `label`: required display label for stdout and notification reports.
- `path`: required local directory where the archive should be copied.

The output directory is created if it does not exist. The archive filename is the
rendered `archive.name`.

### SFTP Output

Use SFTP output to upload the archive to a remote server.

```yaml
outputs:
  - type: sftp
    label: Upload (backup.example.com)
    host: backup.example.com
    port: 22
    username: backup
    remote_path: /backups
    identity_file: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts
```

Fields:

- `type`: must be `sftp`.
- `label`: required display label for stdout and notification reports.
- `host`: required SFTP server hostname.
- `port`: optional SFTP server port. Defaults to `22`.
- `username`: required remote username.
- `remote_path`: required remote directory where the archive should be uploaded.
- `identity_file`: SSH private key used for authentication.
- `password`: password used for authentication.
- `known_hosts`: optional known hosts file. Defaults to `~/.ssh/known_hosts`.
- `insecure_ignore_host_key`: optional boolean. Set to `true` to skip host key
  verification.

Exactly one of `identity_file` or `password` must be set.

`identity_file` and `known_hosts` support `~` and `~/...` expansion.

`remote_path` is created if it does not exist. The uploaded file name is the
rendered `archive.name`.

Prefer `known_hosts` verification for real backups. Use
`insecure_ignore_host_key: true` only for local testing or controlled
environments.

#### SFTP Password Authentication

```yaml
outputs:
  - type: sftp
    label: Upload (backup.example.com)
    host: backup.example.com
    username: backup
    remote_path: /backups
    password: change-me
```

#### SFTP Identity File Authentication

```yaml
outputs:
  - type: sftp
    host: backup.example.com
    username: backup
    remote_path: /backups
    identity_file: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts
```

## Notifications

The optional `notifications` list sends process status messages after a run
succeeds or fails. Notifications do not send the archive file.

Supported notification types:

- `discord`
- `ntfy`

Notification failures are returned as errors. If the backup also failed, the
notification failure is appended to the backup error.

### Discord Notification

```yaml
notifications:
  - type: discord
    webhook_url: https://discord.com/api/webhooks/...
```

Fields:

- `type`: must be `discord`.
- `webhook_url`: required Discord webhook URL.

Discord notifications send a JSON body with a `content` field.

### NTFY Notification

```yaml
notifications:
  - type: ntfy
    url: https://ntfy.sh/my-topic
```

Fields:

- `type`: must be `ntfy`.
- `url`: required NTFY topic URL.
- `username`: optional NTFY username.
- `password`: optional NTFY password.

Set both `username` and `password` to use HTTP Basic authentication. Set neither
for unauthenticated topics. Setting only one of them fails validation.

```yaml
notifications:
  - type: ntfy
    url: https://ntfy.sh/my-private-topic
    username: backup
    password: change-me
```

## Complete Examples

### Minimal Filesystem Backup

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
```

### Command-Generated Database Backup

```yaml
report:
  title: Database Backup Report

sources:
  - type: command
    workdir: "{tmpdir}"
    commands:
      - mkdir -p "{tmpdir}/export"
      - sqlite3 /srv/app/app.db ".backup '{tmpdir}/export/app.db'"
    collect:
      - label: DB Dump: app
        path: "{tmpdir}/export/app.db"
        target: app/app.db

archive:
  name: database-{date}.tar.gz
  format: tar
  compression: gzip

outputs:
  - type: filesystem
    label: Local copy
    path: /var/backups
```

### Multiple Outputs With Notifications

```yaml
report:
  title: App Backup Report

sources:
  - type: filesystem
    label: Site files
    path: /srv/app/data
    target: app/data

archive:
  name: backup-{date}.zip
  format: zip
  compression: none

outputs:
  - type: filesystem
    label: Local copy
    path: /var/backups

  - type: sftp
    label: Upload (backup.example.com)
    host: backup.example.com
    username: backup
    remote_path: /backups
    identity_file: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts

notifications:
  - type: discord
    webhook_url: https://discord.com/api/webhooks/...

  - type: ntfy
    url: https://ntfy.sh/my-topic
```

## Validation Summary

`retentra` validates the config before running a backup.

Common validation failures:

- `report.title` is empty.
- `sources` is empty.
- `outputs` is empty.
- A source, collected artifact, or output is missing a required `label`.
- A source, output, or notification has an unsupported `type`.
- A required field is missing.
- An archive target is absolute, `.`, or contains `..`.
- `archive.format` is unsupported.
- `archive.compression` is unsupported for the selected format.
- An SFTP output sets both `identity_file` and `password`, or neither.
- An NTFY notification sets only one of `username` or `password`.

Secrets such as SFTP passwords, Discord webhook URLs, and NTFY passwords should
not be committed to public repositories.
