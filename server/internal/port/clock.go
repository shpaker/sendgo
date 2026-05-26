package port

import "time"

// Clock is a thin abstraction over time.Now used to mock time in tests.
type Clock interface {
	Now() time.Time
}

// RealClock is the production implementation; it reads wall-clock time.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
