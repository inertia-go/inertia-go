package ssr

import (
	"net/http"
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
