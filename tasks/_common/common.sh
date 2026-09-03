#!/bin/sh
# easy42 task helper common functions

# Ensure safe execution
set -e

# Detect privilege
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        echo "Error: Root privileges required but sudo is not installed or available" >&2
        exit 1
    fi
fi

# Detect OS distribution
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_ID="${ID}"
        OS_ID_LIKE="${ID_LIKE:-}"
    elif [ -f /etc/openwrt_release ] || [ -f /etc/openwrt_version ]; then
        OS_ID="openwrt"
        OS_ID_LIKE=""
    elif [ -f /etc/alpine-release ]; then
        OS_ID="alpine"
        OS_ID_LIKE=""
    elif [ -f /etc/arch-release ]; then
        OS_ID="arch"
        OS_ID_LIKE=""
    elif [ -f /etc/debian_version ]; then
        OS_ID="debian"
        OS_ID_LIKE=""
    else
        OS_ID="unknown"
        OS_ID_LIKE=""
    fi
}

detect_os

# Detect Init System (systemd, openrc, procd, or unknown)
detect_init_system() {
    if [ -n "${INIT_SYSTEM:-}" ]; then
        return 0
    fi

    # 1. Systemd (active systemd runtime directory)
    if [ -d /run/systemd/system ]; then
        INIT_SYSTEM="systemd"
        return 0
    fi

    # 2. OpenRC (active OpenRC runtime directory / runlevels)
    if [ -d /run/openrc ] || [ -f /run/openrc/softlevel ]; then
        INIT_SYSTEM="openrc"
        return 0
    fi

    # 3. Procd / OpenWrt
    if [ -f /etc/rc.common ] || [ -f /sbin/procd ] || [ "${OS_ID:-}" = "openwrt" ]; then
        INIT_SYSTEM="procd"
        return 0
    fi

    # 4. OpenRC command fallback (e.g. Alpine container without /run/openrc mounted)
    if [ -f /sbin/openrc-run ] || (command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1); then
        INIT_SYSTEM="openrc"
        return 0
    fi

    # 5. Systemctl command fallback
    if command -v systemctl >/dev/null 2>&1; then
        INIT_SYSTEM="systemd"
        return 0
    fi

    INIT_SYSTEM="unknown"
}

detect_init_system

# Init system check helpers
has_systemd() {
    [ -z "${INIT_SYSTEM:-}" ] && detect_init_system
    [ "$INIT_SYSTEM" = "systemd" ]
}

has_openrc() {
    [ -z "${INIT_SYSTEM:-}" ] && detect_init_system
    [ "$INIT_SYSTEM" = "openrc" ]
}

has_procd() {
    [ -z "${INIT_SYSTEM:-}" ] && detect_init_system
    [ "$INIT_SYSTEM" = "procd" ]
}

get_init_system() {
    [ -z "${INIT_SYSTEM:-}" ] && detect_init_system
    echo "${INIT_SYSTEM:-unknown}"
}

# Returns the appropriate service file path for the given service name
get_service_file_path() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            echo "/etc/systemd/system/${name}.service"
            ;;
        openrc|procd)
            echo "/etc/init.d/${name}"
            ;;
        *)
            echo "Error: Unknown or unsupported init system '$INIT_SYSTEM' for service '$name'" >&2
            return 1
            ;;
    esac
}

# Checks if a service is installed
is_service_installed() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            [ -f "/etc/systemd/system/${name}.service" ] || \
            [ -f "/lib/systemd/system/${name}.service" ] || \
            [ -f "/usr/lib/systemd/system/${name}.service" ] || \
            (command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files "${name}.service" 2>/dev/null | grep -q "${name}\.service")
            ;;
        openrc|procd)
            [ -f "/etc/init.d/${name}" ]
            ;;
        *)
            return 1
            ;;
    esac
}

# Checks if a service is enabled
is_service_enabled() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            systemctl is-enabled "${name}.service" >/dev/null 2>&1 || systemctl is-enabled "$name" >/dev/null 2>&1
            ;;
        openrc)
            if [ -e "/etc/runlevels/default/${name}" ] || [ -e "/etc/runlevels/boot/${name}" ]; then
                return 0
            elif command -v rc-update >/dev/null 2>&1; then
                rc-update show 2>/dev/null | grep -qw "$name"
            else
                return 1
            fi
            ;;
        procd)
            if [ -x "/etc/init.d/${name}" ]; then
                /etc/init.d/"$name" enabled >/dev/null 2>&1
            else
                ls /etc/rc.d/S*"${name}" >/dev/null 2>&1
            fi
            ;;
        *)
            return 1
            ;;
    esac
}

# Enables a service to start on boot
enable_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl daemon-reload
            $SUDO systemctl enable "${name}.service"
            ;;
        openrc)
            $SUDO rc-update add "$name" default
            ;;
        procd)
            $SUDO /etc/init.d/"$name" enable
            ;;
        *)
            echo "Error: Cannot enable service on unsupported init system '$INIT_SYSTEM'" >&2
            return 1
            ;;
    esac
}

# Disables a service from starting on boot
disable_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl disable "${name}.service" >/dev/null 2>&1 || true
            $SUDO systemctl daemon-reload >/dev/null 2>&1 || true
            ;;
        openrc)
            $SUDO rc-update del "$name" default >/dev/null 2>&1 || true
            ;;
        procd)
            $SUDO /etc/init.d/"$name" disable >/dev/null 2>&1 || true
            ;;
        *)
            return 1
            ;;
    esac
}

# Starts a service
start_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl start "$name"
            ;;
        openrc)
            $SUDO rc-service "$name" start
            ;;
        procd)
            $SUDO /etc/init.d/"$name" start
            ;;
        *)
            return 1
            ;;
    esac
}

# Stops a service
stop_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl stop "$name"
            ;;
        openrc)
            $SUDO rc-service "$name" stop
            ;;
        procd)
            $SUDO /etc/init.d/"$name" stop
            ;;
        *)
            return 1
            ;;
    esac
}

# Restarts a service
restart_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl restart "$name"
            ;;
        openrc)
            $SUDO rc-service "$name" restart
            ;;
        procd)
            $SUDO /etc/init.d/"$name" restart
            ;;
        *)
            return 1
            ;;
    esac
}

# Reloads a service configuration
reload_service() {
    local name="${1%.service}"
    case "$INIT_SYSTEM" in
        systemd)
            $SUDO systemctl reload "$name"
            ;;
        openrc)
            $SUDO rc-service "$name" reload 2>/dev/null || $SUDO rc-service "$name" restart
            ;;
        procd)
            $SUDO /etc/init.d/"$name" reload 2>/dev/null || $SUDO /etc/init.d/"$name" restart
            ;;
        *)
            return 1
            ;;
    esac
}

# Install packages across various distros
install_packages() {
    case "$OS_ID" in
        ubuntu|debian|raspbian|armbian)
            export DEBIAN_FRONTEND=noninteractive
            $SUDO apt-get update -qq && $SUDO apt-get install -y -qq "$@"
            ;;
        centos|rhel|rocky|almalinux|fedora)
            if command -v dnf >/dev/null 2>&1; then
                $SUDO dnf install -y -q "$@"
            else
                $SUDO yum install -y -q "$@"
            fi
            ;;
        alpine)
            $SUDO apk update && $SUDO apk add "$@"
            ;;
        arch|manjaro)
            $SUDO pacman -Sy --noconfirm "$@"
            ;;
        *)
            # Check ID_LIKE
            case "$OS_ID_LIKE" in
                *debian*|*ubuntu*)
                    export DEBIAN_FRONTEND=noninteractive
                    $SUDO apt-get update -qq && $SUDO apt-get install -y -qq "$@"
                    ;;
                *rhel*|*fedora*|*centos*)
                    if command -v dnf >/dev/null 2>&1; then
                        $SUDO dnf install -y -q "$@"
                    else
                        $SUDO yum install -y -q "$@"
                    fi
                    ;;
                *arch*)
                    $SUDO pacman -Sy --noconfirm "$@"
                    ;;
                *)
                    echo "Error: Unsupported distribution '$OS_ID'" >&2
                    return 1
                    ;;
            esac
            ;;
    esac
}

# Create backup of a file with timestamp
backup_file() {
    local target="$1"
    if [ -f "$target" ]; then
        local ts
        ts="$(date +%Y%m%d%H%M%S)"
        $SUDO cp -p "$target" "${target}.easy42.bak.${ts}"
    fi
}
