package inertia

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inertia-go/inertia-go/session"
)

func TestMiddleware_ParsesInertiaHeaders(t *testing.T) {
	i := newTestInertia(t)

	var seen RequestInfo
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromRequest(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "v123")
	req.Header.Set("X-Inertia-Partial-Data", "users,stats")
	req.Header.Set("X-Inertia-Partial-Component", "Users/Index")
	req.Header.Set("X-Inertia-Partial-Except", "stats")
	req.Header.Set("X-Inertia-Reset", "tags")
	req.Header.Set("X-Inertia-Error-Bag", "signup")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !seen.IsInertia {
		t.Error("IsInertia: want true")
	}
	if seen.Version != "v123" {
		t.Errorf("Version: want v123, got %q", seen.Version)
	}
	if got, want := seen.PartialData, []string{"users", "stats"}; !equalStrings(got, want) {
		t.Errorf("PartialData: want %v, got %v", want, got)
	}
	if seen.PartialComponent != "Users/Index" {
		t.Errorf("PartialComponent: %q", seen.PartialComponent)
	}
	if got, want := seen.PartialExcept, []string{"stats"}; !equalStrings(got, want) {
		t.Errorf("PartialExcept: %v", got)
	}
	if got, want := seen.Reset, []string{"tags"}; !equalStrings(got, want) {
		t.Errorf("Reset: %v", got)
	}
	if seen.ErrorBag != "signup" {
		t.Errorf("ErrorBag: %q", seen.ErrorBag)
	}
	if got := rec.Header().Get("Vary"); got != "X-Inertia" {
		t.Errorf("Vary header: want X-Inertia, got %q", got)
	}
}

func TestMiddleware_NonInertiaRequest(t *testing.T) {
	i := newTestInertia(t)

	var seen RequestInfo
	h := i.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = FromRequest(r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen.IsInertia {
		t.Error("IsInertia should be false")
	}
}

// newTestInertia constructs a minimal *Inertia for tests.
func newTestInertia(t *testing.T) *Inertia {
	t.Helper()
	i, err := New(Config{Session: stubSession{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return i
}

type recordingFlusher struct {
	stubSession
	flushCalls int
}

func (f *recordingFlusher) FlushResponse(_ http.ResponseWriter) error {
	f.flushCalls++
	return nil
}

func TestMiddleware_FlushesSessionAfterHandler(t *testing.T) {
	rf := &recordingFlusher{}
	i, _ := New(Config{Session: rf})

	handled := false
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled = true
		if rf.flushCalls != 0 {
			t.Errorf("flush must not fire before handler returns; got %d", rf.flushCalls)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !handled {
		t.Fatal("handler did not run")
	}
	if rf.flushCalls != 1 {
		t.Errorf("FlushResponse calls: got %d, want 1", rf.flushCalls)
	}
}

func TestMiddleware_NoFlushWhenStoreLacksInterface(t *testing.T) {
	// Plain stubSession (no FlushResponse method) must not error.
	i, _ := New(Config{Session: stubSession{}})
	h := i.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
}

// TestMiddleware_CookieFlushSurvivesRealHTTP exercises the flush timing
// against a real net/http server (not httptest.ResponseRecorder, which
// never freezes its header map). A handler that flashes errors and then
// redirects must still emit Set-Cookie: with a deferred flush the header
// is already on the wire by the time FlushResponse runs, so the cookie
// is silently dropped.
func TestMiddleware_CookieFlushSurvivesRealHTTP(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		t.Fatalf("NewCookie: %v", err)
	}
	i, err := New(Config{Session: store})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("email", "required")
		i.Redirect(w, r, "/login")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Do not follow the redirect; inspect the 30x response itself.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL + "/submit")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if len(resp.Cookies()) == 0 {
		t.Fatalf("no Set-Cookie on redirect response; flash was dropped (status %d)", resp.StatusCode)
	}
}

// TestMiddleware_ResponseControllerFlushDrainsSession is the regression for
// the flush-bypass bug: http.NewResponseController(w).Flush() resolves
// FlushError/Flusher before following Unwrap, so a flushWriter that only
// implements Unwrap would let the controller commit headers on the underlying
// writer without first draining the session — dropping Set-Cookie. A handler
// that flashes and then calls ResponseController.Flush() must still emit the
// cookie, proving the drain happens before the underlying flush.
func TestMiddleware_ResponseControllerFlushDrainsSession(t *testing.T) {
	i := newCookieInertia(t)

	var flushErr error
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("email", "required")
		i.persistCollectors(w, r) // buffer the flash into the accumulator
		flushErr = http.NewResponseController(w).Flush()
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if flushErr != nil {
		t.Fatalf("ResponseController.Flush did not reach underlying writer: %v", flushErr)
	}
	if len(resp.Cookies()) == 0 {
		t.Fatalf("no Set-Cookie: ResponseController.Flush committed headers before draining session")
	}
}

// TestMiddleware_HijackDrainsSession confirms that hijacking the connection
// drains the session first. After a hijack the normal response path is gone,
// so any buffered Set-Cookie must be flushed before the handler takes over.
func TestMiddleware_HijackDrainsSession(t *testing.T) {
	rf := &recordingFlusher{}
	i, err := New(Config{Session: rf})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	hijacked := make(chan struct{})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer close(hijacked)
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		if rf.flushCalls != 1 {
			t.Errorf("session must be drained before hijack returns; flushCalls=%d", rf.flushCalls)
		}
		_ = conn.Close()
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ws")
	if err == nil {
		_ = resp.Body.Close()
	}
	<-hijacked
}

// newCookieInertia builds an *Inertia backed by a real CookieStore for tests
// that need genuine Set-Cookie emission over a real HTTP server.
func newCookieInertia(t *testing.T) *Inertia {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		t.Fatalf("NewCookie: %v", err)
	}
	i, err := New(Config{Session: store})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return i
}

func TestMiddleware_ParsesExceptOnceProps(t *testing.T) {
	i := newTestInertia(t)
	var seen RequestInfo
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromRequest(r)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia-Except-Once-Props", "plans, billing")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got, want := seen.ExceptOnceProps, []string{"plans", "billing"}; !equalStrings(got, want) {
		t.Errorf("ExceptOnceProps = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
