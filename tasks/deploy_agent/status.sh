#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

if command -v easy42-agent >/dev/null 2>&1; then
    VER="$(easy42-agent --version 2>&1 | head -n1)"
    if command -v systemctl >/dev/null 2>&1; then
        if systemctl is-active --quiet easy42-agent 2>/dev/null; then
            echo "Easy42 Agent is active and running ($VER)"
            exit 10
        else
            echo "Easy42 Agent binary installed ($VER) but service is inactive"
            exit 0
        fi
    elif command -v service >/dev/null 2>&1; then
        if service easy42-agent status >/dev/null 2>&1; then
            echo "Easy42 Agent is running ($VER)"
            exit 10
        else
            echo "Easy42 Agent binary installed ($VER) but service is stopped"
            exit 0
        fi
    else
        echo "Easy42 Agent binary installed ($VER)"
        exit 10
    fi
fi

echo "Easy42 Agent is not installed"
exit 0
