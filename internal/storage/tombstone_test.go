package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/nbd-wtf/go-nostr"
	dto "github.com/prometheus/client_model/go"
)

// These assert ADMISSION as well as refusal. A guard that refused every write would
// satisfy a refusal-only suite while taking the relay offline for everyone.

func TestRejectRepublishedDeletion_AdmitsAnEventNeverDeleted(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, nil
	})(context.Background(), "abc")
	if reject {
		t.Errorf("refused an event that was never deleted: %s", msg)
	}
}

func TestRejectRepublishedDeletion_RefusesARepublish(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return true, nil
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted a republish of a deleted event")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}

// THE DISCRIMINATOR: an unhealthy database must not read as "never deleted".
func TestRejectRepublishedDeletion_LookupErrorRefuses(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, errors.New("something broke")
	})(context.Background(), "abc")
	if !reject {
		t.Error("a failed lookup admitted the write; a broken database would resurrect deleted events")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}

func TestRejectRepublishedDeletion_RetriesOnConnError(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		if calls.Add(1) == 1 {
			return false, &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"}
		}
		return false, nil
	})(context.Background(), "abc")
	if reject {
		t.Error("refused after a successful retry; a Patroni switchover would reject all writes")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (original + retry), got %d", calls.Load())
	}
}

func TestRejectRepublishedDeletion_NoRetryOnNonConnError(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		calls.Add(1)
		return false, &pq.Error{Code: pqerror.InsufficientPrivilege, Message: "permission denied for table deleted_event"}
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted the write on a non-connection error")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry for non-connection errors), got %d", calls.Load())
	}
}

// A mutation that bumps the retry count (e.g. a loop of 5) must be caught.
// An always-failing connection error must produce exactly 2 lookup calls
// (1 original + 1 retry) and the write must be refused.
func TestRejectRepublishedDeletion_ExactlyOneRetry(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		calls.Add(1)
		return false, &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"}
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted the write after all retries failed")
	}
	if calls.Load() != 2 {
		t.Errorf("expected exactly 2 calls (1 original + 1 retry), got %d", calls.Load())
	}
}

// A hung connection must not block the write for minutes. Each attempt is capped
// at tombstoneAttemptTimeout. The lookup blocks forever; the deadline must rescue it.
func TestRejectRepublishedDeletion_DeadlineOnHungConnection(t *testing.T) {
	orig := tombstoneAttemptTimeout
	tombstoneAttemptTimeout = 100 * time.Millisecond
	defer func() { tombstoneAttemptTimeout = orig }()

	lookup := func(ctx context.Context, _ string) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	}

	start := time.Now()
	reject, msg := RejectRepublishedDeletion(lookup)(context.Background(), "abc")
	elapsed := time.Since(start)

	if !reject {
		t.Error("admitted the write when lookup hung")
	}
	// Two attempts (original + retry), each capped at 100ms.
	maxAllowed := 2*tombstoneAttemptTimeout + 100*time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("took %v; each attempt should be capped at %v", elapsed, tombstoneAttemptTimeout)
	}
	if elapsed < tombstoneAttemptTimeout {
		t.Errorf("finished in %v; should have waited at least one timeout (%v)", elapsed, tombstoneAttemptTimeout)
	}
	if !strings.Contains(msg, "timeout") {
		t.Errorf("message should mention timeout cause, got %q", msg)
	}
}

// A client that disconnects before the lookup finishes should not trigger a retry
// or be logged/counted as a database failure.
func TestRejectRepublishedDeletion_ClientGoneNoRetry(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reject, msg := RejectRepublishedDeletion(func(ctx context.Context, _ string) (bool, error) {
		calls.Add(1)
		return false, ctx.Err()
	})(ctx, "abc")

	if !reject {
		t.Error("admitted the write when client disconnected")
	}
	if calls.Load() > 1 {
		t.Errorf("expected at most 1 call (no retry for client-gone), got %d", calls.Load())
	}
	if !strings.HasPrefix(msg, "client-gone:") {
		t.Errorf("message should start with 'client-gone:', got %q", msg)
	}
}

// A timeout on the first attempt should trigger a retry. If the retry succeeds,
// the write is admitted.
func TestRejectRepublishedDeletion_TimeoutRetriedAndSucceeds(t *testing.T) {
	orig := tombstoneAttemptTimeout
	tombstoneAttemptTimeout = 100 * time.Millisecond
	defer func() { tombstoneAttemptTimeout = orig }()

	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(ctx context.Context, _ string) (bool, error) {
		if calls.Add(1) == 1 {
			<-ctx.Done()
			return false, ctx.Err()
		}
		return false, nil
	})(context.Background(), "abc")

	if reject {
		t.Error("refused after a successful retry on timeout")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (original timed out + successful retry), got %d", calls.Load())
	}
}

func TestClassifyTombstoneError(t *testing.T) {
	bgCtx := context.Background()
	canceledCtx, cancel := context.WithCancel(bgCtx)
	cancel()

	for _, tc := range []struct {
		name      string
		parentCtx context.Context
		err       error
		want      string
	}{
		{"client_gone", canceledCtx, context.Canceled, "client_gone"},
		{"timeout", bgCtx, context.DeadlineExceeded, "timeout"},
		{"wrapped_timeout", bgCtx, fmt.Errorf("tombstone lookup abc: %w", context.DeadlineExceeded), "timeout"},
		{"conn_error", bgCtx, &pq.Error{Code: pqerror.AdminShutdown}, "conn_error"},
		{"conn_error_eof", bgCtx, io.EOF, "conn_error"},
		{"57014_query_canceled", bgCtx, &pq.Error{Code: "57014", Message: "canceling statement due to user request"}, "timeout"},
		{"other", bgCtx, errors.New("permission denied"), "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTombstoneError(tc.parentCtx, tc.err)
			if got != tc.want {
				t.Errorf("classifyTombstoneError = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsConnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		// SQLSTATE class 08 -- connection exception
		{
			name: "pq class 08 connection_exception",
			err:  &pq.Error{Code: pqerror.ConnectionException},
			want: true,
		},
		{
			name: "pq class 08 connection_failure",
			err:  &pq.Error{Code: pqerror.ConnectionFailure},
			want: true,
		},
		// SQLSTATE 57P01/57P02/57P03
		{
			name: "pq 57P01 admin_shutdown",
			err:  &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"},
			want: true,
		},
		{
			name: "pq 57P02 crash_shutdown",
			err:  &pq.Error{Code: pqerror.CrashShutdown, Message: "terminating connection due to crash of another server process"},
			want: true,
		},
		{
			name: "pq 57P03 cannot_connect_now (shutting down)",
			err:  &pq.Error{Code: pqerror.CannotConnectNow, Message: "the database system is shutting down"},
			want: true,
		},
		{
			name: "pq 57P03 cannot_connect_now (starting up)",
			err:  &pq.Error{Code: pqerror.CannotConnectNow, Message: "the database system is starting up"},
			want: true,
		},
		// Go-level transport errors
		{
			name: "driver.ErrBadConn",
			err:  driver.ErrBadConn,
			want: true,
		},
		{
			name: "wrapped driver.ErrBadConn",
			err:  fmt.Errorf("tombstone lookup abc: %w", driver.ErrBadConn),
			want: true,
		},
		{
			name: "io.EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "io.ErrUnexpectedEOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "net.OpError",
			err:  &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
			want: true,
		},
		// Non-connection errors
		{
			name: "pq 42501 insufficient_privilege",
			err:  &pq.Error{Code: pqerror.InsufficientPrivilege, Message: "permission denied for table deleted_event"},
			want: false,
		},
		{
			name: "pq 42P01 undefined_table",
			err:  &pq.Error{Code: pqerror.UndefinedTable, Message: "relation \"deleted_event\" does not exist"},
			want: false,
		},
		{
			name: "pq 42601 syntax_error",
			err:  &pq.Error{Code: pqerror.SyntaxError, Message: "syntax error at or near \"SELECT\""},
			want: false,
		},
		{
			name: "generic error (not pq, not transport)",
			err:  errors.New("something unrelated"),
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := isConnError(tc.err)
			if got != tc.want {
				t.Errorf("isConnError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fake PostgreSQL wire-protocol server for integration tests
// ---------------------------------------------------------------------------

// fakePGServer speaks enough PostgreSQL wire protocol to complete a startup
// handshake, then blocks on the query forever -- simulating the exact
// production failure where lib/pq's ctx cancel never unblocks the read.
type fakePGServer struct {
	ln   net.Listener
	t    *testing.T
	done chan struct{}
}

func newFakePGServer(t *testing.T) *fakePGServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return &fakePGServer{ln: ln, t: t, done: make(chan struct{})}
}

func (s *fakePGServer) addr() string { return s.ln.Addr().String() }
func (s *fakePGServer) close() {
	close(s.done)
	_ = s.ln.Close()
}

func (s *fakePGServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *fakePGServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return
	}
	msgLen := int(binary.BigEndian.Uint32(lenBuf))
	rest := make([]byte, msgLen-4)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return
	}

	// CancelRequest: length=16, code=80877102. Just close.
	if msgLen == 16 && binary.BigEndian.Uint32(rest[:4]) == 80877102 {
		return
	}

	// StartupMessage -- complete the handshake.
	// AuthenticationOk
	_, _ = conn.Write([]byte{'R', 0, 0, 0, 8, 0, 0, 0, 0})

	writePGParam(conn, "server_version", "14.0")
	writePGParam(conn, "client_encoding", "UTF8")
	writePGParam(conn, "DateStyle", "ISO, MDY")
	writePGParam(conn, "standard_conforming_strings", "on")
	writePGParam(conn, "integer_datetimes", "on")

	// BackendKeyData
	kd := make([]byte, 13)
	kd[0] = 'K'
	binary.BigEndian.PutUint32(kd[1:5], 12)
	binary.BigEndian.PutUint32(kd[5:9], 1)
	binary.BigEndian.PutUint32(kd[9:13], 1)
	_, _ = conn.Write(kd)

	// ReadyForQuery
	_, _ = conn.Write([]byte{'Z', 0, 0, 0, 5, 'I'})

	// Read the Query message (type byte + length + body).
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, typeBuf); err != nil {
		return
	}
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return
	}
	qLen := int(binary.BigEndian.Uint32(lenBuf)) - 4
	qBuf := make([]byte, qLen)
	if _, err := io.ReadFull(conn, qBuf); err != nil {
		return
	}

	// Block until the server is shut down. The abandon wrapper must return
	// without waiting for this goroutine.
	<-s.done
}

func writePGParam(conn net.Conn, name, value string) {
	payload := append([]byte(name), 0)
	payload = append(payload, []byte(value)...)
	payload = append(payload, 0)
	msg := make([]byte, 1+4+len(payload))
	msg[0] = 'S'
	binary.BigEndian.PutUint32(msg[1:5], uint32(4+len(payload)))
	copy(msg[5:], payload)
	_, _ = conn.Write(msg)
}

// TestRealPQ_AbandonOnDeadline proves, against the real lib/pq driver, that
// the abandon-on-deadline wrapper returns within the timeout even when the
// server never responds to the query. This is the exact scenario where a
// plain context.WithTimeout fails: pq's watchCancel sends a CancelRequest
// on a new socket but never unblocks the blocked read on the original one.
func TestRealPQ_AbandonOnDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real TCP listener")
	}

	orig := tombstoneAttemptTimeout
	tombstoneAttemptTimeout = 300 * time.Millisecond
	defer func() { tombstoneAttemptTimeout = orig }()

	srv := newFakePGServer(t)
	defer srv.close()
	go srv.acceptLoop()

	dsn := fmt.Sprintf("postgres://test:test@%s/test?sslmode=disable&connect_timeout=2", srv.addr())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)

	lookup := func(ctx context.Context, id string) (bool, error) {
		return IsTombstoned(ctx, db, id)
	}

	start := time.Now()
	reject, msg := RejectRepublishedDeletion(lookup)(context.Background(), "test-event-id")
	elapsed := time.Since(start)

	if !reject {
		t.Error("admitted the write when query hung on a dead connection")
	}
	// Two attempts (original + retry), each capped at 300ms, plus overhead.
	maxAllowed := 2*tombstoneAttemptTimeout + 500*time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("took %v; want under %v (two attempts at %v each)", elapsed, maxAllowed, tombstoneAttemptTimeout)
	}
	if elapsed < tombstoneAttemptTimeout {
		t.Errorf("finished in %v; should have waited at least one timeout (%v)", elapsed, tombstoneAttemptTimeout)
	}
	if !strings.Contains(msg, "timeout") {
		t.Errorf("expected timeout cause, got %q", msg)
	}
}

func TestRealPQ_SemaphorePreventPoolStarvation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real TCP listener")
	}

	orig := tombstoneAttemptTimeout
	tombstoneAttemptTimeout = 200 * time.Millisecond
	defer func() { tombstoneAttemptTimeout = orig }()

	srv := newFakePGServer(t)
	go srv.acceptLoop()

	dsn := fmt.Sprintf("postgres://test:test@%s/test?sslmode=disable&connect_timeout=2", srv.addr())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(12)

	lookup := func(ctx context.Context, id string) (bool, error) {
		return IsTombstoned(ctx, db, id)
	}

	// Fire 16 concurrent tombstone lookups against a server that hangs on
	// every query. Without the semaphore all 12 pool conns would be consumed
	// by abandoned goroutines; with it, at most maxInflightTombstoneLookups
	// are held, leaving headroom for other pool users.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			RejectRepublishedDeletion(lookup)(context.Background(), fmt.Sprintf("id-%d", i))
		}(i)
	}
	wg.Wait()

	// Abandoned goroutines are still blocked on the fake server, holding
	// pool connections. The semaphore should have capped how many.
	stats := db.Stats()
	if stats.InUse > maxInflightTombstoneLookups {
		t.Errorf("InUse=%d exceeds semaphore cap %d; pool starvation not prevented",
			stats.InUse, maxInflightTombstoneLookups)
	}

	// Another pool user should still be able to get a connection.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	poolConn, connErr := db.Conn(ctx)
	cancel()
	if connErr != nil {
		t.Errorf("non-tombstone pool user blocked while abandoned lookups hold conns: %v", connErr)
	} else {
		_ = poolConn.Close()
	}

	// Kill the fake server so abandoned goroutines return and release conns.
	srv.close()
	time.Sleep(200 * time.Millisecond)

	stats = db.Stats()
	if stats.InUse != 0 {
		t.Errorf("InUse=%d after server closed; want 0", stats.InUse)
	}
}

func TestRealPQ_BurstWritesAgainstHealthyLookup(t *testing.T) {
	orig := tombstoneAttemptTimeout
	tombstoneAttemptTimeout = 5 * time.Second
	defer func() { tombstoneAttemptTimeout = orig }()

	lookup := func(ctx context.Context, _ string) (bool, error) {
		time.Sleep(20 * time.Millisecond)
		return false, nil
	}

	var wg sync.WaitGroup
	var refused atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reject, _ := RejectRepublishedDeletion(lookup)(context.Background(), fmt.Sprintf("id-%d", i))
			if reject {
				refused.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if refused.Load() != 0 {
		t.Errorf("%d of 20 writes refused against a healthy 20ms lookup; semaphore should not reject healthy traffic",
			refused.Load())
	}
}

// Ephemeral events (20000-29999) are never stored, so there is nothing to
// re-delete. Production 2026-10-29: a NIP-46 signer message (kind 24133) was
// refused after 10s because its tombstone lookup timed out, stalling a login.
func TestRejectRepublishedDeletionEvent_EphemeralSkipsLookup(t *testing.T) {
	for _, kind := range []int{20000, 24133, 29999} {
		var calls atomic.Int32
		reject, msg := RejectRepublishedDeletionEvent(func(context.Context, string) (bool, error) {
			calls.Add(1)
			return false, errors.New("database unavailable")
		})(context.Background(), &nostr.Event{ID: "abc", Kind: kind})
		if reject {
			t.Errorf("kind %d: refused an ephemeral event: %s", kind, msg)
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("kind %d: ran %d tombstone lookups, want 0", kind, n)
		}
	}
}

// Stored kinds on either side of the ephemeral range still get the check.
func TestRejectRepublishedDeletionEvent_StoredKindsStillChecked(t *testing.T) {
	for _, kind := range []int{1, 1059, 19999, 30000} {
		reject, _ := RejectRepublishedDeletionEvent(func(context.Context, string) (bool, error) {
			return true, nil
		})(context.Background(), &nostr.Event{ID: "abc", Kind: kind})
		if !reject {
			t.Errorf("kind %d: admitted a republish of a deleted event", kind)
		}
	}
}

func TestLookupWithDeadline_RecordsDuration(t *testing.T) {
	var m dto.Metric
	if err := tombstoneLookupSeconds.Write(&m); err != nil {
		t.Fatal(err)
	}
	start := m.GetHistogram().GetSampleCount()

	_, _ = lookupWithDeadline(context.Background(), func(context.Context, string) (bool, error) {
		return false, nil
	}, "abc")

	if err := tombstoneLookupSeconds.Write(&m); err != nil {
		t.Fatal(err)
	}
	if got := m.GetHistogram().GetSampleCount() - start; got != 1 {
		t.Errorf("lookup duration samples recorded = %d, want 1", got)
	}
}
