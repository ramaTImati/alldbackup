# Google Drive with Rclone

alldbackup uses Rclone for Google Drive uploads. The system-wide Rclone config is `/etc/alldbackup/rclone.conf`, next to the main YAML and secrets files. It contains OAuth tokens and is owned by the account that installed alldbackup with mode `0600`. It does not replace or modify `~/.config/rclone/rclone.conf`.

The installer installs the Rclone binary if it is missing. It does not create a remote or log in to Google.

## 1. Create and authorize a Drive remote

Run setup as the regular account that installed alldbackup:

```bash
alldbackup drive setup
```

In the Rclone menu:

1. Enter `n` to create a new remote.
2. Give it a short name, for example `gdrive`.
3. Select Google Drive as the storage type.
4. Complete the OAuth prompts and save the remote.

If the server has no browser, answer `n` when asked to use a browser on that machine. Rclone prints an `rclone authorize ...` command. Run the entire command on a computer with a browser and a compatible Rclone version, sign in to Google, approve access, then paste the complete token response back into the server terminal at the `config_token>` prompt. Keep the server terminal open while authorizing.

If Google shows `redirect_uri_mismatch`, the OAuth desktop app's redirect URI does not match the URI Rclone is using. Use the Rclone-generated authorization flow and its displayed callback URI; do not invent a redirect URI. For details, use Rclone's [remote setup guide](https://rclone.org/remote_setup/).

The remote is saved to `/etc/alldbackup/rclone.conf`. Keep this file private and do not commit it to a repository.

## 2. Set the shared remote and per-job folders

Edit `/etc/alldbackup/config.yaml` as the regular account and enable Drive:

```yaml
storage:
  google_drive:
    enabled: true
    remote: gdrive:alldbackup
    config_file: /etc/alldbackup/rclone.conf
    retention_days: 30
    timeout: 30m
    retries: 3

jobs:
  - name: production
    google_drive_path: teams/finance/production
    # other job fields...
```

`remote` is the shared base path. Each job's relative `google_drive_path` chooses its own folder. The example uploads to:

```text
gdrive:alldbackup/teams/finance/production/<timestamp>.zip
gdrive:alldbackup/teams/finance/production/<timestamp>.zip.sha256
```

Each job path must be unique. If omitted, it defaults to the job name. Changing a path affects future uploads; it does not move older files.

The base remote path must exist for `alldbackup drive test` to list it. Create it once if needed:

```bash
rclone --config /etc/alldbackup/rclone.conf mkdir gdrive:alldbackup
```

Replace `gdrive` with the remote name you created. Job subfolders are created by Rclone during upload.

Validate and test as the regular account:

```bash
alldbackup config validate
alldbackup drive test
alldbackup doctor
```

## 3. OAuth from a desktop

If you initially configure the remote on a computer with a browser, securely copy only the resulting Rclone config into `/etc/alldbackup/rclone.conf` on the server, replacing its empty file. Keep ownership with the alldbackup installer account and mode `0600`:

```bash
install -m 0600 /secure/path/rclone.conf /etc/alldbackup/rclone.conf
```

On the default installation, the regular installer account owns this path and can replace the file. Do not use a root-owned `~/.config/rclone/rclone.conf` as a source unless you have appropriate access. The root cron dispatcher can read the dedicated config because it runs as root.

If using a custom YAML config, pass `--config /path/to/config.yaml` to every alldbackup command and set that YAML's `storage.google_drive.config_file` to the dedicated Rclone file path.

## 4. Uploads, retries, and retention

By default, alldbackup creates an archive under `/var/backups/alldbackup/<job-name>/`, uploads the ZIP and SHA-256 sidecar, then checks the remote copies. A failed upload keeps the local archive for a later retry. Local retention defaults to 7 days; Google Drive retention defaults to 30 days. Set either retention to `0` to disable automatic cleanup for that storage.

Rclone `crypt` remotes are supported: configure the Google Drive remote and a `crypt` remote in the same file, then set `storage.google_drive.remote` to the `crypt` remote path. Rclone handles encryption and credentials.
