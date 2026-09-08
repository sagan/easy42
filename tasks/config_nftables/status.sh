#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

if ! command -v nft >/dev/null 2>&1; then
    echo "Not applicable: nft command not found. Is nftables installed?"
    exit 20
fi

CONF_FILE=""
for f in /etc/nftables.conf /etc/nftables.nft /etc/nftables/nftables.conf /etc/sysconfig/nftables.conf; do
    if [ -f "$f" ]; then
        CONF_FILE="$f"
        break
    fi
done

if [ -n "$CONF_FILE" ] && grep -q "easy42.nft" "$CONF_FILE"; then
    if is_service_installed nftables && ! is_service_enabled nftables; then
        echo "Nftables config ($CONF_FILE) includes /etc/easy42.nft, but nftables service is not enabled"
        exit 0
    fi
    echo "Nftables config ($CONF_FILE) already includes /etc/easy42.nft"
    exit 10
fi

if [ -n "$CONF_FILE" ]; then
    echo "Nftables config ($CONF_FILE) found, ready to add include directive"
else
    echo "Ready to initialize nftables configuration with /etc/easy42.nft include"
fi
exit 0
