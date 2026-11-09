# Docker database backups

alldbackup runs on the Linux host and executes the matching database dump utility inside a running container through Docker CLI. The archive is written on the host, under `/var/backups/alldbackup/<job-name>/` by default; it is not stored in the database container.

## 1. Check host and container access

The host needs Docker CLI access to the daemon. List running containers:

```bash
docker ps --format '{{.Names}}'
```

If this returns a permission error, configure Docker access for the account that runs manual commands and root cron. Adding an account to the Docker group is effectively granting host-root access. Log out and back in after changing group membership.

The selected container must be running and contain the matching utility: `mysqldump` or `mariadb-dump` for MySQL-family databases, `pg_dump` for PostgreSQL, or `sqlite3` for SQLite.

## 2. Configure the job

Edit `/etc/alldbackup/config.yaml` as the regular alldbackup account:

```yaml
jobs:
  - name: production-db
    enabled: true
    schedule: "0 2 * * *"
    google_drive_path: production/postgres
    driver: postgres
    source: docker
    container: postgres-db
    database: production
    username: backup_user
    password_env: POSTGRES_BACKUP_PASSWORD
```

Use the actual container name for `container`. For MySQL/MariaDB/PostgreSQL, alldbackup runs the dump utility in the container and connects locally; the database port need not be published on the host. PostgreSQL defaults to port `5432`; set `port` only if the server uses another port. For SQLite, `file` is the path inside the container, for example `/var/lib/app/database.sqlite`; no username or password is needed.

Add SQL passwords to the host secrets file:

```bash
sudoedit /etc/alldbackup/secrets.env
```

```dotenv
POSTGRES_BACKUP_PASSWORD=replace-with-the-database-password
```

The file remains root-owned, mode `0640`, readable by root and the `alldbackup` group. `password_env` must exactly match the key or an environment variable.

## 3. Validate and run

```bash
alldbackup config validate
alldbackup doctor
alldbackup run --job production-db
```

`run` starts immediately. The automatic root cron dispatcher runs the job at its configured schedule. Host output is written to `/var/backups/alldbackup/production-db/` by default. A ZIP contains the database dump and `metadata.json`; the adjacent `.zip.sha256` sidecar checks it.

## Temporary credentials and access

For SQL jobs, alldbackup creates a mode-`0600` temporary credential file on the host, copies it to a temporary path in the selected container, runs the dump utility using that file, then removes it. SQLite uses `.backup` inside the container, copies the temporary database to the host, and removes the container file. These files are temporary operation files, not permanent configuration or backup locations.

Docker daemon/socket access is effectively host-root access. Only trusted operators should control it. If an interrupted dump reports a leftover container path, remove that exact temporary file from the named container, for example:

```bash
docker exec postgres-db rm -f /tmp/alldbackup-pgpass-example
```

Use the actual path and container name from the error message. Run `alldbackup doctor` before relying on the automatic [cron schedule](cron.md).
