#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

CONF_FILE=""
for f in /etc/bird/bird.conf /etc/bird.conf /etc/bird2/bird.conf; do
    if [ -f "$f" ]; then
        CONF_FILE="$f"
        break
    fi
done

if [ -z "$CONF_FILE" ]; then
    echo "Not applicable: Main BIRD config file not found in /etc/bird/ or /etc/. Is BIRD installed?"
    exit 20
fi

if grep -q "bird_easy42.conf" "$CONF_FILE"; then
    echo "BIRD config ($CONF_FILE) already includes /etc/bird_easy42.conf"
    exit 10
fi

echo "BIRD config ($CONF_FILE) found, ready to add include directive"
exit 0
