# Automatic backup schedules

The installer sets up system cron on Ubuntu/Debian and adds one managed dispatcher entry to **root's crontab**. It runs every minute. `alldbackup` reads the YAML at each tick and starts only enabled, valid jobs whose schedule matches the current server-local minute. You do not need to create a separate cron line per job.

Manual `alldbackup run --job NAME` and `alldbackup run --all` still start backups immediately; they do not wait for schedules.

## 1. Set a schedule for each job

Edit `/etc/alldbackup/config.yaml` as the regular account, or use `alldbackup config add`. A schedule uses five fields: `minute hour day-of-month month day-of-week`.

```yaml
jobs:
  - name: mysql-host
    enabled: true
    schedule: "0 2 * * *" # daily at 02:00
    google_drive_path: mysql-host
    driver: mysql
    source: host
    # connection fields...

  - name: postgres-docker
    enabled: true
    schedule: "30 2 * * *" # daily at 02:30
    google_drive_path: production/postgres
    driver: postgres
    source: docker
    # connection fields...

  - name: sqlite-host
    enabled: true
    # omitted or blank schedule runs daily at 00:00 server time
    google_drive_path: archives/sqlite
    driver: sqlite
    source: host
    # connection fields...
```

Examples:

| Schedule | Runs |
| --- | --- |
| `0 0 * * *` | Every day at 00:00 |
| `0 2 * * *` | Every day at 02:00 |
| `30 2 * * *` | Every day at 02:30 |
| `*/15 * * * *` | Every 15 minutes |
| `0 3 * * 1` | Mondays at 03:00 |

Cron uses the server's local timezone. `timestamp_timezone` only changes archive filename timestamps; metadata remains UTC. The dispatcher re-reads the YAML every minute, so schedule and enabled-state changes need no cron edit or service restart.

Validate YAML after editing:

```bash
alldbackup config validate
alldbackup config list
```

## 2. Confirm the dispatcher

The installer maintains this root crontab entry inside a managed block:

```cron
* * * * * '/usr/local/bin/alldbackup' --config '/etc/alldbackup/config.yaml' cron tick
```

This is one dispatcher line for all jobs. It belongs to root, so `crontab -l` run as a regular user can be empty; check with `sudo crontab -l`. Registering jobs in YAML does not add one crontab line per job.

Inspect root's crontab and the entry that `alldbackup` prints:

```bash
sudo crontab -l
alldbackup cron print
```

`cron print` prints the dispatcher line once, not one line per job. To reinstall the managed block manually, run `sudo alldbackup cron install`. The installer does this automatically and repeat installs are idempotent. The block update preserves unrelated root crontab lines and removes old alldbackup per-job entries for the default config.

If `alldbackup cron install` is not recognized, the installed binary is an older release with the legacy cron command. Updating the installer script alone does not update the binary; install a binary version that includes the dispatcher before expecting root cron scheduling.

If an older version had a custom `--config` path or entries in your personal crontab, inspect and remove those old entries to prevent duplicate backups:

```bash
sudo crontab -e
crontab -e
```

## 3. Test and check results

Check connectivity and run an immediate backup as the regular account:

```bash
alldbackup doctor
alldbackup run --all
```

Cron runs jobs as root, so it can read `/etc/alldbackup/secrets.env` and write the shared backup folders. A failed scheduled job is logged, but the dispatcher can still try other due jobs. Invalid individual jobs are skipped; malformed YAML or invalid shared storage settings stop that tick. Check cron's system logs or root's local mail for command output.

Default output for job `mysql-host`:

```text
/var/backups/alldbackup/mysql-host/<timestamp>.zip
/var/backups/alldbackup/mysql-host/<timestamp>.zip.sha256
```

## 4. Remove scheduling during uninstall

Remove only alldbackup's marked block from root crontab:

```bash
sudo alldbackup cron remove
sudo crontab -l
```

Unrelated root cron entries are retained. Then remove the binary if uninstalling; see [installation and uninstall](installation.md#uninstall).

## Non-Ubuntu/Debian systems

The installer does not install cron on other distributions. Install and enable a cron service yourself, then put the printed `cron tick` dispatcher line in root's crontab. Keep the one-minute interval; job schedules are evaluated by alldbackup from YAML.
