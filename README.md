# alldbackup

[![Latest release](https://img.shields.io/github/v/release/ramaTImati/alldbackup)](https://github.com/ramaTImati/alldbackup/releases/latest)

**Languages:** English · [Bahasa Indonesia](README.id.md)

alldbackup is a Linux command-line tool for scheduled backups of MySQL, MariaDB, PostgreSQL, and SQLite databases from a host or Docker container. Each archive is a ZIP containing the database dump and metadata, with a separate SHA-256 checksum. Backups can be stored locally and optionally uploaded to Google Drive.

## System requirements

- Linux on amd64 or arm64. Published release tags use the vMAJOR.MINOR.PATCH format; the badge above shows the latest release.
- The automatic installer supports Ubuntu and Debian and requires root access through sudo.
- An internet connection and curl are needed for installation. The installer installs and enables cron, and installs Rclone if either is missing.
- Host database jobs need the matching database dump utility installed on the host. Docker jobs need Docker access and the matching dump utility in the selected container.
- Google Drive is optional. To use it, authorize a Google account during Rclone setup.

## Installation, update, and uninstall

Install the latest release on Ubuntu or Debian:

~~~bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
~~~

The installer creates the default config and secrets files, a system group, and the root cron dispatcher. It adds the account that invoked sudo to the alldbackup group; log out and back in once to apply the new group membership. Normal backup commands can then run without sudo.

Update the installed binary without curl:

~~~bash
sudo alldbackup update
~~~

This downloads the latest Linux release, verifies its checksum, and replaces only the binary. Run the installer again when you need its directory-permission or cron migrations.

The update command is available in releases that include this feature. If an older installed binary reports an unknown command, run the installer once to upgrade it.

Uninstall the CLI, its root cron dispatcher, and the config/secrets/Rclone files in /etc/alldbackup with one command:

~~~bash
sudo alldbackup uninstall
~~~

Local backup files are kept. The cron and Rclone packages, alldbackup system group, Google Drive files, and custom config files outside /etc/alldbackup are also left in place. See [installation and permissions](docs/installation.md) for details.

Uninstall stops if the config is invalid or its local backup path is inside /etc/alldbackup, to avoid deleting backup data.

## Job settings

Manage jobs interactively and inspect the current configuration:

~~~bash
alldbackup config add
alldbackup config edit production
alldbackup config delete production
alldbackup config list
alldbackup config validate
~~~

Editing uses each current value as the prompt default. Deleting a job removes its configuration only; existing backups remain. You can also edit /etc/alldbackup/config.yaml directly, then run config validate.

Jobs support MySQL, MariaDB, PostgreSQL, and SQLite with either host or Docker sources. Schedules use an interval such as 1h, 1d, or 7d plus a server-local hour and minute. The default is daily at 00:00; the cron dispatcher checks schedules every 15 minutes. See [configuration](docs/configuration.md), [cron scheduling](docs/cron.md), and [Docker jobs](docs/docker.md).

For SQL passwords, set password_env to an environment variable name and put the value in the protected secrets file:

~~~bash
sudo nano /etc/alldbackup/secrets.env
alldbackup config validate
alldbackup doctor
~~~

An environment variable takes precedence over the same key in secrets.env. Do not put passwords in YAML or command arguments.

## Drive settings

Rclone is installed automatically if missing. Configure a Google Drive remote interactively:

~~~bash
alldbackup drive setup
~~~

Enable Google Drive and set the shared remote in /etc/alldbackup/config.yaml. Each job can set its own google_drive_path beneath that remote. Then check access:

~~~bash
alldbackup drive test
~~~

Rclone credentials are stored separately at /etc/alldbackup/rclone.conf; the setup does not replace a user's existing Rclone config. See [Google Drive setup](docs/google-drive.md) for remote and OAuth instructions.

## Run jobs, clean backups, and verify archives

Run one enabled job now or run all enabled jobs:

~~~bash
alldbackup run --job production
alldbackup run --all
~~~

Remove one backup by its ZIP filename or remove every recognized backup for a job. Cleanup applies to all enabled storage backends:

~~~bash
alldbackup clean --job production 20261109T020000.123456789Z.zip
alldbackup clean --job production --all
~~~

Only alldbackup backup archives and their checksum sidecars are removed. Verify a local archive and its checksum with:

~~~bash
alldbackup verify --file /var/backups/alldbackup/production/20261109T020000.123456789Z.zip
~~~

See [verification and restore](docs/restore.md).

## Files and permissions

| Purpose | Default path | Owner and access |
| --- | --- | --- |
| Binary | /usr/local/bin/alldbackup | root:root, mode 0755 |
| Config directory | /etc/alldbackup/ | root:alldbackup, mode 1775 |
| YAML config | /etc/alldbackup/config.yaml | installer account, mode 0644 |
| Database secrets | /etc/alldbackup/secrets.env | root:alldbackup, mode 0640 |
| Rclone credentials | /etc/alldbackup/rclone.conf | installer account, mode 0600 |
| Local backups | /var/backups/alldbackup/<job>/ | alldbackup group can write; local users can read |
| Cron | root's crontab | one dispatcher entry, checked every 15 minutes |

Local retention defaults to 7 days and Google Drive retention to 30 days. Set a storage retention value to 0 to disable automatic cleanup. Backup ZIP files are readable by local users. Docker access is effectively host-root access.

## License

MIT. See [LICENSE](LICENSE).
