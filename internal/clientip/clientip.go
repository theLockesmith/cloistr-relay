// Package clientip derives the client address the relay keys rate limits on
// and writes to logs.
//
// It reads X-Real-IP and never X-Forwarded-For. The public edge nginx SETS
// X-Real-IP to the address it accepted the connection from, overwriting
// anything the client sent, but only APPENDS to X-Forwarded-For, so the
// leftmost XFF entry is whatever the client chose. khatru.GetIP and
// khatru.GetIPFromRequest return that leftmost public XFF entry, which
// handed every request a fresh rate-limit budget for the cost of one header.
//
// Without X-Real-IP (in-cluster callers that bypass the edge) the TCP peer
// address is used. The cluster router is not publicly reachable, so a client
// that can set X-Real-IP itself is already inside the cluster.
package clientip

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/fiatjaf/khatru"
)

// FromRequest returns the client IP for r, or "" if none can be determined.
func FromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

// FromContext returns the client IP of the khatru WebSocket connection in
// ctx, or "" outside a connection.
func FromContext(ctx context.Context) string {
	conn := khatru.GetConnection(ctx)
	if conn == nil {
		return ""
	}
	return FromRequest(conn.Request)
}
