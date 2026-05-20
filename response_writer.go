package inertia

import (
	"bufio"
	"net"
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
// It implements Header/WriteHeader/Write plus Unwrap, and intercepts the two
// capabilities that commit headers behind the wrapper's back: FlushError and
// Hijack. http.ResponseController resolves these by checking the current writer
// for FlushError/Flusher (and Hijacker) BEFORE following Unwrap; if flushWriter
// only exposed Unwrap, a handler calling NewResponseController(w).Flush() would
// commit headers on the underlying writer without ever draining the session,
// dropping Set-Cookie. By implementing them here — each draining first, then
// delegating to the underlying writer via its own ResponseController — the
// drain is guaranteed to precede the header commit. Other capabilities
// (SetReadDeadline, SetWriteDeadline, EnableFullDuplex) do not commit headers,
// so they are left to reach the underlying writer through Unwrap unchanged.
type flushWriter struct {
	http.ResponseWriter
	once  sync.Once
	flush func()
}

// Unwrap exposes the wrapped writer so http.NewResponseController can reach
// the underlying writer for capabilities flushWriter does not intercept.
func (w *flushWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// FlushError drains the session before flushing the underlying writer, so a
// handler streaming via http.NewResponseController(w).Flush() does not commit
// headers ahead of the Set-Cookie. ResponseController prefers FlushError over
// Flusher and over Unwrap, so this method wins resolution.
func (w *flushWriter) FlushError() error {
	w.once.Do(w.flush)
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// Hijack drains the session before handing the connection over. After a
// hijack the normal response path is gone, so any buffered Set-Cookie must be
// written first. Returns ErrNotSupported (via the underlying controller) if
// the underlying writer is not a Hijacker.
func (w *flushWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.once.Do(w.flush)
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

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
