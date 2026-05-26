package handlers

import (
	"net/http"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Config serves GET /config. Returns the JSON the Node code assembles in
// server/clientConstants.js. The response structure is described by
// usecase.ClientConstants (1:1 with frontend expectations).
type Config struct{ Cfg *config.CLI }

func (h *Config) Handle(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, usecase.BuildClientConstants(h.Cfg))
}
