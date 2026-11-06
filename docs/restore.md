# Restore backups

Verify the checksum and ZIP structure before restore:

```bash
alldbackup verify --file /var/backups/alldbackup/<job>/<backup>.zip
unzip -l /var/backups/alldbackup/<job>/<backup>.zip
```

Extract to a protected temporary directory. Do not restore an untrusted archive directly into a production database.

## MySQL or MariaDB

The dump includes `CREATE DATABASE` and `USE` statements:

```bash
unzip -p backup.zip backup.sql | mysql --defaults-extra-file=/root/restore.cnf
```

Use the matching MySQL or MariaDB client and a protected credential file. Confirm the target database and review the SQL before running it.

## PostgreSQL

Create an empty target database, then restore the plain SQL dump:

```bash
createdb --host=127.0.0.1 --username=restore_user restored_database
unzip -p backup.zip backup.sql | psql --host=127.0.0.1 --username=restore_user --dbname=restored_database --set=ON_ERROR_STOP=1
```

Provide credentials using a protected `.pgpass` file or another PostgreSQL-native method; do not place passwords in command arguments.

## SQLite

Extract `database.sqlite` to a protected path and open it with SQLite:

```bash
unzip -p backup.zip database.sqlite > restored.sqlite
sqlite3 -readonly restored.sqlite 'PRAGMA quick_check;'
```

Test restores regularly against a disposable database. A successful ZIP verification proves archive integrity, not application-level correctness.
