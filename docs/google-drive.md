# Google Drive with Rclone

alldbackup uses Rclone for Google Drive transfers. Its Rclone configuration is kept in a dedicated file beside the other alldbackup settings: `/etc/alldbackup/rclone.conf` by default. This file stores OAuth tokens and remote credentials in Rclone's format; the main YAML stores the file path and remote destination. The dedicated file does not replace or modify `~/.config/rclone/rclone.conf`.

## 1. Install and initialize

The installer adds the Rclone binary if it is missing. Then initialize alldbackup:

```bash
sudo alldbackup init
```

This creates `/etc/alldbackup/rclone.conf` as an empty file with mode `0600`. The config file is separate from the YAML because Rclone manages its own OAuth credentials and remote settings.

### Existing Rclone setup

If you already configured Rclone before upgrading, find its current config path with `sudo rclone config file`. To keep using that file, set `storage.google_drive.config_file` in `/etc/alldbackup/config.yaml` to the reported path. To centralize it under `/etc/alldbackup`, copy it without removing the original:

```bash
sudo cp /root/.config/rclone/rclone.conf /etc/alldbackup/rclone.conf
sudo chown root:root /etc/alldbackup/rclone.conf
sudo chmod 0600 /etc/alldbackup/rclone.conf
```

Replace `/root/.config/rclone/rclone.conf` with the path printed on your server, then run `sudo alldbackup drive test`. The original Rclone config is not changed or deleted.

## 2. Create the Google Drive remote

Run the alldbackup command as the same account that will run backups:

```bash
sudo alldbackup drive setup
```

The command opens Rclone's interactive setup against `/etc/alldbackup/rclone.conf`. Choose `n` to create a remote, name it `gdrive`, choose Google Drive as its type, and complete the OAuth prompts. The name `gdrive` must match the remote prefix configured below.

If you initialized a custom main config path, include the same `--config /path/to/config.yaml` option when running `drive setup`, `drive test`, and other alldbackup commands.

If you create the remote on a computer with a browser, use Rclone's [remote setup instructions](https://rclone.org/remote_setup/) and transfer the resulting config file securely to `/etc/alldbackup/rclone.conf` on the server. Then restrict access to the backup account:

```bash
sudo chown root:root /etc/alldbackup/rclone.conf
sudo chmod 0600 /etc/alldbackup/rclone.conf
```

If `storage.google_drive.config_file` is customized, use that path instead. The file contains OAuth credentials; do not commit it or share it publicly.

## 3. Enable Google Drive

Edit `/etc/alldbackup/config.yaml` and set the remote destination and Rclone config path:

```yaml
storage:
  google_drive:
    enabled: true
    remote: gdrive:alldbackup
    config_file: /etc/alldbackup/rclone.conf
    retention_days: 30
    timeout: 30m
    retries: 3
```

`remote` is the Rclone remote name and folder. Each job uploads to a child folder, for example:

```text
gdrive:alldbackup/production/<timestamp>.zip
gdrive:alldbackup/production/<timestamp>.zip.sha256
```

Validate the settings and test access:

```bash
sudo alldbackup config validate
sudo alldbackup drive test
sudo alldbackup doctor
```

All alldbackup Rclone calls, including backup uploads and remote retention, use the configured `config_file`. `drive setup` edits this same file. A custom relative path is resolved relative to the main YAML file.

## 4. Local and remote files

By default, local files are first created under `/var/backups/alldbackup/<job-name>/`. The ZIP and its `.sha256` sidecar are copied to Google Drive and checked after upload. If upload fails, the local archive and checksum remain available for a later retry.

Local retention defaults to 7 days and Google Drive retention to 30 days. They are configured independently. Pending or failed uploads are protected from local cleanup. Set a retention value to `0` to disable cleanup for that storage.

For encrypted remote storage, configure an Rclone `crypt` remote in addition to the Google Drive remote, then set `storage.google_drive.remote` to the crypt remote path. Rclone handles encryption and associated remote credentials.
