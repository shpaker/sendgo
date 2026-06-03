// Package portfakes provides hand-written in-memory test doubles for the
// port.* interfaces. They are deterministic state machines with optional
// per-method error injection (ErrOn<Method> fields) and call recorders
// (<Method>Calls slices); assertions live in tests, not inside the fakes.
//
// Production code MUST NOT import this package. The depguard ruleset in
// server/.golangci.yml enforces the boundary.
package portfakes
