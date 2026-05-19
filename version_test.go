package inertia

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestVersion_Static(t *testing.T) {
	i, _ := New(Config{Session: stubSession{}, Version: "abc"})
	got := i.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil))
	if got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestVersion_Func(t *testing.T) {
	i, _ := New(Config{
		Session:     stubSession{},
		VersionFunc: func(r *http.Request) string { return "v-" + r.Method },
	})
	got := i.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil))
	if got != "v-GET" {
		t.Errorf("got %q", got)
	}
}

func TestVersion_FromFS_StableAndIndependent(t *testing.T) {
	fs1 := fstest.MapFS{
		"a.js": {Data: []byte("x")},
		"b.js": {Data: []byte("y")},
	}
	i1, _ := New(Config{Session: stubSession{}, VersionFromFS: fs1})
	v1 := i1.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil))
	v1Again := i1.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil))
	if v1 != v1Again {
		t.Errorf("not stable: %q vs %q", v1, v1Again)
	}

	fs2 := fstest.MapFS{
		"a.js": {Data: []byte("x")},
		"b.js": {Data: []byte("Z")}, // different content
	}
	i2, _ := New(Config{Session: stubSession{}, VersionFromFS: fs2})
	v2 := i2.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil))
	if v1 == v2 {
		t.Errorf("expected different versions, got %q", v1)
	}
}

func TestVersion_EmptyWhenUnset(t *testing.T) {
	i, _ := New(Config{Session: stubSession{}})
	if got := i.currentVersion(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("got %q", got)
	}
}
