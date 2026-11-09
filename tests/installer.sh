#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/present-bin" "$work/fixture"

cat > "$work/bin/uname" <<'UNAME'
#!/usr/bin/env bash
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 1 ;;
esac
UNAME
chmod 0755 "$work/bin/uname"
asset="alldbackup_linux_amd64.tar.gz"
cat > "$work/fixture/alldbackup" <<'BIN'
#!/bin/sh
case "$1 $2" in
  "cron install") touch "$CRON_MARKER" ;;
  "version") echo "alldbackup test" ;;
esac
BIN
chmod 0755 "$work/fixture/alldbackup"
tar -czf "$work/fixture/$asset" -C "$work/fixture" alldbackup
checksum="$(sha256sum "$work/fixture/$asset" | awk '{print $1}')"
printf '%s  %s\n' "$checksum" "$asset" > "$work/fixture/checksums.txt"
cat > "$work/fixture/rclone-install.sh" <<'RCLONE_INSTALLER'
#!/usr/bin/env bash
set -euo pipefail
touch "$RCLONE_MARKER"
cat > "$RCLONE_INSTALL_BIN/rclone" <<'RCLONE'
#!/bin/sh
echo "rclone test"
RCLONE
chmod 0755 "$RCLONE_INSTALL_BIN/rclone"
RCLONE_INSTALLER

cat > "$work/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while (($#)); do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
file="${url##*/}"
if [[ "$url" == "https://rclone.org/install.sh" ]]; then
  cp "${FIXTURE_DIR}/rclone-install.sh" "$output"
else
  cp "${FIXTURE_DIR}/${file}" "$output"
fi
if [[ "$file" == "checksums.txt" && "${INVALID_CHECKSUM:-0}" == "1" ]]; then
  sed -i 's/^[[:xdigit:]]*/0000000000000000000000000000000000000000000000000000000000000000/' "$output"
fi
CURL
chmod 0755 "$work/bin/curl"

export PATH="$work/bin:$PATH"
export FIXTURE_DIR="$work/fixture"
export RCLONE_MARKER="$work/rclone-installed"
export RCLONE_INSTALL_BIN="$work/bin"
export CRON_MARKER="$work/cron-installed"
export DESTDIR="$work/install"
bash "$root/scripts/install.sh" >/dev/null
installed="$DESTDIR/usr/local/bin/alldbackup"
config="$DESTDIR/etc/alldbackup/config.yaml"
[[ -x "$installed" ]]
[[ -f "$RCLONE_MARKER" && -x "$work/bin/rclone" ]]
[[ -f "$CRON_MARKER" ]]
[[ -f "$config" && "$(stat -c '%a' "$config")" == 644 ]]
[[ "$(stat -c '%a' "$DESTDIR/etc/alldbackup")" == 1775 ]]
[[ "$(stat -c '%a' "$DESTDIR/var/backups/alldbackup")" == 2775 ]]
[[ -f "$DESTDIR/etc/alldbackup/secrets.env" && "$(stat -c '%a' "$DESTDIR/etc/alldbackup/secrets.env")" == 640 ]]
[[ -f "$DESTDIR/etc/alldbackup/rclone.conf" && "$(stat -c '%a' "$DESTDIR/etc/alldbackup/rclone.conf")" == 600 ]]
grep -q 'retention_days: 7' "$config"
printf '# keep this custom setting\n' >> "$config"
old_backup="$DESTDIR/var/backups/alldbackup/old-job"
mkdir -p "$old_backup" "$DESTDIR/var/backups/alldbackup/.locks"
printf 'archive' > "$old_backup/20261109T020000.123456789Z.zip"
printf 'checksum' > "$old_backup/20261109T020000.123456789Z.zip.sha256"
printf 'lock' > "$DESTDIR/var/backups/alldbackup/.locks/old-job.lock"
chmod 0755 "$old_backup"
chmod 0600 "$old_backup"/*.zip "$DESTDIR/var/backups/alldbackup/.locks/old-job.lock"
before="$(sha256sum "$installed")"
if INVALID_CHECKSUM=1 bash "$root/scripts/install.sh" >/dev/null 2>&1; then
  echo "installer accepted an invalid checksum" >&2
  exit 1
fi
after="$(sha256sum "$installed")"
[[ "$before" == "$after" ]]

bash "$root/scripts/install.sh" >/dev/null
grep -q '# keep this custom setting' "$config"
[[ "$(stat -c '%a' "$old_backup")" == 2775 ]]
[[ "$(stat -c '%a' "$old_backup/20261109T020000.123456789Z.zip")" == 664 ]]
[[ "$(stat -c '%a' "$DESTDIR/var/backups/alldbackup/.locks")" == 2770 ]]
[[ "$(stat -c '%a' "$DESTDIR/var/backups/alldbackup/.locks/old-job.lock")" == 660 ]]

cat > "$work/present-bin/rclone" <<'RCLONE'
#!/bin/sh
echo "rclone already installed"
RCLONE
chmod 0755 "$work/present-bin/rclone"
rm -f "$RCLONE_MARKER"
PATH="$work/present-bin:$work/bin:$PATH" bash "$root/scripts/install.sh" >/dev/null
[[ ! -e "$RCLONE_MARKER" ]]
echo "installer checksum and staging checks passed"
