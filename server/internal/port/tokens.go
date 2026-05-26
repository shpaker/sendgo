package port

import "github.com/sendgo/sendgo/server/internal/domain"

// TokenGenerator produces secret values: share id, owner token, nonce. The
// production implementation in `internal/crypto/tokens.go` uses crypto/rand;
// tests swap in a deterministic implementation.
type TokenGenerator interface {
	ShareID() (domain.ShareID, error)       // 8 bytes hex = 16 chars
	OwnerToken() (domain.OwnerToken, error) // 10 bytes hex = 20 chars
	Nonce() ([]byte, error)                 // 16 random bytes
}
