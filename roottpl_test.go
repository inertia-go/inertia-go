package inertia

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRenderRootHTML_FallbackTemplate(t *testing.T) {
	i, _ := New(Config{Session: stubSession{}})
	var buf bytes.Buffer
	err := i.renderRoot(&buf, RootData{
		InertiaBody: `<div id="app" data-page="{}"></div>`,
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
