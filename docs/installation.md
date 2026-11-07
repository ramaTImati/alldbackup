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

This creates an empty configuration, an empty secrets file, an empty Rclone config file, and the local backup directory. It does not create a database job, database password, or Google Drive remote. It refuses to overwrite an existing alldbackup config.

| Purpose | Default path | Created by |
| --- | --- | --- |
| alldbackup binary | `/usr/local/bin/alldbackup` | Installer |
| Main YAML config | `/etc/alldbackup/config.yaml` | `alldbackup init` |
| Secrets file | `/etc/alldbackup/secrets.env` | `alldbackup init`, mode `0600` |
| Rclone config | `/etc/alldbackup/rclone.conf` | `alldbackup init`, mode `0600`; add a remote with `alldbackup drive setup` |
| Local backup root | `/var/backups/alldbackup/` | `alldbackup init`; mode `0755` |
| Rclone binary | `/usr/bin/rclone` | Rclone's official installer, only if absent |

`/etc/alldbackup` is owned by root and private because it contains database passwords and Rclone OAuth credentials; a regular user will get `Permission denied` when trying to `cd` into it. Use `sudoedit` to edit files there and keep it private. The backup destination is different: `/var/backups/alldbackup` and each job folder use mode `0755`, while completed ZIP and checksum files use `0644`. All local users can list and read backup contents, but only the account running alldbackup can write or delete them. This makes database contents readable to every local account on the server.

Backups made by older versions remain private until their permissions are updated once. For the default path, run:

```bash
sudo chmod 0755 /var/backups/alldbackup
sudo find /var/backups/alldbackup -mindepth 1 -path '*/.locks' -prune -o -type d -exec chmod 0755 {} +
sudo find /var/backups/alldbackup -path '*/.locks' -prune -o -type f \( -name '*.zip' -o -name '*.zip.sha256' \) -exec chmod 0644 {} +
```

Replace `/var/backups/alldbackup` if `storage.local.path` uses a custom value.

The dedicated Rclone config keeps Google Drive credentials beside alldbackup's other setup files without replacing a user's existing `~/.config/rclone/rclone.conf`.

## 3. Add a database job

Start the interactive job editor:

```bash
sudo alldbackup config add
```

Enter a lowercase job name, database driver, source (`host` or `docker`), and connection details. For SQL databases, enter the name of an environment variable for the password, such as `MYSQL_BACKUP_PASSWORD`. The command asks for a five-field cron schedule (default `0 0 * * *`) and a Google Drive path (default: the job name); press Enter to accept either default. The command saves the job to `/etc/alldbackup/config.yaml`; it never prompts for the password value. SQLite does not need a password.

Alternatively, edit the YAML manually as root with `sudoedit` (you do not need to `cd` into `/etc/alldbackup`):

```bash
sudoedit /etc/alldbackup/config.yaml
sudo alldbackup config validate
```

The complete [`config.example.yaml`](../configs/config.example.yaml) shows all supported drivers and sources. In a Docker job, `container` is the container name and the dump utility must be inside that container. In a host job, the dump utility must be installed on the Linux server.

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

These commands start a backup immediately and exit; they do not schedule future runs. Use [cron](cron.md) to install the per-job schedules configured in YAML.

For a job named `production`, the files are written under:

```text
/var/backups/alldbackup/production/
├── 20261106T090000.123456789+0700.zip
└── 20261106T090000.123456789+0700.zip.sha256
```

The `+0700` suffix is an example for `timestamp_timezone: "+07:00"`; UTC is the default and uses a `Z` suffix. Each ZIP contains `backup.sql` or `database.sqlite` and `metadata.json`. The metadata timestamp is always UTC. Run `alldbackup verify --file /absolute/path/to/backup.zip` to check the ZIP and its checksum.

`alldbackup init` writes `storage.local.retention_days: 7` into `config.yaml`. Local backups older than 7 days are removed after a run. If Google Drive is enabled, failed or pending uploads are kept locally for retry. Change this value to keep backups for another number of days; set it to `0` to disable local cleanup.

## Custom config path

For example, to keep all alldbackup files under `/opt/alldbackup`:

```bash
sudo alldbackup --config /opt/alldbackup/config.yaml init
```

For a non-default config path, `init` places `secrets.env`, `rclone.conf`, and the default `backups/` directory beside that YAML file. It does not change the binary path. Edit the YAML if you want other storage paths, and pass the same option on every command:

```bash
sudo alldbackup --config /opt/alldbackup/config.yaml config add
sudo alldbackup --config /opt/alldbackup/config.yaml run --all
```

Cron must also use this exact config path. See [cron](cron.md), [configuration](configuration.md), and [Google Drive](google-drive.md).

## Database utilities and Docker

For host jobs, install `mysqldump` or `mariadb-dump`, `pg_dump`, or `sqlite3` on the Linux server. For Docker jobs, install the matching dump utility in the database container and install Docker CLI on the server. alldbackup copies temporary credential files into a Docker container for SQL dumps and removes them afterward. Access to the Docker socket is effectively host-root access.

## Manual binary installation

Download the Linux amd64 or arm64 tarball and `checksums.txt` from [GitHub Releases](https://github.com/ramaTImati/alldbackup/releases). Verify the archive checksum before extracting `alldbackup`, then install it as `/usr/local/bin/alldbackup` with mode `0755`. This manual method does not install Rclone or configure any database/Google Drive credentials.

## Upgrade

To upgrade an installation made with the installer, run it again. It downloads the latest release, checks its checksum, and replaces `/usr/local/bin/alldbackup`:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
sudo alldbackup version
```

The installer leaves `/etc/alldbackup/config.yaml`, `secrets.env`, `rclone.conf`, and backup files unchanged. It installs Rclone only if `rclone` is missing from `PATH`; it does not upgrade an existing Rclone installation. For a version-pinned upgrade, use the checksum-verified manual binary installation steps above and replace `/usr/local/bin/alldbackup`.

## Uninstall

There is no built-in uninstall command. First remove alldbackup's cron entries so cron does not keep invoking the removed binary:

```bash
sudo crontab -e
```

Remove the generated alldbackup lines. If you installed entries in your own user's crontab, edit that too with `crontab -e`.

Then remove the binary:

```bash
sudo rm -f /usr/local/bin/alldbackup
```

This leaves the configuration and data in place so you can reinstall or recover backups later:

- `/etc/alldbackup/config.yaml` — jobs and storage settings;
- `/etc/alldbackup/secrets.env` — database passwords;
- `/etc/alldbackup/rclone.conf` — Google Drive OAuth credentials;
- `/var/backups/alldbackup/` — local archives and checksums;
- files in Google Drive — remote archives are not deleted by uninstalling the CLI.

Rclone is installed separately and may be used by other programs, so the uninstall steps leave its binary and per-user configuration alone. If you intentionally want to erase alldbackup's default local data too, inspect the configured paths first, then remove the directories interactively:

```bash
sudo rm -ri /etc/alldbackup /var/backups/alldbackup
```

This deletes the local config, database passwords, Rclone OAuth tokens, and local backup archives. It does not delete files stored in Google Drive. If you used a custom config or backup path, remove those paths separately only after checking their contents.
