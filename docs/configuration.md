# Configuration

The system-wide YAML file is `/etc/alldbackup/config.yaml`. The installer creates it with no jobs. The regular account that installed alldbackup owns and edits the YAML; no `sudo` is needed for routine config changes.

```bash
alldbackup config add
# or edit directly:
nano /etc/alldbackup/config.yaml
alldbackup config validate
alldbackup config list
```

Use `sudoedit /etc/alldbackup/secrets.env` for database passwords; that file is root-owned. See [installation](installation.md#2-files-and-access) for ownership and permissions.

## Example YAML

```yaml
version: 1
timestamp_timezone: "+07:00"
secrets_file: /etc/alldbackup/secrets.env

storage:
  local:
    enabled: true
    path: /var/backups/alldbackup
    retention_days: 7
  google_drive:
    enabled: false
    remote: gdrive:alldbackup
    config_file: /etc/alldbackup/rclone.conf
    retention_days: 30
    timeout: 30m
    retries: 3

jobs:
  - name: production
    enabled: true
    schedule: "0 2 * * *"
    google_drive_path: departments/production
    driver: mysql
    source: host
    host: 127.0.0.1
    port: 3306
    database: production
    username: backup_user
    password_env: MYSQL_BACKUP_PASSWORD
```

## Job settings

Every job needs a unique safe name, `enabled`, `driver`, `source`, and driver-specific connection fields. Names use lowercase letters, digits, `_`, and `-`, starting with a lowercase letter or digit.

| Setting | Required | Meaning |
| --- | --- | --- |
| `schedule` | No | Five-field cron expression. Missing or blank means `0 0 * * *` (midnight in server-local time). |
| `google_drive_path` | No | Relative path under the configured Rclone remote; defaults to the job name. Use a unique path for each job. |
| `driver` | Yes | `mysql`, `mariadb`, `postgres`, or `sqlite`. |
| `source` | Yes | `host` or `docker`. |
| `container` | For Docker | Running database container name. |
| `host`, `port` | For host SQL | Database server address and port. |
| `database`, `username`, `password_env` | For MySQL/MariaDB/PostgreSQL | Database name, backup account, and password variable name. |
| `file` | For SQLite | SQLite file path; for Docker, this path is inside the container. |

The automatic cron dispatcher checks the YAML every minute and runs enabled, valid jobs that match `schedule`. Cron runs on the server's local timezone. See [cron scheduling](cron.md). Manual `alldbackup run --job NAME` and `alldbackup run --all` run immediately.

For Docker jobs, Docker CLI must be installed on the host and the account must have Docker daemon access. The selected container must contain the dump utility. Docker daemon access is effectively host-root access.

## Passwords

`password_env` names a key in `secrets.env` or an environment variable. The environment value wins when both exist. For `password_env: MYSQL_BACKUP_PASSWORD`, add:

```dotenv
MYSQL_BACKUP_PASSWORD=replace-with-the-database-password
```

Edit the root-owned file with:

```bash
sudoedit /etc/alldbackup/secrets.env
```

Use one `KEY=value` per line. Values are read literally after the first `=`; do not add shell quotes or commands. Do not put password values in YAML or process arguments. The installer keeps this file at `root:alldbackup` mode `0640`, readable only by root and the runtime group.

## Default paths

| Setting/file | Default | Purpose |
| --- | --- | --- |
| Main YAML | `/etc/alldbackup/config.yaml` | Jobs and storage settings. |
| `secrets_file` | `/etc/alldbackup/secrets.env` | Database password values. |
| `storage.local.path` | `/var/backups/alldbackup` | Local backup root and upload retry spool. Each job uses a subdirectory. |
| `storage.local.retention_days` | `7` | Remove successfully uploaded/local backups older than this many days; `0` disables local cleanup. |
| `storage.google_drive.config_file` | `/etc/alldbackup/rclone.conf` | Rclone remote OAuth and settings. |
| `storage.google_drive.retention_days` | `30` | Independent Google Drive retention; `0` disables it. |

The system installer prepares default path permissions for root cron and regular-user commands. If you change `storage.local.path`, ensure root cron and the `alldbackup` group can access it; set directories to group-writable with setgid and make completed ZIP/checksum files readable by all users if that is desired.

If you use `alldbackup --config /path/to/config.yaml init`, relative `secrets_file`, local backup, and Rclone config paths are resolved relative to the YAML directory. Pass the same `--config` option to all commands. The system-installed root dispatcher uses `/etc/alldbackup/config.yaml`; custom configs need a separate manually managed dispatcher entry.

## Timestamp timezone

`timestamp_timezone` is top-level and accepts `UTC` or a fixed offset between `-14:00` and `+14:00`:

```yaml
timestamp_timezone: "+07:00"
```

With `+07:00`, an archive name looks like `20261109T090000.123456789+0700.zip`. The default UTC name ends in `Z`. `metadata.json` always records UTC. This setting changes filenames only; it does not affect the server-local job schedule.

## Retention and storage

The local destination always exists as the backup destination or, when local storage is disabled, as a temporary spool for uploads/retries. Local and Google Drive retention are independent. With Drive enabled, a local archive that has not uploaded successfully is preserved for retry. The `clean` command manually deletes one named ZIP or all recognized backups for a job from every enabled storage backend; see [restore and cleanup](restore.md#clean-backups-manually).

Validate after editing:

```bash
alldbackup config validate
alldbackup config list
```

For Google Drive fields, see [Google Drive setup](google-drive.md). For Docker job fields, see [Docker jobs](docker.md).
