package domain

// FileShare is the root entity of a single share. Fields mirror the Redis hash
// schema of the current Node code 1:1 (see server/storage/index.js around
// storage.set). The server stores only the client-encrypted payload plus a
// handful of server metadata (nonce, owner, dl, dlimit, prefix, pwd).
type FileShare struct {
	ID                ShareID
	Owner             OwnerToken
	EncryptedMetadata EncryptedMetadata
	AuthKey           AuthKey
	Nonce             Nonce
	Password          bool // in Redis this is the "pwd" field with value "true"/"false"
	DLimit            int
	DL                int // download count
	ExpireSeconds     int // originally requested TTL (used by /info and similar)
	Prefix            int // day-bucket prefix
	SizeBytes         int64
}

// IsFinalDownload is the client-facing flag we return in /api/metadata. Matches
// the Node behavior `meta.dl + 1 === meta.dlimit`.
func (s *FileShare) IsFinalDownload() bool {
	return s.DL+1 == s.DLimit
}

// ReachedLimit reports whether the counter has hit the limit after increment.
func (s *FileShare) ReachedLimit() bool {
	return s.DL >= s.DLimit
}
