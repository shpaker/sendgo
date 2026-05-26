package crypto

import (
	"encoding/base64"
	"strings"
)

// DecodeB64 decodes base64 in any common format: URL-safe or standard, with or
// without padding. The Send client (see app/utils.js:arrayToB64) sends URL-safe
// without padding; the legacy Node server accepted any flavor via
// Buffer.from(str, 'base64'), so we preserve that tolerance.
func DecodeB64(s string) ([]byte, error) {
	// Normalize: strip any existing padding, then try without it.
	noPad := strings.TrimRight(s, "=")
	// 1) URL-safe without padding — the primary frontend format.
	if b, err := base64.RawURLEncoding.DecodeString(noPad); err == nil {
		return b, nil
	}
	// 2) Standard without padding.
	if b, err := base64.RawStdEncoding.DecodeString(noPad); err == nil {
		return b, nil
	}
	// 3) Pad up to a multiple of 4 and try the padded variants.
	padded := noPad + "==="[:(4-len(noPad)%4)%4]
	if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	// 4) Standard with padding — last attempt; its error propagates out.
	return base64.StdEncoding.DecodeString(padded)
}

// EncodeB64 encodes bytes as URL-safe base64 without padding — the format
// `app/utils.js:b64ToArray` on the frontend expects. base64-js also accepts
// the standard variant, but URL-safe aligns better with what the frontend
// itself produces in the opposite direction.
func EncodeB64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
