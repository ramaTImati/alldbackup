# alldbackup

**Languages:** English (primary) · [Bahasa Indonesia](#bahasa-indonesia)

`alldbackup` is a standalone Linux CLI for scheduled, full database backups. It creates a verified ZIP archive for each job, stores it locally, and can upload it to Google Drive through Rclone. It has no daemon, built-in scheduler, web server, or runtime database.

## Features

- Back up MySQL, MariaDB, PostgreSQL, and SQLite on the host or in a running Docker container.
- Create one ZIP archive per job with configurable timestamp offsets, UTC metadata, SHA-256 sidecars, and atomic publication.
- Apply local retention and optionally upload to Google Drive with separate remote retention.
- Store a separate five-field cron schedule per job and generate crontab entries with per-job locking.
- Provide Linux amd64 and arm64 release binaries; backup servers do not need Go installed.

## Requirements

Install the database utility required by each enabled job: `mysqldump` or `mariadb-dump`, `pg_dump`, or `sqlite3`. Docker jobs also require the Docker CLI and access to the Docker daemon. Google Drive uploads require Rclone. Each utility must be available on the host or inside the selected container.

## Install and create your first backup

1. Download and run the installer. The script is saved temporarily in `/tmp`; the installed program is `/usr/local/bin/alldbackup`.

   ```bash
   curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
   sudo bash /tmp/alldbackup-install.sh
   rm /tmp/alldbackup-install.sh
   ```

   The installer verifies the alldbackup release checksum. If `rclone` is missing, it also runs Rclone's official installer. This installs the binary but does not create a Google Drive remote or change an existing Rclone config. Rclone's installer needs `unzip`, `7z`, or `busybox` to extract its download.

2. Create the default config, secrets file, and local backup directory:

   ```bash
   sudo alldbackup init
   ```

   `init` creates an empty config with no database jobs. It does not overwrite a config that already exists.

3. Add a database job and its password. `config add` is interactive and asks for a `password_env` name and a five-field cron schedule. The schedule prompt defaults to `0 0 * * *` (daily at midnight in the server's timezone) if left blank. The command does not ask for or save the password.

   ```bash
   sudo alldbackup config add
   sudoedit /etc/alldbackup/secrets.env
   ```

   In `secrets.env`, add one `KEY=value` line using the exact environment variable name entered for the job, for example `MYSQL_BACKUP_PASSWORD=your-database-password`. SQLite jobs do not need a password. Keep this file readable only by the account that runs alldbackup; the default mode is `0600`.

4. Validate the config and check database connectivity and dependencies:

   ```bash
   sudo alldbackup config validate
   sudo alldbackup doctor
   ```

   For host jobs, the dump utility must be installed on this Linux host. For Docker jobs, the Docker CLI and daemon access must work, and the dump utility must exist inside the selected database container.

5. Run one job or all enabled jobs:

   ```bash
   sudo alldbackup run --job production
   # or
   sudo alldbackup run --all
   ```

   Replace `production` with the job name in the config. A failed job does not stop later jobs in `--all`; the command returns a non-zero exit code if any job fails.

   These commands run the backup immediately and then exit, regardless of each job's schedule. To run jobs automatically, add the configured schedules to cron as shown below.

### Default file locations

| Purpose | Default location | Details |
| --- | --- | --- |
| alldbackup binary | `/usr/local/bin/alldbackup` | Installed or upgraded by the installer. |
| Main config | `/etc/alldbackup/config.yaml` | Database jobs, storage, retention, and timestamp offset. |
| Passwords | `/etc/alldbackup/secrets.env` | `KEY=value` lines; mode `0600`; not stored in YAML. |
| Local backups | `/var/backups/alldbackup/<job-name>/` | One ZIP and one `.zip.sha256` sidecar per backup. |
| Rclone binary | `/usr/bin/rclone` | Installed by Rclone's official installer only when missing. |
| Rclone config | Per-user; ask `rclone config file` | It contains remote credentials. The path depends on the account running alldbackup. |

Inside each ZIP are `backup.sql` (MySQL, MariaDB, or PostgreSQL), or `database.sqlite`, plus `metadata.json`. Local backups are retained for 7 days by default; Google Drive backups for 30 days. Set either retention to `0` to disable deletion.

If you choose a custom config path, for example `sudo alldbackup --config /opt/alldbackup/config.yaml init`, init places `secrets.env` and the default `backups/` directory beside that config. Pass the same `--config` path to every later command and to cron. You can change those paths in the YAML.

## Configuration

The default YAML file is `/etc/alldbackup/config.yaml`. `config add` appends a job to its `jobs:` list. Set a `schedule` such as `0 2 * * *` for each job. If the field is omitted or blank, it defaults to `0 0 * * *` (daily at midnight in the server's timezone). See [`configs/config.example.yaml`](configs/config.example.yaml) for complete host and Docker examples.

For SQL jobs, `password_env` must match a key in `/etc/alldbackup/secrets.env` or an environment variable. Environment variables take precedence over the secrets file. For example, if the job has `password_env: MYSQL_BACKUP_PASSWORD`, store `MYSQL_BACKUP_PASSWORD=...` in the secrets file. Do not put the password itself in YAML or a process argument.

Set the top-level `timestamp_timezone` to `UTC` (the default) or a fixed offset such as `+07:00`. Filenames then use that offset (for example, `...+0700.zip`); `metadata.json` continues to record UTC. See the [configuration guide](docs/configuration.md) for the full YAML structure and [Docker guide](docs/docker.md) for container jobs.

## Google Drive

Rclone's binary is installed automatically if it is missing, but you must still create and authorize a remote. Run `sudo alldbackup drive setup` as the same account that will run backups. Check the config file path with `sudo rclone config file`. If setting up on a computer with a browser, transfer its Rclone config to the path reported on the server and restrict it to its owner. Then enable `storage.google_drive.enabled` in `/etc/alldbackup/config.yaml` and run `sudo alldbackup drive test`. See the [Google Drive guide](docs/google-drive.md).

Failed uploads keep their local archive and checksum. A later `run` retries pending uploads before local retention can remove them. The Rclone remote can be an encrypted `crypt` remote.

## Cron

To create separate cron entries from each job's `schedule`, run `sudo alldbackup cron print`, then paste the output into the crontab of the account that can read the secrets file, access the database/Docker daemon, and write the backup directory. `cron print` does not edit the crontab. See the [cron guide](docs/cron.md).

```cron
0 2 * * * '/usr/local/bin/alldbackup' run --job 'production' --config '/etc/alldbackup/config.yaml'
```

Cron uses the server's local timezone by default. `timestamp_timezone` changes archive filename timestamps only. Manual `run --all` still runs all enabled jobs immediately.

## Verify and restore

Find a ZIP under `/var/backups/alldbackup/<job-name>/`, then pass its full path to `verify`. Replace the example filename below with the one printed by `find`:

```bash
sudo find /var/backups/alldbackup -mindepth 2 -maxdepth 2 -type f -name '*.zip' -print
sudo alldbackup verify --file /var/backups/alldbackup/production/20261106T090000.123456789+0700.zip
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
- Menyimpan jadwal cron lima kolom yang berbeda untuk setiap job dan membuat baris crontab dengan penguncian per job.
- Menyediakan binary rilis Linux amd64 dan arm64; server backup tidak perlu memasang Go.

### Persyaratan

Pasang utilitas basis data yang diperlukan setiap job aktif: `mysqldump` atau `mariadb-dump`, `pg_dump`, atau `sqlite3`. Job Docker juga memerlukan Docker CLI dan akses ke Docker daemon. Unggahan Google Drive memerlukan Rclone. Utilitas tersebut harus tersedia di host atau di dalam container yang dipilih.

### Instalasi dan backup pertama

1. Unduh dan jalankan installer. Skrip disimpan sementara di `/tmp`; program dipasang ke `/usr/local/bin/alldbackup`.

   ```bash
   curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
   sudo bash /tmp/alldbackup-install.sh
   rm /tmp/alldbackup-install.sh
   ```

   Installer memeriksa checksum rilis alldbackup. Jika `rclone` belum ada, installer juga menjalankan installer resmi Rclone. Binary Rclone dipasang, tetapi remote Google Drive tidak dibuat dan konfigurasi Rclone yang sudah ada tidak diubah. Installer Rclone memerlukan `unzip`, `7z`, atau `busybox` untuk mengekstrak unduhannya.

2. Buat file konfigurasi, file secrets, dan direktori backup lokal:

   ```bash
   sudo alldbackup init
   ```

   Perintah `init` membuat konfigurasi kosong tanpa job basis data. Konfigurasi yang sudah ada tidak akan ditimpa.

3. Tambahkan job basis data dan password-nya. `config add` berjalan interaktif dan meminta nama `password_env` serta jadwal cron lima kolom. Jika jadwal dikosongkan, nilainya menjadi `0 0 * * *` (setiap hari pukul 00:00 menurut zona waktu server). Perintah ini tidak meminta atau menyimpan password.

   ```bash
   sudo alldbackup config add
   sudoedit /etc/alldbackup/secrets.env
   ```

   Di `secrets.env`, tambahkan satu baris `KEY=value` dengan nama environment variable yang sama dengan yang dimasukkan untuk job, misalnya `MYSQL_BACKUP_PASSWORD=password-basis-data-anda`. Job SQLite tidak memerlukan password. Batasi file ini agar hanya dapat dibaca akun yang menjalankan alldbackup; mode default-nya `0600`.

4. Validasi konfigurasi dan periksa koneksi serta dependensi:

   ```bash
   sudo alldbackup config validate
   sudo alldbackup doctor
   ```

   Untuk job host, utilitas dump harus terpasang di server Linux ini. Untuk job Docker, Docker CLI dan akses daemon harus berfungsi, serta utilitas dump harus tersedia di dalam container basis data yang dipilih.

5. Jalankan satu job atau semua job aktif:

   ```bash
   sudo alldbackup run --job production
   # atau
   sudo alldbackup run --all
   ```

   Ganti `production` dengan nama job di konfigurasi. Jika satu job gagal saat `--all`, job berikutnya tetap dijalankan. Perintah berakhir dengan kode non-zero jika ada job yang gagal.

   Perintah tersebut langsung menjalankan backup saat itu juga tanpa melihat jadwal job, lalu selesai. Untuk menjalankan job otomatis sesuai jadwal masing-masing, masukkan baris yang dihasilkan ke cron seperti contoh di bawah.

#### Lokasi file default

| Kegunaan | Lokasi default | Keterangan |
| --- | --- | --- |
| Binary alldbackup | `/usr/local/bin/alldbackup` | Dipasang atau diperbarui oleh installer. |
| Konfigurasi utama | `/etc/alldbackup/config.yaml` | Job basis data, storage, retensi, dan offset timestamp. |
| Password | `/etc/alldbackup/secrets.env` | Baris `KEY=value`, mode `0600`; bukan disimpan di YAML. |
| Backup lokal | `/var/backups/alldbackup/<nama-job>/` | Satu file ZIP dan satu file checksum `.zip.sha256` per backup. |
| Binary Rclone | `/usr/bin/rclone` | Dipasang oleh installer resmi Rclone hanya jika belum ada. |
| Konfigurasi Rclone | Per akun; lihat `rclone config file` | Berisi kredensial remote. Lokasinya mengikuti akun yang menjalankan alldbackup. |

Di dalam ZIP terdapat `backup.sql` (MySQL, MariaDB, PostgreSQL) atau `database.sqlite`, serta `metadata.json`. Retensi backup lokal default 7 hari, sedangkan retensi Google Drive 30 hari. Atur retensi ke `0` untuk menonaktifkan penghapusan.

Jika memakai path konfigurasi khusus, misalnya `sudo alldbackup --config /opt/alldbackup/config.yaml init`, perintah `init` menaruh `secrets.env` dan direktori default `backups/` di samping file konfigurasi. Sertakan path `--config` yang sama pada semua perintah berikutnya dan pada cron. Path tersebut juga dapat diubah di YAML.

### Konfigurasi

File YAML default adalah `/etc/alldbackup/config.yaml`. Perintah `config add` menambahkan job ke daftar `jobs:`. Atur `schedule` lima kolom untuk setiap job, misalnya `0 2 * * *`. Jika field tidak ada atau kosong, nilainya menjadi `0 0 * * *` (setiap hari pukul 00:00 menurut zona waktu server). Lihat [`configs/config.example.yaml`](configs/config.example.yaml) untuk contoh lengkap job host dan Docker.

Untuk job SQL, nilai `password_env` harus sama dengan nama key di `/etc/alldbackup/secrets.env` atau environment variable. Environment variable memiliki prioritas. Contoh: jika job berisi `password_env: MYSQL_BACKUP_PASSWORD`, simpan nilai dalam `secrets.env` sebagai `MYSQL_BACKUP_PASSWORD=...`. Jangan menaruh password dalam YAML atau argumen proses.

Atur `timestamp_timezone` tingkat atas ke `UTC` (default) atau offset tetap seperti `+07:00`. Nama file memakai offset tersebut (contoh `...+0700.zip`); `metadata.json` tetap mencatat waktu UTC. Lihat [panduan konfigurasi](docs/configuration.md) untuk struktur YAML lengkap dan [panduan Docker](docs/docker.md) untuk job container.

### Google Drive

Binary Rclone dipasang otomatis jika belum tersedia, tetapi remote Google Drive tetap harus dibuat dan diotorisasi. Jalankan `sudo alldbackup drive setup` sebagai akun yang akan menjalankan backup. Cek lokasi konfigurasi dengan `sudo rclone config file`. Jika setup dilakukan di komputer yang memiliki browser, salin file konfigurasi Rclone ke path yang dilaporkan server dan batasi izinnya hanya untuk pemilik file. Setelah itu aktifkan `storage.google_drive.enabled` di `/etc/alldbackup/config.yaml`, lalu jalankan `sudo alldbackup drive test`. Lihat [panduan Google Drive](docs/google-drive.md).

Jika unggahan gagal, arsip dan checksum lokal tetap disimpan. Perintah `run` berikutnya akan mencoba kembali unggahan yang tertunda sebelum retensi lokal dapat menghapusnya. Remote Rclone juga dapat menggunakan remote terenkripsi `crypt`.

### Cron

Untuk membuat baris cron terpisah dari `schedule` setiap job, jalankan `sudo alldbackup cron print`, lalu salin hasilnya ke crontab akun yang dapat membaca file secrets, mengakses basis data/Docker daemon, dan menulis ke direktori backup. `cron print` tidak mengubah crontab secara otomatis. Lihat [panduan cron](docs/cron.md).

```cron
0 2 * * * '/usr/local/bin/alldbackup' run --job 'production' --config '/etc/alldbackup/config.yaml'
```

Secara default cron menggunakan zona waktu lokal server. `timestamp_timezone` hanya mengubah timestamp nama file arsip. `run --all` manual tetap langsung menjalankan semua job aktif.

### Verifikasi dan pemulihan

Cari file ZIP di `/var/backups/alldbackup/<nama-job>/`, lalu berikan path lengkapnya ke `verify`. Ganti nama file contoh di bawah dengan nama yang ditampilkan oleh `find`:

```bash
sudo find /var/backups/alldbackup -mindepth 2 -maxdepth 2 -type f -name '*.zip' -print
sudo alldbackup verify --file /var/backups/alldbackup/production/20261106T090000.123456789+0700.zip
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
