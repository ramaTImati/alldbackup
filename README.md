# alldbackup

**Languages:** English (primary) · [Bahasa Indonesia](#bahasa-indonesia)

`alldbackup` is a standalone Linux CLI for scheduled, full database backups. It creates a verified ZIP archive for each job, stores it locally, and can upload it to Google Drive through Rclone. It has no daemon, built-in scheduler, web server, or runtime database.

## Features

- Back up MySQL, MariaDB, PostgreSQL, and SQLite on the host or in a running Docker container.
- Create one ZIP archive per job with configurable timestamp offsets, UTC metadata, SHA-256 sidecars, and atomic publication.
- Apply local retention and optionally upload to Google Drive with separate remote retention.
- Support cron with per-job locking and structured `slog` output.
- Provide Linux amd64 and arm64 release binaries; backup servers do not need Go installed.

## Requirements

Install the database utility required by each enabled job: `mysqldump` or `mariadb-dump`, `pg_dump`, or `sqlite3`. Docker jobs also require the Docker CLI and access to the Docker daemon. Google Drive uploads require Rclone. Each utility must be available on the host or inside the selected container.

## Install and create your first backup

Install the latest verified release:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o install.sh
sudo bash install.sh
sudo alldbackup init
sudo alldbackup config add
sudo alldbackup config validate
sudo alldbackup doctor
sudo alldbackup run --all
```

`init` creates `/etc/alldbackup/config.yaml`, creates `/etc/alldbackup/secrets.env` with mode `0600`, and creates `/var/backups/alldbackup`. It will not overwrite an existing configuration. `config add` asks for job details and a password environment variable name; it never asks for the password itself.

## Configuration

Jobs are defined in one YAML file. See [`configs/config.example.yaml`](configs/config.example.yaml) for host and Docker examples covering all supported database types. Passwords stay out of YAML and process arguments.

Set the top-level `timestamp_timezone` to `UTC` (the default) or a fixed offset such as `+07:00`. The archive filename uses that offset (for example, `...+0700.zip`), while `metadata.json` always records UTC.

```yaml
- name: production
  enabled: true
  driver: mysql
  source: host
  host: 127.0.0.1
  port: 3306
  database: production
  username: backup_user
  password_env: MYSQL_BACKUP_PASSWORD
```

Add the referenced value to the secrets file as `MYSQL_BACKUP_PASSWORD=...`, or provide it in the process environment. Environment variables take precedence. Keep the secrets file mode at `0600`. See the [configuration guide](docs/configuration.md) and [Docker guide](docs/docker.md).

## Google Drive

The installer installs Rclone from its official installer if it is missing. Then run `alldbackup drive setup`. On a headless server, configure the Rclone remote on a machine with a browser and transfer its protected configuration to the server. Enable the remote in YAML and run `alldbackup drive test`. See the [Google Drive guide](docs/google-drive.md).

Failed uploads keep their local archive and checksum. A later `run` retries pending uploads before local retention can remove them. The Rclone remote can be an encrypted `crypt` remote.

## Cron

Run backups from cron using an absolute binary and configuration path. See the [cron guide](docs/cron.md). Example:

```cron
0 2 * * * /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

## Verify and restore

Check the ZIP checksum and archive contents:

```bash
alldbackup verify --file /var/backups/alldbackup/production/20261106T020000.123456789Z.zip
```

See the [restore guide](docs/restore.md) for each database type. Restore to a test database before relying on a backup for recovery.

## Troubleshooting

- Run `alldbackup config validate` to check YAML syntax and required fields.
- Run `alldbackup doctor` to check utilities, Docker, connectivity, permissions, and configured Rclone access.
- If a job fails during `run --all`, later jobs still run. The command exits with a non-zero status and logs each failure.
- MySQL `--single-transaction` provides a consistent snapshot for transactional tables such as InnoDB. Non-transactional tables can change during a dump.
- Docker daemon access is effectively host-root access. Temporary credential files are copied into the selected container and removed after use.

## Development and releases

Requires Go 1.27.3 or newer. Run `go mod tidy`, `go test ./...`, and `go vet ./...`. Full database integration tests use `docker compose` (see [`tests/integration/`](tests/integration/)). Push a semantic version tag such as `v1.0.0` to build Linux release archives and checksums through GitHub Actions. See the [installation guide](docs/installation.md).

## License

MIT. See [`LICENSE`](LICENSE).

---

## Bahasa Indonesia

`alldbackup` adalah CLI Linux mandiri untuk pencadangan basis data penuh yang dapat dijadwalkan. Aplikasi ini membuat arsip ZIP terverifikasi untuk setiap job, menyimpannya secara lokal, dan dapat mengunggahnya ke Google Drive melalui Rclone. Aplikasi ini tidak menjalankan daemon, penjadwal bawaan, server web, atau basis data runtime.

### Fitur

- Mencadangkan MySQL, MariaDB, PostgreSQL, dan SQLite dari host atau container Docker yang sedang berjalan.
- Membuat satu arsip ZIP per job dengan offset timestamp yang dapat dikonfigurasi, metadata UTC, file checksum SHA-256, dan publikasi atomik.
- Menerapkan retensi lokal dan, secara opsional, mengunggah ke Google Drive dengan retensi remote terpisah.
- Mendukung cron, penguncian per job, dan log terstruktur `slog`.
- Menyediakan binary rilis Linux amd64 dan arm64; server backup tidak perlu memasang Go.

### Persyaratan

Pasang utilitas basis data yang diperlukan setiap job aktif: `mysqldump` atau `mariadb-dump`, `pg_dump`, atau `sqlite3`. Job Docker juga memerlukan Docker CLI dan akses ke Docker daemon. Unggahan Google Drive memerlukan Rclone. Utilitas tersebut harus tersedia di host atau di dalam container yang dipilih.

### Instalasi dan backup pertama

Pasang rilis terbaru yang checksum-nya telah diverifikasi:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o install.sh
sudo bash install.sh
sudo alldbackup init
sudo alldbackup config add
sudo alldbackup config validate
sudo alldbackup doctor
sudo alldbackup run --all
```

Perintah `init` membuat `/etc/alldbackup/config.yaml`, membuat `/etc/alldbackup/secrets.env` dengan mode `0600`, dan membuat direktori `/var/backups/alldbackup`. Perintah ini tidak menimpa konfigurasi yang sudah ada. `config add` meminta detail job dan nama environment variable untuk password; password itu sendiri tidak diminta.

### Konfigurasi

Semua job ditulis dalam satu file YAML. Lihat [`configs/config.example.yaml`](configs/config.example.yaml) untuk contoh host dan Docker bagi semua jenis basis data yang didukung. Password tidak disimpan dalam YAML atau argumen proses.

Atur `timestamp_timezone` tingkat atas ke `UTC` (default) atau offset tetap seperti `+07:00`. Nama arsip menggunakan offset tersebut (contoh `...+0700.zip`), sementara `metadata.json` tetap menyimpan waktu UTC.

```yaml
- name: production
  enabled: true
  driver: mysql
  source: host
  host: 127.0.0.1
  port: 3306
  database: production
  username: backup_user
  password_env: MYSQL_BACKUP_PASSWORD
```

Tambahkan nilai yang dirujuk ke file secrets dengan format `MYSQL_BACKUP_PASSWORD=...`, atau sediakan melalui environment proses. Nilai environment memiliki prioritas. Atur mode file secrets ke `0600`. Lihat [panduan konfigurasi](docs/configuration.md) dan [panduan Docker](docs/docker.md).

### Google Drive

Installer memasang Rclone dari installer resminya jika belum tersedia. Lalu jalankan `alldbackup drive setup`. Untuk server tanpa browser, konfigurasi remote Rclone di komputer yang memiliki browser, lalu salin konfigurasi Rclone yang terlindungi ke server. Aktifkan remote di YAML dan jalankan `alldbackup drive test`. Lihat [panduan Google Drive](docs/google-drive.md).

Jika unggahan gagal, arsip dan checksum lokal tetap disimpan. Perintah `run` berikutnya akan mencoba kembali unggahan yang tertunda sebelum retensi lokal dapat menghapusnya. Remote Rclone juga dapat menggunakan remote terenkripsi `crypt`.

### Cron

Jalankan backup melalui cron dengan path absolut untuk binary dan file konfigurasi. Lihat [panduan cron](docs/cron.md). Contoh:

```cron
0 2 * * * /usr/local/bin/alldbackup run --all --config /etc/alldbackup/config.yaml
```

### Verifikasi dan pemulihan

Periksa checksum ZIP dan isi arsip:

```bash
alldbackup verify --file /var/backups/alldbackup/production/20261106T020000.123456789Z.zip
```

Lihat [panduan pemulihan](docs/restore.md) untuk setiap jenis basis data. Lakukan restore ke basis data uji sebelum mengandalkan backup untuk pemulihan.

### Pemecahan masalah

- Jalankan `alldbackup config validate` untuk memeriksa YAML dan field wajib.
- Jalankan `alldbackup doctor` untuk memeriksa utilitas, Docker, konektivitas, izin, dan akses Rclone yang dikonfigurasi.
- Jika satu job gagal saat `run --all`, job berikutnya tetap dijalankan. Perintah berakhir dengan status non-zero dan mencatat setiap kegagalan di log.
- MySQL `--single-transaction` memberikan snapshot konsisten untuk tabel transaksional seperti InnoDB. Isi tabel non-transaksional dapat berubah selama proses dump.
- Akses ke Docker daemon setara dengan akses root host. File kredensial sementara disalin ke container yang dipilih lalu dihapus setelah digunakan.

### Pengembangan dan rilis

Memerlukan Go 1.27.3 atau lebih baru. Jalankan `go mod tidy`, `go test ./...`, dan `go vet ./...`. Tes integrasi basis data lengkap menggunakan `docker compose` (lihat [`tests/integration/`](tests/integration/)). Push tag versi semantik, misalnya `v1.0.0`, untuk membuat arsip rilis Linux dan checksum melalui GitHub Actions. Lihat [panduan instalasi](docs/installation.md).

### Lisensi

MIT. Lihat [`LICENSE`](LICENSE).
