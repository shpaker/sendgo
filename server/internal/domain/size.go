package domain

// ECE record framing constants — port of app/ece.js / app/utils.js:267.
// The Send client encrypts the stream with AES-GCM in records of ECERecordSize,
// each adding 16 bytes of tag and 1 delimiter byte; the stream is prefixed
// with a 21-byte ECE header. The server does not encrypt anything itself, but
// must know the formula to enforce MAX_FILE_SIZE in encrypted bytes.
const (
	ECERecordSize = 64 * 1024
	ECETagLength  = 16
)

// EncryptedSize returns the number of bytes the encrypted stream will occupy
// for a given plain size. Direct port of app/utils.js:267 (encryptedSize),
// used by the WS uploader to size the io.LimitReader.
func EncryptedSize(plainSize int64) int64 {
	if plainSize <= 0 {
		return 21
	}
	chunkMeta := int64(ECETagLength + 1)
	chunks := (plainSize + (ECERecordSize - chunkMeta - 1)) / (ECERecordSize - chunkMeta)
	return 21 + plainSize + chunkMeta*chunks
}

// DayPrefix is a port of `Math.max(Math.floor(seconds / 86400), 1)` from
// server/storage/index.js. Used as an S3 key prefix so the lifecycle policy
// can group objects into "days-until-expiration" buckets.
func DayPrefix(expireSeconds int) int {
	days := expireSeconds / 86400
	if days < 1 {
		return 1
	}
	return days
}
