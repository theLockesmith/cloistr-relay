package membership

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

var errMidRead = errors.New("connection lost mid-read")

type midReadConnector struct {
	cols   []string
	values []driver.Value
}

func (c *midReadConnector) Connect(_ context.Context) (driver.Conn, error) {
	return &midReadConn{cols: c.cols, values: c.values}, nil
}

func (c *midReadConnector) Driver() driver.Driver { return &midReadDriver{} }

type midReadDriver struct{}

func (d *midReadDriver) Open(_ string) (driver.Conn, error) {
	return nil, errors.New("use Connector")
}

type midReadConn struct {
	cols   []string
	values []driver.Value
}

func (c *midReadConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *midReadConn) Close() error              { return nil }
func (c *midReadConn) Begin() (driver.Tx, error) { return nil, errors.New("not implemented") }

func (c *midReadConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &midReadRows{cols: c.cols, values: c.values}, nil
}

var _ driver.QueryerContext = (*midReadConn)(nil)

type midReadRows struct {
	cols   []string
	values []driver.Value
	called int
}

func (r *midReadRows) Columns() []string { return r.cols }
func (r *midReadRows) Close() error      { return nil }

func (r *midReadRows) Next(dest []driver.Value) error {
	r.called++
	if r.called == 1 {
		copy(dest, r.values)
		return nil
	}
	return errMidRead
}

func TestScanMembers_MidReadError(t *testing.T) {
	db := sql.OpenDB(&midReadConnector{
		cols: []string{"pubkey", "joined_at", "invite_code", "added_by", "tier", "tier_expires_at", "lightning_address"},
		values: []driver.Value{
			"fake_pk", time.Now(), "inv", "added", "free", nil, "ln@example.com",
		},
	})
	defer func() { _ = db.Close() }()

	store := NewStore(db)
	members, err := store.ListMembers(context.Background())
	if err == nil {
		t.Fatalf("ListMembers returned %d members, nil error; want errMidRead", len(members))
	}
	if !errors.Is(err, errMidRead) {
		t.Fatalf("ListMembers error = %v; want errMidRead", err)
	}
}

func TestScanPayments_MidReadError(t *testing.T) {
	db := sql.OpenDB(&midReadConnector{
		cols: []string{"payment_hash", "pubkey", "tier", "amount_sats", "period_days", "status", "created_at", "settled_at"},
		values: []driver.Value{
			"hash1", "pk1", "basic", int64(1000), int64(30), "pending", time.Now(), nil,
		},
	})
	defer func() { _ = db.Close() }()

	pstore := NewPaymentStore(db)
	payments, err := pstore.ListPending(context.Background())
	if err == nil {
		t.Fatalf("ListPending returned %d payments, nil error; want errMidRead", len(payments))
	}
	if !errors.Is(err, errMidRead) {
		t.Fatalf("ListPending error = %v; want errMidRead", err)
	}
}
