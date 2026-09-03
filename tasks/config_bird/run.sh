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
    # Create default /etc/bird/bird.conf
    $SUDO mkdir -p /etc/bird
    CONF_FILE="/etc/bird/bird.conf"
    echo "# BIRD configuration" | $SUDO tee "$CONF_FILE" >/dev/null
fi

if grep -q "bird_easy42.conf" "$CONF_FILE"; then
    echo "BIRD config ($CONF_FILE) already includes /etc/bird_easy42.conf"
    exit 0
fi

echo "Backing up $CONF_FILE..."
backup_file "$CONF_FILE"

# Ensure /etc/bird_easy42.conf exists as empty placeholder so syntax check passes
if [ ! -f /etc/bird_easy42.conf ]; then
    echo "# Placeholder created by easy42 Device Helper" | $SUDO tee /etc/bird_easy42.conf >/dev/null
    $SUDO chmod 644 /etc/bird_easy42.conf
fi

echo "Adding include to $CONF_FILE..."
printf "\n# Added by easy42 Device Helper\ninclude \"/etc/bird_easy42.conf\";\n" | $SUDO tee -a "$CONF_FILE" >/dev/null

# Verify syntax if bird is installed
if command -v bird >/dev/null 2>&1; then
    if ! $SUDO bird -p -c "$CONF_FILE" >/dev/null 2>&1; then
        echo "Warning: BIRD syntax check failed. Please review $CONF_FILE"
    else
        echo "BIRD syntax check passed."
    fi
fi

# Reload bird configuration if running
if command -v birdc >/dev/null 2>&1; then
    $SUDO birdc configure >/dev/null 2>&1 || true
elif [ "$INIT_SYSTEM" != "unknown" ]; then
    reload_service bird >/dev/null 2>&1 || reload_service bird2 >/dev/null 2>&1 || true
fi

echo "Successfully configured BIRD include in $CONF_FILE"
exit 0
