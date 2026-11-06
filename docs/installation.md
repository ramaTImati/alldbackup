# Installation and first backup

These steps assume a Linux server and use the default paths. Run commands with `sudo` when the files are under `/etc` or `/var`, and use the same account for setup, backup runs, and cron.

## 1. Install alldbackup

Download and run the latest installer:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

The installer verifies the alldbackup release checksum and installs the binary as `/usr/local/bin/alldbackup`. It checks whether `rclone` is on `PATH`; if not, it downloads and runs [Rclone's official installer](https://rclone.org/install/). On Linux, that installer places Rclone in `/usr/bin/rclone`. It needs `unzip`, `7z`, or `busybox` to extract its download. Installing Rclone does not create a remote or modify an existing Rclone config.

## 2. Initialize the default files

```bash
sudo alldbackup init
```

This creates an empty configuration, an empty secrets file, and the local backup directory. It does not create a database job, database password, or Google Drive remote. It refuses to overwrite an existing config.

| Purpose | Default path | Created by |
| --- | --- | --- |
| alldbackup binary | `/usr/local/bin/alldbackup` | Installer |
| Main YAML config | `/etc/alldbackup/config.yaml` | `alldbackup init` |
| Secrets file | `/etc/alldbackup/secrets.env` | `alldbackup init`, mode `0600` |
| Local backup root | `/var/backups/alldbackup/` | `alldbackup init` |
| Rclone binary | `/usr/bin/rclone` | Rclone's official installer, only if absent |
| Rclone remote config | Per-user; see `rclone config file` | `rclone config` |

The directories `/etc/alldbackup` and `/var/backups/alldbackup` are created with restricted permissions. Keep them owned by, or writable to, the account that runs alldbackup.

## 3. Add a database job

Start the interactive job editor:

```bash
sudo alldbackup config add
```

Enter a lowercase job name, database driver, source (`host` or `docker`), and connection details. For SQL databases, enter the name of an environment variable for the password, such as `MYSQL_BACKUP_PASSWORD`. The command saves the job to `/etc/alldbackup/config.yaml`; it never prompts for the password value. SQLite does not need a password.

Alternatively, edit `/etc/alldbackup/config.yaml` directly. The complete [`config.example.yaml`](../configs/config.example.yaml) shows all supported drivers and sources. In a Docker job, `container` is the container name and the dump utility must be inside that container. In a host job, the dump utility must be installed on the Linux server.

## 4. Add database passwords

Edit the secrets file:

```bash
sudoedit /etc/alldbackup/secrets.env
sudo chmod 0600 /etc/alldbackup/secrets.env
```

Add the exact variable name used by the job, with one `KEY=value` entry per line. Example:

```dotenv
MYSQL_BACKUP_PASSWORD=replace-with-the-database-password
POSTGRES_BACKUP_PASSWORD=replace-with-another-password
```

Do not add quotes or shell commands. Values are read literally after the first `=`. If the same variable is in the process environment, that value takes precedence over the secrets file.

## 5. Validate and check dependencies

```bash
sudo alldbackup config validate
sudo alldbackup doctor
```

`doctor` checks the local backup directory, each enabled job's dump utility and database connection, and Rclone access if Google Drive is enabled. Resolve any reported missing utility, connection, permission, or remote error before scheduling backups.

## 6. Run a backup and find its files

Run a single job or all enabled jobs:

```bash
sudo alldbackup run --job production
sudo alldbackup run --all
```

These commands start a backup immediately and exit; they do not schedule future runs. Use [cron](cron.md) to run the same command automatically.

For a job named `production`, the files are written under:

```text
/var/backups/alldbackup/production/
├── 20261106T090000.123456789+0700.zip
└── 20261106T090000.123456789+0700.zip.sha256
```

The `+0700` suffix is an example for `timestamp_timezone: "+07:00"`; UTC is the default and uses a `Z` suffix. Each ZIP contains `backup.sql` or `database.sqlite` and `metadata.json`. The metadata timestamp is always UTC. Run `alldbackup verify --file /absolute/path/to/backup.zip` to check the ZIP and its checksum.

By default, local backups older than 7 days are removed after a run. If Google Drive is enabled, failed or pending uploads are kept locally for retry. Set `storage.local.retention_days: 0` to disable local cleanup.

## Custom config path

For example, to keep all alldbackup files under `/opt/alldbackup`:

```bash
sudo alldbackup --config /opt/alldbackup/config.yaml init
```

For a non-default config path, `init` places `secrets.env` and the default `backups/` directory beside that YAML file. It does not change the binary path. Edit the YAML if you want other storage paths, and pass the same option on every command:

```bash
sudo alldbackup --config /opt/alldbackup/config.yaml config add
sudo alldbackup --config /opt/alldbackup/config.yaml run --all
```

Cron must also use this exact config path. See [cron](cron.md), [configuration](configuration.md), and [Google Drive](google-drive.md).

## Database utilities and Docker

For host jobs, install `mysqldump` or `mariadb-dump`, `pg_dump`, or `sqlite3` on the Linux server. For Docker jobs, install the matching dump utility in the database container and install Docker CLI on the server. alldbackup copies temporary credential files into a Docker container for SQL dumps and removes them afterward. Access to the Docker socket is effectively host-root access.

## Manual binary installation

Download the Linux amd64 or arm64 tarball and `checksums.txt` from [GitHub Releases](https://github.com/ramaTImati/alldbackup/releases). Verify the archive checksum before extracting `alldbackup`, then install it as `/usr/local/bin/alldbackup` with mode `0755`. This manual method does not install Rclone or configure any database/Google Drive credentials.
