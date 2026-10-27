#!/usr/bin/env bash
set -e

OUTPUT_DIR="${1:-.}"
mkdir -p "$OUTPUT_DIR"

VERSION="0.1.0"
if [ -f "m3tal-mesh/plugin.yaml" ]; then
    VERSION=$(python3 -c "
import yaml
try:
    with open('m3tal-mesh/plugin.yaml') as f:
        data = yaml.safe_load(f)
        print(data.get('metadata', {}).get('version', '0.1.0'))
except Exception:
    print('0.1.0')
" 2>/dev/null || echo "0.1.0")
fi

CLEAN_VERSION=$(echo "$VERSION" | sed 's/^v//')

NODE_VERSION=$(python3 -c "
import yaml
with open('m3tal-mesh-node/plugin.yaml') as f:
    print(yaml.safe_load(f).get('metadata', {}).get('version', ''))
")
if [ -z "$NODE_VERSION" ]; then
    echo "Error: unable to read m3tal-mesh-node version from plugin.yaml" >&2
    exit 1
fi
if ! dpkg --compare-versions "${NODE_VERSION#v}" ge "$CLEAN_VERSION"; then
    echo "Error: m3tal-mesh-node version $NODE_VERSION is older than cluster version $CLEAN_VERSION" >&2
    exit 1
fi
PKG_NAME="m3tal-plugin-mesh-cluster"
DEB_NAME="$OUTPUT_DIR/${PKG_NAME}_${CLEAN_VERSION}_amd64.deb"

echo "📦 Packaging $PKG_NAME v${CLEAN_VERSION} (Metapackage)..."

STAGE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/m3tal-cluster-deb".XXXXXX)
trap 'rm -rf "$STAGE_DIR"' EXIT

mkdir -p "$STAGE_DIR/DEBIAN"
mkdir -p "$STAGE_DIR/usr/share/doc/$PKG_NAME"

cat <<EOF > "$STAGE_DIR/DEBIAN/control"
Package: $PKG_NAME
Version: $CLEAN_VERSION
Section: utils
Priority: optional
Architecture: amd64
Depends: m3tal-plugin-mesh (>= $CLEAN_VERSION), m3tal-plugin-mesh-node (>= $CLEAN_VERSION)
Maintainer: jakej985-rgb <jakej985@gmail.com>
Description: M3TAL Mesh Cluster Metapackage (Full Peer: Send + Receive)
 Personal Cloud & Fleet Federation Metapackage.
 Installs both the m3tal-mesh reader dashboard and the m3tal-mesh-node
 headless telemetry broadcaster, making this machine a full peer node.
EOF

cat <<EOF > "$STAGE_DIR/usr/share/doc/$PKG_NAME/copyright"
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: $PKG_NAME
Source: https://github.com/jakej985-rgb/m3tal-plugins

Files: *
Copyright: 2026 jakej985-rgb <jakej985@gmail.com>
License: MIT
EOF

find "$STAGE_DIR" -type d -exec chmod 755 {} +
find "$STAGE_DIR" -type f -exec chmod 644 {} +
dpkg-deb --build --root-owner-group "$STAGE_DIR" "$DEB_NAME"
sha256sum "$DEB_NAME" > "${DEB_NAME}.sha256"
echo "✅ Successfully built: $DEB_NAME"
