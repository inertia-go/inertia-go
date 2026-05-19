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

func TestRender_InitialHTML_DataPageIsValidJSONAfterHTMLParse(t *testing.T) {
	// Regression test: the data-page attribute must contain JSON that a
	// browser-grade HTML parser can extract and JSON.parse. The attribute
	// is single-quoted so JSON's double quotes don't terminate it; HTML
	// entities (&amp;, &lt;, etc.) must be decoded before JSON parsing.
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Users/Index", Props{"users": []int{1, 2}})
	}))
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	// Locate data-page='...' and extract through to the matching single quote.
	const marker = `data-page='`
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("data-page='...' marker not found in body: %s", body)
	}
	rest := body[idx+len(marker):]
	end := strings.IndexByte(rest, '\'')
	if end < 0 {
		t.Fatalf("no closing single quote for data-page in body: %s", body)
	}
	rawAttr := rest[:end]

	// html/template entity-encodes the JSON when interpolating into HTML.
	decoded := strings.NewReplacer(
		"&#34;", `"`,
		"&quot;", `"`,
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&#43;", "+",
	).Replace(rawAttr)

	var page PageObject
	if err := json.Unmarshal([]byte(decoded), &page); err != nil {
		t.Fatalf("data-page is not valid JSON after HTML decode: %v\nraw=%q\ndecoded=%q", err, rawAttr, decoded)
	}
	if page.Component != "Users/Index" {
		t.Errorf("component: %q", page.Component)
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
	if strings.Contains(out, `data-page=`) {
		t.Errorf("CSR fallback body should not be present when SSR succeeded: %s", out)
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
	if !strings.Contains(out, `data-page=`) {
		t.Errorf("expected CSR fallback (data-page attribute), got: %s", out)
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
	if strings.Contains(w.Body.String(), `data-page=`) {
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
