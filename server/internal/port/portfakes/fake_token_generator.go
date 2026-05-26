package portfakes

import (
	"fmt"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// FakeTokenGenerator returns pre-loaded values from queues; when a queue is
// empty, it returns a deterministic synthetic value so tests fail with a
// clear "auto-fake-..." string rather than a panic.
type FakeTokenGenerator struct {
	IDs    []domain.ShareID
	Owners []domain.OwnerToken
	Nonces [][]byte

	ErrOnShareID    error
	ErrOnOwnerToken error
	ErrOnNonce      error

	ShareIDCalls    int
	OwnerTokenCalls int
	NonceCalls      int
}

// NewFakeTokenGenerator pre-loads the three queues; pass nil to leave a
// queue empty (each call will then yield a synthetic placeholder).
func NewFakeTokenGenerator(ids []domain.ShareID, owners []domain.OwnerToken, nonces [][]byte) *FakeTokenGenerator {
	return &FakeTokenGenerator{IDs: ids, Owners: owners, Nonces: nonces}
}

// ShareID pops the head of IDs; if empty, returns "auto-fake-id-N".
func (f *FakeTokenGenerator) ShareID() (domain.ShareID, error) {
	f.ShareIDCalls++
	if f.ErrOnShareID != nil {
		return "", f.ErrOnShareID
	}
	if len(f.IDs) > 0 {
		v := f.IDs[0]
		f.IDs = f.IDs[1:]
		return v, nil
	}
	return domain.ShareID(fmt.Sprintf("auto-fake-id-%d", f.ShareIDCalls)), nil
}

// OwnerToken pops the head of Owners; if empty, returns "auto-fake-owner-N".
func (f *FakeTokenGenerator) OwnerToken() (domain.OwnerToken, error) {
	f.OwnerTokenCalls++
	if f.ErrOnOwnerToken != nil {
		return "", f.ErrOnOwnerToken
	}
	if len(f.Owners) > 0 {
		v := f.Owners[0]
		f.Owners = f.Owners[1:]
		return v, nil
	}
	return domain.OwnerToken(fmt.Sprintf("auto-fake-owner-%d", f.OwnerTokenCalls)), nil
}

// Nonce pops the head of Nonces; if empty, returns a deterministic 16-byte
// slice with the call index encoded into the first byte.
func (f *FakeTokenGenerator) Nonce() ([]byte, error) {
	f.NonceCalls++
	if f.ErrOnNonce != nil {
		return nil, f.ErrOnNonce
	}
	if len(f.Nonces) > 0 {
		v := f.Nonces[0]
		f.Nonces = f.Nonces[1:]
		return v, nil
	}
	b := make([]byte, 16)
	b[0] = byte(f.NonceCalls)
	return b, nil
}

var _ port.TokenGenerator = (*FakeTokenGenerator)(nil)
