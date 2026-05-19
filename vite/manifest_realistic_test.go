package vite

import (
	"strings"
	"testing"
)

func TestManifest_RealisticViteOutput(t *testing.T) {
	m, err := Load("testdata/vite-vue-prod-manifest.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := string(m.Tag("resources/js/app.ts"))

	// Exactly one main script.
	if strings.Count(got, `<script type="module"`) != 1 {
		t.Errorf("expected exactly 1 main script tag, got %q", got)
	}
	if !strings.Contains(got, `<script type="module" src="/assets/app-DvPpZ7K3.js"></script>`) {
		t.Errorf("missing main script: %q", got)
	}

	// At least one modulepreload (the shared chunk).
	if !strings.Contains(got, `<link rel="modulepreload" href="/assets/shared-vue-Bgf4jPYC.js" />`) {
		t.Errorf("missing shared chunk preload: %q", got)
	}

	// Both stylesheets present.
	if !strings.Contains(got, `<link rel="stylesheet" href="/assets/app-CKdMlqEs.css" />`) {
		t.Errorf("missing app CSS: %q", got)
	}
	if !strings.Contains(got, `<link rel="stylesheet" href="/assets/shared-vue-Df3kp9.css" />`) {
		t.Errorf("missing shared CSS: %q", got)
	}

	// No duplicate hrefs.
	for _, href := range []string{
		"/assets/app-DvPpZ7K3.js",
		"/assets/shared-vue-Bgf4jPYC.js",
		"/assets/app-CKdMlqEs.css",
		"/assets/shared-vue-Df3kp9.css",
	} {
		if c := strings.Count(got, `"`+href+`"`); c != 1 {
			t.Errorf("href %s appeared %d times, want 1\nOutput:\n%s", href, c, got)
		}
	}
}
