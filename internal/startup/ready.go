package startup

import (
	"net/http"
	"sync/atomic"
)

// ReadyCheck tracks whether the relay is ready to serve traffic. The /ready
// endpoint returns 503 until MarkReady is called, then 200.
type ReadyCheck struct {
	ready atomic.Bool
}

func NewReadyCheck() *ReadyCheck {
	return &ReadyCheck{}
}

func (rc *ReadyCheck) MarkReady() {
	rc.ready.Store(true)
}

func (rc *ReadyCheck) IsReady() bool {
	return rc.ready.Load()
}

func (rc *ReadyCheck) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rc.ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("waiting for database"))
		}
	}
}
