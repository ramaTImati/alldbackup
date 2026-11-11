<div align="center"><pre>
 .-.   _     _     ___  |__    .-.   ___  |  /  |   | |__
/ _ \ | |   | |   |   \ |  \  / _ \ / __| | /   |   |   |
| |_| | |   | |   |   | |__/  | |_| | |   |&lt;    |   | |__/
|  _| | |   | |   |   | |  \  |  _| | |__ | \   |   | |
|_|   |_|   |_|   |___/ |__/  |_|   \___| |  \  \___/ |
</pre></div>

<p align="center">
  <a href="https://github.com/ramaTImati/alldbackup/releases/latest"><img src="https://img.shields.io/github/v/release/ramaTImati/alldbackup" alt="Rilis terbaru"></a>
</p>

<p align="center">Backup basis data ke penyimpanan lokal dan Google Drive</p>

<p align="center"><strong>Bahasa:</strong> <a href="README.md">English</a> · Bahasa Indonesia</p>

alldbackup adalah CLI Linux untuk backup terjadwal basis data MySQL, MariaDB, PostgreSQL, dan SQLite dari host atau container Docker. Setiap arsip ZIP berisi dump basis data dan metadata, disertai checksum SHA-256 terpisah. Backup dapat disimpan secara lokal dan, jika diaktifkan, diunggah ke Google Drive.

## Persyaratan sistem

- Linux dengan arsitektur amd64 atau arm64. Tag rilis menggunakan format vMAJOR.MINOR.PATCH; badge di atas menampilkan rilis terbaru.
- Installer otomatis mendukung Ubuntu dan Debian serta memerlukan akses root melalui sudo.
- Instalasi memerlukan koneksi internet dan curl. Installer memasang serta mengaktifkan cron, dan memasang Rclone jika belum tersedia.
- Job basis data dari host memerlukan utilitas dump yang sesuai di host. Job Docker memerlukan akses Docker dan utilitas dump yang sesuai di container yang dipilih.
- Google Drive bersifat opsional. Untuk menggunakannya, otorisasi akun Google saat konfigurasi Rclone.

## Instalasi, update, dan uninstall

Pasang rilis terbaru di Ubuntu atau Debian:

~~~bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o /tmp/alldbackup-install.sh
sudo bash /tmp/alldbackup-install.sh
rm /tmp/alldbackup-install.sh
~~~

Installer membuat file konfigurasi dan secrets awal, grup sistem, serta dispatcher cron root. Akun yang menjalankan sudo akan ditambahkan ke grup alldbackup; logout dan login kembali agar keanggotaan grup baru berlaku. Setelah itu, command backup biasa dapat dijalankan tanpa sudo.

Update binary tanpa curl:

~~~bash
sudo alldbackup update
~~~

Command ini mengunduh rilis Linux terbaru, memverifikasi checksum, lalu hanya mengganti binary. Jalankan installer kembali jika perlu migrasi permission direktori atau cron.

Command update tersedia mulai dari rilis yang menyertakan fitur ini. Jika binary lama menampilkan pesan command tidak dikenal, jalankan installer satu kali untuk memperbaruinya.

Hapus CLI, dispatcher cron root, serta file konfigurasi/secrets/Rclone di /etc/alldbackup dengan satu command:

~~~bash
sudo alldbackup uninstall
~~~

File backup lokal tetap disimpan. Paket cron dan Rclone, grup sistem alldbackup, file Google Drive, serta file konfigurasi khusus di luar /etc/alldbackup juga tidak dihapus. Lihat [panduan instalasi dan permission](docs/installation.md) untuk detail.

Uninstall dihentikan jika konfigurasi tidak valid atau path backup lokal berada di dalam /etc/alldbackup, agar data backup tidak ikut terhapus.

## Pengaturan job

Kelola job secara interaktif dan lihat konfigurasi saat ini:

~~~bash
alldbackup config add
alldbackup config edit production
alldbackup config delete production
alldbackup config list
alldbackup config validate
~~~

Saat mengedit, setiap nilai saat ini menjadi default prompt. Menghapus job hanya menghapus konfigurasinya; backup yang sudah ada tetap tersimpan. Anda juga dapat mengedit /etc/alldbackup/config.yaml langsung, lalu menjalankan config validate.

Job mendukung MySQL, MariaDB, PostgreSQL, dan SQLite dengan sumber host atau Docker. Jadwal memakai interval seperti 1h, 1d, atau 7d ditambah jam dan menit menurut waktu lokal server. Default-nya setiap hari pukul 00:00; dispatcher cron memeriksa jadwal setiap 15 menit. Lihat [konfigurasi](docs/configuration.md), [jadwal cron](docs/cron.md), dan [job Docker](docs/docker.md).

Untuk password SQL, isi password_env dengan nama variabel lingkungan lalu simpan nilainya di file secrets yang dilindungi:

~~~bash
sudo nano /etc/alldbackup/secrets.env
alldbackup config validate
alldbackup doctor
~~~

Variabel lingkungan mengutamakan nilai di secrets.env jika namanya sama. Jangan menaruh password di YAML atau argumen command.

## Pengaturan Drive

Rclone dipasang otomatis jika belum tersedia. Konfigurasikan remote Google Drive secara interaktif:

~~~bash
alldbackup drive setup
~~~

Aktifkan Google Drive dan isi remote bersama di /etc/alldbackup/config.yaml. Setiap job dapat mengatur google_drive_path sendiri di bawah remote tersebut. Lalu periksa akses:

~~~bash
alldbackup drive test
~~~

Kredensial Rclone disimpan terpisah di /etc/alldbackup/rclone.conf; setup ini tidak mengganti konfigurasi Rclone milik user. Lihat [panduan Google Drive](docs/google-drive.md) untuk instruksi remote dan OAuth.

## Jalankan job, bersihkan backup, dan verifikasi arsip

Jalankan satu job aktif sekarang atau jalankan semua job aktif:

~~~bash
alldbackup run --job production
alldbackup run --all
~~~

Hapus satu backup berdasarkan nama file ZIP atau hapus semua backup yang dikenali untuk suatu job. Penghapusan berlaku pada semua backend storage yang aktif:

~~~bash
alldbackup clean --job production 20261109T020000.123456789Z.zip
alldbackup clean --job production --all
~~~

Hanya arsip backup alldbackup dan file sidecar checksum-nya yang dihapus. Verifikasi arsip lokal beserta checksum:

~~~bash
alldbackup verify --file /var/backups/alldbackup/production/20261109T020000.123456789Z.zip
~~~

Lihat [panduan verifikasi dan restore](docs/restore.md).

## File dan permission

| Kegunaan | Lokasi default | Pemilik dan akses |
| --- | --- | --- |
| Binary | /usr/local/bin/alldbackup | root:root, mode 0755 |
| Direktori konfigurasi | /etc/alldbackup/ | root:alldbackup, mode 1775 |
| Konfigurasi YAML | /etc/alldbackup/config.yaml | akun installer, mode 0644 |
| Secrets basis data | /etc/alldbackup/secrets.env | root:alldbackup, mode 0640 |
| Kredensial Rclone | /etc/alldbackup/rclone.conf | akun installer, mode 0600 |
| Backup lokal | /var/backups/alldbackup/<job>/ | grup alldbackup dapat menulis; semua user lokal dapat membaca |
| Cron | crontab root | satu dispatcher, diperiksa setiap 15 menit |

Retensi lokal default 7 hari dan retensi Google Drive 30 hari. Isi nilai retensi storage dengan 0 untuk menonaktifkan pembersihan otomatis. File ZIP backup dapat dibaca user lokal. Akses Docker setara dengan akses root host.

## Lisensi

MIT. Lihat [LICENSE](LICENSE).
