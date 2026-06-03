package domain

import "fmt"

// EncryptedMetadata is opaque ciphertext supplied by the client (base64-decoded).
// The server never attempts to decrypt it — that's the whole point of Send's
// E2E scheme.
type EncryptedMetadata []byte

// AuthKey is the HMAC key supplied by the client (base64-decoded), used for
// the HMAC challenge on download/metadata. Matches what the Node server stores
// in the Redis hash field "auth".
type AuthKey []byte

// Nonce is 16 random bytes, rotated on every successful HMAC check (see
// server/middleware/auth.js:hmac in the original Node code).
type Nonce []byte

// DownloadLimit and TTLSeconds are typed numeric values with basic validation.
type DownloadLimit int

func (d DownloadLimit) Validate(max int) error {
	if int(d) < 1 || int(d) > max {
		return fmt.Errorf("%w: dlimit=%d out of range [1, %d]", ErrBadRequest, d, max)
	}
	return nil
}

type TTLSeconds int

func (t TTLSeconds) Validate(max int) error {
	if int(t) <= 0 || int(t) > max {
		return fmt.Errorf("%w: ttl=%d out of range (0, %d]", ErrBadRequest, t, max)
	}
	return nil
}
