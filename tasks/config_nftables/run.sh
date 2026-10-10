#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

# Ensure /etc/easy42.nft exists as placeholder so syntax check passes
if [ ! -f /etc/easy42.nft ]; then
    echo "Creating placeholder /etc/easy42.nft..."
    printf "#!/usr/sbin/nft -f\n# Placeholder created by easy42 Device Helper\n" | $SUDO tee /etc/easy42.nft >/dev/null
    $SUDO chmod 755 /etc/easy42.nft
fi

if is_service_installed nftables; then
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
        printf "#!/usr/sbin/nft -f\n#flush ruleset\n" | $SUDO tee "$CONF_FILE" >/dev/null
        $SUDO chmod 755 "$CONF_FILE"
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

    # Enable and start/reload nftables service
    if ! is_service_enabled nftables; then
        echo "Enabling nftables service..."
        enable_service nftables || true
    fi
    start_service nftables || reload_service nftables || true

    # Make rules take effect now if nft is installed
    if command -v nft >/dev/null 2>&1; then
        $SUDO nft -f "$CONF_FILE" >/dev/null 2>&1 || true
    fi

    echo "Successfully configured nftables include in $CONF_FILE"
    exit 0
else
    # Systems without nftables service (e.g. OpenWrt with fw4)
    case "$INIT_SYSTEM" in
        systemd|openrc|procd)
            ;;
        *)
            echo "Error: nftables service not found and unsupported init system '$INIT_SYSTEM' (supported: systemd, openrc, procd)" >&2
            exit 1
            ;;
    esac

    SERVICE_NAME="easy42-nftables"
    SERVICE_FILE="$(get_service_file_path "$SERVICE_NAME")"

    echo "nftables service not found. Registering $SERVICE_NAME service ($INIT_SYSTEM)..."

    case "$INIT_SYSTEM" in
        systemd)
            cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
[Unit]
Description=easy42 nftables ruleset loader
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'nft -f /etc/easy42.nft'
ExecReload=/bin/sh -c 'nft -f /etc/easy42.nft'

[Install]
WantedBy=multi-user.target
EOF
            $SUDO chmod 644 "$SERVICE_FILE"
            ;;

        openrc)
            $SUDO mkdir -p /etc/init.d
            cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
#!/sbin/openrc-run

description="easy42 nftables ruleset loader"

depend() {
    need net
    after firewall
}

start() {
    ebegin "Loading easy42 nftables rules"
    nft -f /etc/easy42.nft
    eend $?
}

reload() {
    ebegin "Reloading easy42 nftables rules"
    nft -f /etc/easy42.nft
    eend $?
}
EOF
            $SUDO chmod 755 "$SERVICE_FILE"
            ;;

        procd)
            $SUDO mkdir -p /etc/init.d
            cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
#!/bin/sh /etc/rc.common

START=95

EXTRA_COMMANDS="reload"
EXTRA_HELP="        reload  Reload easy42 nftables rules"

start() {
    nft -f /etc/easy42.nft
}

reload() {
    start
}

stop() {
    return 0
}
EOF
            $SUDO chmod 755 "$SERVICE_FILE"
            ;;
    esac

    # Verify syntax if nft is installed
    if command -v nft >/dev/null 2>&1; then
        if ! $SUDO nft -c -f /etc/easy42.nft >/dev/null 2>&1; then
            echo "Warning: nftables syntax check failed for /etc/easy42.nft"
        else
            echo "Nftables syntax check passed."
        fi
    fi

    # Enable and start service
    if ! is_service_enabled "$SERVICE_NAME"; then
        echo "Enabling $SERVICE_NAME service..."
        enable_service "$SERVICE_NAME" || true
    fi
    echo "Starting $SERVICE_NAME service..."
    start_service "$SERVICE_NAME" || reload_service "$SERVICE_NAME" || true

    # Make rules take effect now if nft is installed
    if command -v nft >/dev/null 2>&1; then
        $SUDO nft -f /etc/easy42.nft >/dev/null 2>&1 || true
    fi

    echo "Successfully registered and enabled $SERVICE_NAME service ($INIT_SYSTEM)"
    exit 0
fi
