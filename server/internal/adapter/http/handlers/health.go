package handlers

import (
	"context"
	"net/http"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/port"
)

// Health handles:
//
//	GET /__heartbeat__   — full check, pings meta + blob,
//	GET /__lbheartbeat__ — minimal, always 200,
//	GET /__version__     — JSON with version/commit.
type Health struct {
	Meta port.MetaStore
	Blob port.BlobStorage
}

// Heartbeat is a fail-fast ping of the subsystems (timeout is left to the
// http server).
func (h *Health) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if err := h.Meta.Ping(r.Context()); err != nil {
		http.Error(w, "meta unhealthy", http.StatusServiceUnavailable)
		return
	}
	if err := h.Blob.Ping(r.Context()); err != nil {
		http.Error(w, "blob unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// LBHeartbeat always returns 200, for load balancers that want a simple ping.
func (h *Health) LBHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Version returns JSON with {version, commit, source}.
func (h *Health) Version(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{
		"version": config.Version,
		"commit":  config.Commit,
		"source":  "https://github.com/sendgo/sendgo",
	})
	_ = context.TODO
}
