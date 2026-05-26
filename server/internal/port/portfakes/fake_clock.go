package portfakes

import (
	"time"

	"github.com/sendgo/sendgo/server/internal/port"
)

// FakeClock is a port.Clock whose value is set explicitly. Use Advance to
// move time forward in tests.
type FakeClock struct {
	Current time.Time
}

// NewFakeClock returns a clock pinned to the given moment.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{Current: t}
}

// Now returns the pinned value.
func (c *FakeClock) Now() time.Time { return c.Current }

// Advance shifts the pinned value forward by d.
func (c *FakeClock) Advance(d time.Duration) { c.Current = c.Current.Add(d) }

var _ port.Clock = (*FakeClock)(nil)
