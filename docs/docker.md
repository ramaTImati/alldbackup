# Docker database backups

For a Docker job, alldbackup runs on the Linux host and talks to a database container through Docker CLI. The archive is still written on the host, under `/var/backups/alldbackup/<job-name>/` by default; it is not saved inside the database container.

## 1. Check the host and container

The Linux host needs:

- Docker CLI installed and permission to access the Docker daemon/socket.
- The configured local backup path writable by the account running alldbackup.

The selected container must be running and contain its matching dump utility: `mysqldump`/`mariadb-dump` for MySQL-family databases, `pg_dump` for PostgreSQL, or `sqlite3` for SQLite. Find the actual container name with:

```bash
sudo docker ps --format '{{.Names}}'
```

Only use container names that match the current Compose or Docker deployment.

## 2. Add the job to the host config

Example PostgreSQL job in `/etc/alldbackup/config.yaml`:

```yaml
jobs:
  - name: production-db
    enabled: true
    driver: postgres
    source: docker
    container: postgres-db
    database: production
    username: backup_user
    password_env: POSTGRES_BACKUP_PASSWORD
```

Use the real container name for `container`. For MySQL/MariaDB/PostgreSQL, alldbackup runs the dump utility inside that container and connects locally; the database port does not need to be published to the host. PostgreSQL uses port `5432` by default; set `port` only if the server listens on another port. For SQLite, `file` is the database path inside the container, for example `/var/lib/app/database.sqlite`; there is no database host, port, username, or password.

Add SQL passwords on the host in `/etc/alldbackup/secrets.env`, not inside the database container:

```dotenv
POSTGRES_BACKUP_PASSWORD=replace-with-the-database-password
```

Protect the file with `sudo chmod 0600 /etc/alldbackup/secrets.env`. The `password_env` value in YAML must exactly match the key in this file or an environment variable.

## 3. Check access and run a test backup

```bash
sudo alldbackup config validate
sudo alldbackup doctor
sudo alldbackup run --job production-db
```

The host output is written to `/var/backups/alldbackup/production-db/` by default. The ZIP contains the dump and `metadata.json`; the adjacent `.zip.sha256` file checks the archive. Change the base path in `storage.local.path` if you want another host directory.

## Temporary credentials and permissions

For MySQL-family and PostgreSQL jobs, alldbackup creates a mode-`0600` temporary credential file on the host, copies it to a temporary path in the selected container, invokes the dump utility with that file path, and removes it afterward. SQLite uses `.backup` inside the container, copies the temporary database to the host, then removes the container file. These are temporary operation files, not permanent config or backup locations.

Docker daemon/socket access is effectively host-root access. Only trusted operators should control it. If a dump or cancellation interrupts cleanup and the error reports a leftover container path, remove that exact temporary file from the named container, for example:

```bash
sudo docker exec postgres-db rm -f /tmp/alldbackup-pgpass-example
```

Use the actual path and container name from the error message. Run `alldbackup doctor` before adding the command to [cron](cron.md).
