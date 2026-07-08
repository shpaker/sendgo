package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// UploadWS — WS /api/ws. A full port of server/routes/ws.js.
//
// Protocol (kept 1:1 with Node so the frontend `app/api.js` is untouched):
//  1. Client → server: first JSON frame
//     `{fileMetadata: <b64>, authorization: "send-v1 <b64>", timeLimit?, dlimit?, bearer?}`
//  2. Server → client: JSON `{url, ownerToken, id}` or `{error: 400|401|413|500}`
//  3. Client streams binary WS frames (ECE_RECORD_SIZE per frame).
//  4. EOF: single frame of length 1, byte 0x00.
//  5. Server → client: `{ok: true}` and closes the connection.
//
// ResolveBaseURL converts the current *http.Request to the public-facing base
// URL used in the response (`{url: ...}`). The use case is kept pure; URL
// scheme/host inspection lives in the HTTP adapter.
type UploadWS struct {
	Initiate       *usecase.InitiateUpload
	Stream         *usecase.StreamUpload
	ResolveBaseURL func(*http.Request) string
	// Authorize, when non-nil, gates the upload (set by the router when OIDC
	// auth is enabled). It runs after the WS upgrade so the client receives a
	// parseable {"error": 401} frame — a pre-upgrade reject surfaces as a
	// generic ConnectionError in app/api.js instead.
	Authorize func(*http.Request) bool
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 4 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // same-origin enforced router-level
}

func (h *UploadWS) Handle(w http.ResponseWriter, r *http.Request) {
	lg := observability.FromContext(r.Context())
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade writes the HTTP response itself (400 / 500 depending on
		// cause), but the error text is lost. Log here so the cause (e.g.
		// missing WS headers or a broken Hijacker chain) lands in the access
		// log on the same line with the same request_id.
		lg.Warn("ws upgrade failed",
			"err", err.Error(),
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		return
	}
	defer func() { _ = conn.Close() }()

	if h.Authorize != nil && !h.Authorize(r) {
		lg.Warn("ws upload: unauthorized", "remote", r.RemoteAddr)
		writeWSError(conn, http.StatusUnauthorized)
		return
	}

	// --- step 1: first message ---
	_, msg, err := conn.ReadMessage()
	if err != nil {
		lg.Info("ws upload: read first message failed", "err", err)
		return
	}
	var first struct {
		FileMetadata  string `json:"fileMetadata"`
		Authorization string `json:"authorization"`
		TimeLimit     int    `json:"timeLimit"`
		DLimit        int    `json:"dlimit"`
	}
	if err := json.Unmarshal(msg, &first); err != nil {
		lg.Warn("ws upload: invalid first JSON frame", "err", err)
		writeWSError(conn, http.StatusBadRequest)
		return
	}

	// The frontend (app/utils.js:arrayToB64) sends URL-safe base64 without
	// padding; crypto.DecodeB64 accepts any common flavor.
	metaBytes, err := crypto.DecodeB64(first.FileMetadata)
	if err != nil {
		lg.Warn("ws upload: fileMetadata not valid base64", "err", err)
		writeWSError(conn, http.StatusBadRequest)
		return
	}
	authKey, err := crypto.DecodeB64(stripSendV1Prefix(first.Authorization))
	if err != nil || len(authKey) == 0 {
		lg.Warn("ws upload: authorization not valid base64", "err", err, "auth_present", first.Authorization != "")
		writeWSError(conn, http.StatusBadRequest)
		return
	}

	out, err := h.Initiate.Execute(r.Context(), usecase.InitiateUploadInput{
		FileMetadata: metaBytes,
		AuthKey:      authKey,
		TimeLimit:    first.TimeLimit,
		DLimit:       first.DLimit,
		BaseURL:      h.ResolveBaseURL(r),
	})
	if errors.Is(err, domain.ErrBadRequest) {
		// The concrete cause (TTL or dlimit overrun, missing field) lives in
		// err.Error(); without it the failure is undebuggable (closed WS, only
		// status=500 in the access log).
		lg.Warn("ws upload: initiate rejected", "err", err.Error())
		writeWSError(conn, http.StatusBadRequest)
		return
	}
	if err != nil {
		lg.Error("ws upload: initiate failed", "err", err)
		writeWSError(conn, http.StatusInternalServerError)
		return
	}

	// --- step 2: ack ---
	if err := conn.WriteJSON(map[string]string{
		"url":        out.URL,
		"ownerToken": string(out.OwnerToken),
		"id":         string(out.ID),
	}); err != nil {
		lg.Info("ws upload: write ack failed", "err", err)
		return
	}

	// --- step 3-4: stream body into blob ---
	written, err := h.Stream.Execute(r.Context(), out.Share, out.S3Key, newWSReader(conn))
	if errors.Is(err, domain.ErrPayloadTooLarge) {
		writeWSError(conn, http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		lg.Error("ws upload: stream failed", "err", err, "share_id", string(out.ID))
		writeWSError(conn, http.StatusInternalServerError)
		return
	}

	// --- step 5: final ack ---
	if err := conn.WriteJSON(map[string]bool{"ok": true}); err != nil {
		lg.Info("ws upload: write final ack failed", "err", err)
	}
	lg.Info("ws upload done", "share_id", string(out.ID), "size", written)
}

func writeWSError(conn *websocket.Conn, code int) {
	_ = conn.WriteJSON(map[string]int{"error": code})
}

func stripSendV1Prefix(auth string) string {
	const p = "send-v1 "
	if len(auth) > len(p) && auth[:len(p)] == p {
		return auth[len(p):]
	}
	return auth
}

// --- WS → io.Reader adapter ---

// wsReader aggregates a stream of BinaryMessage frames into an io.Reader,
// treating the special "single 0x00 byte" frame as EOF (port of the `eof`
// Transform from the Node code).
//
// Simple implementation: read the whole message (ReadMessage), detect the
// EOF marker before writing to the blob. Adequate for the expected payload
// shape (~64KB per frame).
type wsReader struct {
	conn *websocket.Conn
	buf  *bytes.Reader
	eof  bool
}

func newWSReader(c *websocket.Conn) *wsReader { return &wsReader{conn: c} }

func (r *wsReader) Read(p []byte) (int, error) {
	if r.eof {
		return 0, io.EOF
	}
	if r.buf == nil || r.buf.Len() == 0 {
		mt, data, err := r.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if mt != websocket.BinaryMessage {
			return 0, io.ErrUnexpectedEOF
		}
		// EOF marker: a single-byte frame with value 0.
		if len(data) == 1 && data[0] == 0 {
			r.eof = true
			return 0, io.EOF
		}
		r.buf = bytes.NewReader(data)
	}
	return r.buf.Read(p)
}
