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

import "errors"

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
