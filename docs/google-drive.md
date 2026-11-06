# Google Drive with Rclone

Rclone's binary is installed by the alldbackup installer if it is missing. The installer does not create a Google Drive remote or change an existing Rclone config. This page walks through remote setup, where its config file lives, and how alldbackup uses it.

## 1. Choose the account that will run backups

Rclone stores remotes in a per-user config file. Its default is usually `~/.config/rclone/rclone.conf`; the exact path is shown by `rclone config file` ([Rclone config docs](https://rclone.org/docs/#config-config-file)).

Use the same account for Rclone setup and alldbackup runs. The examples in the main README use `sudo`, so they run as root. In that case, inspect root's path with:

```bash
sudo rclone config file
```

Root commonly uses `/root/.config/rclone/rclone.conf`. If cron will run as a different account, configure/copy the remote for that account instead. A remote configured for your login account is not automatically available to root or another system user.

## 2. Create a Google Drive remote

On a machine with a browser, run Rclone's interactive setup as the account that will run backups:

```bash
sudo rclone config
```

Choose `n` to create a new remote, enter the name `gdrive`, choose Google Drive as its type, and follow Rclone's OAuth prompts. The name `gdrive` must match the remote prefix in alldbackup's config. See [Rclone's Google Drive backend guide](https://rclone.org/drive/) for provider options.

On a headless server, use Rclone's [remote setup instructions](https://rclone.org/remote_setup/): configure OAuth on a browser-capable machine, run `rclone config file` there to locate the config, then transfer the file securely to the path reported by `sudo rclone config file` on the server. Restrict the destination to the backup account:

```bash
sudo chown root:root /root/.config/rclone/rclone.conf
sudo chmod 0600 /root/.config/rclone/rclone.conf
```

Replace that example path with the path actually reported by `rclone config file`. The file contains OAuth credentials and must not be committed or shared publicly.

## 3. Enable Google Drive in alldbackup

Edit `/etc/alldbackup/config.yaml` and set the remote base path. Keep the existing local settings and jobs:

```yaml
storage:
  local:
    enabled: true
    path: /var/backups/alldbackup
    retention_days: 7
  google_drive:
    enabled: true
    remote: gdrive:alldbackup
    retention_days: 30
    timeout: 30m
    retries: 3
```

Here `gdrive` is the remote name from `rclone config`, and `alldbackup` is a folder beneath that remote. Each job gets a child folder. For example, job `production` uploads to:

```text
gdrive:alldbackup/production/<timestamp>.zip
gdrive:alldbackup/production/<timestamp>.zip.sha256
```

Run validation and test access using the same account as the scheduled backup:

```bash
sudo alldbackup config validate
sudo alldbackup drive test
sudo alldbackup doctor
```

`drive test` lists the configured remote directory. `doctor` also checks Rclone when Google Drive is enabled.

## 4. Understand local and remote files

By default, local files are first created under `/var/backups/alldbackup/<job-name>/`. The ZIP and its `.sha256` sidecar are copied to Google Drive and checked after upload. `metadata.json` is inside the ZIP. If upload fails, the local archive and checksum stay in place for a later retry.

Local retention defaults to 7 days and Google Drive retention to 30 days. They are configured independently. Pending or failed uploads are protected from local cleanup. Set a retention value to `0` to disable cleanup for that storage.

For encrypted remote storage, configure an Rclone `crypt` remote in addition to the Google Drive remote, then set `storage.google_drive.remote` to the crypt remote path. Rclone handles encryption and the associated credentials.
