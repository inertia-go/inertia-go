package inertia

import (
	"bytes"
	"html/template"
	"io"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRenderRootHTML_FallbackTemplate(t *testing.T) {
	i, _ := New(Config{Session: stubSession{}})
	var buf bytes.Buffer
	err := i.renderRoot(&buf, RootData{
		InertiaBody: `<script data-page="app" type="application/json">{}</script><div id="app"></div>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<div id="app"`) {
		t.Errorf("fallback template missing body marker: %s", buf.String())
	}
}

func TestRenderRootHTML_UsesProvidedTemplate(t *testing.T) {
	fs := fstest.MapFS{
		"app.html": {Data: []byte(`<!doctype html><html><body>{{ .InertiaBody }}</body></html>`)},
	}
	i, _ := New(Config{
		Session:    stubSession{},
		RootView:   "app.html",
		TemplateFS: fs,
	})
	var buf bytes.Buffer
	err := i.renderRoot(&buf, RootData{
		InertiaBody: `<div id="app"></div>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), `<!doctype html>`) {
		t.Errorf("custom template not used: %s", buf.String())
	}
}

func TestRenderRootHTML_RootRenderHookOverrides(t *testing.T) {
	called := false
	i, _ := New(Config{
		Session: stubSession{},
		RootRender: func(w io.Writer, data RootData) error {
			called = true
			_, _ = w.Write([]byte("custom"))
			return nil
		},
	})
	var buf bytes.Buffer
	if err := i.renderRoot(&buf, RootData{}); err != nil {
		t.Fatal(err)
	}
	if !called || buf.String() != "custom" {
		t.Errorf("hook not used: %q", buf.String())
	}
}

// stubViteHelper implements ViteHelper for tests, returning canned strings.
type stubViteHelper struct {
	tagOut, assetOut, cssOut, refreshOut string
}

func (s stubViteHelper) Tag(_ string) template.HTML  { return template.HTML(s.tagOut) }
func (s stubViteHelper) Asset(_ string) string       { return s.assetOut }
func (s stubViteHelper) CSS(_ string) template.HTML  { return template.HTML(s.cssOut) }
func (s stubViteHelper) ReactRefresh() template.HTML { return template.HTML(s.refreshOut) }

func TestRenderRootHTML_ViteFuncMap_PropagatesToTemplate(t *testing.T) {
	fs := fstest.MapFS{
		"app.html": {Data: []byte(`{{ vite "x" }}|{{ viteAsset "y" }}|{{ viteCSS "z" }}|{{ viteReactRefresh }}`)},
	}
	i, _ := New(Config{
		Session:    stubSession{},
		RootView:   "app.html",
		TemplateFS: fs,
		Vite: stubViteHelper{
			tagOut:     "<TAG>",
			assetOut:   "ASSET",
			cssOut:     "<CSS>",
			refreshOut: "<REFRESH>",
		},
	})
	var buf bytes.Buffer
	if err := i.renderRoot(&buf, RootData{}); err != nil {
		t.Fatal(err)
	}
	want := "<TAG>|ASSET|<CSS>|<REFRESH>"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestRenderRootHTML_NoVite_HelpersAreNoop(t *testing.T) {
	fs := fstest.MapFS{
		"app.html": {Data: []byte(`{{ vite "x" }}|{{ viteAsset "y" }}|{{ viteCSS "z" }}|{{ viteReactRefresh }}`)},
	}
	i, _ := New(Config{
		Session:    stubSession{},
		RootView:   "app.html",
		TemplateFS: fs,
		// Vite intentionally nil
	})
	var buf bytes.Buffer
	if err := i.renderRoot(&buf, RootData{}); err != nil {
		t.Fatalf("template should still parse and execute: %v", err)
	}
	if buf.String() != "|||" {
		t.Errorf("expected empty helpers, got %q", buf.String())
	}
}
