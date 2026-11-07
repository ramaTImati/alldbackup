#!/usr/bin/env bash
set -euo pipefail

repo="ramaTImati/alldbackup"
install_path="${DESTDIR:-}/usr/local/bin/alldbackup"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "alldbackup installer supports Linux only" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

if ! command -v rclone >/dev/null 2>&1; then
  echo "Rclone is missing; installing it with the official installer..." >&2
  curl --proto '=https' --tlsv1.2 -fsSL https://rclone.org/install.sh -o "${workdir}/rclone-install.sh"
  bash "${workdir}/rclone-install.sh"
  if ! command -v rclone >/dev/null 2>&1; then
    echo "Rclone installer completed, but rclone is still not available on PATH" >&2
    exit 1
  fi
fi

archive="alldbackup_linux_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/latest/download"

curl -fsSL "${base_url}/${archive}" -o "${workdir}/${archive}"
curl -fsSL "${base_url}/checksums.txt" -o "${workdir}/checksums.txt"
checksum="$(awk -v name="${archive}" '$2 == name {print $1}' "${workdir}/checksums.txt")"
if [[ ! "${checksum}" =~ ^[[:xdigit:]]{64}$ ]]; then
  echo "No valid checksum found for ${archive}" >&2
  exit 1
fi
printf '%s  %s\n' "${checksum}" "${archive}" > "${workdir}/expected.sha256"
(cd "${workdir}" && sha256sum --check --status expected.sha256) || {
  echo "Checksum verification failed" >&2
  exit 1
}

tar -xzf "${workdir}/${archive}" -C "${workdir}" alldbackup
install -d "$(dirname "${install_path}")"
install -m 0755 "${workdir}/alldbackup" "${install_path}"
"${install_path}" version
