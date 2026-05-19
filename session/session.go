// Package session provides the SessionStore interface that inertia-go
// uses for flashing errors and messages between requests, plus two
// reference implementations (CookieStore for production, MemoryStore
// for tests).
package session

import "net/http"

// Store is the contract the core package consumes. Implementations must
// provide read-and-clear semantics for TakeErrors and TakeMessages.
type Store interface {
	FlashErrors(w http.ResponseWriter, r *http.Request, bag string, errs map[string]string) error
	TakeErrors(w http.ResponseWriter, r *http.Request, bag string) (map[string]string, error)
	FlashMessage(w http.ResponseWriter, r *http.Request, key string, val any) error
	TakeMessages(w http.ResponseWriter, r *http.Request) (map[string]any, error)
}

// Noop discards all writes and returns empty on reads. Useful for users
// who do not need errors/flash and want to bypass the Session=required
// check during construction.
type Noop struct{}

// NewNoop returns a Noop store.
func NewNoop() Noop { return Noop{} }

func (Noop) FlashErrors(http.ResponseWriter, *http.Request, string, map[string]string) error {
	return nil
}
func (Noop) TakeErrors(http.ResponseWriter, *http.Request, string) (map[string]string, error) {
	return nil, nil
}
func (Noop) FlashMessage(http.ResponseWriter, *http.Request, string, any) error { return nil }
func (Noop) TakeMessages(http.ResponseWriter, *http.Request) (map[string]any, error) {
	return nil, nil
}
