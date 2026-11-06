# Cron

The application has no internal scheduler. Use the system cron service and provide absolute paths because cron does not load an interactive shell profile.

```cron
0 2 * * * /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

Run cron as the account that can read the secrets file, access the database and Docker socket, and write the backup directory. The command logs to standard error and returns non-zero if any selected job fails. A per-job file lock prevents duplicate backups of the same job.

To test the exact command before installing it in cron:

```bash
sudo /usr/local/bin/alldbackup doctor --config /etc/alldbackup/config.yaml
sudo /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```
