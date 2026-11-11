# Installation, permissions, update, and uninstall

The installer supports Ubuntu and Debian. Installation and upgrade are system-wide and require `sudo`; after the first login refresh, normal job configuration and backup commands run as your regular account. Cron runs as root and dispatches due jobs every 15 minutes.

## 1. Install

Run these commands from a regular account with `sudo` access:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

The installer verifies the latest release archive against `checksums.txt` before replacing the binary. It creates the `alldbackup` system group and adds the account that invoked `sudo` to it. Log out and back in so the new group membership is available to your shell. If you need it immediately in the current terminal, run `newgrp alldbackup`.

If Rclone is not installed, the installer runs Rclone's official installer. If cron is missing, the installer installs it with APT and enables/starts the service. These automatic system steps support Ubuntu and Debian only. A non-Debian distribution must install and start cron manually before using the scheduler.

The installer creates an empty default YAML config and credentials files; it does not create a database job or authorize Google Drive. It never replaces existing config, secrets, Rclone config, or backups during upgrade.

## 2. Files and access

| Purpose | Default path | Owner/mode and use |
| --- | --- | --- |
| CLI binary | `/usr/local/bin/alldbackup` | `root:root`, `0755`; installer manages updates |
| Main YAML | `/etc/alldbackup/config.yaml` | installer account:`alldbackup`, `0644`; edit as the regular account |
| Database secrets | `/etc/alldbackup/secrets.env` | `root:alldbackup`, `0640`; edit with `sudo nano` |
| Rclone credentials | `/etc/alldbackup/rclone.conf` | installer account:`alldbackup`, `0600`; edit with `alldbackup drive setup` |
| Config directory | `/etc/alldbackup/` | `root:alldbackup`, `1775` with sticky bit, so the runtime account can update its config |
| Local backup root | `/var/backups/alldbackup/` | `root:alldbackup`, `2775`; readable by all, writable by the group |
| Job backup folder | `/var/backups/alldbackup/<job-name>/` | `alldbackup` group can create and remove backup files; all users can read them |
| Cron dispatcher | root's crontab | one managed entry; checks YAML schedules every 15 minutes |

ZIP and checksum files are publicly readable to local accounts by design. The group can write and remove backups, including when root cron and a regular user both run backup commands. Docker jobs require access to the Docker daemon; membership in the Docker group is effectively host-root access.

If this account needs Docker access, add it to the Docker group according to your host's Docker setup, then log out and back in. The alldbackup installer does not alter Docker permissions.

## 3. Add a database job

After logging back in as the installer account, add a job interactively:

```bash
alldbackup config add
```

The prompts ask for a unique job name, driver (`mysql`, `mariadb`, `postgres`, or `sqlite`), source (`host` or `docker`), connection fields, password variable name, interval, run hour/minute, and Google Drive path. The schedule defaults to every day at `00:00` server time; the Drive path defaults to the job name. The command never asks for the password itself.

You can also edit the file directly as the regular account:

```bash
nano /etc/alldbackup/config.yaml
alldbackup config validate
```

The installer-created file starts with `jobs: []`. For complete examples, see [`config.example.yaml`](../configs/config.example.yaml) and [configuration](configuration.md). Host jobs need the database dump utility on the Linux host. Docker jobs need Docker CLI access and the matching dump utility inside the selected container.

## 4. Add database passwords

Edit the root-owned secrets file with `sudo nano`:

```bash
sudo nano /etc/alldbackup/secrets.env
```

Add the exact variable name entered as `password_env`, one `KEY=value` per line:

```dotenv
MYSQL_BACKUP_PASSWORD=replace-with-the-database-password
POSTGRES_BACKUP_PASSWORD=replace-with-another-password
```

Do not add shell quotes or commands. Values are read literally after the first `=`. A process environment variable with the same name takes precedence. The file remains `root:alldbackup` mode `0640`; do not change it to world-readable. SQLite does not need a password.

## 5. Set up Google Drive (optional)

Rclone's executable is installed automatically if missing, but the installer does not create or authorize a remote. Configure it as the regular account:

```bash
alldbackup drive setup
```

The Rclone setup is saved to `/etc/alldbackup/rclone.conf`. Choose `n` to create a remote, choose Google Drive, complete OAuth, then set the matching remote in `/etc/alldbackup/config.yaml`. Enable `storage.google_drive.enabled` and set each job's `google_drive_path`. Test access with:

```bash
alldbackup config validate
alldbackup drive test
```

See [Google Drive setup](google-drive.md) for headless server and desktop OAuth steps.

## 6. Check and run a backup

Check configured utilities, database access, backup path, and Drive if enabled:

```bash
alldbackup doctor
```

Run a backup immediately, regardless of its configured schedule:

```bash
alldbackup run --job production
alldbackup run --all
```

For the default local path, a job named `production` writes its files to:

```text
/var/backups/alldbackup/production/
├── 20261109T020000.123456789Z.zip
└── 20261109T020000.123456789Z.zip.sha256
```

Each ZIP contains `backup.sql` or `database.sqlite` and `metadata.json`. Local retention defaults to 7 days and Google Drive retention to 30 days; set either value to `0` to disable its automatic cleanup. Verify a ZIP with `alldbackup verify --file /path/to/file.zip`.

## 7. Automatic schedule

The installer registers one root crontab line that runs `alldbackup cron tick` every 15 minutes. The dispatcher reads `/etc/alldbackup/config.yaml` and runs enabled, valid jobs due at their configured interval and server-local time. A missing schedule defaults to daily at midnight. Changes take effect on the next 15-minute tick; do not add a second `run --all` cron entry.

Inspect the entry with:

```bash
sudo crontab -l
alldbackup cron print
```

The output of `cron print` is the 15-minute dispatcher entry. It does not list a separate crontab line per job. Installer upgrades update the managed root crontab block and remove older generated per-job lines for the default config.

For details, see [cron scheduling](cron.md). `cron install` and `cron remove` operate on the current account's crontab; use `sudo` when managing the installed root dispatcher.

## Upgrade

For a routine Linux binary update, use the built-in command. It downloads the latest release, verifies its checksum, and replaces only the binary:

```bash
sudo alldbackup update
```

Run the installer when you need its directory-permission or root cron migrations. It also updates the binary:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

The installer keeps existing YAML, secrets, Rclone OAuth config, and backup contents. Old default-config cron entries generated by alldbackup are replaced by the dispatcher. If you had a custom-config cron entry or a per-user cron entry, inspect that account's crontab and remove the old line to avoid duplicate runs:

```bash
sudo crontab -e   # root crontab
crontab -e        # current user's crontab, if previously used
```

An existing account may need to log out and back in after an installer run if it was newly added to the group.

## Uninstall

Run the uninstall command as root to remove the binary, root dispatcher, and config directory (including YAML, secrets, and Rclone credentials):

```bash
sudo alldbackup uninstall
```

The uninstall command keeps local backup files and Google Drive files. It leaves the cron and Rclone packages, the `alldbackup` group, and config files outside `/etc/alldbackup` in place. If the configuration is invalid or the configured local backup path resolves inside `/etc/alldbackup`, uninstall stops so the backup data is not removed; fix the config or move the backups first. Custom config files outside the default config directory must be removed separately if desired.
