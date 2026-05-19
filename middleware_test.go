package inertia

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
