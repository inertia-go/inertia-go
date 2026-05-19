package vite

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestEntry_JSONRoundTrip(t *testing.T) {
	in := Entry{
		Src:            "resources/js/app.ts",
		Name:           "app",
		File:           "assets/app-Hash.js",
		CSS:            []string{"assets/app-Hash.css"},
		Assets:         []string{"assets/logo.png"},
		IsEntry:        true,
		IsDynamicEntry: false,
		Imports:        []string{"_shared.js"},
		DynamicImports: []string{"_lazy.js"},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Entry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch\nin:  %+v\nout: %+v", in, out)
	}
}

func TestErrManifestNotFound_IsExported(t *testing.T) {
	// Sanity: the sentinel must be a comparable error value.
	if ErrManifestNotFound == nil {
		t.Fatal("ErrManifestNotFound should be non-nil")
	}
	if !errors.Is(ErrManifestNotFound, ErrManifestNotFound) {
		t.Fatal("errors.Is on ErrManifestNotFound should be true")
	}
}
