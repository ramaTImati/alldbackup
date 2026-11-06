# Verify and restore backups

By default, a job named `production` stores its ZIP files under `/var/backups/alldbackup/production/`. If `storage.local.path` was changed, look under `<configured-path>/<job-name>/`.

If local storage is disabled, download both the ZIP and its `.sha256` sidecar from Google Drive before verifying. Replace the timestamp below with the filename shown in the remote:

```bash
sudo rclone --config /etc/alldbackup/rclone.conf copyto 'gdrive:alldbackup/production/20261106T090000.123456789+0700.zip' /tmp/production.zip
sudo rclone --config /etc/alldbackup/rclone.conf copyto 'gdrive:alldbackup/production/20261106T090000.123456789+0700.zip.sha256' /tmp/production.zip.sha256
```

Then set `BACKUP=/tmp/production.zip`. If `storage.google_drive.config_file` uses a custom path, pass that path with Rclone's `--config` option.

## 1. Find and verify a backup

List local archives and copy the exact filename you want to restore:

```bash
sudo find /var/backups/alldbackup -mindepth 2 -maxdepth 2 -type f -name '*.zip' -print
```

Set `BACKUP` to one of the returned paths. For example (replace the timestamp with the real filename):

```bash
BACKUP='/var/backups/alldbackup/production/20261106T090000.123456789+0700.zip'
sudo alldbackup verify --file "$BACKUP"
sudo unzip -p "$BACKUP" metadata.json
```

`verify` checks the adjacent file named `<archive>.sha256`, the ZIP contents, and the timestamp in `metadata.json`. It does not prove who created an archive; only restore archives from a trusted source. `metadata.json` is stored inside the ZIP. The database dump is either `backup.sql` or `database.sqlite`.

Use `unzip -p` below to stream the dump directly into a restore tool. If you need to extract it first, save it to a protected temporary directory and remove the temporary copy after the restore.

Run the restore commands in the same shell where you set `BACKUP`, or set `BACKUP` to the chosen ZIP path again.

## MySQL or MariaDB

The SQL file contains the database creation/use statements and dump contents. Create a protected client option file for the account that will restore the database:

```bash
sudoedit /root/restore.cnf
sudo chmod 0600 /root/restore.cnf
```

Example `/root/restore.cnf`:

```ini
[client]
user=restore_user
password=replace-with-the-restore-password
host=127.0.0.1
port=3306
```

Choose one command for the target server: use `mysql` for MySQL or `mariadb` for MariaDB.

```bash
sudo unzip -p "$BACKUP" backup.sql | sudo mysql --defaults-extra-file=/root/restore.cnf
sudo unzip -p "$BACKUP" backup.sql | sudo mariadb --defaults-extra-file=/root/restore.cnf
```

The dump's `CREATE DATABASE` and `USE` statements restore the database name recorded in the backup. To use another name, review and edit a separate copy of the SQL before restoring. Test against an empty disposable database first.

## PostgreSQL

Create an empty destination database, then stream in the plain SQL dump:

```bash
createdb --host=127.0.0.1 --username=restore_user restored_database
unzip -p "$BACKUP" backup.sql | psql --host=127.0.0.1 --username=restore_user --dbname=restored_database --set=ON_ERROR_STOP=1
```

Provide credentials using a protected `.pgpass` file or another PostgreSQL-native method. Do not put passwords in command arguments. Use the restored database name you created with `createdb`.

## SQLite

Extract the SQLite database entry to the exact path where you want the restored file. Here it is written to the current directory as `restored.sqlite`:

```bash
unzip -p "$BACKUP" database.sqlite > restored.sqlite
sqlite3 -readonly restored.sqlite 'PRAGMA quick_check;'
```

The integrity check should return `ok`. Move the file to the application's expected path only after confirming the application is stopped and the destination is correct.

## After restore

Check schema, row counts, and application behavior before redirecting production traffic. A successful checksum and ZIP verification proves archive integrity, not that the restored application is correct. Schedule periodic restore tests on a disposable database.
