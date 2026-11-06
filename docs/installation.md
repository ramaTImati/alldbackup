# Installation

## Install from a release

The installer supports Linux amd64 and arm64. It downloads the latest GitHub release, verifies the SHA-256 entry from `checksums.txt`, and installs `/usr/local/bin/alldbackup`. Run it as root to write that path:

```bash
curl -fsSL https://raw.githubusercontent.com/ramaTImati/alldbackup/main/scripts/install.sh -o install.sh
sudo bash install.sh
```

Running it again upgrades the binary. It does not install Docker, Rclone, or database utilities and does not modify configuration.

## Initialize

```bash
sudo alldbackup init
sudo alldbackup config add
sudo alldbackup config validate
sudo alldbackup doctor
```

The default config is `/etc/alldbackup/config.yaml`; local backups go to `/var/backups/alldbackup`. `init` refuses to replace an existing configuration. To use a separate location, pass `--config /absolute/path/config.yaml` to every command.

## Dependencies

Install the matching client utility on the host for host jobs. For Docker jobs, install the dump utility inside the database container. Install and configure Rclone separately when Google Drive is enabled. See [Docker](docker.md) and [Google Drive](google-drive.md).

## Manual binary installation

Download the matching tarball and `checksums.txt` from [GitHub Releases](https://github.com/ramaTImati/alldbackup/releases), verify the checksum, extract `alldbackup`, and install it as `/usr/local/bin/alldbackup` with mode `0755`.
