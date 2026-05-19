package vite

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
