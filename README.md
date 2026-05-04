# grunt

A simple chat protocol for Grugs.

## Features

- WebSocket-based pub/sub messaging
- In-memory SQLite message storage
- JSON wire protocol
- Server-to-client join/leave notifications
- Message history sync endpoint

## Usage

### Server

```bash
grunt server
```

### Client

Send a message:

```bash
grunt send "Hello, Grugs!"
```

Receive messages:

```bash
grunt recv
```

### Sync Endpoint

Retrieve message history:

```bash
curl http://localhost:54765/sync?since=0
```

## Architecture

- `cmd/grunt/` - CLI commands (server, send, recv)
- `internal/server/` - WebSocket hub and HTTP server
- `internal/storage/` - SQLite message storage
- `internal/message/` - Message types and protocol definitions

## Dependencies

- `github.com/gin-gonic/gin` - HTTP routing
- `github.com/gorilla/websocket` - WebSocket support
- `github.com/matoous/go-nanoid/v2` - Client ID generation
- `github.com/spf13/cobra` - CLI parsing
- `modernc.org/sqlite` - In-memory SQLite (pure Go)
