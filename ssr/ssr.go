// Package ssr implements an HTTP client for the Inertia.js server-side
// rendering (SSR) protocol. The *HTTPClient type satisfies the
// inertia.SSRClient interface by POSTing serialized PageObjects to a
// Node SSR service (e.g. @inertiajs/server) and decoding head/body
// fragments from the response.
//
// Construct via NewHTTP("http://127.0.0.1:13714") for the conventional
// loopback setup, or build *HTTPClient directly to override URL,
// Health, or the underlying *http.Client (custom Transport, retries,
// instrumentation, etc.).
//
// This package has zero dependencies on the main inertia package: the
// SSRClient contract returns stdlib types only ([]string, string,
// error), so users can consume *HTTPClient.Render directly without
// pulling in the core package.
package ssr

import (
	"net/http"
	"strings"
	"time"
)

// HTTPClient speaks the Inertia.js SSR HTTP protocol. Field values are
// exported so users can override defaults — typically only HTTP, to
// supply a custom Transport or a longer timeout for cold-start
// environments.
type HTTPClient struct {
	URL    string       // POST endpoint; default base + "/render"
	Health string       // GET endpoint; default base + "/health"
	HTTP   *http.Client // default &http.Client{Timeout: 2 * time.Second}
}

// NewHTTP returns an HTTPClient pointing at baseURL. A trailing slash
// on baseURL is stripped. The default *http.Client has a 2-second
// timeout, suitable for a typical loopback Node SSR service; override
// HTTP for cold-start or remote-cluster setups.
func NewHTTP(baseURL string) *HTTPClient {
	base := strings.TrimRight(baseURL, "/")
	return &HTTPClient{
		URL:    base + "/render",
		Health: base + "/health",
		HTTP:   &http.Client{Timeout: 2 * time.Second},
	}
}
