package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/logging"
)

// Span represents a trace span for timing operations
type Span struct {
	TraceID   string
	SpanID    string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Status    string
	Attrs     map[string]interface{}
	parent    *Span
	children  []*Span
	mu        sync.Mutex
}

// Tracer manages distributed tracing
type Tracer struct {
	serviceName string
	enabled     bool
	logger      *logging.Logger
}

// Config holds tracer configuration
type Config struct {
	ServiceName string
	Enabled     bool
}

// ctxKey for span context
type ctxKey string

const spanCtxKey ctxKey = "tracing_span"

var (
	globalTracer *Tracer
	once         sync.Once
)

// Init initializes the global tracer
func Init(cfg *Config) {
	once.Do(func() {
		globalTracer = &Tracer{
			serviceName: cfg.ServiceName,
			enabled:     cfg.Enabled,
			logger:      logging.Default().WithComponent("tracing"),
		}
	})
}

// Default returns the global tracer
func Default() *Tracer {
	if globalTracer == nil {
		Init(&Config{ServiceName: "relay", Enabled: true})
	}
	return globalTracer
}

// StartSpan starts a new span
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		SpanID:    generateID(8),
		Name:      name,
		StartTime: time.Now(),
		Attrs:     make(map[string]interface{}),
	}

	// Check for parent span
	if parent, ok := ctx.Value(spanCtxKey).(*Span); ok {
		span.TraceID = parent.TraceID
		span.parent = parent
		parent.mu.Lock()
		parent.children = append(parent.children, span)
		parent.mu.Unlock()
	} else {
		span.TraceID = generateID(16)
	}

	return context.WithValue(ctx, spanCtxKey, span), span
}

// End completes the span and logs it
func (s *Span) End() {
	s.EndTime = time.Now()
	duration := s.EndTime.Sub(s.StartTime)

	if globalTracer != nil && globalTracer.enabled {
		fields := map[string]interface{}{
			"trace_id":    s.TraceID,
			"span_id":     s.SpanID,
			"span_name":   s.Name,
			"duration_ms": float64(duration.Microseconds()) / 1000.0,
		}

		// Add custom attributes
		for k, v := range s.Attrs {
			fields[k] = v
		}

		if s.Status != "" {
			fields["status"] = s.Status
		}

		ctx := logging.WithRequestID(context.Background(), s.TraceID)
		globalTracer.logger.Debug(ctx, "span completed", fields)
	}
}

// SetStatus sets the span status
func (s *Span) SetStatus(status string) {
	s.Status = status
}

// SetAttribute sets a span attribute
func (s *Span) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Attrs[key] = value
}

// SpanFromContext extracts the current span from context
func SpanFromContext(ctx context.Context) *Span {
	if span, ok := ctx.Value(spanCtxKey).(*Span); ok {
		return span
	}
	return nil
}

// TraceID returns the trace ID from context
func TraceID(ctx context.Context) string {
	if span := SpanFromContext(ctx); span != nil {
		return span.TraceID
	}
	return ""
}

// generateID generates a random hex ID
func generateID(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Middleware wraps HTTP handlers with tracing
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := Default()
		ctx, span := tracer.StartSpan(r.Context(), "http.request")
		defer span.End()

		span.SetAttribute("http.method", r.Method)
		span.SetAttribute("http.path", r.URL.Path)
		span.SetAttribute("http.remote_addr", r.RemoteAddr)

		// Add trace ID to response header for debugging
		w.Header().Set("X-Trace-ID", span.TraceID)

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r.WithContext(ctx))

		span.SetAttribute("http.status_code", wrapped.statusCode)
		if wrapped.statusCode >= 400 {
			span.SetStatus("error")
		} else {
			span.SetStatus("ok")
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Package-level convenience functions

// StartSpan starts a new span using the default tracer
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	return Default().StartSpan(ctx, name)
}
