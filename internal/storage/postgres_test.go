package storage

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

// TestTunedConnector_ConnectTimeout proves that a pool created via
// newTunedConnector enforces the connect timeout. The server accepts TCP
// connections but never responds to the PG startup message, so without
// connect_timeout the handshake blocks for the full TCP retransmission
// timeout (~15 min on Linux defaults).
func TestTunedConnector_ConnectTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real TCP listener")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { <-make(chan struct{}); _ = c.Close() }(conn)
		}
	}()

	orig := tunedConnectTimeout
	tunedConnectTimeout = 500 * time.Millisecond
	defer func() { tunedConnectTimeout = orig }()

	dsn := "postgres://test:test@" + ln.Addr().String() + "/test?sslmode=disable"
	connector, err := newTunedConnector(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()

	start := time.Now()
	err = db.Ping()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected connection to fail against a server that never responds")
	}
	if elapsed > 3*time.Second {
		t.Errorf("connect_timeout did not fire: Ping took %v (want ~500ms)", elapsed)
	}
	t.Logf("Ping failed in %v: %v", elapsed, err)
}

// TestTunedConnector_HandshakeSucceeds proves the tuned connector's dialer
// is wired correctly by connecting to the fakePGServer. If the dialer were
// missing or misconfigured, the handshake would fail.
func TestTunedConnector_HandshakeSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real TCP listener")
	}

	srv := newFakePGServer(t)
	defer srv.close()
	go srv.acceptLoop()

	dsn := "postgres://test:test@" + srv.addr() + "/test?sslmode=disable"
	connector, err := newTunedConnector(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	// db.Conn() opens a connection (handshake only, no query) and returns
	// a handle. This proves the tuned connector's dialer reaches the
	// server and the PG handshake completes.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	conn, err := db.Conn(ctx)
	cancel()
	if err != nil {
		t.Fatalf("tuned connector failed to establish connection: %v", err)
	}
	_ = conn.Close()

	stats := db.Stats()
	if stats.OpenConnections == 0 {
		t.Error("no connections opened; tuned dialer may not be wired into the connector")
	}
	t.Logf("connection established via tuned connector; OpenConnections=%d", stats.OpenConnections)
}

// TestNewPostgresBackend_ConnectTimeout exercises the actual NewPostgresBackend
// code path (not newTunedConnector directly). A silent TCP server accepts the
// connection but never responds to the PG startup. With the tuned connector
// this fails in ~500ms; if NewPostgresBackend were reverted to plain
// sqlx.Connect, it would hang for the full TCP retransmission timeout.
func TestNewPostgresBackend_ConnectTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real TCP listener")
	}

	// Own deadline: if the tuned connector is removed, NewPostgresBackend
	// blocks for the full TCP retransmission timeout. This kills the test
	// in 3s rather than waiting for the package timeout.
	timer := time.AfterFunc(3*time.Second, func() {
		panic("TestNewPostgresBackend_ConnectTimeout: NewPostgresBackend did not return within 3s -- tuned connector may be missing")
	})
	defer timer.Stop()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { <-make(chan struct{}); _ = c.Close() }(conn)
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}

	orig := tunedConnectTimeout
	tunedConnectTimeout = 500 * time.Millisecond
	defer func() { tunedConnectTimeout = orig }()

	cfg := &config.Config{
		DBHost:            host,
		DBPort:            p,
		DBName:            "test",
		DBUser:            "test",
		DBPassword:        "test",
		DBMaxOpenConns:    2,
		DBMaxIdleConns:    1,
		DBConnMaxLifetime: time.Minute,
		DBConnMaxIdleTime: time.Minute,
	}

	start := time.Now()
	_, err = NewPostgresBackend(cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected NewPostgresBackend to fail against a server that never responds")
	}
	if elapsed > 3*time.Second {
		t.Errorf("connect_timeout did not fire: NewPostgresBackend took %v (want ~500ms)", elapsed)
	}
	t.Logf("NewPostgresBackend failed in %v: %v", elapsed, err)
}
