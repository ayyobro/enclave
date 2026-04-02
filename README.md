# Enclave

Secure, end-to-end encrypted chat for developers. Fully on-prem. Zero cloud dependencies.

Enclave is a TUI (terminal UI) chat application that runs in your terminal alongside your editor and tools. The server only routes opaque encrypted blobs — messages are encrypted client-side using NaCl box (X25519 + XSalsa20-Poly1305) and the server never sees plaintext. Everything runs on your own hardware. One binary, no accounts, no cloud.

## Features

### Core
- **End-to-end encrypted** — Messages encrypted client-side before leaving your machine
- **Fully on-prem** — Server and clients run on your own hardware, zero cloud dependencies
- **Single binary** — One executable serves as both server and client
- **Group chat** — Create groups with `/group`, invite members with `/invite`, messages encrypted per-recipient
- **Direct messages** — 1:1 encrypted conversations
- **Message persistence** — Chat history survives restarts, stored locally in SQLite with FTS5
- **Offline delivery** — Messages queue on the server when recipients are offline

### Developer Experience
- **Collaborative AI coding (`/vibe2gether`)** — Start a shared Claude Code session from within the chat. Participants send `@claude` prompts, the host's machine runs them, and output streams to everyone in real-time. Host approves or rejects remote prompts before execution. All communication E2E encrypted
- **Rich TUI** — Split-pane interface with contacts, chat viewport, input, and status bar
- **Syntax highlighting** — Fenced code blocks render with full syntax highlighting (200+ languages via Chroma)
- **Inline code** — Backtick-wrapped `code` renders with a distinct background
- **Code block copy** — Numbered code blocks with `/copy N` to yank to clipboard
- **Diff rendering** — Unified diff format renders with `+` green / `-` red coloring
- **Clickable URLs** — URLs render as styled links (OSC 8 hyperlinks in supporting terminals)
- **Encrypted file transfer** — `/send <path>` encrypts and transmits files up to 10MB. Code files render inline with syntax highlighting on the recipient side; all files saved to `~/.enclave/files/`
- **Full-text search** — `/search <query>` across all conversations
- **Paste support** — Paste large content (code files, logs) directly into the input with bracketed paste

### Social
- **Typing indicators** — See when contacts are typing in real-time (debounced, 2s intervals)
- **Presence** — Online/offline status with visual indicators in the sidebar
- **Read receipts** — Single check (delivered) and double check (read) on sent messages
- **Message reactions** — `/react <emoji>` to react to the last message, persisted and synced
- **Message pinning** — `/pin` to bookmark messages, `/pins` to list them

### Security & Identity
- **Challenge-response auth** — No passwords; identity is your NaCl keypair
- **Invite-only access** — Users must receive a one-time token to register
- **Key verification** — `/verify` shows fingerprints and visual IDs for out-of-band verification
- **Contact detail overlay** — Full key info with colored visual fingerprint blocks
- **QR code export** — `enclave export-key --format qr` renders a scannable QR code in the terminal
- **TLS support** — Optional encrypted transport with auto-generated self-signed ECDSA P-256 certs

### Infrastructure
- **SSH gateway** — `enclave serve --ssh` starts an SSH server for zero-install onboarding
- **Color themes** — Dark (Tokyo Night), Light (One Light), Dracula, and Nord
- **Slash commands** — Type `/` for fuzzy autocomplete command palette
- **Ephemeral mode** — `/ephemeral 30s` enables disappearing messages that auto-delete from both sides' databases after the timer expires. Both parties are notified and enforce the same expiry independently
- **Conversation export** — `/export` saves conversations as timestamped text files
- **Cross-platform** — Builds for Linux, macOS, and Windows (amd64 and arm64)
- **No CGO** — Pure Go, including SQLite, for effortless cross-compilation

## Quick Start

### Build

```bash
git clone https://github.com/ayyobro/enclave.git && cd enclave
make build
```

Pick a deployment option below based on your setup.

---

### Option A: Local Network (LAN testing)

For testing on your home network or with teammates in the same office.

**1. Start the server**

```bash
./build/enclave serve --bind 0.0.0.0:9300 --data-dir ./server-data
```

Note the **admin key** printed on startup.

**2. Generate invite tokens**

```bash
./build/enclave invite --server <your-lan-ip>:9300 --admin-key <admin-key>
```

**3. Each user registers and chats**

```bash
./build/enclave init --display-name alice --server <your-lan-ip>:9300 --token <token>
./build/enclave chat
```

**Quick local test with two users:**

```bash
./scripts/local-test.sh
```

---

### Option B: Tailscale (recommended for remote teams)

The easiest way to chat across locations. Tailscale creates a private WireGuard mesh network — no port forwarding, no public IP exposure, no firewall rules. Free for personal use.

**1. Install Tailscale on all machines**

```bash
# Linux
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# macOS
brew install tailscale

# Windows
# Download from https://tailscale.com/download
```

**2. Start the server on any machine in your tailnet**

```bash
# Find your Tailscale IP
tailscale ip -4
# Example: 100.64.0.1

./build/enclave serve --bind 0.0.0.0:9300 --data-dir ./server-data --tls
```

**3. Generate invite tokens**

```bash
./build/enclave invite --server 100.64.0.1:9300 --admin-key <admin-key> --tls
```

**4. Each user on the tailnet registers and chats**

```bash
./build/enclave init --display-name alice --server 100.64.0.1:9300 --tls --token <token>
./build/enclave chat
```

That's it. Tailscale handles all the networking. Traffic is encrypted by WireGuard (network layer) on top of Enclave's E2E encryption (application layer). The server can run on a Raspberry Pi, an old laptop, or any machine you control.

---

### Option C: Hardened VPS (always-on, accessible from anywhere)

For a dedicated server that's always reachable. This example uses DigitalOcean but works with any VPS provider (Hetzner, Linode, Vultr, etc).

**1. Create a VPS**

- DigitalOcean: Create a $6/mo droplet (1 vCPU, 1GB RAM, Ubuntu 24.04)
- Or Hetzner: Create a €3.79/mo CX22 (2 vCPU, 4GB RAM)

**2. Harden the server**

```bash
# SSH into your VPS
ssh root@<vps-ip>

# Create a non-root user
adduser enclave
usermod -aG sudo enclave

# Disable root SSH login
sed -i 's/PermitRootLogin yes/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl restart sshd

# Set up firewall — only allow SSH and Enclave
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp    # SSH
ufw allow 9300/tcp  # Enclave
ufw enable

# Automatic security updates
apt install -y unattended-upgrades
dpkg-reconfigure -plow unattended-upgrades
```

**3. Install and run Enclave**

```bash
# Switch to the enclave user
su - enclave

# Download or build the binary
# Option A: Build from source
sudo apt install -y golang-go git
git clone https://github.com/ayyobro/enclave.git && cd enclave
make build

# Option B: Download a release binary
# curl -L <release-url> -o enclave && chmod +x enclave

# Create data directory
mkdir -p ~/enclave-data

# Start with TLS enabled
./build/enclave serve --bind 0.0.0.0:9300 --data-dir ~/enclave-data --tls
```

**4. Run as a systemd service (auto-start on boot)**

```bash
sudo tee /etc/systemd/system/enclave.service > /dev/null <<EOF
[Unit]
Description=Enclave Chat Server
After=network.target

[Service]
Type=simple
User=enclave
WorkingDirectory=/home/enclave/enclave
ExecStart=/home/enclave/enclave/build/enclave serve --bind 0.0.0.0:9300 --data-dir /home/enclave/enclave-data --tls
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable enclave
sudo systemctl start enclave

# Check status
sudo systemctl status enclave

# View logs
sudo journalctl -u enclave -f
```

**5. Generate invites and connect**

From your local machine:

```bash
./build/enclave invite --server <vps-ip>:9300 --admin-key <admin-key> --tls
```

Each user:

```bash
./build/enclave init --display-name alice --server <vps-ip>:9300 --tls --token <token>
./build/enclave chat
```

**VPS security notes:**
- TLS is mandatory for VPS deployments — always use `--tls`
- The server only sees encrypted blobs — even on a VPS you don't fully trust, message content is safe
- The admin key is stored on the VPS at `enclave-data/admin.key` — protect it like a password
- Consider adding fail2ban for SSH: `apt install fail2ban`
- For extra security, combine with Tailscale: run the VPS on your tailnet and bind to the Tailscale IP instead of `0.0.0.0`

---

### Choosing a deployment

| | LAN | Tailscale | VPS |
|---|---|---|---|
| **Cost** | Free | Free (personal) | $4-6/mo |
| **Setup** | 1 minute | 5 minutes | 15 minutes |
| **Always on** | Only when your machine is on | Only when host machine is on | Yes |
| **Remote access** | Same network only | Anywhere with Tailscale | Anywhere |
| **Port forwarding** | No | No | No |
| **Public IP needed** | No | No | Yes (VPS has one) |
| **Security layers** | E2E encryption | E2E + WireGuard | E2E + TLS |

## CLI Reference

| Command | Description |
|---------|-------------|
| `enclave init` | Generate keypair and register with server |
| `enclave chat` | Launch the TUI chat client |
| `enclave serve` | Start the relay server |
| `enclave invite` | Generate an invite token (requires admin key) |
| `enclave export-key` | Print your public key and fingerprint |
| `enclave status` | Check connectivity and identity status |
| `enclave version` | Print version |

### `enclave init` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--display-name` | *(required)* | Your display name shown to other users |
| `--server` | `localhost:9300` | Server address to register with |
| `--token` | *(required)* | Invite token from the server admin |
| `--tls` | `false` | Connect to server using TLS |
| `--force` | `false` | Overwrite existing keys and config |

### `enclave chat` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | *(from config)* | Override server address |
| `--theme` | `dark` | Color theme: `dark`, `light`, `dracula`, `nord` |

### `enclave serve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--bind` | `0.0.0.0:9300` | Listen address |
| `--db` | `enclave-server.db` | Path to server SQLite database |
| `--data-dir` | `.` | Directory for server keys, admin key, and TLS certs |
| `--tls` | `false` | Enable TLS (auto-generates self-signed cert if none provided) |
| `--tls-cert` | | Path to TLS certificate file |
| `--tls-key` | | Path to TLS private key file |
| `--ssh` | `false` | Enable SSH gateway for zero-install onboarding |
| `--ssh-bind` | `0.0.0.0:2222` | SSH gateway listen address |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |

### `enclave invite` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | `localhost:9300` | Address of the running server |
| `--admin-key` | *(required)* | Admin key printed on server startup |
| `--tls` | `false` | Connect to server using TLS |
| `--expires` | `72h` | Token expiration duration |
| `--uses` | `1` | Maximum uses for the token |

### `enclave export-key` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `base64` | Output format: `base64`, `hex`, `qr` |

## Chat Commands

Type `/` in the message input to see all available commands with fuzzy autocomplete. Navigate with `Up/Down`, select with `Tab` or `Enter`, dismiss with `Esc`.

| Command | Description |
|---------|-------------|
| `/help` | List all available commands |
| `/clear` | Clear the chat view (local only, messages are preserved in history) |
| `/whoami` | Show your display name, public key, and fingerprint |
| `/verify` | In a DM: open contact detail overlay. In a group: list all member fingerprints. `/verify <name>` for a specific user |
| `/users` | List all registered users with online/offline status |
| `/copy [N]` | Copy code block N to clipboard (`/copy` copies the most recent block) |
| `/search <query>` | Full-text search across all message history |
| `/send <path>` | Encrypt and send a file (up to 10MB). Text/code files render inline with syntax highlighting on the recipient side. All files saved to `~/.enclave/files/` |
| `/react [emoji]` | React to the last message (default: thumbs up). Reactions persist and sync to other users |
| `/group <name> <user1> [user2...]` | Create a new group chat with the specified members |
| `/invite <name>` | Invite a user to the current group (only works in group conversations) |
| `/pin` | Pin the last message. Pinned messages show a pin indicator |
| `/pins` | List all pinned messages in the current conversation |
| `/ephemeral <duration>` | Enable disappearing messages — both sides are notified and messages are deleted from both databases after the timer. Accepts Go durations: `30s`, `5m`, `1h`. Use `/ephemeral off` to disable |
| `/vibe2gether <path>` | Start a collaborative Claude Code session on a local repo. Others can send `@claude` prompts. Host approves/rejects remote prompts before execution |
| `/endvibe` | End the active collaborative coding session |
| `/export` | Save the current conversation to `~/.enclave/exports/` as a text file |
| `/quit` | Exit Enclave |

## Keybindings

| Key | Action |
|-----|--------|
| `Tab` | Cycle focus: sidebar → input → chat viewport |
| `Enter` | Send message (input) / select contact (sidebar) / open contact detail (re-select) |
| `Ctrl+Enter` | Insert newline in message |
| `Ctrl+J` | Insert newline (alternative) |
| `Up/Down` or `j/k` | Navigate contacts (sidebar) / scroll messages (chat viewport) / navigate autocomplete |
| `PgUp/PgDn` | Scroll messages by page |
| `Ctrl+V` | Paste from clipboard |
| `Ctrl+C` | Quit |
| `Esc` | Close overlay / dismiss autocomplete |

## Themes

Four built-in color themes. Set with `--theme` when launching chat:

```bash
enclave chat --theme dracula
```

| Theme | Description |
|-------|-------------|
| `dark` | Tokyo Night — default, blue-purple accents on dark background |
| `light` | One Light — bright background, dark text, blue accents |
| `dracula` | Dracula — purple and cyan on dark gray |
| `nord` | Nord — muted blues and greens, arctic palette |

## Architecture

```
┌──────────────────────────┐     ┌──────────────────────────┐
│  enclave chat (Alice)    │     │  enclave chat (Bob)      │
│                          │     │                          │
│  ┌────────┐ ┌─────────┐ │     │ ┌─────────┐ ┌────────┐  │
│  │  TUI   │ │ NaCl    │ │     │ │ NaCl    │ │  TUI   │  │
│  │        │ │ encrypt │ │     │ │ decrypt │ │        │  │
│  └────────┘ └────┬────┘ │     │ └────┬────┘ └────────┘  │
│                  │       │     │      │                   │
│  ┌───────────────┘       │     │      └───────────────┐  │
│  │ local SQLite          │     │        local SQLite   │  │
│  │ (plaintext + FTS5)    │     │   (plaintext + FTS5)  │  │
│  └───────────────┐       │     │      ┌───────────────┘  │
└──────────────────┼───────┘     └──────┼──────────────────┘
                   │   WebSocket (ws/wss) │
            ┌──────┴──────────────┴──────┐
            │       enclave serve        │
            │                            │
            │  ┌──────────────────────┐  │
            │  │  WebSocket Hub       │  │
            │  │  (routes ciphertext) │  │
            │  └──────────┬───────────┘  │
            │             │              │
            │  ┌──────────┴───────────┐  │
            │  │  Server SQLite       │  │
            │  │  - users & keys      │  │
            │  │  - groups & members  │  │
            │  │  - invite tokens     │  │
            │  │  - pending messages  │  │
            │  │    (encrypted blobs) │  │
            │  └──────────────────────┘  │
            │                            │
            │  ┌──────────────────────┐  │
            │  │  SSH Gateway (opt)   │  │
            │  │  (onboarding TUI)    │  │
            │  └──────────────────────┘  │
            └────────────────────────────┘
```

**The server never sees plaintext.** It routes encrypted blobs between connected clients and stores them for offline delivery. Even a fully compromised server reveals nothing about message content — it only sees who is talking to whom and when.

### Group chat encryption

Group messages are encrypted per-recipient. When you send to a group, the client encrypts the plaintext separately with each member's public key (NaCl box). The server receives one envelope containing all recipient-specific ciphertexts and fans it out to each member. Each member decrypts only their own ciphertext using their private key and the sender's public key.

## Collaborative Coding (`/vibe2gether`)

Enclave includes a built-in multiplayer Claude Code experience — the first tool to combine E2E encrypted chat with collaborative AI-assisted coding.

### How it works

```
┌─────────────────────┐          ┌─────────────────────┐
│  alice (host)        │          │  bob (participant)   │
│                      │          │                      │
│  enclave chat        │◄────────►│  enclave chat        │
│       │              │ encrypted│                      │
│       ▼              │          │  types:              │
│  claude code         │          │  @claude add tests   │
│  (subprocess)        │          │                      │
│       │              │          │  sees:               │
│       ▼              │          │  [reading main.go]   │
│  local git repo      │          │  [writing test.go]   │
│  (files modified)    │          │  "Here are the tests"│
└─────────────────────┘          └─────────────────────┘
```

### Starting a session

```
alice> /vibe2gether ~/projects/my-api
🎸 Collaborative coding session started on: my-api
   Others can now use @claude <prompt> to send prompts.
   Use /endvibe to stop.
```

Bob sees:
```
🎸 alice started a collaborative coding session on: my-api
   Use @claude <prompt> to send prompts.
```

### Sending prompts

Anyone in the conversation can type `@claude` followed by a prompt:

```
bob> @claude add a health check endpoint
```

### Host approval

When a remote participant sends a prompt, the host sees an approval request:

```
🔒 bob wants to run:
   @claude add a health check endpoint

   Press [y] to approve or [n] to reject
```

The host's own `@claude` prompts run immediately — no approval needed for your own machine.

### Output streaming

Once approved, Claude Code runs on the host's machine and output streams to all participants:

```
claude · 14:32
[reading main.go]

claude · 14:32
[writing internal/health.go]

claude · 14:33
Here's the health check endpoint. I've added...
```

Tool calls (file reads, edits, bash commands) appear as status lines. The final response appears as a regular chat message with full syntax highlighting.

### Ending a session

```
alice> /endvibe
🎸 Collaborative coding session ended.
```

### Security model

- Claude Code runs **only on the host's machine** — no one else gets shell access
- All prompts and output flow through Enclave's **E2E encryption**
- Remote prompts require **explicit host approval** before execution
- The server sees nothing — prompts and output are encrypted blobs
- Host opts in with `--dangerously-skip-permissions` (required for non-interactive Claude Code)

### Requirements

- [Claude Code CLI](https://claude.ai/claude-code) must be installed on the host's machine
- The host must be authenticated with Claude Code (`claude` command works)

## Encryption

| Property | Detail |
|----------|--------|
| Key exchange | X25519 (Curve25519 Diffie-Hellman) |
| Authenticated encryption | XSalsa20-Poly1305 (NaCl box) |
| Nonce | 24 bytes, randomly generated per message |
| File transfer | Each 32KB chunk individually encrypted with NaCl box |
| Implementation | `golang.org/x/crypto/nacl/box` |
| Key storage | `~/.enclave/identity.key` (32 bytes raw, file mode 0600) |
| Client refuses to run if private key permissions are too open (like SSH) |

### What the server sees

- Who is talking to whom (public keys in message envelopes)
- Group membership
- Message sizes and timestamps
- Online/offline presence

### What the server cannot see

- Message content (encrypted with keys the server doesn't possess)
- File contents (encrypted per-chunk before transmission)
- Users' private keys
- Reaction content (relayed as opaque payloads)

## Authentication

Enclave uses a three-layer authentication model with no passwords:

1. **Invite token** — One-time, time-limited token generated by the admin via the API. Required for registration. Hashed (SHA-256) before storage. Configurable expiry and use count.
2. **Challenge-response** — On each connection, the server sends a random 32-byte nonce. The client proves identity by encrypting it with their private key and the server's public key (NaCl box). The server verifies using the client's registered public key.
3. **Admin API key** — A random 32-byte key generated on first server start, required for the `POST /api/invite` endpoint. Stored in `data-dir/admin.key`. Protected by `Authorization: Bearer` header.

## Building

```bash
make build          # Build for current platform (stripped, ~24MB)
make test           # Run all tests
make build-all      # Cross-compile for linux/darwin/windows x amd64/arm64
make clean          # Remove build artifacts
```

### Cross-compilation

The project uses pure-Go SQLite (`modernc.org/sqlite`) with no CGO dependency, so cross-compilation works without a C toolchain:

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows
```

### Releases

Goreleaser config is included for automated multi-platform builds:

```bash
goreleaser release --snapshot --clean
```

## Security

- Messages are end-to-end encrypted using NaCl box (X25519 + XSalsa20-Poly1305)
- Group messages encrypted per-recipient (one NaCl box per member)
- File transfers encrypted per-chunk (32KB chunks, each with unique nonce)
- Server authenticates users via cryptographic challenge-response — no passwords
- Invite tokens are one-time use, time-limited (default 72h), and hashed before storage
- Admin API is protected by a random key generated on first server start
- Private keys stored with 0600 permissions; the client refuses to run if permissions are too open
- TLS support for encrypted transport (optional, layered on top of E2E encryption)
- Server generates ECDSA P-256 self-signed certificates automatically when TLS is enabled
- Message content is never logged, stored, or accessible server-side
- Reactions and read receipts are relayed through the server without content inspection

## Data

### Client data (`~/.enclave/`)

| File | Purpose |
|------|---------|
| `config.toml` | Server address, display name, TLS setting, registration status |
| `identity.key` | NaCl private key (32 bytes, mode 0600) |
| `identity.pub` | NaCl public key (32 bytes) |
| `contacts/` | Directory of saved contact public keys |
| `messages.db` | Local message history, reactions, conversations — SQLite with FTS5 search |
| `files/` | Received files from `/send` transfers |
| `exports/` | Exported conversation text files (created by `/export`) |
| `enclave.log` | Client debug log |

The `ENCLAVE_HOME` environment variable overrides the default `~/.enclave/` location.

### Server data (`--data-dir`)

| File | Purpose |
|------|---------|
| `server.pub` / `server.key` | Server's NaCl keypair for challenge-response auth |
| `admin.key` | Admin API key for invite generation |
| `server.crt` / `server-tls.key` | Auto-generated TLS certificate (when `--tls` enabled) |
| `ssh_host_key` | SSH gateway host key (when `--ssh` enabled) |
| `enclave-server.db` | Server SQLite database (users, groups, invite tokens, pending messages) |

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go (pure, no CGO) |
| CLI framework | `spf13/cobra` |
| TUI framework | `charmbracelet/bubbletea` |
| TUI styling | `charmbracelet/lipgloss` |
| TUI components | `charmbracelet/bubbles` (textarea, viewport, list, spinner) |
| SSH gateway | `charmbracelet/wish` |
| Encryption | `golang.org/x/crypto/nacl/box` (X25519 + XSalsa20-Poly1305) |
| Syntax highlighting | `alecthomas/chroma` (200+ languages) |
| WebSocket | `nhooyr.io/websocket` |
| Database | `modernc.org/sqlite` (pure Go, no CGO) |
| QR codes | `skip2/go-qrcode` |
| Config | `BurntSushi/toml` |

## License

MIT
