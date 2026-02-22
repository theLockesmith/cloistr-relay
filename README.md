# cloistr-relay

A Nostr relay implementation using the khatru framework.

## Overview

Cloistr Relay is a high-performance Nostr relay built on the [khatru](https://github.com/fiatjaf/khatru) framework. It provides:

- WebSocket-based Nostr protocol support (NIP-01)
- PostgreSQL event storage backend
- NIP-42 authentication support (placeholder)
- Configurable event acceptance policies
- Basic subscription and filtering

## Quick Start

### Prerequisites

- Go 1.21 or later
- PostgreSQL 12 or later
- Docker and Docker Compose (for containerized setup)

### Local Development

Start the relay and PostgreSQL database:

```bash
docker-compose up
```

The relay will be available at `ws://localhost:3334`

### Configuration

Configure the relay via environment variables:

```bash
cp configs/config.example.env .env
# Edit .env with your settings
docker-compose up
```

Key environment variables:
- `RELAY_PORT` - WebSocket server port (default: 3334)
- `RELAY_NAME` - Relay name for NIP-11 metadata
- `DB_HOST` - PostgreSQL hostname
- `DB_PORT` - PostgreSQL port
- `DB_NAME` - Database name
- `DB_USER` - Database user
- `DB_PASSWORD` - Database password

### Building

Build the relay binary:

```bash
go build -o relay ./cmd/relay
```

Build Docker image:

```bash
docker build -t coldforge-relay:latest .
```

## Testing

Run the test suite:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

## Project Structure

```
.
├── cmd/relay/                 # Entry point
├── internal/
│   ├── config/               # Configuration management
│   ├── relay/                # Relay initialization
│   ├── handlers/             # Event handling logic
│   ├── storage/              # Database backends
│   └── auth/                 # NIP-42 authentication
├── configs/                  # Configuration templates
├── tests/                    # Integration tests
├── Dockerfile                # Container build
└── docker-compose.yml        # Local development setup
```

## NIPs Supported

- **NIP-01**: Basic protocol (events, subscriptions)
- **NIP-42**: User authentication (placeholder implementation)

## Development

See [CLAUDE.md](CLAUDE.md) for development guidelines and agent usage.

## License

See LICENSE file in the repository.
