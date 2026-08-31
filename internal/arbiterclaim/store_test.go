package arbiterclaim

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// A counting stub driver. Dependency-free on purpose: this repo builds
// CGO_ENABLED=0 against a shared CI template, so a cgo database driver added for
// tests is a pipeline risk. The COUNT is the point -- it is what proves that
// non-claim traffic never reaches the database.

var dbCalls struct {
	sync.Mutex
	n       int
	queries []string
}

func resetDBCalls() {
	dbCalls.Lock()
	defer dbCalls.Unlock()
	dbCalls.n = 0
	dbCalls.queries = nil
}

func dbCallCount() int {
	dbCalls.Lock()
	defer dbCalls.Unlock()
	return dbCalls.n
}

type stubDriver struct{}

type stubConn struct{ mode, holder string }

func (stubDriver) Open(dsn string) (driver.Conn, error) {
	c := &stubConn{}
	for _, p := range strings.Split(dsn, "&") {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		switch k {
		case "mode":
			c.mode = v
		case "holder":
			c.holder = v
		}
	}
	return c, nil
}

func (c *stubConn) Prepare(q string) (driver.Stmt, error) { return &stubStmt{c: c, q: q}, nil }
func (c *stubConn) Close() error                          { return nil }
func (c *stubConn) Begin() (driver.Tx, error)             { return nil, io.ErrUnexpectedEOF }

type stubStmt struct {
	c *stubConn
	q string
}

func (s *stubStmt) Close() error                                 { return nil }
func (s *stubStmt) NumInput() int                                { return -1 }
func (s *stubStmt) Exec(_ []driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }

func (s *stubStmt) Query(args []driver.Value) (driver.Rows, error) {
	dbCalls.Lock()
	dbCalls.n++
	dbCalls.queries = append(dbCalls.queries, s.q)
	dbCalls.Unlock()

	switch {
	case strings.Contains(s.q, "INSERT INTO arbiter_claims"):
		switch s.c.mode {
		case "lose":
			return &stubRows{cols: []string{"event_id"}, empty: true}, nil
		case "err":
			return nil, errors.New("stub: database unavailable")
		default: // win: the conditional insert returns our own event id
			var id driver.Value = ""
			if len(args) > 1 {
				id = args[1]
			}
			return &stubRows{cols: []string{"event_id"}, val: id}, nil
		}
	case strings.Contains(s.q, "SELECT event_id"):
		if s.c.holder == "" {
			return &stubRows{cols: []string{"event_id"}, empty: true}, nil
		}
		return &stubRows{cols: []string{"event_id"}, val: s.c.holder}, nil
	}
	return nil, errors.New("stub: unexpected query: " + s.q)
}

type stubRows struct {
	cols  []string
	val   driver.Value
	empty bool
	done  bool
}

func (r *stubRows) Columns() []string { return r.cols }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.empty || r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.val
	return nil
}

func init() { sql.Register("stubclaims", stubDriver{}) }

func storeWith(t *testing.T, dsn string) *Store {
	t.Helper()
	db, err := sql.Open("stubclaims", dsn)
	if err != nil {
		t.Fatalf("open stub: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db, 0)
}

func TestTryClaim_FirstWriterWins(t *testing.T) {
	s := storeWith(t, "mode=win")
	if err := s.TryClaim(Claim{TaskID: "t1", EventID: "aaa", Claimant: "role-a"}); err != nil {
		t.Fatalf("an uncontested claim must succeed, got %v", err)
	}
}

// The load-bearing rejection: a second claim for the same task is REFUSED, and
// the refusal names the winner so the loser can tell a lost race from an outage
// and learns the fencing token.
func TestTryClaim_SecondClaimIsRefusedAndNamesTheWinner(t *testing.T) {
	s := storeWith(t, "mode=lose&holder=aaa")
	err := s.TryClaim(Claim{TaskID: "t1", EventID: "zzz", Claimant: "role-b"})
	if err == nil {
		t.Fatal("a second claim for a held task MUST be refused")
	}
	var lost ErrLost
	if !errors.As(err, &lost) {
		t.Fatalf("refusal must be an ErrLost, got %T: %v", err, err)
	}
	if lost.WinnerE != "aaa" {
		t.Errorf("refusal must carry the winning event id as a fencing token; got %q", lost.WinnerE)
	}
	if !strings.Contains(lost.Error(), "aaa") {
		t.Errorf("error text must name the winner: %q", lost.Error())
	}
}

// A database outage must NOT be reported as a lost race -- that is the exact
// failure/success ambiguity this mechanism exists to remove.
func TestTryClaim_StoreErrorIsDistinguishableFromLosing(t *testing.T) {
	s := storeWith(t, "mode=err")
	err := s.TryClaim(Claim{TaskID: "t1", EventID: "aaa"})
	if err == nil {
		t.Fatal("a store failure must surface as an error")
	}
	var lost ErrLost
	if errors.As(err, &lost) {
		t.Error("a database outage must NOT be reported as ErrLost")
	}
}
