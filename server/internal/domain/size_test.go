package domain

import "testing"

// TestEncryptedSize_Vectors — known values from app/utils.js:encryptedSize.
// These numbers are cross-checked against the Node implementation via `node -e`
// (port of the formula 1:1).
func TestEncryptedSize_Vectors(t *testing.T) {
	cases := []struct {
		plain int64
		want  int64
	}{
		{0, 21},          // edge case: empty plaintext
		{1, 21 + 1 + 17}, // one 1-byte record + 17 bytes of overhead
		{65519, 21 + 65519 + 17},
		{65520, 21 + 65520 + 17*2}, // exactly two records
		{1024 * 1024, encryptedSizeJS(1024 * 1024)},
		{10 * 1024 * 1024, encryptedSizeJS(10 * 1024 * 1024)},
	}
	for _, c := range cases {
		got := EncryptedSize(c.plain)
		if got != c.want {
			t.Errorf("EncryptedSize(%d) = %d, want %d", c.plain, got, c.want)
		}
	}
}

// encryptedSizeJS is a direct port of the JS formula used for cross-checking.
// If EncryptedSize ever diverges from this version it would mean the
// contract with the frontend is broken.
func encryptedSizeJS(plain int64) int64 {
	const rs = 64 * 1024
	const tag = 16
	chunkMeta := int64(tag + 1)
	chunks := (plain + (rs - chunkMeta - 1)) / (rs - chunkMeta)
	return 21 + plain + chunkMeta*chunks
}

func TestDayPrefix(t *testing.T) {
	cases := map[int]int{
		300:    1, // 5 minutes → prefix 1 (minimum)
		3600:   1,
		86400:  1, // exactly one day
		86401:  1, // just over one day
		172800: 2, // two days
		604800: 7, // a week
	}
	for in, want := range cases {
		if got := DayPrefix(in); got != want {
			t.Errorf("DayPrefix(%d) = %d, want %d", in, got, want)
		}
	}
}
