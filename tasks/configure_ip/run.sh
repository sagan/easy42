#!/bin/sh
set -e
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
# Hardcode whitelist to support only "lo" and "dummy*" interfaces to prevent mis-configuration
case "$NODE_INTERFACE" in
    lo|dummy*)
        ;;
    *)
        echo "Error: interface '${NODE_INTERFACE:-<empty>}' is not supported (only 'lo' and 'dummy*' are permitted)" >&2
        exit 20
        ;;
esac

# 2. Extract and clean IP addresses
CLEAN_IP="${NODE_IP%/*}"
CLEAN_IP6="${NODE_IP6%/*}"
CLEAN_EXT_IP="${NODE_EXTERNAL_IP%/*}"
CLEAN_EXT_IP6="${NODE_EXTERNAL_IP6%/*}"

if [ -z "$CLEAN_IP" ] && [ -z "$CLEAN_IP6" ] && [ -z "$CLEAN_EXT_IP" ] && [ -z "$CLEAN_EXT_IP6" ]; then
    echo "Error: no IP or IPv6 addresses configured for node" >&2
    exit 20
fi

# 3. Thorough IP format validation
validate_ipv4() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    echo "$ip" | grep -E -q '^([0-9]{1,3}\.){3}[0-9]{1,3}$' || return 1
    local old_ifs="$IFS"
    IFS='.'
    set -- $ip
    IFS="$old_ifs"
    for octet in "$@"; do
        if [ "$octet" -lt 0 ] 2>/dev/null || [ "$octet" -gt 255 ] 2>/dev/null; then
            return 1
        fi
    done
    return 0
}

validate_ipv6() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    # Must contain ':' and only valid hex/colons
    echo "$ip" | grep -E -i -q '^[0-9a-f:]+$' && echo "$ip" | grep -q ':'
}

for v4 in "$CLEAN_IP" "$CLEAN_EXT_IP"; do
    if [ -n "$v4" ] && ! validate_ipv4 "$v4"; then
        echo "Error: invalid IPv4 address format: '$v4'" >&2
        exit 1
    fi
done

for v6 in "$CLEAN_IP6" "$CLEAN_EXT_IP6"; do
    if [ -n "$v6" ] && ! validate_ipv6 "$v6"; then
        echo "Error: invalid IPv6 address format: '$v6'" >&2
        exit 1
    fi
done

# 4. Build list of desired IPv4 and IPv6 addresses
IPV4_LIST=""
add_ipv4() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    for existing in $IPV4_LIST; do
        if [ "$existing" = "$ip" ]; then
            return 0
        fi
    done
    if [ -z "$IPV4_LIST" ]; then
        IPV4_LIST="$ip"
    else
        IPV4_LIST="$IPV4_LIST $ip"
    fi
}

IPV6_LIST=""
add_ipv6() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    for existing in $IPV6_LIST; do
        if [ "$existing" = "$ip" ]; then
            return 0
        fi
    done
    if [ -z "$IPV6_LIST" ]; then
        IPV6_LIST="$ip"
    else
        IPV6_LIST="$IPV6_LIST $ip"
    fi
}

[ -n "$CLEAN_IP" ] && add_ipv4 "$CLEAN_IP"
if [ -n "$CLEAN_EXT_IP" ] && [ "$CLEAN_EXT_IP" != "$CLEAN_IP" ]; then
    add_ipv4 "$CLEAN_EXT_IP"
fi

[ -n "$CLEAN_IP6" ] && add_ipv6 "$CLEAN_IP6"
if [ -n "$CLEAN_EXT_IP6" ] && [ "$CLEAN_EXT_IP6" != "$CLEAN_IP6" ]; then
    add_ipv6 "$CLEAN_EXT_IP6"
fi

ALL_DESIRED_IPS="$IPV4_LIST $IPV6_LIST"

# 5. Check if all desired IPs are already configured on the node
ip_exists_on_host() {
    local ip="$1"
    [ -z "$ip" ] && return 0
    if ip -o addr show to "$ip" 2>/dev/null | grep -q .; then
        return 0
    fi
    if ip addr show 2>/dev/null | grep -E -q "inet6?[[:space:]]+$ip(/[0-9]+)?[[:space:]]"; then
        return 0
    fi
    return 1
}

ALL_PRESENT=1
for ip in $ALL_DESIRED_IPS; do
    if ! ip_exists_on_host "$ip"; then
        ALL_PRESENT=0
        break
    fi
done

if [ "$NODE_INTERFACE" = "lo" ]; then
    if ! ip addr show dev lo 2>/dev/null | grep -q "127\.0\.0\.1"; then
        ALL_PRESENT=0
    fi
fi

if [ "$ALL_PRESENT" -eq 1 ]; then
    echo "Already configured: all desired IP addresses ($ALL_DESIRED_IPS) already exist on the node."
    exit 0
fi

# 6. Ensure kernel module / support for dummy interface
case "$NODE_INTERFACE" in
    dummy*)
        $SUDO modprobe dummy 2>/dev/null || true
        # Proactively ensure dummy interface exists and is up
        if ! ip link show "$NODE_INTERFACE" >/dev/null 2>&1; then
            $SUDO ip link add "$NODE_INTERFACE" type dummy 2>/dev/null || true
        fi
        $SUDO ip link set "$NODE_INTERFACE" up 2>/dev/null || true
        ;;
    lo)
        $SUDO ip link set lo up 2>/dev/null || true
        ;;
esac

# 7. Detect native network manager
NET_MGR=""
if command -v netplan >/dev/null 2>&1 && [ -d /etc/netplan ]; then
    NET_MGR="netplan"
elif [ -f /etc/network/interfaces ] || [ "$OS_ID" = "alpine" ]; then
    NET_MGR="interfaces"
elif [ -d /etc/systemd/network ] && (is_service_running systemd-networkd || is_service_enabled systemd-networkd); then
    NET_MGR="networkd"
elif [ -f /etc/config/network ] && command -v uci >/dev/null 2>&1; then
    NET_MGR="openwrt"
else
    echo "Error: No supported native network manager found (checked: netplan, /etc/network/interfaces, systemd-networkd, openwrt)" >&2
    exit 1
fi

echo "Applying permanent IP configuration for '$NODE_INTERFACE' via $NET_MGR..."

# 8. Configure based on native network manager
case "$NET_MGR" in
    netplan)
        NETPLAN_FILE="/etc/netplan/99-easy42.yaml"
        backup_file "$NETPLAN_FILE"

        TMP_NETPLAN="/tmp/99-easy42.yaml.$$"
        {
            echo "network:"
            echo "  version: 2"
            echo "  renderer: networkd"
            if [ "$NODE_INTERFACE" = "lo" ]; then
                echo "  ethernets:"
                echo "    lo:"
                echo "      addresses:"
                echo "        - 127.0.0.1/8"
                echo "        - ::1/128"
                for ip in $IPV4_LIST; do
                    echo "        - ${ip}/32"
                done
                for ip in $IPV6_LIST; do
                    echo "        - ${ip}/128"
                done
            else
                echo "  dummy-devices:"
                echo "    ${NODE_INTERFACE}:"
                echo "      addresses:"
                for ip in $IPV4_LIST; do
                    echo "        - ${ip}/32"
                done
                for ip in $IPV6_LIST; do
                    echo "        - ${ip}/128"
                done
            fi
        } > "$TMP_NETPLAN"

        $SUDO cp -f "$TMP_NETPLAN" "$NETPLAN_FILE"
        $SUDO chmod 600 "$NETPLAN_FILE"
        rm -f "$TMP_NETPLAN"

        # Validate syntax with netplan generate first
        if command -v netplan >/dev/null 2>&1; then
            if ! $SUDO netplan generate; then
                echo "Error: netplan generate failed syntax check for $NETPLAN_FILE" >&2
                exit 1
            fi
            echo "Applying Netplan configuration..."
            $SUDO netplan apply
        fi
        ;;

    interfaces)
        IFACES_FILE="/etc/network/interfaces"
        if [ ! -f "$IFACES_FILE" ]; then
            $SUDO mkdir -p /etc/network
            $SUDO touch "$IFACES_FILE"
        fi
        backup_file "$IFACES_FILE"

        TMP_BLOCK="/tmp/easy42_iface_block.$$"
        {
            echo "### EASY42_START"
            if [ "$NODE_INTERFACE" = "lo" ]; then
                echo "auto lo"
                echo "iface lo inet loopback"
                echo "    address 127.0.0.1"
                echo "    netmask 255.0.0.0"
                for ip in $IPV4_LIST; do
                    echo ""
                    echo "iface lo inet static"
                    echo "    address $ip"
                    echo "    netmask 255.255.255.255"
                done
                for ip in $IPV6_LIST; do
                    echo ""
                    echo "iface lo inet6 static"
                    echo "    address $ip"
                    echo "    netmask 128"
                done
            else
                echo "auto $NODE_INTERFACE"
                FIRST_V4=1
                for ip in $IPV4_LIST; do
                    echo "iface $NODE_INTERFACE inet static"
                    echo "    address $ip"
                    echo "    netmask 255.255.255.255"
                    if [ "$FIRST_V4" -eq 1 ]; then
                        echo "    pre-up ip link add $NODE_INTERFACE type dummy || true"
                        FIRST_V4=0
                    fi
                    echo ""
                done
                for ip in $IPV6_LIST; do
                    echo "iface $NODE_INTERFACE inet6 static"
                    echo "    address $ip"
                    echo "    netmask 128"
                    echo ""
                done
            fi
            echo "### EASY42_END"
        } > "$TMP_BLOCK"

        TMP_OUT="/tmp/interfaces.out.$$"
        if grep -q "### EASY42_START" "$IFACES_FILE" 2>/dev/null; then
            # Replace existing EASY42 block
            awk -v block_file="$TMP_BLOCK" '
                BEGIN {
                    while ((getline line < block_file) > 0) {
                        block = block (block == "" ? "" : "\n") line
                    }
                    close(block_file)
                }
                /### EASY42_START/ { in_block=1; print block; next }
                /### EASY42_END/ { in_block=0; next }
                !in_block { print }
            ' "$IFACES_FILE" > "$TMP_OUT"
        else
            # Append EASY42 block
            cp -f "$IFACES_FILE" "$TMP_OUT"
            printf "\n" >> "$TMP_OUT"
            cat "$TMP_BLOCK" >> "$TMP_OUT"
        fi

        rm -f "$TMP_BLOCK"
        $SUDO cp -f "$TMP_OUT" "$IFACES_FILE"
        rm -f "$TMP_OUT"

        # Apply networking changes
        if [ "$OS_ID" = "alpine" ] || (command -v rc-service >/dev/null 2>&1 && [ -f /etc/init.d/networking ]); then
            echo "Restarting networking via rc-service..."
            $SUDO rc-service networking restart || true
        else
            echo "Bringing up interface $NODE_INTERFACE..."
            $SUDO ifup "$NODE_INTERFACE" 2>/dev/null || true
            if is_service_installed networking; then
                restart_service networking || true
            fi
        fi

        # Ensure interface is up
        $SUDO ip link set "$NODE_INTERFACE" up 2>/dev/null || true
        ;;

    networkd)
        NETWORKD_DIR="/etc/systemd/network"
        $SUDO mkdir -p "$NETWORKD_DIR"
        if [ "$NODE_INTERFACE" != "lo" ]; then
            NETDEV_FILE="${NETWORKD_DIR}/99-easy42-${NODE_INTERFACE}.netdev"
            backup_file "$NETDEV_FILE"
            cat << EOF | $SUDO tee "$NETDEV_FILE" >/dev/null
[NetDev]
Name=${NODE_INTERFACE}
Kind=dummy
EOF
        fi

        NETWORK_FILE="${NETWORKD_DIR}/99-easy42-${NODE_INTERFACE}.network"
        backup_file "$NETWORK_FILE"
        {
            echo "[Match]"
            echo "Name=${NODE_INTERFACE}"
            echo ""
            echo "[Network]"
            if [ "$NODE_INTERFACE" = "lo" ]; then
                echo "Address=127.0.0.1/8"
                echo "Address=::1/128"
            fi
            for ip in $IPV4_LIST; do
                echo "Address=${ip}/32"
            done
            for ip in $IPV6_LIST; do
                echo "Address=${ip}/128"
            done
        } | $SUDO tee "$NETWORK_FILE" >/dev/null

        echo "Reloading systemd-networkd..."
        $SUDO networkctl reload 2>/dev/null || restart_service systemd-networkd || true
        $SUDO ip link set "$NODE_INTERFACE" up 2>/dev/null || true
        ;;

    openwrt)
        echo "Configuring OpenWrt interface $NODE_INTERFACE via uci..."
        $SUDO uci -q delete network.easy42_node || true
        $SUDO uci set network.easy42_node=interface
        $SUDO uci set network.easy42_node.device="$NODE_INTERFACE"
        $SUDO uci set network.easy42_node.proto='static'
        for ip in $IPV4_LIST; do
            $SUDO uci add_list network.easy42_node.ipaddr="${ip}/32"
        done
        for ip in $IPV6_LIST; do
            $SUDO uci add_list network.easy42_node.ip6addr="${ip}/128"
        done
        $SUDO uci commit network
        $SUDO /etc/init.d/network restart || true
        ;;
esac

# 9. Post-Apply Verification
# Thoroughly check that the desired IPs are bound to the node
FAILED_VERIFY=0
for ip in $ALL_DESIRED_IPS; do
    if ! ip_exists_on_host "$ip"; then
        FAILED_VERIFY=1
        echo "Verification error: IP address '$ip' is not bound to node after applying configuration" >&2
    fi
done

if [ "$NODE_INTERFACE" = "lo" ]; then
    if ! ip addr show dev lo 2>/dev/null | grep -q "127\.0\.0\.1"; then
        # Ensure 127.0.0.1/8 is added to lo
        $SUDO ip addr add 127.0.0.1/8 dev lo 2>/dev/null || true
        if ! ip addr show dev lo 2>/dev/null | grep -q "127\.0\.0\.1"; then
            FAILED_VERIFY=1
            echo "Verification error: loopback address 127.0.0.1/8 is not bound on lo" >&2
        fi
    fi
fi

if [ "$FAILED_VERIFY" -ne 0 ]; then
    echo "Error: Network configuration applied via $NET_MGR but verification checks failed" >&2
    exit 1
fi

echo "Successfully configured interface '$NODE_INTERFACE' with IP addresses ($ALL_DESIRED_IPS) via $NET_MGR"
exit 0
