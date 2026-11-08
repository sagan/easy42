#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

PHANTUN_FALLBACK_VERSION="v0.8.1-fix8"
PHANTUN_VERSION="$PHANTUN_FALLBACK_VERSION"
PHANTUN_API_URL="https://api.github.com/repos/sagan/phantun/releases/latest"

case "$SYSTEM_ARCH" in
    x86_64)
        TARGET="x86_64-unknown-linux-musl"
        FALLBACK_SHA256="c470ccfb45ddb15b7ec0b9742df73479920a8b677d603bbd5ac657b0e70aa3fa"
        ;;
    i386)
        TARGET="i686-unknown-linux-musl"
        FALLBACK_SHA256="42b1888aa26102729075f724270cc01840824c4dd18983d0e2fabe537ee0f77b"
        ;;
    arm64)
        TARGET="aarch64-unknown-linux-musl"
        FALLBACK_SHA256="d531e2b076e6080471277784d50c44793c686b5c6bb764ccda2cd42d31bfe5af"
        ;;
    arm|armv6)
        TARGET="arm-unknown-linux-musleabihf"
        FALLBACK_SHA256="c7d354c5b763b3efdad50d2b698553c5339c84e7ac3e5eadd83ce57a3823f725"
        ;;
    armv7)
        TARGET="armv7-unknown-linux-musleabihf"
        FALLBACK_SHA256="2f8e98819c382357fe169f64ae7edf602c3874dc95d5e85acbce5565fb6a301f"
        ;;
    mips)
        TARGET="mips-unknown-linux-musl_nightly"
        FALLBACK_SHA256="ae35e3b44c90b6e8b1e9d49d3e5135f2b901032d73257e1f839206dca8481fe8"
        ;;
    mipsle)
        TARGET="mipsel-unknown-linux-musl_nightly"
        FALLBACK_SHA256="6dcbfcf0686fcb61ae6d4795a8fd9bb7ad70ab2b40d5de6eb3c0e348110944c9"
        ;;
    mips64)
        TARGET="mips64-unknown-linux-muslabi64_nightly"
        FALLBACK_SHA256="20395b64114b0bb4760f1117736856c3bfe4da97f1e7d1710e5a8d9ab47b69a8"
        ;;
    *)
        echo "Error: unsupported architecture '$SYSTEM_ARCH' for the Phantun binary task" >&2
        exit 1
        ;;
esac

ARCHIVE="phantun_${TARGET}.zip"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM

fetch_file() {
    url="$1"
    output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -H "Accept: application/vnd.github+json" -o "$output" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$output" --header="Accept: application/vnd.github+json" "$url"
    else
        return 1
    fi
}

extract_asset_field() {
    field="$1"
    tr ',' '\n' < "$RELEASE_JSON" | awk -v want="$ARCHIVE" -v field="$field" '
        {
            if ($0 ~ /"name"[[:space:]]*:/) {
                rest = $0
                sub(/^.*"name"[[:space:]]*:[[:space:]]*"/, "", rest)
                sub(/".*$/, "", rest)
                wanted = (rest == want)
            }
            if (wanted && $0 ~ ("\"" field "\"[[:space:]]*:")) {
                rest = $0
                marker = "\"" field "\""
                pat = "^.*" marker "[[:space:]]*:[[:space:]]*\""
                sub(pat, "", rest)
                sub(/".*$/, "", rest)
                value = rest
                if (field == "digest") {
                    sub(/^sha256:/, "", value)
                }
                print value
                exit
            }
        }
    '
}

RELEASE_JSON="$WORK_DIR/release.json"
API_URL=""
API_DIGEST=""
if fetch_file "$PHANTUN_API_URL" "$RELEASE_JSON"; then
    RESOLVED_VERSION="$(tr ',' '\n' < "$RELEASE_JSON" | awk -F'"' '/"tag_name"[[:space:]]*:/ {print $4; exit}')"
    case "$RESOLVED_VERSION" in
        ""|*[!A-Za-z0-9._-]*)
            ;;
        *)
            PHANTUN_VERSION="$RESOLVED_VERSION"
            ;;
    esac
    API_URL="$(extract_asset_field "browser_download_url")"
    API_DIGEST="$(extract_asset_field "digest")"
fi

PHANTUN_BASE_URL="https://github.com/sagan/phantun/releases/download/${PHANTUN_VERSION}"
URL="${PHANTUN_BASE_URL}/${ARCHIVE}"
case "$API_URL" in
    https://*)
        URL="$API_URL"
        ;;
esac

EXPECTED_SHA256=""
case "$API_DIGEST" in
    [0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]*)
        if [ ${#API_DIGEST} -eq 64 ]; then
            EXPECTED_SHA256="$API_DIGEST"
        fi
        ;;
esac
if [ -z "$EXPECTED_SHA256" ] && [ "$PHANTUN_VERSION" = "$PHANTUN_FALLBACK_VERSION" ]; then
    EXPECTED_SHA256="$FALLBACK_SHA256"
fi

ZIP_FILE="$WORK_DIR/$ARCHIVE"
echo "Using Phantun release $PHANTUN_VERSION"

INSTALL_DIR="/usr/bin"
case ":$PATH:" in
    *:/usr/local/bin:*)
        if [ -d /usr/local/bin ]; then
            INSTALL_DIR="/usr/local/bin"
        fi
        ;;
esac

compute_sha256() {
    file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | awk '{print $1}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$file" | awk '{print $NF}'
    else
        return 1
    fi
}

install_file() {
    src="$1"
    dst="$2"
    if command -v install >/dev/null 2>&1; then
        $SUDO install -m 0755 "$src" "$dst"
    else
        $SUDO cp -f "$src" "$dst"
        $SUDO chmod 0755 "$dst"
    fi
}

echo "Downloading $ARCHIVE from $URL ..."
if ! fetch_file "$URL" "$ZIP_FILE"; then
    echo "Error: curl or wget is required to download Phantun" >&2
    exit 1
fi

if [ -n "$EXPECTED_SHA256" ]; then
    ACTUAL_SHA256="$(compute_sha256 "$ZIP_FILE")"
    if [ -z "$ACTUAL_SHA256" ]; then
        echo "Error: no SHA-256 tool (sha256sum, shasum, or openssl) is available" >&2
        exit 1
    fi
    if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
        echo "Error: SHA-256 mismatch for $ARCHIVE" >&2
        echo "Expected: $EXPECTED_SHA256" >&2
        echo "Actual:   $ACTUAL_SHA256" >&2
        exit 1
    fi
    echo "Verified SHA-256: $ACTUAL_SHA256"
else
    echo "Warning: no SHA-256 digest available for Phantun $PHANTUN_VERSION; skipping integrity verification"
fi

echo "Extracting $ARCHIVE ..."
if ! command -v unzip >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1 && ! command -v python >/dev/null 2>&1; then
    echo "Neither unzip nor python is available; attempting to install unzip via the system package manager ..."
    install_packages unzip
fi
if command -v unzip >/dev/null 2>&1; then
    unzip -q -o "$ZIP_FILE" -d "$WORK_DIR"
elif command -v python3 >/dev/null 2>&1; then
    python3 -m zipfile -e "$ZIP_FILE" "$WORK_DIR"
elif command -v python >/dev/null 2>&1; then
    python -m zipfile -e "$ZIP_FILE" "$WORK_DIR"
else
    echo "Error: unzip or python is required to extract the Phantun archive" >&2
    exit 1
fi

if [ ! -f "$WORK_DIR/phantun_client" ] || [ ! -f "$WORK_DIR/phantun_server" ]; then
    echo "Error: Phantun archive does not contain phantun_client and phantun_server" >&2
    exit 1
fi

$SUDO mkdir -p "$INSTALL_DIR"
install_file "$WORK_DIR/phantun_client" "$INSTALL_DIR/phantun_client"
install_file "$WORK_DIR/phantun_server" "$INSTALL_DIR/phantun_server"

if [ ! -x "$INSTALL_DIR/phantun_client" ] || [ ! -x "$INSTALL_DIR/phantun_server" ]; then
    echo "Error: Phantun installation completed but binaries are not executable in $INSTALL_DIR" >&2
    exit 1
fi

# Refresh the shell lookup so the confirmation uses the installed binaries.
CLIENT_BIN="$(command -v phantun_client 2>/dev/null || true)"
SERVER_BIN="$(command -v phantun_server 2>/dev/null || true)"
echo "Phantun $PHANTUN_VERSION installed: ${CLIENT_BIN:-$INSTALL_DIR/phantun_client} ${SERVER_BIN:-$INSTALL_DIR/phantun_server}"
exit 0
