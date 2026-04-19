#!/usr/bin/env bash
set -euo pipefail

repo="Low-Stack-Technologies/retentra"
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
binary_path="$install_dir/retentra"
api_url="https://api.github.com/repos/$repo/releases/latest"
releases_api_url="https://api.github.com/repos/$repo/releases"

os="$(uname -s)"
arch="$(uname -m)"

if [[ "$os" != "Linux" ]]; then
  echo "retentra installer currently supports Linux only; detected $os" >&2
  exit 1
fi

case "$arch" in
  x86_64 | amd64)
    asset="retentra-linux-amd64"
    ;;
  aarch64 | arm64)
    asset="retentra-linux-arm64"
    ;;
  *)
    echo "retentra installer currently supports amd64 and arm64 only; detected $arch" >&2
    exit 1
    ;;
esac
checksum_asset="$asset.sha256"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$install_dir"

fetch_url() {
  local url="$1"
  local output="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
  else
    echo "curl or wget is required to install retentra" >&2
    exit 1
  fi
}

release_asset_url() {
  local asset_name="$1"
  local release_json="$2"

  awk -v asset="$asset_name" '
    /"name":[[:space:]]*"/ {
      in_asset = ($0 ~ "\"name\":[[:space:]]*\"" asset "\"")
    }
    in_asset && /"browser_download_url":[[:space:]]*"/ {
      url = $0
      sub(/^.*"browser_download_url":[[:space:]]*"/, "", url)
      sub(/".*$/, "", url)
      print url
      exit
    }
  ' "$release_json"
}

verify_checksum() {
  local checksum_file="$1"
  local binary_file="$2"

  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum is required to verify retentra release integrity" >&2
    exit 1
  fi

  local expected
  expected="$(awk 'NF >= 1 { print $1; exit }' "$checksum_file")"
  if [[ ! "$expected" =~ ^[0-9a-fA-F]{64}$ ]]; then
    echo "Invalid SHA-256 checksum file for $asset" >&2
    exit 1
  fi

  printf '%s  %s\n' "$expected" "$binary_file" | sha256sum -c -
}

echo "Resolving latest retentra release from $api_url"
if ! fetch_url "$api_url" "$tmpdir/release.json"; then
  echo "GitHub latest release endpoint was unavailable; checking published releases list." >&2
  if ! fetch_url "$releases_api_url" "$tmpdir/release.json"; then
    cat >&2 <<EOF
Failed to fetch release metadata.

Expected API endpoints:
  $api_url
  $releases_api_url

Make sure $repo has a published, non-draft GitHub Release.
EOF
    exit 1
  fi
fi

download_url="$(release_asset_url "$asset" "$tmpdir/release.json")"
checksum_url="$(release_asset_url "$checksum_asset" "$tmpdir/release.json")"

if [[ -z "$download_url" ]]; then
  cat >&2 <<EOF
Failed to find release asset.

Expected asset name:
  $asset

Latest release API endpoint:
  $api_url

Releases list API endpoint:
  $releases_api_url

At least one published, non-draft release must include an asset named $asset.
EOF
  exit 1
fi

if [[ -z "$checksum_url" ]]; then
  cat >&2 <<EOF
Failed to find release checksum asset.

Expected checksum asset name:
  $checksum_asset

Latest release API endpoint:
  $api_url

Releases list API endpoint:
  $releases_api_url

Every published release asset must include a matching .sha256 file.
EOF
  exit 1
fi

echo "Downloading retentra from $download_url"
if ! fetch_url "$download_url" "$tmpdir/$asset"; then
  cat >&2 <<EOF
Failed to download retentra.

Resolved release asset:
  $download_url

Latest release API endpoint:
  $api_url
EOF
  exit 1
fi

echo "Downloading checksum from $checksum_url"
if ! fetch_url "$checksum_url" "$tmpdir/$checksum_asset"; then
  cat >&2 <<EOF
Failed to download retentra checksum.

Resolved checksum asset:
  $checksum_url

Latest release API endpoint:
  $api_url
EOF
  exit 1
fi

echo "Verifying SHA-256 checksum"
verify_checksum "$tmpdir/$checksum_asset" "$tmpdir/$asset"

chmod +x "$tmpdir/$asset"
mv "$tmpdir/$asset" "$binary_path"

echo "Installed retentra to $binary_path"
if ! command -v retentra >/dev/null 2>&1; then
  echo "Add $install_dir to your PATH to run retentra from anywhere."
fi
