// Package usecase is the application layer. Each function / struct is one
// use case (classic Robert-Martin Clean Architecture "interactor"). Allowed
// dependencies: only domain + port; no imports from adapter/.
package usecase

import (
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
)

// --- Input DTO ---

// InitiateUploadInput — parameters from the first WS message.
// BaseURL is filled by the HTTP adapter (it inspects request scheme + Host
// headers); the use case must not know about *http.Request.
type InitiateUploadInput struct {
	FileMetadata []byte // base64-decoded, opaque ciphertext
	AuthKey      []byte // base64-decoded HMAC key
	TimeLimit    int
	DLimit       int
	BaseURL      string // e.g. "http://127.0.0.1:1443" — already resolved
}

// InitiateUploadOutput is what the WS handler sends back to the client as a
// JSON response.
type InitiateUploadOutput struct {
	ID         domain.ShareID
	OwnerToken domain.OwnerToken
	URL        string // public share URL (`/download/<id>/`)
	S3Key      string // blob key (`{prefix}-{id}`), used by the handler for StreamUpload
	Share      *domain.FileShare
}

// --- Other use case outputs ---

type MetadataOutput struct {
	EncryptedMetadata []byte
	FinalDownload     bool
	TTL               time.Duration
}

type ShareInfoOutput struct {
	DLimit int
	DL     int
	TTL    time.Duration
}

type ExistsOutput struct {
	Exists           bool
	RequiresPassword bool
}
