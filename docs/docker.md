# Docker database backups

Docker jobs use the Docker CLI. The container must be running and must contain the relevant database dump utility; SQLite jobs need `sqlite3`. The application checks container status and utility availability before dumping. It does not assume Bash or database environment variables are present.

```yaml
- name: production-db
  enabled: true
  driver: postgres
  source: docker
  container: postgres-db
  database: production
  username: backup_user
  password_env: POSTGRES_BACKUP_PASSWORD
```

For MySQL/MariaDB and PostgreSQL, alldbackup writes a temporary credential file on the host, copies it into the container, passes only its path to the database utility, then removes it. Docker daemon access can grant host-root privileges; only trusted operators should control the Docker socket. If cleanup fails, the command reports the container path so it can be removed manually.

SQLite uses `.backup` inside the container, copies the resulting temporary database to the host, and removes the container temporary file. The source database is opened read-only for integrity checks and online backup.

Run `alldbackup doctor` to check the container and database before scheduling a backup.
