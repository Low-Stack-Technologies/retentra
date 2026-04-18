#!/usr/bin/env bash
set -euo pipefail

repo="Low-Stack-Technologies/retentra"
asset="retentra-linux-amd64"
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
binary_path="$install_dir/retentra"
download_url="https://github.com/$repo/releases/latest/download/$asset"

os="$(uname -s)"
arch="$(uname -m)"

if [[ "$os" != "Linux" ]]; then
  echo "retentra installer currently supports Linux only; detected $os" >&2
  exit 1
fi

case "$arch" in
  x86_64 | amd64)
    ;;
  *)
    echo "retentra installer currently supports amd64 only; detected $arch" >&2
    exit 1
    ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$install_dir"

echo "Downloading retentra from $download_url"
if command -v curl >/dev/null 2>&1; then
  if ! curl -fsSL "$download_url" -o "$tmpdir/retentra"; then
    cat >&2 <<EOF
Failed to download retentra.

Expected release asset:
  $download_url

Publish a GitHub Release for $repo and wait for the release workflow to attach
the $asset asset. If the release already exists, rerun the release workflow for
that tag.
EOF
    exit 1
  fi
elif command -v wget >/dev/null 2>&1; then
  if ! wget -qO "$tmpdir/retentra" "$download_url"; then
    cat >&2 <<EOF
Failed to download retentra.

Expected release asset:
  $download_url

Publish a GitHub Release for $repo and wait for the release workflow to attach
the $asset asset. If the release already exists, rerun the release workflow for
that tag.
EOF
    exit 1
  fi
else
  echo "curl or wget is required to install retentra" >&2
  exit 1
fi

chmod +x "$tmpdir/retentra"
mv "$tmpdir/retentra" "$binary_path"

echo "Installed retentra to $binary_path"
if ! command -v retentra >/dev/null 2>&1; then
  echo "Add $install_dir to your PATH to run retentra from anywhere."
fi
