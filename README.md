# Enclave

Secure, end-to-end encrypted chat for developers. Fully on-prem. Zero cloud dependencies.

Enclave is a TUI (terminal UI) chat application that runs in your terminal alongside your editor and tools. The server only routes opaque encrypted blobs — messages are encrypted client-side using NaCl box (X25519 + XSalsa20-Poly1305) and the server never sees plaintext. Everything runs on your own hardware. One binary, no accounts, no cloud.

## Features

- **End-to-end encrypted** — Messages encrypted client-side before leaving your machine
- **Fully on-prem** — Server and clients run on your own hardware, zero cloud dependencies
- **Single binary** — One executable serves as both server and client
- **Rich TUI** — Split-pane interface with contacts, chat viewport, input, and status bar
- **Syntax highlighting** — Fenced code blocks render with full syntax highlighting (200+ languages)
- **Inline code** — Backtick-wrapped `code` renders with a distinct background
- **Code block copy** — Numbered code blocks with `/copy N` to yank to clipboard
- **Slash commands** — Type `/` for autocomplete command palette
- **Message persistence** — Chat history survives restarts, stored locally in SQLite
- **Full-text search** — `/search <query>` across all conversations
- **Typing indicators** — See when contacts are typing in real-time
- **Presence** — Online/offline status with visual indicators
- **Offline delivery** — Messages queue on the server when recipients are offline
- **QR code export** — Share your public key as a scannable QR code in the terminal
- **Color themes** — Dark (Tokyo Night), Light, Dracula, and Nord
- **TLS support** — Optional encrypted transport with auto-generated self-signed certs
- **Challenge-response auth** — No passwords; identity is your keypair
- **Invite-only access** — Users must receive a one-time token to register
- **Cross-platform** — Builds for Linux, macOS, and Windows (amd64 and arm64)

## Quick Start

### 1. Build

```bash
git clone <repo-url> && cd enclave
make build
```

### 2. Start the server

```bash
./build/enclave serve --bind 0.0.0.0:9300 --data-dir ./server-data
```

The server prints an **admin key** on startup — this is used to generate invite tokens. It's saved to `server-data/admin.key` and persists across restarts.

### 3. Generate invite tokens

From any machine that can reach the server:

```bash
./build/enclave invite --server 192.168.1.50:9300 --admin-key <admin-key>
```

Share the printed token with each person you want to invite out-of-band (in person, phone call, etc). Tokens are one-time use and expire after 72 hours by default.

### 4. Users initialize and register

Each user runs:

```bash
./build/enclave init --display-name alice --server 192.168.1.50:9300 --token <invite-token>
```

This generates their X25519 keypair, registers their public key with the server, and saves config to `~/.enclave/`. Registration happens during `init` — users don't need the token again.

### 5. Chat

```bash
./build/enclave chat
```

Use `Tab` to switch between the contact sidebar, chat viewport, and message input. Type `/` for the command palette.

### Local testing

To test with two users locally:

```bash
./scripts/local-test.sh
```

This starts a server, generates two invite tokens, registers alice and bob, and prints the commands to open their chat sessions in separate terminals.

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
| `/verify` | Open contact detail overlay with key fingerprint and visual ID for verification |
| `/users` | List all registered users with online/offline status |
| `/copy [N]` | Copy code block N to clipboard (`/copy` copies the most recent block) |
| `/search <query>` | Full-text search across all message history |
| `/export` | Save the current conversation to `~/.enclave/exports/` as a text file |
| `/quit` | Exit Enclave |

## Keybindings

| Key | Action |
|-----|--------|
| `Tab` | Cycle focus: sidebar → input → chat viewport |
| `Enter` | Send message (input) / select contact (sidebar) |
| `Ctrl+Enter` | Insert newline in message |
| `Ctrl+J` | Insert newline (alternative) |
| `Up/Down` or `j/k` | Navigate contacts (sidebar) / scroll messages (chat viewport) |
| `PgUp/PgDn` | Scroll messages by page |
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
│  │ (plaintext history)   │     │   (plaintext history) │  │
│  └───────────────┐       │     │      ┌───────────────┘  │
└──────────────────┼───────┘     └──────┼──────────────────┘
                   │   WebSocket (ws/wss) │
                   │   ┌────────────┐   │
                   └──►│  enclave   │◄──┘
                       │  serve     │
                       │            │
                       │ routes     │
                       │ opaque     │
                       │ ciphertext │
                       │            │
                       │ server     │
                       │ SQLite     │
                       │ (encrypted │
                       │  blobs     │
                       │  only)     │
                       └────────────┘
```

**The server never sees plaintext.** It routes encrypted blobs between connected clients and stores them for offline delivery. Even a fully compromised server reveals nothing about message content — it only sees who is talking to whom and when.

## Encryption

| Property | Detail |
|----------|--------|
| Key exchange | X25519 (Curve25519 Diffie-Hellman) |
| Authenticated encryption | XSalsa20-Poly1305 (NaCl box) |
| Nonce | 24 bytes, randomly generated per message |
| Implementation | `golang.org/x/crypto/nacl/box` |
| Key storage | `~/.enclave/identity.key` (32 bytes raw, file mode 0600) |
| Client refuses to run if private key permissions are too open (like SSH) |

### What the server sees

- Who is talking to whom (public keys in message envelopes)
- Message sizes and timestamps
- Online/offline presence

### What the server cannot see

- Message content (encrypted with keys the server doesn't possess)
- Users' private keys

## Authentication

Enclave uses a three-layer authentication model with no passwords:

1. **Invite token** — One-time, time-limited token generated by the admin. Required for registration. Hashed (SHA-256) before storage.
2. **Challenge-response** — On each connection, the server sends a random nonce. The client proves identity by encrypting it with their private key and the server's public key (NaCl box). The server verifies using the client's registered public key.
3. **Admin API key** — A random 32-byte key generated on first server start, required for the invite generation endpoint. Stored in `data-dir/admin.key`.

## Building

```bash
make build          # Build for current platform (stripped, ~17MB)
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
- Server authenticates users via cryptographic challenge-response — no passwords
- Invite tokens are one-time use, time-limited (default 72h), and hashed before storage
- Admin API is protected by a random key generated on first server start
- Private keys stored with 0600 permissions; the client refuses to run if permissions are too open
- TLS support for encrypted transport (optional, layered on top of E2E encryption)
- Server generates ECDSA P-256 self-signed certificates automatically when TLS is enabled
- Message content is never logged, stored, or accessible server-side

## Data

### Client data (`~/.enclave/`)

| File | Purpose |
|------|---------|
| `config.toml` | Server address, display name, TLS setting, registration status |
| `identity.key` | NaCl private key (32 bytes, mode 0600) |
| `identity.pub` | NaCl public key (32 bytes) |
| `contacts/` | Directory of saved contact public keys |
| `messages.db` | Local message history — SQLite with FTS5 full-text search |
| `exports/` | Exported conversation text files (created by `/export`) |
| `enclave.log` | Client debug log |

The `ENCLAVE_HOME` environment variable overrides the default `~/.enclave/` location.

### Server data (`--data-dir`)

| File | Purpose |
|------|---------|
| `server.pub` / `server.key` | Server's NaCl keypair for challenge-response auth |
| `admin.key` | Admin API key for invite generation |
| `server.crt` / `server-tls.key` | Auto-generated TLS certificate (when `--tls` enabled) |
| `enclave-server.db` | Server SQLite database (users, invite tokens, pending messages) |

## License

MIT
