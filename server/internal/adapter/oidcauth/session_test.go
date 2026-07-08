package oidcauth

import (
	"strings"
	"testing"
	"time"
)

func testCodec() codec {
	return codec{secret: []byte("0123456789abcdef0123456789abcdef")}
}

func TestCodec_RoundTrip(t *testing.T) {
	c := testCodec()
	in := Session{Sub: "user-1", Email: "u@example.com", Name: "U", Iat: 100, Exp: 200}
	val, err := c.encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out Session
	if err := c.decode(val, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, in)
	}
}

func TestCodec_TamperedPayloadRejected(t *testing.T) {
	c := testCodec()
	val, err := c.encode(Session{Sub: "user-1", Exp: 200})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	body, sig, _ := strings.Cut(val, ".")
	// Flip a payload character while keeping the original signature.
	tampered := body[:len(body)-1] + flip(body[len(body)-1]) + "." + sig
	var out Session
	if err := c.decode(tampered, &out); err == nil {
		t.Error("tampered payload accepted")
	}
}

func TestCodec_TamperedSignatureRejected(t *testing.T) {
	c := testCodec()
	val, err := c.encode(Session{Sub: "user-1", Exp: 200})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	body, sig, _ := strings.Cut(val, ".")
	tampered := body + "." + sig[:len(sig)-1] + flip(sig[len(sig)-1])
	var out Session
	if err := c.decode(tampered, &out); err == nil {
		t.Error("tampered signature accepted")
	}
}

func TestCodec_WrongSecretRejected(t *testing.T) {
	val, err := testCodec().encode(Session{Sub: "user-1", Exp: 200})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	other := codec{secret: []byte("fedcba9876543210fedcba9876543210")}
	var out Session
	if err := other.decode(val, &out); err == nil {
		t.Error("value signed with another secret accepted")
	}
}

func TestCodec_MalformedRejected(t *testing.T) {
	c := testCodec()
	for _, v := range []string{"", "no-dot", "a.b.c", "!!!.###"} {
		var out Session
		if err := c.decode(v, &out); err == nil {
			t.Errorf("malformed value %q accepted", v)
		}
	}
}

func TestExpired(t *testing.T) {
	now := time.Unix(1000, 0)
	if expired(now.Add(time.Hour).Unix(), now) {
		t.Error("future deadline reported expired")
	}
	// Inside the leeway window: not expired yet.
	if expired(now.Add(-expLeeway/2).Unix(), now) {
		t.Error("deadline within leeway reported expired")
	}
	if !expired(now.Add(-expLeeway-time.Second).Unix(), now) {
		t.Error("past deadline not reported expired")
	}
}

// flip returns a different base64url character than the input.
func flip(b byte) string {
	if b == 'A' {
		return "B"
	}
	return "A"
}
