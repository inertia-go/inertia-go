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

// ValidationErrors returns the request-scoped error collector. Returns a
// no-op collector if called outside the Middleware (callers should not
// rely on the absence — log noise is intentional).
func ValidationErrors(r *http.Request) *ErrorBagCollector {
	if c, ok := r.Context().Value(ctxKeyErrorBag).(*ErrorBagCollector); ok {
		return c
	}
	return newErrorBag()
}

// Flash returns the request-scoped flash collector.
func Flash(r *http.Request) *FlashCollector {
	if c, ok := r.Context().Value(ctxKeyFlashBag).(*FlashCollector); ok {
		return c
	}
	return newFlashBag()
}
