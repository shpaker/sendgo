// Package crypto contains TokenGenerator implementations and HMAC helpers.
// Server-side cryptography only: HMAC verification of the downloader challenge
// and generation of public identifiers. Content encryption happens on the
// client — the server never touches it.
package crypto

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"

	"github.com/sendgo/sendgo/server/internal/domain"
)

// ComputeHMAC returns HMAC-SHA256(authKey, nonce). Used in the auth middleware
// (port of server/middleware/auth.js:hmac).
func ComputeHMAC(authKey, nonce []byte) []byte {
	mac := hmac.New(sha256.New, authKey)
	mac.Write(nonce)
	return mac.Sum(nil)
}

// VerifyHMAC performs a constant-time comparison of HMAC(auth, nonce) against
// the supplied signature.
func VerifyHMAC(authKey, nonce, supplied []byte) bool {
	expected := ComputeHMAC(authKey, nonce)
	return subtle.ConstantTimeCompare(expected, supplied) == 1
}

// CompareTokens does a constant-time comparison of two tokens of arbitrary
// length. Used for owner-token verification. Returns false when lengths differ
// (subtle.ConstantTimeCompare already handles this, but we duplicate the
// check for clarity).
func CompareTokens(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// --- TokenGenerator implementation ---

// RandomTokens is the production implementation of the TokenGenerator port,
// backed by crypto/rand.
type RandomTokens struct{}

// ShareID returns 8 random bytes encoded as 16 hex characters (port of
// `crypto.randomBytes(8).toString('hex')`).
func (RandomTokens) ShareID() (domain.ShareID, error) {
	b := make([]byte, 8)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("share id rand: %w", err)
	}
	return domain.ShareID(hex.EncodeToString(b)), nil
}

// OwnerToken returns 10 random bytes encoded as 20 hex characters.
func (RandomTokens) OwnerToken() (domain.OwnerToken, error) {
	b := make([]byte, 10)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("owner token rand: %w", err)
	}
	return domain.OwnerToken(hex.EncodeToString(b)), nil
}

// Nonce returns 16 random bytes. Returned raw; base64 encoding happens in the
// HTTP layer.
func (RandomTokens) Nonce() ([]byte, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return nil, fmt.Errorf("nonce rand: %w", err)
	}
	return b, nil
}
