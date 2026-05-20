package inertia

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestProtocol_PageObject_HasV3Fields(t *testing.T) {
	// Compile-time assertion: PageObject must have all v3 fields so
	// downstream code can populate them. This test catches struct-shape
	// regressions before the wrappers wire up.
	var p PageObject
	_ = p.PrependProps
	_ = p.MatchPropsOn
	_ = p.SharedProps
	_ = p.ScrollProps
	_ = p.OnceProps
	_ = p.RescuedProps

	// Sanity-check JSON tags via marshal: all six should be omitempty
	// (nil-valued in this zero-value page).
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, tag := range []string{
		"prependProps", "matchPropsOn", "sharedProps",
		"scrollProps", "onceProps", "rescuedProps",
	} {
		if strings.Contains(js, tag) {
			t.Errorf("zero-value PageObject must not emit %q (need omitempty): %s", tag, js)
		}
	}
}

func TestProtocol_PageObject_V5Types(t *testing.T) {
	var p PageObject
	// Corrected typed maps (were map[string]map[string]any in v0.4).
	p.ScrollProps = map[string]ScrollConfig{"posts": {PageName: "page", CurrentPage: 1}}
	p.OnceProps = map[string]OnceConfig{"plans": {Prop: "plans"}}
	p.RescuedProps = []string{"activity"}
	p.PreserveFragment = true

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{
		`"scrollProps":{"posts":{"pageName":"page","previousPage":null,"nextPage":null,"currentPage":1}}`,
		`"onceProps":{"plans":{"prop":"plans","expiresAt":null}}`,
		`"rescuedProps":["activity"]`,
		`"preserveFragment":true`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("missing %s in %s", want, js)
		}
	}

	// Zero-value page must emit none of them (omitempty).
	z, _ := json.Marshal(PageObject{})
	for _, tag := range []string{"scrollProps", "onceProps", "rescuedProps", "preserveFragment"} {
		if strings.Contains(string(z), tag) {
			t.Errorf("zero-value PageObject must omit %q: %s", tag, z)
		}
	}
}

func TestProtocol_SharedPropsListed(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	i.ShareValue("appName", "Acme")
	i.Share("auth", func(_ *http.Request) any { return map[string]any{"id": 1} })

	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Home", Props{"localOnly": 1})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	want := []string{"appName", "auth"}
	if !reflect.DeepEqual(page.SharedProps, want) {
		t.Errorf("sharedProps: got %v, want %v", page.SharedProps, want)
	}
	for _, k := range page.SharedProps {
		if k == "errors" || k == "flash" || k == "localOnly" {
			t.Errorf("sharedProps must not include %q: %v", k, page.SharedProps)
		}
	}
}

func TestProtocol_RescuedProps_DropsFailedDeferred(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Dashboard", Props{
			"user": "alice",
			"activity": Defer(func() (any, error) {
				return nil, errors.New("boom")
			}).Rescue(),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Dashboard")
	req.Header.Set("X-Inertia-Partial-Data", "activity")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("rescue must not 500; got %d", rec.Code)
	}
	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if _, present := page.Props["activity"]; present {
		t.Errorf("failed rescued prop must be dropped: %v", page.Props)
	}
	want := []string{"activity"}
	if !reflect.DeepEqual(page.RescuedProps, want) {
		t.Errorf("rescuedProps = %v, want %v", page.RescuedProps, want)
	}
}

func TestProtocol_OnceProps_FirstLoadAndCached(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	mk := func(except string) PageObject {
		h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i.Render(w, r, "Billing", Props{
				"plans": Once(func() (any, error) { return []string{"basic", "pro"}, nil }),
			})
		}))
		req := httptest.NewRequest(http.MethodGet, "/billing", nil)
		req.Header.Set("X-Inertia", "true")
		if except != "" {
			req.Header.Set("X-Inertia-Except-Once-Props", except)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var p PageObject
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := mk("")
	if _, ok := first.Props["plans"]; !ok {
		t.Error("first load must include plans in props")
	}
	if got := first.OnceProps["plans"]; got.Prop != "plans" || got.ExpiresAt != nil {
		t.Errorf("onceProps[plans] = %+v, want {plans, nil}", got)
	}
	cached := mk("plans")
	if _, ok := cached.Props["plans"]; ok {
		t.Error("cached once prop must be omitted from props")
	}
	if _, ok := cached.OnceProps["plans"]; !ok {
		t.Error("onceProps metadata must persist on cached response")
	}
}

func TestProtocol_NoRescue_StillFailsResponse(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Dashboard", Props{
			"activity": Defer(func() (any, error) { return nil, errors.New("boom") }),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Dashboard")
	req.Header.Set("X-Inertia-Partial-Data", "activity")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("non-rescued deferred error must 500; got %d", rec.Code)
	}
}

func TestProtocol_ScrollProps_WireFormat(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := 2
		i.Render(w, r, "Posts/Index", Props{
			"posts": Scroll([]map[string]any{{"id": 1}}, ScrollConfig{CurrentPage: 1, NextPage: &next}),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/posts?page=1", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	posts, ok := page.Props["posts"].(map[string]any)
	if !ok {
		t.Fatalf("posts must be an object with a data key: %T", page.Props["posts"])
	}
	if _, ok := posts["data"]; !ok {
		t.Errorf("posts must contain data: %v", posts)
	}
	found := false
	for _, m := range page.MergeProps {
		if m == "posts.data" {
			found = true
		}
	}
	if !found {
		t.Errorf("mergeProps must include posts.data: %v", page.MergeProps)
	}
	sc := page.ScrollProps["posts"]
	if sc.PageName != "page" || sc.CurrentPage != 1 || sc.NextPage == nil || *sc.NextPage != 2 {
		t.Errorf("scrollProps[posts] = %+v", sc)
	}
}
