#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi
if [ -f "$SCRIPT_DIR/node_ip.env" ]; then
    . "$SCRIPT_DIR/node_ip.env"
fi

# Ensure standard PATH
case ":$PATH:" in
    *:/usr/local/bin:*) ;;
    *) PATH="/usr/local/bin:$PATH" ;;
esac
case ":$PATH:" in
    *:/usr/local/sbin:*) ;;
    *) PATH="/usr/local/sbin:$PATH" ;;
esac
export PATH

# 1. Interface Whitelist Check
# Hardcode whitelist to support "lo", "dummy*", or standard interfaces
case "$NODE_INTERFACE" in
    lo|dummy*|eth*|ens*|enp*|eno*|wl*|wlan*)
        ;;
    *)
        echo "Not Supported: interface '${NODE_INTERFACE:-<empty>}' is not supported (permitted: 'lo', 'dummy*', 'eth*', etc.)"
        exit 20
        ;;
esac

# 2. Extract and clean IP addresses (strip CIDR mask like /32 or /128)
CLEAN_IP="${NODE_IP%/*}"
CLEAN_IP6="${NODE_IP6%/*}"
CLEAN_EXT_IP="${NODE_EXTERNAL_IP%/*}"
CLEAN_EXT_IP6="${NODE_EXTERNAL_IP6%/*}"

if [ -z "$CLEAN_IP" ] && [ -z "$CLEAN_IP6" ] && [ -z "$CLEAN_EXT_IP" ] && [ -z "$CLEAN_EXT_IP6" ] && [ "$HAS_CUSTOM_SCRIPT" != "1" ]; then
    echo "Not Supported: no IP addresses or custom network script configured for node"
    exit 20
fi

# 3. Collect desired unique IP addresses
DESIRED_IPS=""
add_desired_ip() {
    local target="$1"
    [ -z "$target" ] && return 0
    for existing in $DESIRED_IPS; do
        if [ "$existing" = "$target" ]; then
            return 0
        fi
    done
    if [ -z "$DESIRED_IPS" ]; then
        DESIRED_IPS="$target"
    else
        DESIRED_IPS="$DESIRED_IPS $target"
    fi
}

[ -n "$CLEAN_IP" ] && add_desired_ip "$CLEAN_IP"
[ -n "$CLEAN_IP6" ] && add_desired_ip "$CLEAN_IP6"
if [ -n "$CLEAN_EXT_IP" ] && [ "$CLEAN_EXT_IP" != "$CLEAN_IP" ]; then
    add_desired_ip "$CLEAN_EXT_IP"
fi
if [ -n "$CLEAN_EXT_IP6" ] && [ "$CLEAN_EXT_IP6" != "$CLEAN_IP6" ]; then
    add_desired_ip "$CLEAN_EXT_IP6"
fi

# 4. Helper to check if an IP exists on ANY interface on the node
ip_exists_on_host() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    if ip -o addr show to "$ip" 2>/dev/null | grep -q .; then
        return 0
    fi
    # Fallback pattern match across ip addr output
    if ip addr show 2>/dev/null | grep -E -q "inet6?[[:space:]]+$ip(/[0-9]+)?[[:space:]]"; then
        return 0
    fi
    return 1
}

ALL_PRESENT=1
MISSING_IPS=""
for ip in $DESIRED_IPS; do
    if ! ip_exists_on_host "$ip"; then
        ALL_PRESENT=0
        if [ -z "$MISSING_IPS" ]; then
            MISSING_IPS="$ip"
        else
            MISSING_IPS="$MISSING_IPS, $ip"
        fi
    fi
done

# If interface is 'lo', verify 127.0.0.1/8 is configured on lo
if [ "$NODE_INTERFACE" = "lo" ]; then
    if ! ip addr show dev lo 2>/dev/null | grep -q "127\.0\.0\.1"; then
        ALL_PRESENT=0
        if [ -z "$MISSING_IPS" ]; then
            MISSING_IPS="127.0.0.1/8 (on lo)"
        else
            MISSING_IPS="$MISSING_IPS, 127.0.0.1/8 (on lo)"
        fi
    fi
fi

# 5. Helper to resolve target interface for systemd networkd-dispatcher
get_target_iface() {
    local iface=""
    if [ -n "$NODE_INTERFACE" ] && [ "$NODE_INTERFACE" != "lo" ]; then
        iface="$NODE_INTERFACE"
    else
        iface=$(ip route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev") {print $(i+1); exit}}')
        if [ -z "$iface" ]; then
            iface=$(ip -6 route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev") {print $(i+1); exit}}')
        fi
        if [ -z "$iface" ]; then
            iface=$(ip -o link show 2>/dev/null | awk -F': ' '$2 != "lo" {print $2; exit}' | cut -d'@' -f1)
        fi
    fi
    echo "${iface:-eth0}"
}

# 6. Check custom network script configuration
CUSTOM_SCRIPT_CONFIGURED=1
CUSTOM_MISSING_REASONS=""

if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
    TARGET_IFACE="$(get_target_iface)"

    if has_systemd || [ -d /etc/systemd/system ] || (command -v systemctl >/dev/null 2>&1); then
        DISPATCHER_SCRIPT="/etc/networkd-dispatcher/routable.d/99-easy42.sh"
        OVERRIDE_FILE="/etc/systemd/system/networkd-dispatcher.service.d/90-easy42.conf"
        NETWORKD_OVERRIDE_FILE="/etc/systemd/system/systemd-networkd.service.d/90-easy42.conf"
        if [ ! -x "$DISPATCHER_SCRIPT" ]; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="missing $DISPATCHER_SCRIPT"
        elif [ ! -f "$OVERRIDE_FILE" ]; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="missing $OVERRIDE_FILE"
        elif [ ! -f "$NETWORKD_OVERRIDE_FILE" ]; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="missing $NETWORKD_OVERRIDE_FILE"
        elif ! grep -q "ManageForeignRoutingPolicyRules=no" "$NETWORKD_OVERRIDE_FILE" 2>/dev/null; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="foreign rule management not disabled in $NETWORKD_OVERRIDE_FILE"
        else
            CURRENT_CUSTOM=$(sed -n '/### EASY42_CUSTOM_START/,/### EASY42_CUSTOM_END/p' "$DISPATCHER_SCRIPT" 2>/dev/null | sed '1d;$d')
            DESIRED_CUSTOM=$(cat "$SCRIPT_DIR/custom_network.sh")
            if [ "$CURRENT_CUSTOM" != "$DESIRED_CUSTOM" ]; then
                CUSTOM_SCRIPT_CONFIGURED=0
                CUSTOM_MISSING_REASONS="custom script outdated in $DISPATCHER_SCRIPT"
            elif ! grep -q "if \[ \"\$IFACE\" != \"$TARGET_IFACE\" \];" "$DISPATCHER_SCRIPT" 2>/dev/null; then
                CUSTOM_SCRIPT_CONFIGURED=0
                CUSTOM_MISSING_REASONS="target interface mismatch in $DISPATCHER_SCRIPT (expected $TARGET_IFACE)"
            fi
        fi
    elif [ "$OS_ID" = "alpine" ] || [ -f /etc/network/interfaces ]; then
        EASY42_SH="/etc/network/easy42.sh"
        IFACES_FILE="/etc/network/interfaces"
        if [ ! -x "$EASY42_SH" ]; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="missing $EASY42_SH"
        elif ! grep -q "up /etc/network/easy42.sh" "$IFACES_FILE" 2>/dev/null; then
            CUSTOM_SCRIPT_CONFIGURED=0
            CUSTOM_MISSING_REASONS="missing up hook in $IFACES_FILE"
        else
            CURRENT_CUSTOM=$(sed -n '/### EASY42_CUSTOM_START/,/### EASY42_CUSTOM_END/p' "$EASY42_SH" 2>/dev/null | sed '1d;$d')
            DESIRED_CUSTOM=$(cat "$SCRIPT_DIR/custom_network.sh")
            if [ "$CURRENT_CUSTOM" != "$DESIRED_CUSTOM" ]; then
                CUSTOM_SCRIPT_CONFIGURED=0
                CUSTOM_MISSING_REASONS="custom script outdated in $EASY42_SH"
            fi
        fi
    fi
fi

# 7. Return status
if [ "$ALL_PRESENT" -eq 1 ] && [ "$CUSTOM_SCRIPT_CONFIGURED" -eq 1 ]; then
    if [ "$HAS_CUSTOM_SCRIPT" = "1" ]; then
        echo "Already Configured: all desired IP addresses ($DESIRED_IPS) and custom network script configured on the node"
    else
        echo "Already Configured: all desired IP addresses ($DESIRED_IPS) already configured on the node"
    fi
    exit 10
else
    REASONS=""
    if [ "$ALL_PRESENT" -ne 1 ]; then
        REASONS="missing IPs: $MISSING_IPS"
    fi
    if [ "$CUSTOM_SCRIPT_CONFIGURED" -ne 1 ]; then
        if [ -n "$REASONS" ]; then
            REASONS="$REASONS; $CUSTOM_MISSING_REASONS"
        else
            REASONS="$CUSTOM_MISSING_REASONS"
        fi
    fi
    echo "Ready: node network needs configuration ($REASONS)"
    exit 0
fi
