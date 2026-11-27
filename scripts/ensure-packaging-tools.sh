#!/usr/bin/env bash
# Install the Debian packaging tools a build step needs, without letting an
# unrelated apt source abort it.
#
# `apt-get update` refreshes every configured source, and the runner image ships
# third-party lists (packages.microsoft.com among them) that answer 403 often
# enough to have failed a real publish. The tools themselves come from the
# Ubuntu archive, so this script:
#   1. does nothing when every requested package is already installed, and
#   2. otherwise refreshes only the Ubuntu archive, never `sources.list.d`.
# A refresh failure falls back to the image's cached lists, and every package is
# verified at the end so that fallback cannot hide a genuinely missing tool.
#
# Usage: scripts/ensure-packaging-tools.sh <package> [package...]
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <package> [package...]" >&2
  exit 2
fi

packages=("$@")

is_installed() {
  dpkg-query -W -f='${Status}' "$1" 2>/dev/null | grep -q 'install ok installed'
}

missing=()
for package in "${packages[@]}"; do
  is_installed "$package" || missing+=("$package")
done

if [ "${#missing[@]}" -eq 0 ]; then
  echo "Packaging tools already installed (${packages[*]}); skipping apt."
  exit 0
fi

echo "Missing packaging tools: ${missing[*]}"

# deb822 sources on 24.04+, one-line sources.list before that.
sources=/etc/apt/sources.list.d/ubuntu.sources
[ -f "$sources" ] || sources=/etc/apt/sources.list
echo "Refreshing only ${sources} (third-party apt sources are not consulted)."
if ! sudo apt-get update -qq -o Dir::Etc::sourceparts='-' -o Dir::Etc::sourcelist="$sources"; then
  echo "apt-get update failed for ${sources}; falling back to the image's cached package lists."
fi

sudo apt-get install -y --no-install-recommends "${missing[@]}"

# The skip above must never be able to hide a tool that is genuinely absent.
for package in "${packages[@]}"; do
  if ! is_installed "$package"; then
    echo "${package} is still not installed after apt-get install." >&2
    exit 1
  fi
done
echo "Packaging tools ready (${packages[*]})."
