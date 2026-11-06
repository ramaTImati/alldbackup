# Configuration

Use one YAML file with `version: 1`, storage settings, a secrets file path, and a list of jobs. The complete example is [`../configs/config.example.yaml`](../configs/config.example.yaml).

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
```

`timestamp_timezone` controls the offset written into archive filenames. Use `UTC` (the default) or a fixed offset such as `+07:00` or `-05:00`. Filenames keep the offset without a colon, for example `20261106T090000.123456789+0700.zip`; `metadata.json` continues to store the UTC timestamp.

Each job needs a unique lowercase name, `enabled`, a `driver` (`mysql`, `mariadb`, `postgres`, or `sqlite`), and a `source` (`host` or `docker`). SQL jobs also need `database`, `username`, and `password_env`. Host jobs use `host` and `port`; Docker jobs use `container`. SQLite jobs use `file`; a Docker SQLite file path is interpreted inside the container.

Job names are limited to lowercase ASCII letters, digits, `_`, and `-`, starting with a letter or digit. This keeps backup paths and remote names safe.

Passwords are loaded from the process environment first, then from `secrets_file`. File entries use `NAME=value`; values are read literally after the first `=`. Do not add shell syntax or quote values. Set the file mode to `0600`:

```bash
sudo chmod 0600 /etc/alldbackup/secrets.env
```

`retention_days: 0` disables cleanup. If Google Drive is enabled, local retention only removes backups that have been uploaded successfully. Pending uploads are retried on a later run.

Validate or edit jobs with:

```bash
alldbackup config validate
alldbackup config list
alldbackup config add
```
