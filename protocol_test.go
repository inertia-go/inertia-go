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
		`"scrollProps":{"posts":{"pageName":"page","previousPage":null,"nextPage":null,"currentPage":1,"reset":false}}`,
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

// TestProtocol_OnceProps_AsAliasCacheSkip verifies the round-trip for an
// aliased once prop: Once(fn).As("billing") registers onceProps["billing"],
// and when the client reports it cached via X-Inertia-Except-Once-Props:
// billing, the server skips re-resolving it (absent from props) while still
// emitting the onceProps["billing"] metadata. Without the except header the
// prop is present.
func TestProtocol_OnceProps_AsAliasCacheSkip(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	mk := func(except string) PageObject {
		h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i.Render(w, r, "Billing", Props{
				"plans": Once(func() (any, error) { return []string{"basic", "pro"}, nil }).As("billing"),
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
		t.Error("without except header, aliased once prop must be present in props")
	}
	// onceProps is keyed by the cache key ("billing", the .As() alias), but
	// the prop field is the actual page-prop name ("plans") per the v3
	// protocol — the client maps the cached value back to the right prop.
	if _, ok := first.OnceProps["billing"]; !ok {
		t.Errorf("onceProps must be keyed by the alias 'billing': %+v", first.OnceProps)
	}
	if got := first.OnceProps["billing"]; got.Prop != "plans" {
		t.Errorf("onceProps[billing].prop = %q, want plans (the real prop name)", got.Prop)
	}
	cached := mk("billing")
	if _, ok := cached.Props["plans"]; ok {
		t.Error("aliased once prop reported cached via alias must be omitted from props")
	}
	if _, ok := cached.OnceProps["billing"]; !ok {
		t.Error("onceProps[billing] metadata must persist on cached response")
	}
}

// TestProtocol_OnceProps_PartialReloadResolvesAlreadyLoaded verifies alignment
// with the official PropsResolver: the once cache-skip
// (wasAlreadyLoadedByClient) is reachable only inside excludeFromInitialResponse,
// which is gated on !isPartial. So on a PARTIAL reload an already-loaded once
// prop is NOT cache-skipped — it is re-resolved and SENT. Its onceProps metadata
// is still emitted.
func TestProtocol_OnceProps_PartialReloadResolvesAlreadyLoaded(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Billing", Props{
			"plans": Once(func() (any, error) { return []string{"basic", "pro"}, nil }),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Billing")
	req.Header.Set("X-Inertia-Partial-Data", "plans")
	req.Header.Set("X-Inertia-Except-Once-Props", "plans")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var p PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Props["plans"]; !ok {
		t.Errorf("on a partial reload an already-loaded once prop must be re-resolved and sent: %v", p.Props)
	}
	if _, ok := p.OnceProps["plans"]; !ok {
		t.Error("onceProps metadata must still be emitted on a partial reload")
	}
}

// TestProtocol_OnceProps_FreshForcesRefresh verifies that .Fresh() overrides the
// cache skip: a Once(fn).Fresh() prop is re-resolved (present in props) even
// when reported cached via X-Inertia-Except-Once-Props.
func TestProtocol_OnceProps_FreshForcesRefresh(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Billing", Props{
			"plans": Once(func() (any, error) { return []string{"basic", "pro"}, nil }).Fresh(),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Billing")
	req.Header.Set("X-Inertia-Partial-Data", "plans")
	req.Header.Set("X-Inertia-Except-Once-Props", "plans")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var p PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Props["plans"]; !ok {
		t.Errorf("Fresh() once prop must re-resolve despite Except-Once-Props: %v", p.Props)
	}
	if _, ok := p.OnceProps["plans"]; !ok {
		t.Error("onceProps metadata must still be emitted on a Fresh once prop")
	}
}

// TestProtocol_OnceProps_NestedPathCacheSkip verifies the cache-skip key is the
// full dot path, not the leaf key. A once prop at config.locale reported cached
// via X-Inertia-Except-Once-Props: config.locale is skipped (value omitted),
// and its onceProps[config.locale] metadata is still emitted.
func TestProtocol_OnceProps_NestedPathCacheSkip(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	mk := func(except string) PageObject {
		h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i.Render(w, r, "Settings", Props{
				"config": map[string]any{
					"locale": Once(func() (any, error) { return "en", nil }),
				},
			})
		}))
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
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
	cfg, _ := first.Props["config"].(map[string]any)
	if cfg == nil || cfg["locale"] != "en" {
		t.Errorf("first load must include config.locale: %v", first.Props)
	}
	if _, ok := first.OnceProps["config.locale"]; !ok {
		t.Errorf("onceProps must be keyed by full path config.locale: %+v", first.OnceProps)
	}
	cached := mk("config.locale")
	cfg2, _ := cached.Props["config"].(map[string]any)
	if cfg2 != nil {
		if _, ok := cfg2["locale"]; ok {
			t.Errorf("nested once prop reported cached by full path must be skipped: %v", cfg2)
		}
	}
	if _, ok := cached.OnceProps["config.locale"]; !ok {
		t.Error("onceProps[config.locale] metadata must persist on cached response")
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
			"posts": Scroll(ScrollConfig{CurrentPage: 1, NextPage: &next}, func() any { return []map[string]any{{"id": 1}} }),
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

// TestProtocol_ScrollPrependMergeIntent verifies that the
// X-Inertia-Infinite-Scroll-Merge-Intent header switches a Scroll prop
// between appending (mergeProps) and prepending (prependProps), matching the
// official ScrollProp::configureMergeIntent(). With intent "prepend",
// <key>.<wrapper> goes to prependProps and must NOT appear in mergeProps.
func TestProtocol_ScrollPrependMergeIntent(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Chat", Props{
			"messages": Scroll(ScrollConfig{CurrentPage: 2}, func() any { return []int{1} }),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Infinite-Scroll-Merge-Intent", "prepend")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	inPrepend := false
	for _, p := range page.PrependProps {
		if p == "messages.data" {
			inPrepend = true
		}
	}
	if !inPrepend {
		t.Errorf("prepend intent must put messages.data in prependProps: %v", page.PrependProps)
	}
	for _, m := range page.MergeProps {
		if m == "messages.data" {
			t.Errorf("prepend intent must NOT put messages.data in mergeProps: %v", page.MergeProps)
		}
	}
	// scrollProps metadata is still emitted regardless of merge direction.
	if page.ScrollProps["messages"].CurrentPage != 2 {
		t.Errorf("scrollProps[messages] = %+v", page.ScrollProps["messages"])
	}
}

// TestProtocol_ResetSuppressesMerge verifies that a prop listed in
// X-Inertia-Reset is not collected into mergeProps (the official resolver
// early-returns before adding merge metadata for a reset path).
func TestProtocol_ResetSuppressesMerge(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Feed", Props{
			"items":  Merge([]int{1, 2}),
			"others": Merge([]int{3}),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/feed", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Reset", "items")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, m := range page.MergeProps {
		if m == "items" {
			t.Errorf("reset must suppress 'items' from mergeProps: %v", page.MergeProps)
		}
	}
	// A non-reset merge prop is unaffected.
	found := false
	for _, m := range page.MergeProps {
		if m == "others" {
			found = true
		}
	}
	if !found {
		t.Errorf("non-reset merge prop 'others' must remain in mergeProps: %v", page.MergeProps)
	}
}

// TestProtocol_ResetScrollFlag verifies that scrollProps.<key>.reset reflects
// whether the key appears in X-Inertia-Reset (the scroll prop is still
// collected; only the per-key flag changes).
func TestProtocol_ResetScrollFlag(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	render := func(reset string) PageObject {
		h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i.Render(w, r, "Chat", Props{
				"messages": Scroll(ScrollConfig{CurrentPage: 1}, func() any { return []int{1} }),
			})
		}))
		req := httptest.NewRequest(http.MethodGet, "/chat", nil)
		req.Header.Set("X-Inertia", "true")
		if reset != "" {
			req.Header.Set("X-Inertia-Reset", reset)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var page PageObject
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}

	if got := render("messages").ScrollProps["messages"].Reset; !got {
		t.Error("scrollProps[messages].reset must be true when 'messages' is in X-Inertia-Reset")
	}
	if got := render("").ScrollProps["messages"].Reset; got {
		t.Error("scrollProps[messages].reset must be false without X-Inertia-Reset")
	}
}

// TestProtocol_ScrollWrapperAndAdapterRoundTrip complements
// TestProtocol_ScrollProps_WireFormat (which covers the default "data"
// wrapper + identity adapter) by exercising a CUSTOM paginator adapter and a
// CUSTOM wrapper ("items"). It asserts the full round trip: data nests under
// the custom wrapper, mergeProps lists <key>.<wrapper> using the SAME wrapper,
// and scrollProps carries the adapter-derived currentPage. This guards the
// emit-vs-consume wrapper-key invariant (a past once-.As() bug came from such
// a divergence).
func TestProtocol_ScrollWrapperAndAdapterRoundTrip(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	RegisterScrollAdapter(fakeAdapter{}) // matches fakePaginator, CurrentPage = cur

	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Feed", Props{
			"posts": Scroll(fakePaginator{cur: 2}, func() any {
				return []map[string]any{{"id": 1}, {"id": 2}}
			}, WithWrapper("items"), WithPageName("orders")),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/feed", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal page: %v\nbody=%s", err, rec.Body.String())
	}

	// props.posts.items holds the data (wrapper = "items", not "data").
	posts, ok := page.Props["posts"].(map[string]any)
	if !ok {
		t.Fatalf("props.posts not an object: %#v", page.Props["posts"])
	}
	if _, ok := posts["items"]; !ok {
		t.Errorf("data not nested under wrapper 'items': %#v", posts)
	}
	if _, ok := posts["data"]; ok {
		t.Errorf("data must NOT be under default 'data' key when wrapper is 'items': %#v", posts)
	}

	// mergeProps lists posts.items (the SAME wrapper).
	found := false
	for _, m := range page.MergeProps {
		if m == "posts.items" {
			found = true
		}
	}
	if !found {
		t.Errorf("mergeProps missing 'posts.items': %#v", page.MergeProps)
	}

	// scrollProps.posts carries the adapter-derived currentPage and the
	// WithPageName override flowed through deriveScroll to the wire format.
	if sc := page.ScrollProps["posts"]; sc.CurrentPage != 2 {
		t.Errorf("scrollProps[posts].currentPage = %d, want 2", sc.CurrentPage)
	}
	if sc := page.ScrollProps["posts"]; sc.PageName != "orders" {
		t.Errorf("scrollProps[posts].pageName = %q, want orders (WithPageName override)", sc.PageName)
	}
}

// TestProtocol_ScrollProp_LazyExcludedOnPartial proves the lazy guarantee
// the CHANGELOG promises: when a partial reload excludes the scroll prop, its
// data callback is never invoked. The goroutine that calls dataFn is only
// spawned for kept keys, so an excluded scroll prop must not run its query.
func TestProtocol_ScrollProp_LazyExcludedOnPartial(t *testing.T) {
	called := false
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Feed", Props{
			"posts":  Scroll(map[string]any{"currentPage": 1}, func() any { called = true; return nil }),
			"counts": []int{1, 2},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Feed")
	req.Header.Set("X-Inertia-Partial-Data", "counts") // posts excluded
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if called {
		t.Error("dataFn must not be called when the scroll prop is excluded by partial reload")
	}
}

func TestProtocol_NestedPrependPath(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Chat", Props{
			"chat": Merge(map[string]any{"messages": []int{1}}).Prepend("messages"),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range page.PrependProps {
		if p == "chat.messages" {
			found = true
		}
	}
	if !found {
		t.Errorf("prependProps must contain chat.messages: %v", page.PrependProps)
	}
	// A nested prepend target merges only chat.messages and replaces the
	// rest of chat, so the root "chat" key must NOT appear in mergeProps.
	for _, m := range page.MergeProps {
		if m == "chat" {
			t.Errorf("root 'chat' must not be in mergeProps for a nested prepend: %v", page.MergeProps)
		}
	}
}

// TestProtocol_PrecognitionEndToEnd drives the full precognition flow over a
// real http server: a handler validates, calls Precognition, and returns
// early on a precognitive request. Clean input → 204 + Precognition-Success;
// bad input → 422 + errors JSON; the real action (and its redirect) never
// runs on a precognitive request.
func TestProtocol_PrecognitionEndToEnd(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	actionRan := false
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "" {
			ValidationErrors(r).Add("name", "required")
		}
		if i.Precognition(w, r) {
			return
		}
		actionRan = true
		i.Redirect(w, r, "/done")
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// Clean precognitive request → 204 + Precognition-Success, action NOT run.
	reqOK, _ := http.NewRequest(http.MethodGet, srv.URL+"/submit?name=ada", nil)
	reqOK.Header.Set("Precognition", "true")
	respOK, err := client.Do(reqOK)
	if err != nil {
		t.Fatalf("clean precog: %v", err)
	}
	_ = respOK.Body.Close()
	if respOK.StatusCode != http.StatusNoContent {
		t.Errorf("clean precog status = %d, want 204", respOK.StatusCode)
	}
	if respOK.Header.Get("Precognition-Success") != "true" {
		t.Error("clean precog missing Precognition-Success")
	}
	if actionRan {
		t.Error("real action must not run on a precognitive request")
	}

	// Bad precognitive request → 422 + errors, action NOT run.
	reqBad, _ := http.NewRequest(http.MethodGet, srv.URL+"/submit", nil)
	reqBad.Header.Set("Precognition", "true")
	respBad, err := client.Do(reqBad)
	if err != nil {
		t.Fatalf("bad precog: %v", err)
	}
	defer func() { _ = respBad.Body.Close() }()
	if respBad.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad precog status = %d, want 422", respBad.StatusCode)
	}
	var body map[string]map[string]string
	if err := json.NewDecoder(respBad.Body).Decode(&body); err != nil {
		t.Fatalf("decode 422 body: %v", err)
	}
	if body["errors"]["name"] != "required" {
		t.Errorf("422 body = %v, want errors.name=required", body)
	}
	if actionRan {
		t.Error("real action must not run on a failed precognitive request")
	}
}

func TestProtocol_ComponentMismatch_FallsBackToFull(t *testing.T) {
	// A partial-component header that does not match the rendered component is
	// not a partial reload: the response is full, so an Optional prop is still
	// excluded from the initial full response, and an eager prop is present.
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/Index", Props{
			"users":    []string{"alice"},
			"optional": Optional(func() (any, error) { return "lazy", nil }),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Other/Page") // does NOT match
	req.Header.Set("X-Inertia-Partial-Data", "optional")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if _, ok := page.Props["users"]; !ok {
		t.Errorf("eager prop must be present on full fallback: %v", page.Props)
	}
	if _, ok := page.Props["optional"]; ok {
		t.Errorf("component mismatch is not a partial; Optional must be excluded: %v", page.Props)
	}
}

func TestProtocol_NestedPartialDataSelector(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "App", Props{
			"auth": map[string]any{
				"user":  Optional(func() (any, error) { return "alice", nil }),
				"token": Optional(func() (any, error) { return "xyz", nil }),
			},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "App")
	req.Header.Set("X-Inertia-Partial-Data", "auth.user")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	auth, ok := page.Props["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth must be present: %v", page.Props)
	}
	if auth["user"] != "alice" {
		t.Errorf("auth.user must be included: %v", auth)
	}
	if _, ok := auth["token"]; ok {
		t.Errorf("auth.token must be excluded by the dot selector: %v", auth)
	}
}

func TestProtocol_NestedPartialExcept(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "App", Props{
			"auth": map[string]any{"user": "alice", "token": "xyz"},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "App")
	req.Header.Set("X-Inertia-Partial-Except", "auth.token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	auth, _ := page.Props["auth"].(map[string]any)
	if auth["user"] != "alice" {
		t.Errorf("auth.user must remain: %v", auth)
	}
	if _, ok := auth["token"]; ok {
		t.Errorf("auth.token must be excluded by nested except: %v", auth)
	}
}

func TestProtocol_TopLevelDotKeyUnpacks(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "App", Props{
			"auth.user": "alice",
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var page PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	auth, ok := page.Props["auth"].(map[string]any)
	if !ok || auth["user"] != "alice" {
		t.Errorf("top-level dot key must unpack to nested auth.user: %v", page.Props)
	}
}

// TestProtocol_ArrayElementOptionalResolvedOnPartial is the e2e for FIX #3: a
// partial only=["foos"] over an indexed array whose elements each carry an
// Optional field resolves that field inside every element.
func TestProtocol_ArrayElementOptionalResolvedOnPartial(t *testing.T) {
	i, _ := New(Config{Session: session.NewMemory()})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Listing", Props{
			"foos": []any{
				map[string]any{"name": "First", "bar": Optional(func() (any, error) { return "b1", nil })},
				map[string]any{"name": "Second", "bar": Optional(func() (any, error) { return "b2", nil })},
			},
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/listing", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Partial-Component", "Listing")
	req.Header.Set("X-Inertia-Partial-Data", "foos")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	foos, ok := page.Props["foos"].([]any)
	if !ok || len(foos) != 2 {
		t.Fatalf("foos must be a 2-element array: %#v", page.Props["foos"])
	}
	e0, _ := foos[0].(map[string]any)
	if e0 == nil || e0["name"] != "First" || e0["bar"] != "b1" {
		t.Errorf("foos[0] must include name and resolved Optional bar: %#v", e0)
	}
	e1, _ := foos[1].(map[string]any)
	if e1 == nil || e1["name"] != "Second" || e1["bar"] != "b2" {
		t.Errorf("foos[1] must include name and resolved Optional bar: %#v", e1)
	}
}
