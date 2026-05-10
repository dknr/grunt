# grunt

A simple chat protocol for Grugs.

## Features

- SSE-based pub/sub messaging (Server-Sent Events)
- SQLite message storage (in-memory or on-disk)
- JSON wire protocol
- Server-to-client join/leave notifications
- Message history sync endpoint
- REST API for sending messages

## Usage

### Server

```bash
grunt serve                           # in-memory database (default)
grunt serve /path/to/database.sqlite  # on-disk database
```

### Client

Send a message:

```bash
GRUNT_LOGIN=user:pass grunt send "Hello, Grugs!" --invite-code CODE
```

Receive messages:

```bash
GRUNT_LOGIN=user:pass grunt recv --invite-code CODE
```

### Deno Client

A Deno-based send client is also available:

```bash
GRUNT_LOGIN=user:pass deno run --allow-env --allow-net deno-client/send.ts "Hello, Grugs!" --invite-code CODE
```

### Sync Endpoint

Retrieve message history:

```bash
curl http://localhost:54765/api/chat/sync?since=0
```

## Architecture

- `cmd/grunt/` - CLI commands (server, send, recv, repl)
- `server/` - SSE hub and HTTP server
- `server/storage/` - SQLite message storage
- `client/` - Go client library

## Dependencies

- `github.com/matoous/go-nanoid/v2` - Client ID generation
- `github.com/spf13/cobra` - CLI parsing
- `modernc.org/sqlite` - In-memory SQLite (pure Go)

