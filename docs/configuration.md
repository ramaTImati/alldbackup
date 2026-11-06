# Configuration

By default, alldbackup reads `/etc/alldbackup/config.yaml` and `/etc/alldbackup/secrets.env`. `alldbackup init` creates both files and the local backup directory `/var/backups/alldbackup/`. The full example is [`config.example.yaml`](../configs/config.example.yaml).

## Add a job

Use the interactive command:

```bash
sudo alldbackup config add
```

It asks for the job name, driver, source, database connection fields, the name of the password environment variable, and a schedule. The schedule prompt defaults to `0 0 * * *` (daily at midnight in the server's timezone); press Enter to use it or type another five-field cron expression. It appends the job to `/etc/alldbackup/config.yaml`; it does not save the password. Or edit the YAML directly with `sudoedit /etc/alldbackup/config.yaml`.

### Example YAML

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
    retention_days: 30
    timeout: 30m
    retries: 3

jobs:
  - name: production
    enabled: true
    schedule: "0 2 * * *"
    driver: mysql
    source: host
    host: 127.0.0.1
    port: 3306
    database: production
    username: backup_user
    password_env: MYSQL_BACKUP_PASSWORD
```

## Job fields

Every job has a unique lowercase `name`, `enabled`, a `driver`, and a `source`.

Every job has an effective schedule. It accepts a standard five-field cron expression in this order: minute, hour, day of month, month, day of week. For example, `0 2 * * *` means 02:00 every day. A blank or omitted `schedule` defaults to `0 0 * * *` (daily at midnight). Cron uses the server's local timezone (or `CRON_TZ` if configured in the crontab); `timestamp_timezone` only changes archive filename timestamps. Use `alldbackup cron print` to generate separate entries; see [the cron guide](cron.md).

| Driver | Required fields | Notes |
| --- | --- | --- |
| `mysql`, `mariadb`, `postgres` | `database`, `username`, `password_env` | For `source: host`, also set `host` and `port`. For `source: docker`, set `container` instead. |
| `sqlite` | `file` | For `source: host`, `file` is a path on the Linux server. For `source: docker`, it is a path inside the container; also set `container`. No password is needed. |

| Job setting | Required | Meaning |
| --- | --- | --- |
| `schedule` | No | Five-field cron schedule. A blank or omitted value defaults to `0 0 * * *` (daily at server midnight). |

For Docker jobs, the database container must be running and contain the matching dump utility. The host must have Docker CLI and access to the Docker daemon. For host jobs, the dump utility must be installed on the server.

## Store SQL database passwords

The `password_env` value names a key in the secrets file or an environment variable. For the example above, create this entry in `/etc/alldbackup/secrets.env`:

```dotenv
MYSQL_BACKUP_PASSWORD=replace-with-the-database-password
```

Create/edit the file and restrict its permissions:

```bash
sudoedit /etc/alldbackup/secrets.env
sudo chmod 0600 /etc/alldbackup/secrets.env
```

Use one `KEY=value` entry per line. Values are read literally after the first `=`; do not add quotes or shell syntax. If the variable is also present in the process environment, the environment value wins. Passwords never go in the YAML file or process arguments.

## File paths and custom config

| Setting | Default | Use |
| --- | --- | --- |
| Config file | `/etc/alldbackup/config.yaml` | Main YAML file passed to CLI commands. |
| `secrets_file` | `/etc/alldbackup/secrets.env` | Password values referenced by `password_env`. |
| `storage.local.path` | `/var/backups/alldbackup` | Root for local archive folders. Each job uses a subdirectory, e.g. `/var/backups/alldbackup/production/`. |

Absolute paths are used as written. Relative `secrets_file` and `storage.local.path` paths are resolved relative to the directory containing the config YAML. If you initialize `/opt/alldbackup/config.yaml`, `init` defaults those paths to `/opt/alldbackup/secrets.env` and `/opt/alldbackup/backups`.

Pass a custom YAML path to every command, including cron:

```bash
sudo alldbackup --config /opt/alldbackup/config.yaml config validate
sudo alldbackup run --all --config /opt/alldbackup/config.yaml
```

## Timestamp offset in archive names

`timestamp_timezone` is a top-level setting. It accepts `UTC` or a fixed offset in `±HH:MM` format between `-14:00` and `+14:00`:

```yaml
timestamp_timezone: "+07:00"
```

With `+07:00`, a filename looks like `20261106T090000.123456789+0700.zip`. The default `UTC` filename ends in `Z`. The `timestamp_utc` value inside `metadata.json` is always UTC. Existing `Z` archives remain valid for verification and retention.

## Storage and retention

`storage.local.path` is always required. If local storage is enabled, completed archives remain under `<path>/<job-name>/`. If only Google Drive is enabled, this path is a temporary spool for creating archives and retrying failed uploads.

`retention_days: 0` disables cleanup. Local and Google Drive retention are configured independently. When Google Drive is enabled, local cleanup preserves archives that failed to upload and removes an archive only after a successful upload has been marked.

Validate the YAML and list jobs with:

```bash
sudo alldbackup config validate
sudo alldbackup config list
```

Use `sudo` or omit it consistently according to the account that owns the config/secrets and will run backups. See [installation](installation.md), [Docker](docker.md), [Google Drive](google-drive.md), and [cron](cron.md).
