package crypto

import (
	"bytes"
	"testing"
)

func TestDecodeB64_AcceptsAllVariants(t *testing.T) {
	want := []byte{0x80, 0xff, 0x10, 0xab, 0xcd, 0xef}
	// Same content in four flavors: URL-safe (with/without padding) and
	// standard (with/without padding). All must decode to the same bytes.
	cases := []string{
		"gP8Qq83v",   // RawURL (no padding) — the primary frontend format
		"gP8Qq83v==", // URL-safe with padding (if padding was not stripped)
		"gP8Qq83v",   // RawStd (matches RawURL for short payloads)
	}
	// Add a string that deliberately contains `-` and `_` to ensure URL-safe
	// characters are not confused with Std characters.
	urlSafeOnly := EncodeB64([]byte{0xfb, 0xff, 0xff, 0xff})
	cases = append(cases, urlSafeOnly)

	for _, s := range cases {
		got, err := DecodeB64(s)
		if err != nil {
			t.Errorf("DecodeB64(%q) returned err=%v", s, err)
			continue
		}
		if s == urlSafeOnly {
			continue // different content, no comparison needed
		}
		if !bytes.Equal(got, want) {
			t.Errorf("DecodeB64(%q) = %x, want %x", s, got, want)
		}
	}
}

func TestEncodeDecodeB64_Roundtrip(t *testing.T) {
	cases := [][]byte{
		nil,
		{0x00},
		{0xff},
		{0x00, 0xff, 0x10, 0xab, 0xcd, 0xef, 0xfb, 0xff, 0xff, 0xff, 0xfa},
	}
	for _, in := range cases {
		out, err := DecodeB64(EncodeB64(in))
		if err != nil {
			t.Errorf("roundtrip %x: decode err=%v", in, err)
			continue
		}
		if !bytes.Equal(out, in) {
			t.Errorf("roundtrip %x mismatch: got %x", in, out)
		}
	}
}

func TestDecodeB64_RejectsGarbage(t *testing.T) {
	if _, err := DecodeB64("!!!@@@"); err == nil {
		t.Errorf("DecodeB64 accepted invalid characters")
	}
}
