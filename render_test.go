package inertia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRender_InitialHTML_EmbedsPageObject(t *testing.T) {
	i := newTestInertia(t)

	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/Index", Props{"users": []int{1, 2}})
	}))

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-page=`) {
		t.Errorf("missing data-page attribute: %s", body)
	}
	if !strings.Contains(body, `Users/Index`) {
		t.Errorf("missing component name: %s", body)
	}
}

func TestRender_InertiaJSON(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/Index", Props{"users": []int{1, 2}})
	}))

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: %q", got)
	}
	if rec.Header().Get("X-Inertia") != "true" {
		t.Errorf("missing X-Inertia: true")
	}
	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Component != "Users/Index" {
		t.Errorf("component: %q", page.Component)
	}
	if _, ok := page.Props["users"]; !ok {
		t.Errorf("missing users prop: %v", page.Props)
	}
}

func TestRender_VersionMismatch_Returns409(t *testing.T) {
	i, _ := New(Config{Session: stubSession{}, Version: "v2"})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "X", Props{})
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "v1") // mismatch
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("code: %d", rec.Code)
	}
	if rec.Header().Get("X-Inertia-Location") == "" {
		t.Error("missing X-Inertia-Location")
	}
}

func TestRender_PartialReload_FiltersProps(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/Index", Props{
			"users": []int{1, 2},
			"stats": Optional(func() (any, error) { return 99, nil }),
			"auth":  Always("u"),
		})
	}))

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Data", "stats")
	req.Header.Set("X-Inertia-Partial-Component", "Users/Index")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &page)

	if _, ok := page.Props["users"]; ok {
		t.Errorf("users should be excluded: %v", page.Props)
	}
	if v := page.Props["stats"]; v != float64(99) {
		t.Errorf("stats: %v", v)
	}
	if page.Props["auth"] != "u" {
		t.Errorf("auth should be included via Always: %v", page.Props["auth"])
	}
}

func TestRender_SharedProps_Merged(t *testing.T) {
	i := newTestInertia(t)
	i.ShareValue("appName", "Acme")
	i.Share("auth", func(_ *http.Request) any { return "user42" })

	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Home", Props{"feature": true})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page.Props["appName"] != "Acme" {
		t.Errorf("appName: %v", page.Props["appName"])
	}
	if page.Props["auth"] != "user42" {
		t.Errorf("auth: %v", page.Props["auth"])
	}
}
