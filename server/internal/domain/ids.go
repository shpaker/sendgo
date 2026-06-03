package domain

import "regexp"

// ShareID is the public share identifier. 8 random bytes hex = 16 characters
// (port of crypto.randomBytes(8).toString('hex') from server/routes/ws.js).
// We validate a 10..16 hex range to accept legacy links (the frontend's
// app/utils.js:isFile uses the regex ^[0-9a-fA-F]{10,16}$).
type ShareID string

// OwnerToken is longer: 10 random bytes hex = 20 characters
// (crypto.randomBytes(10).toString('hex')).
type OwnerToken string

var shareIDRe = regexp.MustCompile(`^[0-9a-fA-F]{10,16}$`)

// ValidShareID checks the ID format at HTTP handler entry.
func ValidShareID(s string) bool {
	return shareIDRe.MatchString(s)
}
