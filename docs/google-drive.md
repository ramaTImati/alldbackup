# Google Drive with Rclone

Install Rclone from its [official instructions](https://rclone.org/install/). Configure a Google Drive remote:

```bash
alldbackup drive setup
```

Rclone performs OAuth and stores its token in its own config, not the alldbackup YAML. For headless servers, use Rclone's remote setup or remote authorization flow on a browser-capable machine, then copy the Rclone config securely to the server. Protect that config according to Rclone's security guidance.

Enable and test the remote:

```yaml
storage:
  google_drive:
    enabled: true
    remote: gdrive:alldbackup
    retention_days: 30
    timeout: 30m
    retries: 3
```

```bash
alldbackup drive test
```

Archives are uploaded beneath `<remote>/<job>/`. Both ZIP and checksum files are uploaded and checked with Rclone. Upload failures leave local files for a later retry. Remote retention lists the configured job prefix and deletes only expired files with alldbackup's timestamped archive names; it does not use `sync`.

For encrypted remote storage, configure an Rclone `crypt` remote and set `remote` to that remote name. Rclone manages encryption and OAuth credentials.
