# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o relay ./cmd/relay

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates postgresql-client

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/relay .

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -zv localhost 3334 || exit 1

# Default port for Nostr relay
EXPOSE 3334

# Run the relay
ENTRYPOINT ["./relay"]
