#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_SCRIPT="$SCRIPT_DIR/audit-plugin.py"

if [ -z "$1" ]; then
    echo "Usage: $0 <plugin-name-or-path> [--json|--markdown|--strict]"
    exit 1
fi

python3 "$PYTHON_SCRIPT" "$@"
