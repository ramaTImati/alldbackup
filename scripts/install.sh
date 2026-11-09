#!/usr/bin/env bash
set -euo pipefail

repo="ramaTImati/alldbackup"
prefix="${DESTDIR:-}"
install_path="${prefix}/usr/local/bin/alldbackup"
etc_dir="${prefix}/etc/alldbackup"
backup_dir="${prefix}/var/backups/alldbackup"
staged=0
[[ -n "$prefix" ]] && staged=1

die() { echo "$*" >&2; exit 1; }

if [[ "$(uname -s)" != "Linux" ]]; then
  die "alldbackup installer supports Linux only"
fi

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) die "Unsupported architecture: $(uname -m)" ;;
esac

if (( ! staged )); then
  if (( EUID != 0 )); then
    die "Run the installer with sudo: sudo bash install.sh"
  fi
  if [[ ! -r /etc/os-release ]]; then
    die "Cannot identify Linux distribution; automatic cron setup supports Ubuntu and Debian"
  fi
  # shellcheck disable=SC1091
  . /etc/os-release
  if [[ "${ID:-}" != ubuntu && "${ID:-}" != debian && " ${ID_LIKE:-} " != *" debian "* ]]; then
    die "Automatic cron setup supports Ubuntu and Debian only"
  fi
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
archive="alldbackup_linux_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/latest/download"
curl -fsSL "${base_url}/${archive}" -o "${workdir}/${archive}"
curl -fsSL "${base_url}/checksums.txt" -o "${workdir}/checksums.txt"
checksum="$(awk -v name="${archive}" '$2 == name {print $1}' "${workdir}/checksums.txt")"
if [[ ! "${checksum}" =~ ^[[:xdigit:]]{64}$ ]]; then
  die "No valid checksum found for ${archive}"
fi
printf '%s  %s\n' "$checksum" "$archive" > "${workdir}/expected.sha256"
(cd "$workdir" && sha256sum --check --status expected.sha256) || die "Checksum verification failed"
tar -xzf "${workdir}/${archive}" -C "$workdir" alldbackup

if (( ! staged )); then
  if ! command -v crontab >/dev/null 2>&1; then
    command -v apt-get >/dev/null 2>&1 || die "cron is missing; install it manually on this Ubuntu/Debian system"
    apt-get update
    apt-get install -y cron
  fi
  if command -v systemctl >/dev/null 2>&1 && systemctl enable --now cron; then
    :
  elif command -v service >/dev/null 2>&1 && command -v update-rc.d >/dev/null 2>&1 && update-rc.d cron defaults && service cron start; then
    :
  else
    die "Could not enable and start the cron service"
  fi

  if ! getent group alldbackup >/dev/null 2>&1; then
    groupadd --system alldbackup
  fi
  runtime_user="${SUDO_USER:-root}"
  id "$runtime_user" >/dev/null 2>&1 || die "Runtime user $runtime_user does not exist"
  if [[ "$runtime_user" != root ]]; then
    usermod -aG alldbackup "$runtime_user"
  fi
else
  runtime_user="$(id -un)"
fi

if [[ -L "$etc_dir" || -L "$backup_dir" ]]; then
  die "Refusing to install through a symlinked config or backup directory"
fi

if (( staged )); then
  install -d -m 1775 "$etc_dir"
  install -d -m 2775 "$backup_dir"
else
  install -d -o root -g alldbackup -m 1775 "$etc_dir"
  install -d -o root -g alldbackup -m 2775 "$backup_dir"
  chown root:alldbackup "$etc_dir" "$backup_dir"
  chmod 1775 "$etc_dir"
  chmod 2775 "$backup_dir"
fi
chmod 1775 "$etc_dir"
chmod 2775 "$backup_dir"

for file in config.yaml secrets.env rclone.conf; do
  path="$etc_dir/$file"
  if [[ -L "$path" || ( -e "$path" && ! -f "$path" ) ]]; then
    die "Refusing to replace non-regular config path: $path"
  fi
done

if [[ ! -e "$etc_dir/config.yaml" ]]; then
  cat > "$etc_dir/config.yaml" <<'CONFIG'
version: 2
timestamp_timezone: UTC
secrets_file: /etc/alldbackup/secrets.env
storage:
  local:
    enabled: true
    path: /var/backups/alldbackup
    retention_days: 7
  google_drive:
    enabled: false
    remote: ""
    config_file: /etc/alldbackup/rclone.conf
    retention_days: 30
    timeout: 30m
    retries: 3
jobs: []
CONFIG
fi
if [[ ! -e "$etc_dir/secrets.env" ]]; then
  printf '# Add NAME=value entries referenced by password_env.\n' > "$etc_dir/secrets.env"
fi
if [[ ! -e "$etc_dir/rclone.conf" ]]; then
  : > "$etc_dir/rclone.conf"
fi

if (( staged )); then
  chmod 0644 "$etc_dir/config.yaml"
  chmod 0640 "$etc_dir/secrets.env"
  chmod 0600 "$etc_dir/rclone.conf"
else
  chown "$runtime_user:alldbackup" "$etc_dir/config.yaml" "$etc_dir/rclone.conf"
  chown root:alldbackup "$etc_dir/secrets.env"
  chmod 0644 "$etc_dir/config.yaml"
  chmod 0640 "$etc_dir/secrets.env"
  chmod 0600 "$etc_dir/rclone.conf"
fi

if (( ! staged )); then
  find "$backup_dir" -xdev -path "$backup_dir/.scheduler" -prune -o -type d ! -path "$backup_dir/.locks" -exec chgrp alldbackup {} +
  find "$backup_dir" -xdev -type f \( -name '*.zip' -o -name '*.zip.sha256' \) -exec chgrp alldbackup {} +
fi
find "$backup_dir" -xdev -path "$backup_dir/.scheduler" -prune -o -type d ! -path "$backup_dir/.locks" -exec chmod 2775 {} +
find "$backup_dir" -xdev -type f \( -name '*.zip' -o -name '*.zip.sha256' \) -exec chmod 0664 {} +
if [[ -e "$backup_dir/.locks" || -L "$backup_dir/.locks" ]]; then
  [[ -d "$backup_dir/.locks" && ! -L "$backup_dir/.locks" ]] || die "Refusing to use invalid lock directory: $backup_dir/.locks"
  if (( ! staged )); then
    chgrp alldbackup "$backup_dir/.locks"
  fi
  chmod 2770 "$backup_dir/.locks"
  if (( ! staged )); then
    find "$backup_dir/.locks" -xdev -type f -exec chgrp alldbackup {} + -exec chmod 0660 {} +
  else
    find "$backup_dir/.locks" -xdev -type f -exec chmod 0660 {} +
  fi
fi

install -d "$(dirname "$install_path")"
install -m 0755 "$workdir/alldbackup" "$install_path"

if ! command -v rclone >/dev/null 2>&1; then
  echo "Rclone is missing; installing it with the official installer..." >&2
  curl --proto '=https' --tlsv1.2 -fsSL https://rclone.org/install.sh -o "$workdir/rclone-install.sh"
  bash "$workdir/rclone-install.sh" || die "Rclone installer failed"
  command -v rclone >/dev/null 2>&1 || die "Rclone installer completed, but rclone is still not available on PATH"
fi

"$install_path" cron install
"$install_path" version
if (( ! staged )) && [[ "$runtime_user" != root ]]; then
  echo "Added $runtime_user to the alldbackup group. Log out and back in before using alldbackup without sudo."
fi
