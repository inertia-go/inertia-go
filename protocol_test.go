package inertia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inertia-go/inertia-go/session"
)

func TestProtocol_VaryHeaderAlwaysSet(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Vary") != "X-Inertia" {
		t.Errorf("Vary header: %q", rec.Header().Get("Vary"))
	}
}

func TestProtocol_InertiaJSONResponseShape(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory(), Version: "abc"})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Events", Props{
			"events": []map[string]any{{"id": 80, "title": "Birthday party"}},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/events/80", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("code: %d", rec.Code)
	}
	if rec.Header().Get("X-Inertia") != "true" {
		t.Errorf("missing X-Inertia: true")
	}
	if rec.Header().Get("X-Inertia-Version") != "abc" {
		t.Errorf("missing X-Inertia-Version")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: %q", rec.Header().Get("Content-Type"))
	}

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Component != "Events" {
		t.Errorf("component: %q", page.Component)
	}
	if page.URL != "/events/80" {
		t.Errorf("url: %q", page.URL)
	}
	if page.Version != "abc" {
		t.Errorf("version: %q", page.Version)
	}
}

func TestProtocol_PartialReload(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory(), Version: "abc"})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Events", Props{
			"auth":       Always(map[string]any{"id": 1}),
			"categories": []string{"a", "b"},
			"events":     []map[string]any{{"id": 80}},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "abc")
	req.Header.Set("X-Inertia-Partial-Data", "events")
	req.Header.Set("X-Inertia-Partial-Component", "Events")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if _, ok := page.Props["categories"]; ok {
		t.Errorf("categories should be excluded: %v", page.Props)
	}
	if _, ok := page.Props["auth"]; !ok {
		t.Errorf("auth (Always) should be included: %v", page.Props)
	}
	if _, ok := page.Props["events"]; !ok {
		t.Errorf("events (requested) should be included: %v", page.Props)
	}
}

func TestProtocol_VersionMismatch_Returns409WithLocation(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory(), Version: "v2"})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "X", Props{})
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "v1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("code: %d", rec.Code)
	}
	if rec.Header().Get("X-Inertia-Location") == "" {
		t.Error("missing X-Inertia-Location")
	}
}

func TestProtocol_303OnUnsafeMethodRedirect(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	for _, m := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(m, "/", nil)
		i.Redirect(w, r, "/dest")
		if w.Code != http.StatusSeeOther {
			t.Errorf("%s: code %d", m, w.Code)
		}
	}
}

func TestProtocol_InitialHTMLContainsAppDiv(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Home", Props{"x": 1})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `<script data-page="app" type="application/json">`) {
		t.Errorf("missing v3 script tag: %s", body)
	}
	if !strings.Contains(body, `<div id="app"></div>`) {
		t.Errorf("missing mount div: %s", body)
	}
	if strings.Contains(body, `data-page='`) {
		t.Errorf("legacy single-quoted data-page attribute must be gone: %s", body)
	}
}

func TestProtocol_DeferredMetadataOnInitial(t *testing.T) {
	// On the initial (non-partial) HTML response, the PageObject embedded
	// inside <script data-page="app"> must include deferredProps so the
	// v3 client knows to auto-fetch deferred values after mount.
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Dashboard", Props{
			"user":     "alice",
			"activity": Defer(func() (any, error) { return []int{1, 2, 3}, nil }, "feed"),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	const open = `<script data-page="app" type="application/json">`
	const close = `</script>`
	start := strings.Index(body, open)
	if start < 0 {
		t.Fatalf("script open tag missing: %s", body)
	}
	rest := body[start+len(open):]
	end := strings.Index(rest, close)
	if end < 0 {
		t.Fatalf("script close tag missing: %s", body)
	}
	var page PageObject
	if err := json.Unmarshal([]byte(rest[:end]), &page); err != nil {
		t.Fatalf("page JSON: %v", err)
	}

	if page.DeferredProps == nil {
		t.Fatalf("deferredProps missing from initial response: %+v", page)
	}
	if got := page.DeferredProps["feed"]; len(got) != 1 || got[0] != "activity" {
		t.Errorf("deferredProps[feed] = %v, want [activity]", got)
	}
	if _, present := page.Props["activity"]; present {
		t.Errorf("activity must not be evaluated on initial response: %v", page.Props)
	}
}
