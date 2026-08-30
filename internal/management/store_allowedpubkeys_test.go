package management

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

// The allow-list gate is the difference between a relay that restricts writes and
// a relay that rejects all of them, so it is tested against a stub driver rather
// than left to integration. A stub keeps this dependency-free: the production
// build is CGO_ENABLED=0 and CI runs a shared template, so pulling in a cgo sqlite
// driver purely for tests risks breaking the pipeline.

type stubDriver struct{}

// stubConn answers the two queries IsPubkeyAllowed issues. Behaviour comes from
// the DSN ("count=<n>&has=<pubkey>") so tests stay independent of each other.
type stubConn struct {
	count int
	has   string
}

func (stubDriver) Open(dsn string) (driver.Conn, error) {
	c := &stubConn{}
	for _, part := range strings.Split(dsn, "&") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "count":
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("bad count %q: %w", v, err)
			}
			c.count = n
		case "has":
			c.has = v
		}
	}
	return c, nil
}

func (c *stubConn) Prepare(q string) (driver.Stmt, error) { return &stubStmt{conn: c, q: q}, nil }
func (c *stubConn) Close() error                          { return nil }
func (c *stubConn) Begin() (driver.Tx, error)             { return nil, io.ErrUnexpectedEOF }

type stubStmt struct {
	conn *stubConn
	q    string
}

func (s *stubStmt) Close() error                                 { return nil }
func (s *stubStmt) NumInput() int                                { return -1 }
func (s *stubStmt) Exec(_ []driver.Value) (driver.Result, error) { return nil, io.ErrUnexpectedEOF }

func (s *stubStmt) Query(args []driver.Value) (driver.Rows, error) {
	switch {
	case strings.Contains(s.q, "COUNT(*)"):
		return &stubRows{cols: []string{"count"}, val: int64(s.conn.count)}, nil
	case strings.Contains(s.q, "EXISTS"):
		want := ""
		if len(args) > 0 {
			want, _ = args[0].(string)
		}
		return &stubRows{cols: []string{"exists"}, val: want != "" && want == s.conn.has}, nil
	}
	return nil, fmt.Errorf("stub driver got unexpected query: %s", s.q)
}

type stubRows struct {
	cols []string
	val  driver.Value
	done bool
}

func (r *stubRows) Columns() []string { return r.cols }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.val
	return nil
}

func init() { sql.Register("stubpg", stubDriver{}) }

func storeWith(t *testing.T, dsn string) *Store {
	t.Helper()
	db, err := sql.Open("stubpg", dsn)
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db)
}

const (
	ownerKey     = "ac16282f720514d926a57b5c13f02d1f4e32bd6fe3e00f713f50964571685f62"
	discoveryKey = "532aceee51a63b3a7a242aca4e0b79f57352046b8743d0ea1833d135d2034ce6"
)

// An EMPTY allow list must mean UNRESTRICTED. If this regresses, the gate
// registered in RegisterBanHandlers rejects every write on any relay whose
// operator never used the feature -- it takes the relay offline.
func TestIsPubkeyAllowed_EmptyListAllowsEveryone(t *testing.T) {
	s := storeWith(t, "count=0")
	for _, pk := range []string{ownerKey, discoveryKey, ""} {
		if !s.IsPubkeyAllowed(pk) {
			t.Errorf("empty allow list must permit %q, but it was rejected", pk)
		}
	}
}

// Once populated the list is RESTRICTIVE -- what pubkeys.html promises.
func TestIsPubkeyAllowed_PopulatedListRestricts(t *testing.T) {
	s := storeWith(t, "count=1&has="+ownerKey)
	if !s.IsPubkeyAllowed(ownerKey) {
		t.Error("a listed pubkey must be allowed")
	}
	if s.IsPubkeyAllowed(discoveryKey) {
		t.Error("an unlisted pubkey must be rejected once the list is populated")
	}
}

// A database error surfacing at the count query (here a connection failure, which
// database/sql defers to first use) must degrade to unrestricted, not to a total
// write outage. Mirrors IsKindAllowed.
func TestIsPubkeyAllowed_CountErrorFailsOpen(t *testing.T) {
	s := storeWith(t, "count=notanumber")
	if !s.IsPubkeyAllowed(discoveryKey) {
		t.Error("a database error on the count query must fail open, not reject writes")
	}
}
