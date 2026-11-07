#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/present-bin" "$work/fixture"

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported test architecture: $arch" >&2; exit 1 ;;
esac
cat > "$work/bin/uname" <<'UNAME'
#!/usr/bin/env bash
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 1 ;;
esac
UNAME
chmod 0755 "$work/bin/uname"
asset="alldbackup_linux_${arch}.tar.gz"
cat > "$work/fixture/alldbackup" <<'BIN'
#!/bin/sh
echo "alldbackup test"
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
export DESTDIR="$work/install"
bash "$root/scripts/install.sh" >/dev/null
installed="$DESTDIR/usr/local/bin/alldbackup"
[[ -x "$installed" ]]
[[ -f "$RCLONE_MARKER" && -x "$work/bin/rclone" ]]
before="$(sha256sum "$installed")"
if INVALID_CHECKSUM=1 bash "$root/scripts/install.sh" >/dev/null 2>&1; then
  echo "installer accepted an invalid checksum" >&2
  exit 1
fi
after="$(sha256sum "$installed")"
[[ "$before" == "$after" ]]

cat > "$work/present-bin/rclone" <<'RCLONE'
#!/bin/sh
echo "rclone already installed"
RCLONE
chmod 0755 "$work/present-bin/rclone"
rm -f "$RCLONE_MARKER"
PATH="$work/present-bin:$work/bin:$PATH" bash "$root/scripts/install.sh" >/dev/null
[[ ! -e "$RCLONE_MARKER" ]]
echo "installer checksum checks passed"
