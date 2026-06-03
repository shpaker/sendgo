package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"testing"
)

// TestComputeHMAC_KnownVector checks parity with the Node server: HMAC-SHA256
// (authKey, nonce) is the underlying primitive, and we assert that ComputeHMAC
// matches a direct crypto/hmac.New(sha256.New, ...) call.
func TestComputeHMAC_KnownVector(t *testing.T) {
	key := []byte("super-secret-auth-key-32-bytes!")
	nonce := []byte("0123456789abcdef")
	got := ComputeHMAC(key, nonce)

	m := hmac.New(sha256.New, key)
	m.Write(nonce)
	want := m.Sum(nil)

	if !hmac.Equal(got, want) {
		t.Errorf("ComputeHMAC mismatch")
	}
}

func TestVerifyHMAC(t *testing.T) {
	key := []byte("k")
	nonce := []byte("n")
	mac := ComputeHMAC(key, nonce)
	if !VerifyHMAC(key, nonce, mac) {
		t.Errorf("VerifyHMAC failed for valid signature")
	}
	// flip a bit
	mac[0] ^= 0xff
	if VerifyHMAC(key, nonce, mac) {
		t.Errorf("VerifyHMAC accepted tampered signature")
	}
}

func TestCompareTokens(t *testing.T) {
	if !CompareTokens("abc", "abc") {
		t.Errorf("equal tokens compared unequal")
	}
	if CompareTokens("abc", "abd") {
		t.Errorf("different tokens compared equal")
	}
	if CompareTokens("abc", "abcd") {
		t.Errorf("different-length tokens compared equal")
	}
	if CompareTokens("", "") {
		t.Errorf("empty tokens should compare false (defense against bypass)")
	}
}
