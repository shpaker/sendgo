package observability

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// ReopenableWriter is an io.Writer that wraps either stdout or a file. On
// SIGHUP it closes the old descriptor and reopens the file at the same path.
// This is the standard logrotate handshake: rotation moves the file aside,
// sends SIGHUP, and we reopen — a fresh file appears at the original path.
type ReopenableWriter struct {
	path string // empty → stdout, reopen is a no-op
	mu   sync.Mutex
	w    io.Writer
	file *os.File // nil when path == ""
}

// NewReopenableWriter returns a writer. With path=="" we write to os.Stdout
// without opening a file.
func NewReopenableWriter(path string) (*ReopenableWriter, error) {
	rw := &ReopenableWriter{path: path}
	if path == "" {
		rw.w = os.Stdout
		return rw, nil
	}
	if err := rw.open(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (r *ReopenableWriter) open() error {
	// 0o640: owner rw, group r, world none. Group-readable is intentional so
	// log shippers / observers running in the same group can read without
	// being the file owner; matches typical /var/log conventions and lets
	// logrotate run as a separate user from sendgo.
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640) //nolint:gosec // G302: see above
	if err != nil {
		return fmt.Errorf("open log file %q: %w", r.path, err)
	}
	r.file = f
	r.w = f
	return nil
}

// Write implements io.Writer. Holds mu to synchronize with reopen().
func (r *ReopenableWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.w.Write(p)
}

// Reopen closes the current file and opens it again at path. Invoked on
// SIGHUP. No-op in stdout mode.
func (r *ReopenableWriter) Reopen() error {
	if r.path == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.file
	if err := r.open(); err != nil {
		return err
	}
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// Close closes the descriptor when a file is open. Stdout is left untouched.
func (r *ReopenableWriter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// WatchSIGHUP spawns a goroutine that calls Reopen on every SIGHUP. Stops
// when the returned cancel function is invoked.
func (r *ReopenableWriter) WatchSIGHUP() (cancel func()) {
	if r.path == "" {
		return func() {} // stdout — nothing to watch
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				if err := r.Reopen(); err != nil {
					// Cannot log through ourselves — write to stderr directly.
					fmt.Fprintf(os.Stderr, "log reopen failed: %v\n", err)
				}
			case <-done:
				signal.Stop(ch)
				return
			}
		}
	}()
	return func() { close(done) }
}
