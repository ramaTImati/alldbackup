# Automatic backup schedules

The installer keeps one dispatcher entry in **root's crontab**. It runs every 15 minutes and reads `/etc/alldbackup/config.yaml`; jobs do not need separate crontab lines. A regular user's `crontab -l` can be empty. Check the system dispatcher with `sudo crontab -l`.

## Configure each job

Use an interval plus a local start time:

```yaml
jobs:
  - name: hourly-db
    enabled: true
    schedule:
      interval: 1h
      hour_at: 0
      minute_at: 15
    google_drive_path: hourly-db
    driver: mysql
    source: host
    # connection fields...

  - name: weekly-db
    enabled: true
    schedule:
      interval: 7d
      hour_at: 20
      minute_at: 30
    google_drive_path: weekly-db
    driver: postgres
    source: docker
    # connection fields...
```

`interval` accepts whole hours (`1h`, `2h`, and so on) or calendar days (`1d`, `3d`, `7d`). The minimum is `1h`. Set `hour_at` from `0` to `23`; `minute_at` must be `0`, `15`, `30`, or `45`. The first scheduled backup runs at the next matching server-local time. After a scheduled success, the interval is counted from that run slot. A missing or blank schedule defaults to every day at `00:00`.

The dispatcher stores per-job timing and retry state in `<storage.local.path>/.scheduler/`. The first attempt runs at the configured slot; failures are retried at the next 15-minute tick, up to 3 attempts total. If all attempts fail, the job waits for its next normal interval. Manual `alldbackup run --job NAME` and `alldbackup run --all` run immediately and do not change automatic schedule state.

The schedule uses the server's local timezone. `timestamp_timezone` only controls archive filename timestamps. Validate changes with:

```bash
alldbackup config validate
alldbackup config list
```

## Check the dispatcher

The root crontab contains one managed entry:

```cron
*/15 * * * * '/usr/local/bin/alldbackup' --config '/etc/alldbackup/config.yaml' cron tick
```

Inspect or reinstall it with:

```bash
sudo crontab -l
alldbackup cron print
sudo alldbackup cron install
```

The installer adds this entry automatically. Reinstalling it is idempotent and preserves unrelated root crontab lines. If `cron install` is not recognized, the installed binary predates interval scheduling; updating the installer script alone does not update the binary.

## Update an existing version 1 configuration

Make a copy of `/etc/alldbackup/config.yaml`, set `version: 2`, and change each cron string to the new mapping. For example:

```yaml
schedule:
  interval: 1d
  hour_at: 2
  minute_at: 0
```

There is no automatic migration command. Run `alldbackup config validate` after editing. See [configuration](configuration.md) for the complete schema.

## Remove scheduling during uninstall

Remove only the alldbackup managed block from root's crontab:

```bash
sudo alldbackup cron remove
sudo crontab -l
```

Unrelated root cron entries are retained. See [installation and uninstall](installation.md#uninstall).

## Non-Ubuntu/Debian systems

The installer does not configure cron on other distributions. Install a cron service and add the printed `cron tick` entry to root's crontab with the 15-minute interval shown above.
