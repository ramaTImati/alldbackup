# Schedule each job with cron

alldbackup has no background scheduler. The optional `schedule` field on each job stores its five-field cron expression, and the system cron service starts alldbackup at those times. `run --all` and `run --job` still run immediately when called manually.

## 1. Set a schedule per job

Edit the YAML file (default `/etc/alldbackup/config.yaml`) or run `sudo alldbackup config add`. The interactive command asks for an optional schedule; leave it blank for a manual-only job.

```yaml
jobs:
  - name: mysql-host
    enabled: true
    schedule: "0 2 * * *" # daily at 02:00
    driver: mysql
    source: host
    # connection fields...

  - name: postgres-docker
    enabled: true
    schedule: "30 2 * * *" # daily at 02:30
    driver: postgres
    source: docker
    # connection fields...

  - name: sqlite-host
    enabled: true
    # no schedule: manual only
    driver: sqlite
    source: host
    # connection fields...
```

The five fields are `minute hour day-of-month month day-of-week`. Cron uses the server's local timezone by default. If you set `CRON_TZ` in the crontab, cron uses that timezone for the schedule. The top-level `timestamp_timezone` setting only affects archive filenames; it does not change when a job runs.

## 2. Generate and install the crontab entries

After setting schedules, print the entries:

```bash
sudo /usr/local/bin/alldbackup --config /etc/alldbackup/config.yaml cron print
```

The command prints one entry for each enabled job that has a schedule. It uses the absolute path of the running binary and the selected config file, so the output can be pasted directly into the crontab. It does not modify the crontab automatically.

Open root's crontab:

```bash
sudo crontab -e
```

Paste the output from `cron print`, save, and exit. For example:

```cron
0 2 * * * '/usr/local/bin/alldbackup' run --job 'mysql-host' --config '/etc/alldbackup/config.yaml'
30 2 * * * '/usr/local/bin/alldbackup' run --job 'postgres-docker' --config '/etc/alldbackup/config.yaml'
```

Do not put `sudo` in root's crontab. If you use a non-root user's crontab, ensure that user can read the config and secrets, access the database or Docker daemon, and write the backup directory. Run `cron print` again whenever job schedules, binary path, or config path change, then replace the old entries.

## 3. Check the setup

Run both checks as the same account that owns the crontab:

```bash
sudo /usr/local/bin/alldbackup doctor --config /etc/alldbackup/config.yaml
sudo /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

The second command is an immediate manual run of every enabled job; it ignores the configured schedules. Successful archives and `.zip.sha256` sidecars are written under `/var/backups/alldbackup/<job-name>/` by default.

## 4. Logs and troubleshooting

Cron does not write a log file by default. Use cron mail or redirect output to a file writable by the cron account, for example:

```cron
0 2 * * * '/usr/local/bin/alldbackup' run --job 'mysql-host' --config '/etc/alldbackup/config.yaml' >> /var/log/alldbackup.log 2>&1
```

Create the log file with permissions appropriate for the crontab account and rotate it separately. Cron has a limited environment, so use absolute paths and store database credentials in the protected secrets file rather than relying on interactive shell variables.
