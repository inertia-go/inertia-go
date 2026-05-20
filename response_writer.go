package inertia

import (
	"net/http"
	"sync"
)

// flushWriter wraps an http.ResponseWriter to run a flush callback exactly
// once, immediately before the response headers are committed to the wire.
//
// The session accumulator (e.g. CookieStore) buffers Set-Cookie writes and
// only emits them when its FlushResponse hook runs. Because net/http freezes
// the header map on the first WriteHeader/Write, the hook must fire before
// that point — a deferred call after the handler returns is too late and the
// cookie is silently dropped. flushWriter triggers the callback on the first
// WriteHeader or Write so the flush's header mutations still reach the client.
//
// It deliberately implements only Header/WriteHeader/Write plus Unwrap. Optional
// capabilities (http.Flusher, http.Hijacker, io.ReaderFrom, http.Pusher) are
// reached by callers via http.NewResponseController, which follows Unwrap to
// the underlying writer.
type flushWriter struct {
	http.ResponseWriter
	once  sync.Once
	flush func()
}

// Unwrap exposes the wrapped writer so http.NewResponseController can locate
// the underlying Flusher/Hijacker/etc.
func (w *flushWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader runs the flush callback before committing the status line, then
// delegates to the wrapped writer.
func (w *flushWriter) WriteHeader(status int) {
	w.once.Do(w.flush)
	w.ResponseWriter.WriteHeader(status)
}

// Write runs the flush callback before the first byte is written (which would
// otherwise commit an implicit 200), then delegates.
func (w *flushWriter) Write(b []byte) (int, error) {
	w.once.Do(w.flush)
	return w.ResponseWriter.Write(b)
}

// flushNow runs the callback if it has not already fired. Middleware calls this
// after the handler returns so a handler that wrote nothing (e.g. an empty 200)
// still drains the accumulator. The sync.Once guarantees it never double-flushes.
func (w *flushWriter) flushNow() { w.once.Do(w.flush) }
