package ssr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTP_DefaultURLs(t *testing.T) {
	c := NewHTTP("http://127.0.0.1:13714")
	if c.URL != "http://127.0.0.1:13714/render" {
		t.Errorf("URL: got %q, want %q", c.URL, "http://127.0.0.1:13714/render")
	}
	if c.Health != "http://127.0.0.1:13714/health" {
		t.Errorf("Health: got %q, want %q", c.Health, "http://127.0.0.1:13714/health")
	}
	if c.HTTP == nil {
		t.Fatal("HTTP client should be non-nil")
	}
	if c.HTTP.Timeout != 2*time.Second {
		t.Errorf("Timeout: got %v, want 2s", c.HTTP.Timeout)
	}
}

func TestNewHTTP_TrimsTrailingSlash(t *testing.T) {
	c := NewHTTP("http://127.0.0.1:13714/")
	if c.URL != "http://127.0.0.1:13714/render" {
		t.Errorf("URL: got %q, want no double slash", c.URL)
	}
	if c.Health != "http://127.0.0.1:13714/health" {
		t.Errorf("Health: got %q, want no double slash", c.Health)
	}
}

// Compile-time check: ensure HTTPClient is a struct with the exact field types.
var _ = (&HTTPClient{
	URL:    "",
	Health: "",
	HTTP:   &http.Client{},
}).URL

func TestRender_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"head":["<title>Hi</title>","<meta name=\"x\" content=\"y\">"],"body":"<div id=\"app\">SSR</div>"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	head, body, err := c.Render(context.Background(), json.RawMessage(`{"component":"X"}`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(head) != 2 {
		t.Errorf("head len = %d, want 2", len(head))
	}
	if head[0] != "<title>Hi</title>" {
		t.Errorf("head[0] = %q", head[0])
	}
	if body != `<div id="app">SSR</div>` {
		t.Errorf("body = %q", body)
	}
}

func TestRender_PostsRawPageJSON(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"head":[],"body":""}`))
	}))
	t.Cleanup(srv.Close)

	page := json.RawMessage(`{"component":"Users/Index","props":{"a":1}}`)
	c := NewHTTP(srv.URL)
	if _, _, err := c.Render(context.Background(), page); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(captured, []byte(page)) {
		t.Errorf("server received %q, want %q", captured, page)
	}
}

func TestRender_ContentTypeHeader(t *testing.T) {
	var ct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"head":[],"body":""}`))
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	if _, _, err := c.Render(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestRender_NullHead_BecomesEmptySlice(t *testing.T) {
	// Per spec §3.2: missing or null head decodes to nil slice and
	// produces empty InertiaHead at the call site.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"head":null,"body":"<div></div>"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	head, body, err := c.Render(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 0 {
		t.Errorf("expected empty head, got %v", head)
	}
	if body != "<div></div>" {
		t.Errorf("body = %q", body)
	}
}

func TestRender_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream down"))
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	_, _, err := c.Render(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error should contain status code, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "upstream down") {
		t.Errorf("error should contain response snippet, got %q", err.Error())
	}
}

func TestRender_MalformedJSON_ReturnsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	_, _, err := c.Render(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got %q", err.Error())
	}
}

func TestRender_RespectsContextCancel(t *testing.T) {
	// Server blocks until the request body is closed (i.e. ctx cancel
	// propagates). It returns nothing useful — the test just verifies
	// the client errors out promptly when ctx is canceled.
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-hold
	}))
	t.Cleanup(func() {
		close(hold)
		srv.Close()
	})

	c := NewHTTP(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled
	_, _, err := c.Render(ctx, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestRender_TimeoutFires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)

	c := NewHTTP(srv.URL)
	c.HTTP = &http.Client{Timeout: 20 * time.Millisecond}
	_, _, err := c.Render(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
