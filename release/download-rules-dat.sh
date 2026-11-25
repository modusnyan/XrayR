#!/usr/bin/env bash
#
# Download the latest geoip.dat / geosite.dat rule files.
#
# These files are intentionally NOT stored in this repository: they are ~14 MB,
# they change constantly, and a stale copy silently breaks routing rules. Every
# install fetches the newest release from Loyalsoldier/v2ray-rules-dat and
# verifies the published sha256 checksum before the file is used.
#
# Usage:
#   bash release/download-rules-dat.sh [output_dir]
#
# Environment overrides:
#   RULES_DAT_REPO      default: Loyalsoldier/v2ray-rules-dat
#   RULES_DAT_BASE_URL  default: https://github.com/${RULES_DAT_REPO}/releases/latest/download
set -euo pipefail

REPO="${RULES_DAT_REPO:-Loyalsoldier/v2ray-rules-dat}"
BASE_URL="${RULES_DAT_BASE_URL:-https://github.com/${REPO}/releases/latest/download}"
OUTPUT_DIR="${1:-release/config}"

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

download() {
    local name="$1"
    local target="${OUTPUT_DIR}/${name}"
    local tmp="${target}.tmp"
    local checksum_file="${tmp}.sha256sum"

    echo "Downloading ${name} from ${REPO} (latest release)..."
    curl -fsSL --retry 3 --retry-delay 2 -o "${tmp}" "${BASE_URL}/${name}"

    if curl -fsSL --retry 3 --retry-delay 2 -o "${checksum_file}" "${BASE_URL}/${name}.sha256sum"; then
        local expected actual
        expected="$(awk '{print $1}' "${checksum_file}")"
        actual="$(sha256_of "${tmp}")"
        rm -f "${checksum_file}"
        if [ "${expected}" != "${actual}" ]; then
            echo "ERROR: ${name} checksum mismatch" >&2
            echo "  expected: ${expected}" >&2
            echo "  actual:   ${actual}" >&2
            rm -f "${tmp}"
            exit 1
        fi
        echo "  sha256 verified: ${actual}"
    else
        echo "WARNING: ${name}.sha256sum is unavailable; skipping checksum verification" >&2
    fi

    mv "${tmp}" "${target}"
}

mkdir -p "${OUTPUT_DIR}"
download geoip.dat
download geosite.dat
echo "Rule files written to ${OUTPUT_DIR}"
