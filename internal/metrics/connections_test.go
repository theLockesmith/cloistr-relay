package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/fiatjaf/khatru"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// nostr_relay_connections_active read -96 on a production pod half an hour
// after it started. A gauge of open sockets can never be negative, so every
// close must be matched by exactly one open, whichever way the client leaves.
func TestConnectionsActive_ReturnsToBaselineAfterEveryKindOfClose(t *testing.T) {
	relay := khatru.NewRelay()
	RegisterRelayMetrics(relay)
	srv := httptest.NewServer(relay)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	base := testutil.ToFloat64(connectionsActive)

	dial := func() *websocket.Conn {
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return c
	}

	var conns []*websocket.Conn
	for i := 0; i < 9; i++ {
		conns = append(conns, dial())
	}
	waitFor(t, func() bool { return testutil.ToFloat64(connectionsActive) == base+9 }, "9 open")

	for i, c := range conns {
		switch i % 3 {
		case 0: // polite close frame
			_ = c.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			_ = c.Close()
		case 1: // TCP drop, no close frame
			_ = c.UnderlyingConn().Close()
		case 2: // garbage then drop
			_ = c.WriteMessage(websocket.TextMessage, []byte("not json"))
			_ = c.Close()
		}
	}
	waitFor(t, func() bool { return testutil.ToFloat64(connectionsActive) == base }, "back to baseline")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s (connections_active=%v)", what, testutil.ToFloat64(connectionsActive))
}
