# alldbackup

**Languages:** English (primary) · [Bahasa Indonesia](#bahasa-indonesia)

`alldbackup` is a Linux CLI for scheduled database backups. It supports MySQL, MariaDB, PostgreSQL, and SQLite on the host or in Docker. Each backup is a ZIP containing a database dump and `metadata.json`, plus an adjacent SHA-256 file. Backups can be stored locally and uploaded to Google Drive through Rclone.

## Install

The installer supports Ubuntu and Debian. It verifies the release checksum, installs Rclone if missing, creates the `alldbackup` group and default config files, prepares the backup directory, installs/enables cron if needed, and registers one root crontab entry that checks schedules every 15 minutes.

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

The installer adds the account that invoked `sudo` to the `alldbackup` group. Log out and back in once so the new group membership takes effect. The install and upgrade need `sudo`; routine commands below do not.

## Configure a job

Add a job interactively, or edit the YAML directly:

```bash
alldbackup config add
# or: nano /etc/alldbackup/config.yaml
```

Each job uses a simple interval such as `1h`, `1d`, or `7d`, plus `hour_at` and `minute_at`. The default is daily at 00:00 in the server's local timezone. Minutes are limited to `0`, `15`, `30`, or `45`; the root dispatcher checks jobs every 15 minutes. See [configuration](docs/configuration.md) and [`configs/config.example.yaml`](configs/config.example.yaml).

For SQL database passwords, set `password_env` to a variable name, then add that `KEY=value` to the protected secrets file:

```bash
sudo nano /etc/alldbackup/secrets.env
alldbackup config validate
alldbackup doctor
```

Environment variables take precedence over `secrets.env`. Do not put passwords in YAML or command arguments.

## Google Drive

Rclone is installed automatically when absent. To authorize a Google Drive remote, run:

```bash
alldbackup drive setup
```

Enable `storage.google_drive.enabled` and set `storage.google_drive.remote` in `/etc/alldbackup/config.yaml`. Each job's `google_drive_path` chooses its own folder under that remote. Then test access:

```bash
alldbackup drive test
```

The dedicated Rclone file is `/etc/alldbackup/rclone.conf`; it does not replace a user's existing Rclone config. See [Google Drive setup](docs/google-drive.md), including OAuth setup on a computer with a browser.

## Run, clean, and verify

Manual `run` starts immediately and does not wait for the configured schedule:

```bash
alldbackup run --job production
alldbackup run --all
```

Clean one backup by its ZIP basename or clean all backups for a job. The command deletes from all enabled storage backends:

```bash
alldbackup clean --job production 20261109T020000.123456789Z.zip
alldbackup clean --job production --all
```

Only alldbackup backup names and their sidecars are removed; unrelated files are kept. Verify a local archive with:

```bash
alldbackup verify --file /var/backups/alldbackup/production/20261109T020000.123456789Z.zip
```

## Files and permissions

| Purpose | Default path | Access |
| --- | --- | --- |
| Binary | `/usr/local/bin/alldbackup` | root-owned; install/upgrade with `sudo` |
| YAML config | `/etc/alldbackup/config.yaml` | writable by installer account; readable by all |
| Database secrets | `/etc/alldbackup/secrets.env` | root-owned, mode `0640`; root and `alldbackup` group can read |
| Rclone credentials | `/etc/alldbackup/rclone.conf` | mode `0600`; owned by installer account |
| Local backups | `/var/backups/alldbackup/<job>/` | all local users can read; `alldbackup` group can write/delete |
| Cron | root's crontab | one managed dispatcher entry; checks every 15 minutes |

The installer keeps these files and backups when upgrading. Backup ZIP contents are readable by every local user, so treat this host's account access accordingly. Docker jobs require access to the Docker daemon, which is effectively host-root access.

Local retention defaults to 7 days; Google Drive retention defaults to 30 days. Set a retention value to `0` to disable automatic deletion for that storage. Configure archive filename timezone at the top level, for example `timestamp_timezone: "+07:00"`; metadata remains UTC.

## Update or uninstall

Run the installer again to upgrade. It checksum-verifies and replaces the binary without replacing YAML, secrets, Rclone credentials, or backups:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

To uninstall the CLI, remove its root cron entry and binary. Config and backup data remain:

```bash
sudo alldbackup cron remove
sudo rm -f /usr/local/bin/alldbackup
```

The uninstall leaves the cron package, Rclone, `/etc/alldbackup`, `/var/backups/alldbackup`, and Google Drive files in place. Remove data separately only after checking the configured paths. Full steps are in [installation](docs/installation.md).

## Guides

- [Installation, permissions, upgrade, and uninstall](docs/installation.md)
- [Configuration and retention](docs/configuration.md)
- [Automatic schedules and cron](docs/cron.md)
- [Google Drive and Rclone OAuth](docs/google-drive.md)
- [Docker database jobs](docs/docker.md)
- [Verification and restore](docs/restore.md)

## Development

Requires Go 1.27.3 or newer. Run `go test ./...` and `go vet ./...`. Database restore integration tests use Docker Compose. Version tags (`v*.*.*`) trigger Linux amd64/arm64 releases with `checksums.txt`.

## License

MIT. See [`LICENSE`](LICENSE).

---

## Bahasa Indonesia

`alldbackup` adalah CLI Linux untuk backup basis data terjadwal. Aplikasi ini mendukung MySQL, MariaDB, PostgreSQL, dan SQLite dari host maupun Docker. Setiap backup berupa ZIP yang berisi dump basis data dan `metadata.json`, dengan file SHA-256 terpisah. Backup dapat disimpan lokal dan diunggah ke Google Drive melalui Rclone.

### Instalasi

Installer mendukung Ubuntu dan Debian. Installer memeriksa checksum rilis, memasang Rclone jika belum tersedia, membuat grup `alldbackup` dan konfigurasi awal, menyiapkan folder backup, memasang/mengaktifkan cron bila diperlukan, serta membuat satu entri crontab root yang memeriksa jadwal setiap 15 menit.

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

Akun yang menjalankan `sudo` akan ditambahkan ke grup `alldbackup`. Logout lalu login kembali satu kali agar keanggotaan grup aktif. Instalasi dan upgrade memerlukan `sudo`; perintah penggunaan sehari-hari di bawah ini tidak.

### Konfigurasi job

Tambahkan job secara interaktif atau edit YAML langsung:

```bash
alldbackup config add
# atau: nano /etc/alldbackup/config.yaml
```

Setiap job memakai interval sederhana seperti `1h`, `1d`, atau `7d`, ditambah `hour_at` dan `minute_at`. Default-nya setiap hari pukul 00:00 menurut waktu server. Nilai menit hanya `0`, `15`, `30`, atau `45`; dispatcher root memeriksa job setiap 15 menit. Lihat [panduan konfigurasi](docs/configuration.md) dan [`configs/config.example.yaml`](configs/config.example.yaml).

Untuk password basis data SQL, isi `password_env` dengan nama variabel, lalu tambahkan pasangan `KEY=value` ke file secrets:

```bash
sudo nano /etc/alldbackup/secrets.env
alldbackup config validate
alldbackup doctor
```

Environment variable mengalahkan nilai di `secrets.env`. Jangan menaruh password di YAML atau argumen perintah.

### Google Drive

Rclone dipasang otomatis jika belum tersedia. Untuk mengotorisasi remote Google Drive:

```bash
alldbackup drive setup
```

Aktifkan `storage.google_drive.enabled` dan isi `storage.google_drive.remote` di `/etc/alldbackup/config.yaml`. Setiap job menentukan folder masing-masing melalui `google_drive_path`. Lalu uji koneksi:

```bash
alldbackup drive test
```

File khusus Rclone berada di `/etc/alldbackup/rclone.conf`; file ini tidak mengganti konfigurasi Rclone milik user. Lihat [panduan Google Drive](docs/google-drive.md), termasuk proses OAuth menggunakan komputer dengan browser.

### Jalankan, bersihkan, dan verifikasi

Perintah `run` manual langsung berjalan dan tidak menunggu jadwal:

```bash
alldbackup run --job production
alldbackup run --all
```

Hapus satu backup berdasarkan nama file ZIP atau hapus semua backup suatu job. Penghapusan berlaku ke semua storage yang aktif:

```bash
alldbackup clean --job production 20261109T020000.123456789Z.zip
alldbackup clean --job production --all
```

Hanya arsip dengan pola nama alldbackup beserta file sidecar yang dihapus; file lain tetap ada. Verifikasi arsip lokal dengan:

```bash
alldbackup verify --file /var/backups/alldbackup/production/20261109T020000.123456789Z.zip
```

### File dan izin akses

| Kegunaan | Lokasi default | Akses |
| --- | --- | --- |
| Binary | `/usr/local/bin/alldbackup` | milik root; instalasi/upgrade memakai `sudo` |
| Konfigurasi YAML | `/etc/alldbackup/config.yaml` | dapat diedit akun installer; dapat dibaca semua user |
| Password database | `/etc/alldbackup/secrets.env` | milik root, mode `0640`; root dan grup `alldbackup` dapat membaca |
| Kredensial Rclone | `/etc/alldbackup/rclone.conf` | mode `0600`; milik akun installer |
| Backup lokal | `/var/backups/alldbackup/<job>/` | semua user lokal dapat membaca; grup `alldbackup` dapat menulis/menghapus |
| Cron | crontab root | satu entri dispatcher; memeriksa setiap 15 menit |

Upgrade mempertahankan seluruh konfigurasi dan backup. Isi ZIP dapat dibaca semua user lokal, jadi perhatikan akun yang memiliki akses ke server. Job Docker memerlukan akses Docker daemon yang setara dengan akses root host.

Retensi lokal default 7 hari dan retensi Google Drive 30 hari. Isi `0` untuk menonaktifkan penghapusan otomatis pada storage tersebut. Zona waktu nama file diatur dengan `timestamp_timezone`, misalnya `"+07:00"`; metadata tetap UTC.

### Update atau uninstall

Jalankan installer kembali untuk upgrade. Installer memeriksa checksum dan mengganti binary tanpa mengganti YAML, secrets, kredensial Rclone, atau backup:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
```

Untuk menghapus CLI, hapus entri cron root dan binary. Konfigurasi dan backup tetap tersimpan:

```bash
sudo alldbackup cron remove
sudo rm -f /usr/local/bin/alldbackup
```

Proses uninstall tidak menghapus paket cron, Rclone, `/etc/alldbackup`, `/var/backups/alldbackup`, maupun file Google Drive. Hapus data secara terpisah hanya setelah memeriksa path yang dikonfigurasi. Langkah lengkap ada di [panduan instalasi](docs/installation.md).

### Panduan

- [Instalasi, izin, upgrade, dan uninstall](docs/installation.md)
- [Konfigurasi dan retensi](docs/configuration.md)
- [Jadwal otomatis dan cron](docs/cron.md)
- [Google Drive dan Rclone OAuth](docs/google-drive.md)
- [Job database Docker](docs/docker.md)
- [Verifikasi dan restore](docs/restore.md)
