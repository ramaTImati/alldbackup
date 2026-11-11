# Google Drive setup with Rclone

alldbackup uses Rclone to upload backup ZIP files and their SHA-256 sidecars to Google Drive. The installer installs the Rclone program if it is missing, but it does not create a remote or authorize a Google account.

The dedicated Rclone configuration is <code>/etc/alldbackup/rclone.conf</code>. It contains OAuth client credentials and refresh tokens, is owned by the account that installed alldbackup, and has mode <code>0600</code>. It does not replace the account's usual Rclone configuration.

Use your own Google OAuth client ID and client secret. Rclone's shared Google Drive client ID is being retired during 2026. An OAuth client is different from an API key: Rclone needs the OAuth client ID and secret, then a Google user must sign in and grant access.

## 1. Create Google OAuth client credentials

1. Open the [Google Cloud Console](https://console.cloud.google.com/) and select a project or create one.
2. Open APIs & Services > Library, find <code>Google Drive API</code>, and enable it for the project.
3. Configure the OAuth consent screen (shown as Google Auth Platform in some Console versions):
   - Set an app name such as alldbackup, a support email, and a contact email.
   - Choose <code>External</code> for a personal Google account. Choose <code>Internal</code> only if the Google account is in the same Google Workspace organization.
   - In Data Access, add these scopes:
     - https://www.googleapis.com/auth/drive
     - https://www.googleapis.com/auth/docs
     - https://www.googleapis.com/auth/drive.metadata.readonly
   - For an External app in Testing, add the Google account that will own the backups under <code>Test users</code>.
4. Open Clients or Credentials, create an OAuth client, and select <code>Desktop app</code> as its application type. Rclone is a desktop/installed application, even when it runs on a server.
5. Copy the Client ID and Client secret. If the Console downloads a JSON file, open it locally and copy the <code>client_id</code> and <code>client_secret</code> values. Do not use an API key in these fields.

For an External app left in Testing, Google expires refresh tokens after seven days for the Drive scopes used here. For unattended backups, publish the app to Production after adding the intended user. A personal app may still show an unverified-app warning at sign-in; review the current [Rclone guidance for creating a client ID](https://rclone.org/drive/#making-your-own-client-id) and Google's verification rules.

## 2. Create and authorize the Rclone remote

Run setup as the regular account that installed alldbackup, without <code>sudo</code>:

~~~bash
alldbackup drive setup
~~~

In the Rclone menu:

1. Enter <code>n</code> to create a new remote.
2. Name it <code>gdrive</code> (or another short name).
3. Select Google Drive. The backend name is <code>drive</code>.
4. Paste the Client ID and Client secret from the previous section.
5. For the scope, choose <code>1</code> (<code>drive</code>) for the most straightforward setup. This grants full access to the Google account's Drive. The narrower <code>drive.file</code> scope only lets Rclone see files and folders it created; the remote base folder must also be created using this same Rclone remote.
6. Leave <code>Service Account Credentials</code> blank for normal user-account OAuth.
7. At “Use web browser to automatically authenticate Rclone with remote?”, choose <code>Y</code> if this server has a browser you can use. Sign in to the intended Google account and grant access.
8. Choose <code>N</code> for the browser question on a headless server, then follow the next section.
9. Choose Yes if Rclone asks whether this is a Shared Drive. Choose No for a regular My Drive.
10. Confirm that you want to keep the remote.

The OAuth token is written to <code>/etc/alldbackup/rclone.conf</code>. Keep this file private. Do not commit it, paste it into chat, or share it publicly. Do not protect this dedicated config with Rclone's interactive config-password prompt; scheduled cron jobs cannot enter a password. The file's <code>0600</code> permissions protect the credentials at rest.

### Authorize from a computer when the server has no browser

Keep the server's <code>alldbackup drive setup</code> session open. When Rclone asks for <code>config_token</code>, it prints an <code>rclone authorize</code> command. On a trusted computer with a browser, install Rclone (the same version is recommended) and run the exact command printed by the server. Sign in to the intended Google account and approve access.

Copy the complete token output from the desktop terminal and paste it at <code>config_token</code> on the server. The token is a credential; do not post it or save it in a public log. Finish the remaining prompts and confirm that the remote should be kept.

If Google reports redirect_uri_mismatch, use the authorization command and callback URL shown by Rclone. Do not invent or add a different redirect URL. See [Rclone's headless setup guide](https://rclone.org/remote_setup/) for the authorization and SSH-tunnel alternatives.

If you configure Rclone entirely on a desktop instead, use a dedicated Rclone config file, then transfer it securely to <code>/etc/alldbackup/rclone.conf</code>. For example, as the installer account:

~~~bash
install -m 0600 /secure/path/rclone.conf /etc/alldbackup/rclone.conf
~~~

A config file can contain every remote configured on that computer, so check its contents before transferring it. Do not use an Rclone config protected by a config password with unattended cron runs.

## 3. Enable Drive in alldbackup

Edit <code>/etc/alldbackup/config.yaml</code> as the installer account and enable the shared remote plus the per-job folder:

~~~yaml
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
~~~

The <code>remote</code> value is the shared base path. Each job's relative <code>google_drive_path</code> selects its folder below that path. In this example, alldbackup uploads:

~~~text
gdrive:alldbackup/teams/finance/production/<timestamp>.zip
gdrive:alldbackup/teams/finance/production/<timestamp>.zip.sha256
~~~

Each job path must be unique. If omitted, it defaults to the job name. Changing a path affects future uploads and does not move existing files.

The base remote path must exist before <code>alldbackup drive test</code> can list it. Create it once if necessary:

~~~bash
rclone --config /etc/alldbackup/rclone.conf mkdir gdrive:alldbackup
alldbackup config validate
alldbackup drive test
alldbackup doctor
~~~

Replace <code>gdrive</code> with the remote name you chose. Rclone creates each job subfolder during upload.

## 4. Uploads, retries, and retention

By default, alldbackup creates an archive under /var/backups/alldbackup/<job-name>/, uploads the ZIP and SHA-256 sidecar, then checks the remote copies. A failed upload keeps the local archive for a later retry. Local retention defaults to 7 days; Google Drive retention defaults to 30 days. Set either retention value to 0 to disable automatic cleanup for that storage.

Rclone crypt remotes are supported. Configure both the Google Drive remote and a crypt remote in the same file, then set storage.google_drive.remote to the crypt remote path. Rclone handles encryption and credentials.

## Official references

- [Rclone Google Drive backend and custom OAuth client](https://rclone.org/drive/#making-your-own-client-id)
- [Rclone remote/headless authorization](https://rclone.org/remote_setup/)
- [Rclone authorize command](https://rclone.org/commands/rclone_authorize/)
- [Google OAuth 2.0 for installed apps](https://developers.google.com/identity/protocols/oauth2#installed)
- [Google OAuth refresh-token expiration](https://developers.google.com/identity/protocols/oauth2#expiration)
