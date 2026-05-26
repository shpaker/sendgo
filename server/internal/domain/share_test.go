package domain

import "testing"

func TestValidShareID(t *testing.T) {
	good := []string{
		"abcdef1234567890", // 16 chars
		"1234567890abcdef", // different case still fine
		"AABBCCDDEEFF1234", // upper
		"1234567890",       // 10 chars — minimum
	}
	bad := []string{
		"",
		"short",
		"deadbeef-cafebabe",      // dash not allowed
		"GGGGGGGGGGGGGGGG",       // not hex
		"abcdef1234567890ab1234", // longer than 16
	}
	for _, s := range good {
		if !ValidShareID(s) {
			t.Errorf("ValidShareID(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if ValidShareID(s) {
			t.Errorf("ValidShareID(%q) = true, want false", s)
		}
	}
}

func TestFileShare_FinalAndReached(t *testing.T) {
	s := &FileShare{DL: 0, DLimit: 1}
	if !s.IsFinalDownload() {
		t.Errorf("dl=0 dlimit=1 → expected final download")
	}
	if s.ReachedLimit() {
		t.Errorf("dl=0 dlimit=1 → not yet reached")
	}
	s.DL = 1
	if s.IsFinalDownload() {
		t.Errorf("dl=1 dlimit=1 → not final anymore (already done)")
	}
	if !s.ReachedLimit() {
		t.Errorf("dl=1 dlimit=1 → reached")
	}
}
