package vite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEntry_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
	}{
		{
			name: "primary_entry",
			entry: Entry{
				Src:            "resources/js/app.ts",
				Name:           "app",
				File:           "assets/app-Hash.js",
				CSS:            []string{"assets/app-Hash.css"},
				Assets:         []string{"assets/logo.png"},
				IsEntry:        true,
				IsDynamicEntry: false,
				Imports:        []string{"_shared.js"},
				DynamicImports: []string{"_lazy.js"},
			},
		},
		{
			name: "dynamic_entry",
			entry: Entry{
				File:           "assets/lazy-Hash.js",
				IsDynamicEntry: true,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.entry)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var out Entry
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(tc.entry, out) {
				t.Errorf("round-trip mismatch\nin:  %+v\nout: %+v", tc.entry, out)
			}
		})
	}
}

func TestErrManifestNotFound_IsExported(t *testing.T) {
	if ErrManifestNotFound == nil {
		t.Fatal("ErrManifestNotFound should be non-nil")
	}
	wrapped := fmt.Errorf("loading manifest: %w", ErrManifestNotFound)
	if !errors.Is(wrapped, ErrManifestNotFound) {
		t.Fatal("wrapped error should unwrap to ErrManifestNotFound")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "manifest.json")
	body := `{
		"resources/js/app.ts": {
			"file": "assets/app-Hash.js",
			"isEntry": true
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m == nil {
		t.Fatal("Load returned nil manifest")
	}
	e, ok := m.Entry("resources/js/app.ts")
	if !ok {
		t.Fatal("entry should be present")
	}
	if e.File != "assets/app-Hash.js" {
		t.Errorf("File: %q", e.File)
	}
}

func TestLoad_FileNotFound_ReturnsErrManifestNotFound(t *testing.T) {
	_, err := Load("/nonexistent/manifest.json")
	if !errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("expected ErrManifestNotFound, got %v", err)
	}
}

func TestLoad_MalformedJSON_ReturnsWrappedError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "manifest.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("malformed JSON should not match ErrManifestNotFound, got %v", err)
	}
}

func TestMustLoad_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = MustLoad("/nonexistent/manifest.json")
}

func TestDev_Constructs(t *testing.T) {
	m := Dev("http://localhost:5173")
	if m == nil {
		t.Fatal("Dev returned nil")
	}
}

func TestDev_TrimsTrailingSlash(t *testing.T) {
	// We assert the user-observable behavior via Asset later; for now
	// just construct two equivalent Manifests and rely on Asset tests.
	m1 := Dev("http://localhost:5173")
	m2 := Dev("http://localhost:5173/")
	if m1 == nil || m2 == nil {
		t.Fatal("Dev returned nil")
	}
}

func TestAsset_ProdMode_ReturnsHashedURL(t *testing.T) {
	m := newProdManifest(t, map[string]Entry{
		"resources/images/logo.png": {File: "assets/logo-Hash.png"},
	})
	got := m.Asset("resources/images/logo.png")
	if got != "/assets/logo-Hash.png" {
		t.Errorf("got %q, want %q", got, "/assets/logo-Hash.png")
	}
}

func TestAsset_DevMode_PrependsBaseURL(t *testing.T) {
	m := Dev("http://localhost:5173")
	got := m.Asset("resources/images/logo.png")
	if got != "http://localhost:5173/resources/images/logo.png" {
		t.Errorf("got %q", got)
	}
}

func TestAsset_DevMode_TrailingSlashOnBaseStripped(t *testing.T) {
	m := Dev("http://localhost:5173/")
	got := m.Asset("resources/images/logo.png")
	if got != "http://localhost:5173/resources/images/logo.png" {
		t.Errorf("got %q", got)
	}
}

func TestAsset_MissingEntry_ReturnsOriginalAndLogsOnce(t *testing.T) {
	// Capture log output via a slog handler with a buffer.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	m := newProdManifest(t, map[string]Entry{})
	m.SetLogger(logger)

	got1 := m.Asset("missing.png")
	if got1 != "missing.png" {
		t.Errorf("first call: got %q, want original 'missing.png'", got1)
	}
	got2 := m.Asset("missing.png")
	if got2 != "missing.png" {
		t.Errorf("second call: got %q, want original 'missing.png'", got2)
	}
	count := strings.Count(buf.String(), "missing.png")
	if count == 0 {
		t.Fatalf("expected at least one log entry, got none")
	}
	logEntries := strings.Count(buf.String(), "vite: entry not found")
	if logEntries != 1 {
		t.Errorf("expected 1 warning line, got %d (log buf: %s)", logEntries, buf.String())
	}
}

func TestAsset_ZeroValueManifest_NoPanic(t *testing.T) {
	// Construct a Manifest without going through Load/Dev — the
	// atomic.Pointer logger is nil. logMissing must not deref nil.
	var m Manifest
	got := m.Asset("missing.png")
	if got != "missing.png" {
		t.Errorf("got %q, want %q", got, "missing.png")
	}
}

// newProdManifest is a test helper that builds a prod-mode Manifest
// from an entries map without touching the filesystem.
func newProdManifest(t *testing.T, entries map[string]Entry) *Manifest {
	t.Helper()
	m := &Manifest{
		entries: entries,
		base:    "/",
		isDev:   false,
	}
	m.logger.Store(slog.Default())
	return m
}
