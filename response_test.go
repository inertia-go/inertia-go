package inertia

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirect_PromotesToSeeOtherForUnsafeMethods(t *testing.T) {
	i := newTestInertia(t)
	cases := []struct {
		method string
		want   int
	}{
		{http.MethodGet, http.StatusFound},
		{http.MethodPost, http.StatusFound},
		{http.MethodPut, http.StatusSeeOther},
		{http.MethodPatch, http.StatusSeeOther},
		{http.MethodDelete, http.StatusSeeOther},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(c.method, "/", nil)
		i.Redirect(w, r, "/dest")
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.method, w.Code, c.want)
		}
		if loc := w.Header().Get("Location"); loc != "/dest" {
			t.Errorf("%s Location: %q", c.method, loc)
		}
	}
}

func TestLocation_External409(t *testing.T) {
	i := newTestInertia(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Inertia", "true")
	// Wrap with Middleware so FromRequest sees IsInertia=true via context.
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Location(w, r, "https://external.example.com/page")
	}))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("code: %d", w.Code)
	}
	if got := w.Header().Get("X-Inertia-Location"); got != "https://external.example.com/page" {
		t.Errorf("X-Inertia-Location: %q", got)
	}
}

func TestRedirect_FragmentUses409Redirect(t *testing.T) {
	i := newTestInertia(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Inertia", "true")
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Redirect(w, r, "/page#section")
	}))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("code: %d", w.Code)
	}
	if got := w.Header().Get("X-Inertia-Redirect"); got != "/page#section" {
		t.Errorf("X-Inertia-Redirect: %q", got)
	}
}

func TestLocation_NonInertia_FallsBackToStandardRedirect(t *testing.T) {
	i := newTestInertia(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	i.Location(w, r, "https://external.example.com/page")
	if w.Code != http.StatusFound {
		t.Errorf("code: %d", w.Code)
	}
}

func TestBack_ReadsReferer(t *testing.T) {
	i := newTestInertia(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Referer", "/previous")
	i.Back(w, r)
	if got := w.Header().Get("Location"); got != "/previous" {
		t.Errorf("Location: %q", got)
	}
}

func TestBack_NoReferer_FallsBackToRoot(t *testing.T) {
	i := newTestInertia(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	// Referer header intentionally absent
	i.Back(w, r)
	if got := w.Header().Get("Location"); got != "/" {
		t.Errorf("Location: %q, want /", got)
	}
}
