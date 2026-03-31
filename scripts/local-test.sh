#!/usr/bin/env bash
#
# Local two-user test for Enclave.
#
# This script:
#   1. Builds the binary
#   2. Starts a server
#   3. Generates two invite tokens
#   4. Initializes two users (alice & bob)
#   5. Opens two chat TUIs in new terminal tabs/panes
#
# Usage:
#   ./scripts/local-test.sh          # auto-detect terminal
#   ./scripts/local-test.sh tmux     # force tmux split
#   ./scripts/local-test.sh tabs     # force new terminal windows

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_DIR/build/enclave"
TEST_DIR="/tmp/enclave-local-test"
SERVER_DIR="$TEST_DIR/server"
ALICE_DIR="$TEST_DIR/alice"
BOB_DIR="$TEST_DIR/bob"
BIND="127.0.0.1:9300"
MODE="${1:-auto}"

cleanup() {
    echo ""
    echo "Cleaning up..."
    # Kill any chat clients we spawned
    if [[ -f "$TEST_DIR/alice.pid" ]]; then
        kill "$(cat "$TEST_DIR/alice.pid")" 2>/dev/null || true
    fi
    if [[ -f "$TEST_DIR/bob.pid" ]]; then
        kill "$(cat "$TEST_DIR/bob.pid")" 2>/dev/null || true
    fi
    # Kill the server
    if [[ -f "$TEST_DIR/server.pid" ]]; then
        kill "$(cat "$TEST_DIR/server.pid")" 2>/dev/null || true
    fi
    echo "Done. Test data is in $TEST_DIR"
}
trap cleanup EXIT

echo "=== Enclave Local Test ==="
echo ""

# Build
echo "Building..."
cd "$PROJECT_DIR"
go build -o "$BINARY" ./cmd/enclave/
echo "  Binary: $BINARY"

# Clean previous test
rm -rf "$TEST_DIR"
mkdir -p "$SERVER_DIR" "$ALICE_DIR" "$BOB_DIR"

# Start server
echo ""
echo "Starting server on $BIND..."
"$BINARY" serve --bind "$BIND" --db "$SERVER_DIR/server.db" --data-dir "$SERVER_DIR" --log-level info > "$TEST_DIR/server.log" 2>&1 &
SERVER_PID=$!
echo "$SERVER_PID" > "$TEST_DIR/server.pid"
sleep 1

if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "ERROR: Server failed to start. Check $TEST_DIR/server.log"
    exit 1
fi

# Read admin key
ADMIN_KEY=$(cat "$SERVER_DIR/admin.key")
echo "  Server PID: $SERVER_PID"
echo "  Admin key:  $ADMIN_KEY"

# Generate invite tokens
echo ""
echo "Generating invite tokens..."
ALICE_TOKEN=$("$BINARY" invite --server "$BIND" --admin-key "$ADMIN_KEY" 2>&1 | grep "Invite token:" | awk '{print $NF}')
BOB_TOKEN=$("$BINARY" invite --server "$BIND" --admin-key "$ADMIN_KEY" 2>&1 | grep "Invite token:" | awk '{print $NF}')
echo "  Alice token: $ALICE_TOKEN"
echo "  Bob token:   $BOB_TOKEN"

# Initialize users
echo ""
echo "Initializing alice..."
ENCLAVE_HOME="$ALICE_DIR" "$BINARY" init --display-name "alice" --server "$BIND" --token "$ALICE_TOKEN" 2>&1 | grep -E "(Registered|Fingerprint)"

echo "Initializing bob..."
ENCLAVE_HOME="$BOB_DIR" "$BINARY" init --display-name "bob" --server "$BIND" --token "$BOB_TOKEN" 2>&1 | grep -E "(Registered|Fingerprint)"

echo ""
echo "=== Setup complete ==="
echo ""
echo "To chat, open two terminals and run:"
echo ""
echo "  Terminal 1 (alice):"
echo "    ENCLAVE_HOME=$ALICE_DIR $BINARY chat"
echo ""
echo "  Terminal 2 (bob):"
echo "    ENCLAVE_HOME=$BOB_DIR $BINARY chat"
echo ""

# Try to auto-launch based on mode
launch_chats() {
    if [[ "$MODE" == "tmux" ]] || ([[ "$MODE" == "auto" ]] && command -v tmux &>/dev/null && [[ -n "${TMUX:-}" ]]); then
        echo "Launching in tmux split panes..."
        tmux split-window -h "ENCLAVE_HOME=$ALICE_DIR $BINARY chat; read -p 'Press enter to close'"
        tmux split-window -v -t '{right}' "ENCLAVE_HOME=$BOB_DIR $BINARY chat; read -p 'Press enter to close'"
        tmux select-pane -t '{left}'
        echo "Chat panes opened. Press Ctrl+C here to stop the server."
        wait "$SERVER_PID"
        return 0
    fi

    # Fallback: just wait
    echo "Server is running. Press Ctrl+C to stop everything."
    echo ""
    wait "$SERVER_PID"
}

launch_chats
