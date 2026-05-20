package inertia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inertia-go/inertia-go/session"
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
	if !strings.Contains(body, `<script data-page="app" type="application/json">`) {
		t.Errorf("missing v3 script tag: %s", body)
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

func TestRender_InitialHTML_PageObjectIsValidJSONInsideScript(t *testing.T) {
	// Regression: the script element with type="application/json" must
	// carry valid JSON that survives the </script close-sequence escape.
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prop value contains literal "</script>" bytes; renderer must
		// escape them so the script block does not terminate early.
		i.Render(w, r, "Users/Index", Props{
			"hostile": "</script><script>alert(1)</script>",
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	const open = `<script data-page="app" type="application/json">`
	const close = `</script>`
	openIdx := strings.Index(body, open)
	if openIdx < 0 {
		t.Fatalf("script open tag not found: %s", body)
	}
	rest := body[openIdx+len(open):]
	closeIdx := strings.Index(rest, close)
	if closeIdx < 0 {
		t.Fatalf("script close tag not found: %s", body)
	}
	rawJSON := rest[:closeIdx]
	if strings.Contains(rawJSON, "</script>") {
		t.Fatalf("inline script payload must not contain literal </script>: %s", rawJSON)
	}

	var page PageObject
	if err := json.Unmarshal([]byte(rawJSON), &page); err != nil {
		t.Fatalf("inline script JSON does not parse: %v\nraw=%q", err, rawJSON)
	}
	if page.Props["hostile"] != "</script><script>alert(1)</script>" {
		t.Errorf("hostile prop did not round-trip: %v", page.Props["hostile"])
	}
}

func TestRenderScriptSafe_EscapesCloseTag(t *testing.T) {
	// Raw input where < was NOT pre-escaped (e.g. a future caller passing
	// json.RawMessage or using an encoder with HTMLEscape disabled).
	raw := []byte(`{"h":"</script><script>alert(1)"}`)
	out := renderScriptSafe(raw)
	// Strip the leading <script ...> wrapper so we only inspect the inline payload.
	const open = `<script data-page="app" type="application/json">`
	if !strings.HasPrefix(out, open) {
		t.Fatalf("missing open tag: %s", out)
	}
	payload := out[len(open):]
	closeIdx := strings.Index(payload, `</script>`)
	if closeIdx < 0 {
		t.Fatalf("missing closing tag: %s", out)
	}
	inline := payload[:closeIdx]
	if strings.Contains(inline, `</script`) {
		t.Fatalf("renderScriptSafe did not neutralize </script in raw input; inline=%q", inline)
	}
	if !strings.Contains(inline, `\u003c/script`) {
		t.Fatalf("expected \\u003c/script escape in inline payload; got %q", inline)
	}
}

// stubSSRClient is a test-only SSRClient with canned returns and an
// invocation counter for skip-check tests.
type stubSSRClient struct {
	head      []string
	body      string
	err       error
	callCount int
	lastPage  json.RawMessage
}

func (s *stubSSRClient) Render(_ context.Context, page json.RawMessage) ([]string, string, error) {
	s.callCount++
	s.lastPage = page
	return s.head, s.body, s.err
}

func TestRender_SSREnabled_InjectsHeadAndBody(t *testing.T) {
	stub := &stubSSRClient{
		head: []string{"<title>SSR Title</title>", `<meta name="x" content="y">`},
		body: `<div id="app">SSR-rendered</div>`,
	}
	i, _ := New(Config{Session: stubSession{}, SSR: stub})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users", nil)
	i.Render(w, r, "Users/Index", Props{"foo": "bar"})

	if stub.callCount != 1 {
		t.Errorf("SSR called %d times, want 1", stub.callCount)
	}
	out := w.Body.String()
	if !strings.Contains(out, "<title>SSR Title</title>") {
		t.Errorf("head fragment missing: %s", out)
	}
	if !strings.Contains(out, `<meta name="x" content="y">`) {
		t.Errorf("second head fragment missing: %s", out)
	}
	if !strings.Contains(out, `<div id="app">SSR-rendered</div>`) {
		t.Errorf("SSR body missing: %s", out)
	}
	if strings.Contains(out, `<script data-page="app"`) {
		t.Errorf("CSR fallback body should not be present when SSR succeeded: %s", out)
	}

	// Verify the SSR client received the serialized PageObject — not
	// some pre-marshal value. The page should contain the component
	// name and prop key/value.
	if !strings.Contains(string(stub.lastPage), `"component":"Users/Index"`) {
		t.Errorf("lastPage missing component: %s", string(stub.lastPage))
	}
	if !strings.Contains(string(stub.lastPage), `"foo":"bar"`) {
		t.Errorf("lastPage missing props: %s", string(stub.lastPage))
	}
}

func TestRender_SSRHeadJoinedOnNewline(t *testing.T) {
	stub := &stubSSRClient{
		head: []string{"a", "b"},
		body: `<div id="app"></div>`,
	}
	i, _ := New(Config{Session: stubSession{}, SSR: stub})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	i.Render(w, r, "X", Props{})

	if !strings.Contains(w.Body.String(), "a\nb") {
		t.Errorf("head fragments should be newline-joined: %s", w.Body.String())
	}
}

func TestRender_SSRFails_FailSoft_FallsBackToCSR(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	stub := &stubSSRClient{err: errors.New("connection refused")}
	i, _ := New(Config{Session: stubSession{}, SSR: stub, Logger: logger})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	i.Render(w, r, "Dashboard", Props{})

	out := w.Body.String()
	if !strings.Contains(out, `<script data-page="app"`) {
		t.Errorf("expected CSR fallback (script tag), got: %s", out)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	log := logBuf.String()
	if !strings.Contains(log, "ssr render failed") {
		t.Errorf("expected SSR warn log, got: %s", log)
	}
	if !strings.Contains(log, "/dashboard") {
		t.Errorf("expected URL in log, got: %s", log)
	}
}

func TestRender_SSRFails_SSRRequired_RoutesToErrorHandler(t *testing.T) {
	stub := &stubSSRClient{err: errors.New("oops")}
	var handlerErr error
	handlerCalls := 0
	i, _ := New(Config{
		Session:     stubSession{},
		SSR:         stub,
		SSRRequired: true,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			handlerCalls++
			handlerErr = err
			w.WriteHeader(http.StatusInternalServerError)
		},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	i.Render(w, r, "X", Props{})

	if handlerCalls != 1 {
		t.Fatalf("ErrorHandler called %d times, want 1", handlerCalls)
	}
	if !errors.Is(handlerErr, ErrSSRUnavailable) {
		t.Errorf("error should wrap ErrSSRUnavailable, got %v", handlerErr)
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), `<script data-page="app"`) {
		t.Errorf("CSR fallback should not be rendered in fail-hard mode: %s", w.Body.String())
	}
}

func TestRender_InertiaJSON_SkipsSSR(t *testing.T) {
	stub := &stubSSRClient{
		head: []string{"<title>X</title>"},
		body: `<div>SSR</div>`,
	}
	i, _ := New(Config{Session: stubSession{}, SSR: stub})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Inertia", "true")
	// Run through Middleware so FromRequest populates IsInertia=true.
	i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "X", Props{})
	})).ServeHTTP(w, r)

	if stub.callCount != 0 {
		t.Errorf("SSR should not be called for Inertia XHR; got %d calls", stub.callCount)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON response, got %q", w.Header().Get("Content-Type"))
	}
}

func TestPreserveFragment_DefaultAndOverride(t *testing.T) {
	// Global default true, emitted when no override.
	i, _ := New(Config{Session: session.NewMemory(), PreserveFragment: true})
	render := func(setup func(r *http.Request)) PageObject {
		h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if setup != nil {
				setup(r)
			}
			i.Render(w, r, "Home", Props{"x": 1})
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Inertia", "true")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var p PageObject
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if !render(nil).PreserveFragment {
		t.Error("global default true must emit preserveFragment")
	}
	if render(func(r *http.Request) { SetPreserveFragment(r, false) }).PreserveFragment {
		t.Error("override false must turn off preserveFragment")
	}

	j, _ := New(Config{Session: session.NewMemory(), PreserveFragment: false})
	hh := j.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetPreserveFragment(r, true)
		j.Render(w, r, "Home", Props{"x": 1})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	hh.ServeHTTP(rec, req)
	var p PageObject
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if !p.PreserveFragment {
		t.Error("override true must turn on preserveFragment over global false")
	}
}

func TestRedirect_FlashesErrorsAndMessages(t *testing.T) {
	store := session.NewMemory()
	i, _ := New(Config{Session: store})

	// POST that fails validation and redirects-back.
	createHandler := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("email", "invalid")
		Flash(r).Set("success", "Saved")
		i.Redirect(w, r, "/new")
	}))

	req := httptest.NewRequest(http.MethodPost, "/users", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	createHandler.ServeHTTP(rec, req)

	// Verify redirect actually happened.
	if rec.Code != http.StatusFound {
		// POST -> 302 (Inertia v3 promotes only PUT/PATCH/DELETE to 303;
		// POST stays 302 unless this changes in the protocol).
		t.Fatalf("redirect code: %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set after Redirect flashed collectors")
	}

	// Follow-up GET should see errors + flash injected as props.
	formHandler := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/New", Props{})
	}))
	req2 := httptest.NewRequest(http.MethodGet, "/new", nil)
	req2.Header.Set("X-Inertia", "true")
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	formHandler.ServeHTTP(rec2, req2)

	var page PageObject
	if err := json.Unmarshal(rec2.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errs, _ := page.Props["errors"].(map[string]any)
	if errs["email"] != "invalid" {
		t.Errorf("expected errors.email=invalid, got %v", page.Props["errors"])
	}
	flash, _ := page.Props["flash"].(map[string]any)
	if flash["success"] != "Saved" {
		t.Errorf("expected flash.success=Saved, got %v", page.Props["flash"])
	}
}
