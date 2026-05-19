// Package vite implements a Vite (https://vite.dev) manifest reader and
// matching HTML helpers that satisfy inertia.ViteHelper.
//
// Construct a Manifest with one of three functions:
//
//	m, err := vite.Load("public/build/manifest.json") // production
//	m := vite.MustLoad("public/build/manifest.json")  // production, panics on error
//	m := vite.Dev("http://localhost:5173")            // development
//
// All four template helpers (Tag, Asset, CSS, ReactRefresh) are safe for
// concurrent use after construction. The manifest itself is read once at
// startup and never refreshed — restart the process to pick up a new
// build.
package vite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// ErrManifestNotFound is returned by Load when the manifest file is absent.
var ErrManifestNotFound = errors.New("vite: manifest file not found")

// Entry mirrors a single record in a Vite manifest.json file. The field
// set matches Vite 5/6/7 output and is forward-compatible: extra unknown
// fields in newer Vite versions are silently ignored by encoding/json.
type Entry struct {
	Src            string   `json:"src,omitempty"`
	Name           string   `json:"name,omitempty"`
	File           string   `json:"file"`
	CSS            []string `json:"css,omitempty"`
	Assets         []string `json:"assets,omitempty"`
	IsEntry        bool     `json:"isEntry,omitempty"`
	IsDynamicEntry bool     `json:"isDynamicEntry,omitempty"`
	Imports        []string `json:"imports,omitempty"`
	DynamicImports []string `json:"dynamicImports,omitempty"`
}

// Manifest holds a parsed Vite manifest plus mode metadata. After
// construction the entries and base are immutable and safe for concurrent
// reads. SetLogger may be called at any time and is safe to interleave
// with concurrent log writes; in practice it should run once at startup.
type Manifest struct {
	entries map[string]Entry
	base    string // prod: "/" ; dev: stripped trailing-slash baseURL
	isDev   bool

	warned sync.Map // entry name → struct{} for log-once
	logger atomic.Pointer[slog.Logger]
}

// Load reads and parses a Vite manifest from path.
// Returns ErrManifestNotFound when the file does not exist; wraps
// JSON parse errors with "vite: parse manifest: ..." prefix.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrManifestNotFound, path)
		}
		return nil, err
	}
	var entries map[string]Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("vite: parse manifest: %w", err)
	}
	m := &Manifest{
		entries: entries,
		base:    "/",
		isDev:   false,
	}
	m.logger.Store(slog.Default())
	return m, nil
}

// MustLoad is like Load but panics on any error. Use it during application
// startup where a missing manifest is unrecoverable.
func MustLoad(path string) *Manifest {
	m, err := Load(path)
	if err != nil {
		panic(err)
	}
	return m
}

// Dev constructs a Manifest pointing at a running Vite dev server.
// baseURL must include the scheme + host + optional base path (e.g.
// "http://localhost:5173" or "http://localhost:5173/build"). A trailing
// slash is removed automatically.
func Dev(baseURL string) *Manifest {
	m := &Manifest{
		base:  strings.TrimRight(baseURL, "/"),
		isDev: true,
	}
	m.logger.Store(slog.Default())
	return m
}

// Entry returns the manifest entry for name. The second return value is
// false when the entry is absent. This is a low-level data accessor; for
// HTML output use Tag / Asset / CSS / ReactRefresh.
func (m *Manifest) Entry(name string) (Entry, bool) {
	e, ok := m.entries[name]
	return e, ok
}

// SetLogger replaces the slog.Logger used for missing-entry warnings.
// Safe for concurrent use; typically called once during application
// startup, before the manifest is wired into request handlers.
func (m *Manifest) SetLogger(l *slog.Logger) {
	m.logger.Store(l)
}

// Asset resolves a single asset entry to its URL. Use for non-script,
// non-stylesheet resources (e.g. <img src=...>, <link rel="icon" ...>).
//
// In prod mode the URL is the manifest base ("/") + the entry's File field.
// In dev mode the URL is baseURL + "/" + entry. The entry argument is the
// Vite source path (e.g. "resources/images/logo.png") and must not have a
// leading slash.
//
// Missing entries return the original entry string (so layout doesn't
// break on a typo) and log a one-time slog.Warn for that entry.
func (m *Manifest) Asset(entry string) string {
	if m.isDev {
		return m.base + "/" + entry
	}
	e, ok := m.entries[entry]
	if !ok {
		m.logMissing(entry)
		return entry
	}
	return m.base + e.File
}

// logMissing emits a slog.Warn the first time entry is observed missing.
// Subsequent calls for the same entry are silent. Concurrent-safe via
// sync.Map.LoadOrStore.
func (m *Manifest) logMissing(entry string) {
	if _, loaded := m.warned.LoadOrStore(entry, struct{}{}); loaded {
		return
	}
	if l := m.logger.Load(); l != nil {
		l.Warn("vite: entry not found", "entry", entry)
	}
}
