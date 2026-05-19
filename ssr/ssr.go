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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// Render POSTs page (a serialized Inertia PageObject) to c.URL with
// Content-Type application/json, decodes the JSON response into head
// and body, and returns them. The context is forwarded to the
// underlying HTTP request so callers' cancellation propagates.
//
// Returns an error for transport failures (timeout, dial, ctx cancel),
// non-2xx responses (with a short response-body snippet for diagnostics),
// or malformed JSON in the response.
func (c *HTTPClient) Render(ctx context.Context, page json.RawMessage) (head []string, body string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(page))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, "", fmt.Errorf("ssr: render status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var payload struct {
		Head []string `json:"head"`
		Body string   `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("ssr: decode response: %w", err)
	}
	return payload.Head, payload.Body, nil
}

// Ping issues a GET to c.Health. Returns nil on any 2xx. Useful for
// liveness probes in your own health-check handlers or a startup gate;
// the package never calls Ping automatically.
func (c *HTTPClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Health, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ssr: health check returned status %d", resp.StatusCode)
	}
	return nil
}
