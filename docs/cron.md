# Schedule backups with cron

alldbackup does not run as a service and has no internal scheduler. Cron starts the CLI at the scheduled time. The backup files are still written to `storage.local.path` (default `/var/backups/alldbackup/<job-name>/`); cron does not move or copy them.

Running `alldbackup run --all` manually starts a backup immediately and exits. It does not install a cron entry. Add the command to a crontab to run it automatically.

## 1. Choose the cron account

Use the same account that owns or can read:

- `/etc/alldbackup/config.yaml` and `/etc/alldbackup/secrets.env`;
- the Rclone config file, if Google Drive is enabled;
- the database connection and Docker socket, if needed;
- the local backup directory.

The commands in this guide use root, matching `sudo alldbackup ...` in the quickstart. Root's crontab reads root's Rclone config. If you use a dedicated service user instead, install the crontab for that user and make these files and permissions available to it.

## 2. Test the exact commands manually

Run both checks as the same account cron will use:

```bash
sudo /usr/local/bin/alldbackup doctor --config /etc/alldbackup/config.yaml
sudo /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

Confirm that the expected files appear under `/var/backups/alldbackup/<job-name>/`. `doctor` checks configured dependencies and connectivity; `run` creates a real backup and applies retention.

## 3. Add the schedule

Edit root's crontab:

```bash
sudo crontab -e
```

Add this line to run all enabled jobs every day at 02:00:

```cron
0 2 * * * /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

Because this line is in root's crontab, do not put `sudo` in the cron command. If you edit a regular user's crontab instead, remove `sudo` there too and grant that account access to the config, secrets, database, and backup directory.

For one job only, replace `--all` with `--job production`. The CLI returns a non-zero exit code if any selected job fails; later jobs still run after a failure.

## 4. Where to find results and errors

Successful archives are written under `/var/backups/alldbackup/<job-name>/`, with a `.zip.sha256` checksum beside each ZIP. Cron itself does not save output to a log file. Configure your system's cron mail, or redirect output to a log file that the cron account can write, for example:

```cron
0 2 * * * /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml >> /var/log/alldbackup.log 2>&1
```

If using the example log path, create the file and ensure it is writable by the cron account. Rotate the log separately if you keep it long-term. `--quiet` only changes CLI log verbosity; it does not disable cron or change the exit code.

For root's crontab, create the example log file as root before adding the redirected command:

```bash
sudo touch /var/log/alldbackup.log
sudo chmod 0640 /var/log/alldbackup.log
```

If you initialized a custom config, put its exact path after `--config` here. Cron has a limited environment, so use absolute paths and do not depend on interactive shell variables for credentials; use the protected secrets file instead.
