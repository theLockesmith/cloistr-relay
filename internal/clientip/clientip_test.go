package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{
			// The attack: edge nginx overwrites X-Real-IP but appends to XFF,
			// so the leftmost XFF entry is client-chosen.
			name:    "forged XFF is ignored in favour of X-Real-IP",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.66, 198.51.100.7", "X-Real-IP": "198.51.100.7"},
			remote:  "10.128.0.5:41000",
			want:    "198.51.100.7",
		},
		{
			name:    "XFF alone never wins over the TCP peer",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.66"},
			remote:  "10.128.0.5:41000",
			want:    "10.128.0.5",
		},
		{
			name:    "CF-Connecting-IP is not trusted",
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.66", "X-Real-IP": "198.51.100.7"},
			remote:  "10.128.0.5:41000",
			want:    "198.51.100.7",
		},
		{
			name:    "unparseable X-Real-IP falls back to the TCP peer",
			headers: map[string]string{"X-Real-IP": "not-an-ip"},
			remote:  "10.128.0.5:41000",
			want:    "10.128.0.5",
		},
		{
			name:    "IPv6 X-Real-IP is normalised",
			headers: map[string]string{"X-Real-IP": " 2001:DB8::1 "},
			remote:  "10.128.0.5:41000",
			want:    "2001:db8::1",
		},
		{
			name:   "no headers uses RemoteAddr",
			remote: "192.0.2.10:5555",
			want:   "192.0.2.10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := FromRequest(r); got != tt.want {
				t.Errorf("FromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}
