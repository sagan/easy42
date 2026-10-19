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
# Hardcode whitelist to support only "lo" and "dummy*" interfaces
case "$NODE_INTERFACE" in
    lo|dummy*)
        ;;
    *)
        echo "Not Supported: interface '${NODE_INTERFACE:-<empty>}' is not supported (only 'lo' and 'dummy*' are permitted)"
        exit 20
        ;;
esac

# 2. Extract and clean IP addresses (strip CIDR mask like /32 or /128)
CLEAN_IP="${NODE_IP%/*}"
CLEAN_IP6="${NODE_IP6%/*}"
CLEAN_EXT_IP="${NODE_EXTERNAL_IP%/*}"
CLEAN_EXT_IP6="${NODE_EXTERNAL_IP6%/*}"

if [ -z "$CLEAN_IP" ] && [ -z "$CLEAN_IP6" ] && [ -z "$CLEAN_EXT_IP" ] && [ -z "$CLEAN_EXT_IP6" ]; then
    echo "Not Supported: no IP or IPv6 addresses configured for node"
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

# 5. Return status
if [ "$ALL_PRESENT" -eq 1 ]; then
    echo "Already Configured: all desired IP addresses ($DESIRED_IPS) already configured on the node"
    exit 10
else
    echo "Ready: interface '$NODE_INTERFACE' needs configuration (missing: $MISSING_IPS)"
    exit 0
fi
