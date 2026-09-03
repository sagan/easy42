#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

CONF_FILE="/etc/sysctl.d/99-easy42.conf"

FWD="$(cat /proc/sys/net/ipv4/ip_forward 2>/dev/null || echo "0")"
RP_ALL="$(cat /proc/sys/net/ipv4/conf/all/rp_filter 2>/dev/null || echo "1")"
RP_DEF="$(cat /proc/sys/net/ipv4/conf/default/rp_filter 2>/dev/null || echo "1")"

if [ -f "$CONF_FILE" ] && [ "$FWD" = "1" ] && [ "$RP_ALL" = "0" ] && [ "$RP_DEF" = "0" ]; then
    echo "Sysctl parameters are already configured and active in $CONF_FILE"
    exit 10
fi

echo "Sysctl parameters need configuration (ip_forward=$FWD, rp_filter all=$RP_ALL def=$RP_DEF)"
exit 0
