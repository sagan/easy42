#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

CONF_FILE=""
for f in /etc/nftables.conf /etc/nftables.nft /etc/nftables/nftables.conf /etc/sysconfig/nftables.conf; do
    if [ -f "$f" ]; then
        CONF_FILE="$f"
        break
    fi
done

if [ -z "$CONF_FILE" ]; then
    if [ "$OS_ID" = "alpine" ]; then
        CONF_FILE="/etc/nftables.nft"
    else
        CONF_FILE="/etc/nftables.conf"
    fi
    echo "Creating $CONF_FILE..."
    $SUDO mkdir -p "$(dirname "$CONF_FILE")"
    printf "#!/usr/sbin/nft -f\nflush ruleset\n" | $SUDO tee "$CONF_FILE" >/dev/null
    $SUDO chmod 644 "$CONF_FILE"
fi

# Ensure /etc/easy42.nft exists as placeholder so syntax check passes
if [ ! -f /etc/easy42.nft ]; then
    echo "Creating placeholder /etc/easy42.nft..."
    printf "#!/usr/sbin/nft -f\n# Placeholder created by easy42 Device Helper\n" | $SUDO tee /etc/easy42.nft >/dev/null
    $SUDO chmod 755 /etc/easy42.nft
fi

if grep -q "easy42.nft" "$CONF_FILE"; then
    echo "Nftables config ($CONF_FILE) already includes /etc/easy42.nft"
else
    echo "Backing up $CONF_FILE..."
    backup_file "$CONF_FILE"

    echo "Adding include to $CONF_FILE..."
    printf "\n# Added by easy42 Device Helper\ninclude \"/etc/easy42.nft\"\n" | $SUDO tee -a "$CONF_FILE" >/dev/null
fi

# Verify syntax if nft is installed
if command -v nft >/dev/null 2>&1; then
    if ! $SUDO nft -c -f "$CONF_FILE" >/dev/null 2>&1; then
        echo "Warning: nftables syntax check failed. Please review $CONF_FILE"
    else
        echo "Nftables syntax check passed."
    fi
fi

# Enable and start/reload service if available
if is_service_installed nftables; then
    if ! is_service_enabled nftables; then
        echo "Enabling nftables service..."
        enable_service nftables || true
    fi
    start_service nftables || reload_service nftables || true
elif [ "$INIT_SYSTEM" = "openrc" ] && [ -f /etc/init.d/nftables ]; then
    enable_service nftables || true
    start_service nftables || true
fi

# Make rules take effect now if nft is installed
if command -v nft >/dev/null 2>&1; then
    $SUDO nft -f "$CONF_FILE" >/dev/null 2>&1 || true
fi

echo "Successfully configured nftables include in $CONF_FILE"
exit 0
