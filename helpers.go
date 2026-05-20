package inertia

import (
	"net/http"
	"sync"
)

// ErrorBagCollector accumulates validation errors for a single request.
// Retrieve it via ValidationErrors(r).
type ErrorBagCollector struct {
	mu      sync.Mutex
	entries map[string]map[string]string // bag -> field -> message
	dirty   bool
}

func newErrorBag() *ErrorBagCollector {
	return &ErrorBagCollector{entries: map[string]map[string]string{}}
}

// Add records a validation error in the default bag.
func (e *ErrorBagCollector) Add(field, message string) *ErrorBagCollector {
	e.Bag("").Add(field, message)
	return e
}

// Bag returns a per-bag handle. Empty name resolves to "default" later.
func (e *ErrorBagCollector) Bag(name string) *bagHandle {
	return &bagHandle{parent: e, name: name}
}

type bagHandle struct {
	parent *ErrorBagCollector
	name   string
}

func (b *bagHandle) Add(field, message string) *bagHandle {
	b.parent.mu.Lock()
	defer b.parent.mu.Unlock()
	if b.parent.entries[b.name] == nil {
		b.parent.entries[b.name] = map[string]string{}
	}
	b.parent.entries[b.name][field] = message
	b.parent.dirty = true
	return b
}

// snapshot returns a copy of the field→message map for the named bag
// (empty name = the default unnamed bag). The copy is safe to mutate and
// to read without holding the collector's mutex. Returns an empty map when
// the bag has no entries.
func (e *ErrorBagCollector) snapshot(bag string) map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]string, len(e.entries[bag]))
	for k, v := range e.entries[bag] {
		out[k] = v
	}
	return out
}

// FlashCollector accumulates flash messages for a single request.
type FlashCollector struct {
	mu      sync.Mutex
	entries map[string]any
	dirty   bool
}

func newFlashBag() *FlashCollector {
	return &FlashCollector{entries: map[string]any{}}
}

// Set stores a flash entry under key.
func (f *FlashCollector) Set(key string, value any) *FlashCollector {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key] = value
	f.dirty = true
	return f
}

// ValidationErrors returns the request-scoped error collector populated by
// Middleware. If the request has not passed through Middleware, a fresh
// orphan collector is returned: writes to it are silently discarded and
// will not reach the session. Always install Middleware to enable error
// flashing across redirects.
func ValidationErrors(r *http.Request) *ErrorBagCollector {
	if c, ok := r.Context().Value(ctxKeyErrorBag).(*ErrorBagCollector); ok {
		return c
	}
	return newErrorBag()
}

// Flash returns the request-scoped flash collector populated by Middleware.
// If the request has not passed through Middleware, a fresh orphan
// collector is returned: writes to it are silently discarded and will not
// reach the session.
func Flash(r *http.Request) *FlashCollector {
	if c, ok := r.Context().Value(ctxKeyFlashBag).(*FlashCollector); ok {
		return c
	}
	return newFlashBag()
}

// preserveFragmentHolder carries a per-request *bool override for the
// page object's preserveFragment flag. Middleware installs an empty
// holder; SetPreserveFragment writes into it; Render reads it. A nil
// value means "use the Config default".
type preserveFragmentHolder struct {
	mu  sync.Mutex
	val *bool
}

// SetPreserveFragment overrides preserveFragment for the current response,
// in either direction, winning over Config.PreserveFragment. No-op if the
// request did not pass through Middleware.
func SetPreserveFragment(r *http.Request, v bool) {
	if h, ok := r.Context().Value(ctxKeyPreserveFragment).(*preserveFragmentHolder); ok {
		h.mu.Lock()
		h.val = &v
		h.mu.Unlock()
	}
}
