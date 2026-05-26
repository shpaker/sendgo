package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
)

func TestFSStorage_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	ctx := context.Background()
	body := bytes.Repeat([]byte("x"), 1024*10)

	if err := s.Put(ctx, "1-abcdef1234567890", bytes.NewReader(body), 60); err != nil {
		t.Fatalf("Put: %v", err)
	}
	size, exists, err := s.Head(ctx, "1-abcdef1234567890")
	if err != nil || !exists {
		t.Fatalf("Head: exists=%v err=%v", exists, err)
	}
	if size != int64(len(body)) {
		t.Errorf("Head size: want %d got %d", len(body), size)
	}
	rc, err := s.Get(ctx, "1-abcdef1234567890")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Errorf("Get returned wrong bytes")
	}
	if err := s.Delete(ctx, "1-abcdef1234567890"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, exists, _ = s.Head(ctx, "1-abcdef1234567890")
	if exists {
		t.Errorf("Head after Delete: still exists")
	}
}

func TestFSStorage_NotFound(t *testing.T) {
	s, _ := NewFSStorage(t.TempDir())
	if _, err := s.Get(context.Background(), "1-abcdef1234567890"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get missing: want ErrNotFound, got %v", err)
	}
	if _, err := s.Length(context.Background(), "1-abcdef1234567890"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Length missing: want ErrNotFound, got %v", err)
	}
}

func TestFSStorage_PathTraversal(t *testing.T) {
	s, _ := NewFSStorage(t.TempDir())
	if err := s.Put(context.Background(), "../escape", bytes.NewReader([]byte("x")), 60); err == nil {
		t.Errorf("Put with '..' should fail")
	}
}
