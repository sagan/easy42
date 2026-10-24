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
case "$NODE_INTERFACE" in
    lo|dummy*|eth*|ens*|enp*|eno*|wl*|wlan*)
        ;;
    *)
        echo "Error: interface '${NODE_INTERFACE:-<empty>}' is not supported (permitted: 'lo', 'dummy*', 'eth*', etc.)" >&2
        exit 20
        ;;
esac

# 2. Extract and clean IP addresses
CLEAN_IP="${NODE_IP%/*}"
CLEAN_IP6="${NODE_IP6%/*}"
CLEAN_EXT_IP="${NODE_EXTERNAL_IP%/*}"
CLEAN_EXT_IP6="${NODE_EXTERNAL_IP6%/*}"

if [ -z "$CLEAN_IP" ] && [ -z "$CLEAN_IP6" ] && [ -z "$CLEAN_EXT_IP" ] && [ -z "$CLEAN_EXT_IP6" ] && [ "$HAS_CUSTOM_SCRIPT" != "1" ]; then
    echo "Error: no IP addresses or custom network script configured for node" >&2
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

# Helper to resolve target interface for systemd networkd-dispatcher
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

# 5. Check if all desired IPs and custom scripts are already configured on the node
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

# Check if custom network script is already configured
CUSTOM_CONFIGURED=1
if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
    TARGET_IFACE="$(get_target_iface)"
    if has_systemd || [ -d /etc/systemd/system ] || (command -v systemctl >/dev/null 2>&1); then
        DISPATCHER_SCRIPT="/etc/networkd-dispatcher/routable.d/99-easy42.sh"
        OVERRIDE_FILE="/etc/systemd/system/networkd-dispatcher.service.d/90-easy42.conf"
        NETWORKD_OVERRIDE_FILE="/etc/systemd/system/systemd-networkd.service.d/90-easy42.conf"
        if [ ! -x "$DISPATCHER_SCRIPT" ] || [ ! -f "$OVERRIDE_FILE" ] || [ ! -f "$NETWORKD_OVERRIDE_FILE" ]; then
            CUSTOM_CONFIGURED=0
        elif ! grep -q "ManageForeignRoutingPolicyRules=no" "$NETWORKD_OVERRIDE_FILE" 2>/dev/null; then
            CUSTOM_CONFIGURED=0
        else
            CURRENT_CUSTOM=$(sed -n '/### EASY42_CUSTOM_START/,/### EASY42_CUSTOM_END/p' "$DISPATCHER_SCRIPT" 2>/dev/null | sed '1d;$d')
            DESIRED_CUSTOM=$(cat "$SCRIPT_DIR/custom_network.sh")
            if [ "$CURRENT_CUSTOM" != "$DESIRED_CUSTOM" ] || ! grep -q "if \[ \"\$IFACE\" != \"$TARGET_IFACE\" \];" "$DISPATCHER_SCRIPT" 2>/dev/null; then
                CUSTOM_CONFIGURED=0
            fi
        fi
    elif [ "$OS_ID" = "alpine" ] || [ -f /etc/network/interfaces ]; then
        EASY42_SH="/etc/network/easy42.sh"
        IFACES_FILE="/etc/network/interfaces"
        if [ ! -x "$EASY42_SH" ] || ! grep -q "up /etc/network/easy42.sh" "$IFACES_FILE" 2>/dev/null; then
            CUSTOM_CONFIGURED=0
        else
            CURRENT_CUSTOM=$(sed -n '/### EASY42_CUSTOM_START/,/### EASY42_CUSTOM_END/p' "$EASY42_SH" 2>/dev/null | sed '1d;$d')
            DESIRED_CUSTOM=$(cat "$SCRIPT_DIR/custom_network.sh")
            if [ "$CURRENT_CUSTOM" != "$DESIRED_CUSTOM" ]; then
                CUSTOM_CONFIGURED=0
            fi
        fi
    fi
fi

if [ "$ALL_PRESENT" -eq 1 ] && [ "$CUSTOM_CONFIGURED" -eq 1 ]; then
    echo "Already configured: all desired IP addresses and custom network rules are up to date."
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
elif has_systemd || command -v systemctl >/dev/null 2>&1; then
    NET_MGR="networkd"
else
    echo "Error: No supported native network manager found (checked: netplan, /etc/network/interfaces, systemd-networkd, openwrt)" >&2
    exit 1
fi

echo "Applying network configuration for '$NODE_INTERFACE' via $NET_MGR..."

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
            $SUDO netplan apply || true
        fi
        ;;

    interfaces)
        IFACES_FILE="/etc/network/interfaces"
        if [ ! -f "$IFACES_FILE" ]; then
            $SUDO mkdir -p /etc/network
            $SUDO touch "$IFACES_FILE"
        fi
        backup_file "$IFACES_FILE"

        # Write Alpine / interfaces custom script if configured
        if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
            $SUDO mkdir -p /etc/network
            TMP_SCRIPT="/tmp/easy42.sh.$$"
            {
                echo "#!/bin/sh"
                echo "# easy42 custom network script"
                echo "### EASY42_CUSTOM_START"
                cat "$SCRIPT_DIR/custom_network.sh"
                echo ""
                echo "### EASY42_CUSTOM_END"
            } > "$TMP_SCRIPT"
            $SUDO cp -f "$TMP_SCRIPT" /etc/network/easy42.sh
            $SUDO chmod 755 /etc/network/easy42.sh
            $SUDO chown root:root /etc/network/easy42.sh 2>/dev/null || true
            rm -f "$TMP_SCRIPT"
        else
            if [ -f /etc/network/easy42.sh ]; then
                $SUDO rm -f /etc/network/easy42.sh
            fi
        fi

        TMP_BLOCK="/tmp/easy42_iface_block.$$"
        {
            echo "### EASY42_START"
            if [ "$NODE_INTERFACE" = "lo" ]; then
                echo "auto lo"
                echo "iface lo inet loopback"
                echo "    address 127.0.0.1"
                echo "    netmask 255.0.0.0"
                if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
                    echo "    up /etc/network/easy42.sh"
                fi
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
                FIRST_IFACE=1
                for ip in $IPV4_LIST; do
                    echo "iface $NODE_INTERFACE inet static"
                    echo "    address $ip"
                    echo "    netmask 255.255.255.255"
                    if [ "$FIRST_IFACE" -eq 1 ]; then
                        echo "    pre-up ip link add $NODE_INTERFACE type dummy || true"
                        if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
                            echo "    up /etc/network/easy42.sh"
                        fi
                        FIRST_IFACE=0
                    fi
                    echo ""
                done
                for ip in $IPV6_LIST; do
                    echo "iface $NODE_INTERFACE inet6 static"
                    echo "    address $ip"
                    echo "    netmask 128"
                    if [ "$FIRST_IFACE" -eq 1 ]; then
                        if [ "$NODE_INTERFACE" != "lo" ]; then
                            echo "    pre-up ip link add $NODE_INTERFACE type dummy || true"
                        fi
                        if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
                            echo "    up /etc/network/easy42.sh"
                        fi
                        FIRST_IFACE=0
                    fi
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

        # Add override for systemd-networkd to prevent clearing foreign network rules
        NETWORKD_CONF_DIR="/etc/systemd/system/systemd-networkd.service.d"
        $SUDO mkdir -p "$NETWORKD_CONF_DIR"
        cat << 'EOF' | $SUDO tee "$NETWORKD_CONF_DIR/90-easy42.conf" >/dev/null
[Network]
ManageForeignRoutingPolicyRules=no
ManageForeignRoutes=no
ManageForeignNextHops=no
EOF
        $SUDO chmod 644 "$NETWORKD_CONF_DIR/90-easy42.conf"

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

# 9. Configure custom network script for systemd systems via networkd-dispatcher
if has_systemd || [ -d /etc/systemd/system ] || (command -v systemctl >/dev/null 2>&1); then
    ROUTABLE_DIR="/etc/networkd-dispatcher/routable.d"
    OVERRIDE_DIR="/etc/systemd/system/networkd-dispatcher.service.d"
    NETWORKD_OVERRIDE_DIR="/etc/systemd/system/systemd-networkd.service.d"

    if [ "$HAS_CUSTOM_SCRIPT" = "1" ] && [ -f "$SCRIPT_DIR/custom_network.sh" ]; then
        TARGET_IFACE="$(get_target_iface)"
        echo "Configuring custom network script for interface '$TARGET_IFACE' via networkd-dispatcher..."

        # Try to ensure networkd-dispatcher is installed
        if ! command -v networkd-dispatcher >/dev/null 2>&1 && ! [ -d /etc/networkd-dispatcher ]; then
            echo "Attempting to install networkd-dispatcher package..."
            install_packages networkd-dispatcher 2>/dev/null || true
        fi

        # 1. Store script in /etc/networkd-dispatcher/routable.d/99-easy42.sh
        $SUDO mkdir -p "$ROUTABLE_DIR"
        TMP_DISPATCHER="/tmp/99-easy42.sh.$$"
        {
            echo "#!/bin/sh"
            echo ""
            echo "if [ \"\$IFACE\" != \"$TARGET_IFACE\" ]; then"
            echo "  exit 0"
            echo "fi"
            echo ""
            echo "# user defined scripts"
            echo "### EASY42_CUSTOM_START"
            cat "$SCRIPT_DIR/custom_network.sh"
            echo ""
            echo "### EASY42_CUSTOM_END"
        } > "$TMP_DISPATCHER"
        $SUDO cp -f "$TMP_DISPATCHER" "$ROUTABLE_DIR/99-easy42.sh"
        $SUDO chmod 755 "$ROUTABLE_DIR/99-easy42.sh"
        $SUDO chown root:root "$ROUTABLE_DIR/99-easy42.sh" 2>/dev/null || true
        rm -f "$TMP_DISPATCHER"

        # 2. Add override for networkd-dispatcher.service
        $SUDO mkdir -p "$OVERRIDE_DIR"
        OVERRIDE_FILE="$OVERRIDE_DIR/90-easy42.conf"
        TMP_OVERRIDE="/tmp/networkd-dispatcher-override.conf.$$"
        cat << 'EOF' > "$TMP_OVERRIDE"
[Unit]
PartOf=systemd-networkd.service
After=systemd-networkd.service

[Service]
Restart=on-failure
EOF
        $SUDO cp -f "$TMP_OVERRIDE" "$OVERRIDE_FILE"
        $SUDO chmod 644 "$OVERRIDE_FILE"
        rm -f "$TMP_OVERRIDE"

        # 3. Add override for systemd-networkd to prevent clearing foreign network rules
        $SUDO mkdir -p "$NETWORKD_OVERRIDE_DIR"
        NETWORKD_OVERRIDE_FILE="$NETWORKD_OVERRIDE_DIR/90-easy42.conf"
        TMP_NETWORKD_OVERRIDE="/tmp/systemd-networkd-override.conf.$$"
        cat << 'EOF' > "$TMP_NETWORKD_OVERRIDE"
[Network]
ManageForeignRoutingPolicyRules=no
ManageForeignRoutes=no
ManageForeignNextHops=no
EOF
        $SUDO cp -f "$TMP_NETWORKD_OVERRIDE" "$NETWORKD_OVERRIDE_FILE"
        $SUDO chmod 644 "$NETWORKD_OVERRIDE_FILE"
        rm -f "$TMP_NETWORKD_OVERRIDE"

        # 4. Reload systemd daemon
        echo "Reloading systemd manager configuration..."
        $SUDO systemctl daemon-reload

        # 5. Enable and start networkd-dispatcher
        if is_service_installed networkd-dispatcher || command -v systemctl >/dev/null 2>&1; then
            $SUDO systemctl enable networkd-dispatcher 2>/dev/null || true
            $SUDO systemctl start networkd-dispatcher 2>/dev/null || true
        fi
    else
        # Remove custom network script if no longer configured
        if [ -f "$ROUTABLE_DIR/99-easy42.sh" ]; then
            echo "Removing custom network script from $ROUTABLE_DIR/99-easy42.sh..."
            $SUDO rm -f "$ROUTABLE_DIR/99-easy42.sh"
        fi
        if [ -f "$OVERRIDE_DIR/90-easy42.conf" ]; then
            $SUDO rm -f "$OVERRIDE_DIR/90-easy42.conf"
        fi
        if [ -f "$NETWORKD_OVERRIDE_DIR/90-easy42.conf" ]; then
            $SUDO rm -f "$NETWORKD_OVERRIDE_DIR/90-easy42.conf"
        fi
    fi
fi

# 10. Restart network manager to make all changes take effect
echo "Restarting network services to apply all changes..."
if has_systemd || command -v systemctl >/dev/null 2>&1; then
    $SUDO systemctl restart systemd-networkd 2>/dev/null || true
    if is_service_installed networkd-dispatcher || [ -d /etc/networkd-dispatcher ]; then
        $SUDO systemctl restart networkd-dispatcher 2>/dev/null || true
    fi
elif [ "$OS_ID" = "alpine" ] || (command -v rc-service >/dev/null 2>&1 && [ -f /etc/init.d/networking ]); then
    $SUDO rc-service networking restart 2>/dev/null || true
elif is_service_installed networking; then
    restart_service networking 2>/dev/null || true
fi

# 11. Post-Apply Verification
FAILED_VERIFY=0
for ip in $ALL_DESIRED_IPS; do
    if ! ip_exists_on_host "$ip"; then
        FAILED_VERIFY=1
        echo "Verification error: IP address '$ip' is not bound to node after applying configuration" >&2
    fi
done

if [ "$NODE_INTERFACE" = "lo" ]; then
    if ! ip addr show dev lo 2>/dev/null | grep -q "127\.0\.0\.1"; then
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

echo "Successfully configured network for node '$NODE_NAME' (interface: $NODE_INTERFACE, IPs: ${ALL_DESIRED_IPS:-none}) via $NET_MGR"
exit 0
