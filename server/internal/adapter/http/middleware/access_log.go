package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
)

// AccessLog logs each request in a Grafana/Loki-friendly format: method,
// path, status, duration, size. Uses the logger from context so the line
// includes the request_id.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(rec, r)
		dur := time.Since(start)
		lg := observability.FromContext(r.Context())
		lg.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes_out", rec.bytes,
			"duration_ms", dur.Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusRecorder intercepts WriteHeader and counts response bytes. Implements
// http.ResponseWriter plus the interfaces needed for WebSocket upgrade
// (http.Hijacker) and streaming (http.Flusher), so handlers don't break.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(p)
	s.bytes += n
	return n, err
}

// Flush is for streaming handlers (download).
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack is required for WebSocket upgrade (gorilla/websocket type-asserts
// ResponseWriter to http.Hijacker and hijacks the TCP connection). Without
// this, upgrade returns 500 "response does not implement http.Hijacker" —
// which is exactly what happened before this fix.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter is not a Hijacker")
	}
	conn, rw, err := h.Hijack()
	if err == nil {
		// After Hijack there are no further WriteHeader/Write calls through us.
		// Treat the status as 101 (Switching Protocols) so the access log shows
		// the right metric.
		if !s.wrote {
			s.status = http.StatusSwitchingProtocols
			s.wrote = true
		}
	}
	return conn, rw, err
}
