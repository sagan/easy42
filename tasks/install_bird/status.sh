#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

if command -v bird >/dev/null 2>&1 || command -v bird2 >/dev/null 2>&1; then
    BIRD_CMD="bird"
    command -v bird >/dev/null 2>&1 || BIRD_CMD="bird2"
    BIRD_VER="$($BIRD_CMD --version 2>&1 | head -n1)"
    echo "BIRD is already installed ($BIRD_VER)"
    exit 10
fi

echo "BIRD is not installed"
exit 0
