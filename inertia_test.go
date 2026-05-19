package inertia

import (
	"errors"
	"net/http"
	"testing"
	"testing/fstest"
)

func TestNew_RequiresSession_WhenErrorsOrFlashAreUsed(t *testing.T) {
	// Session is required; New must reject nil Session.
	_, err := New(Config{})
	if !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("expected ErrSessionRequired, got %v", err)
	}
}

func TestNew_AcceptsMinimalConfig(t *testing.T) {
	i, err := New(Config{Session: stubSession{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i == nil {
		t.Fatal("expected non-nil *Inertia")
	}
}

type stubSession struct{}

func (stubSession) FlashErrors(_ http.ResponseWriter, _ *http.Request, _ string, _ map[string]string) error {
	return nil
}
func (stubSession) TakeErrors(_ http.ResponseWriter, _ *http.Request, _ string) (map[string]string, error) {
	return nil, nil
}
func (stubSession) FlashMessage(_ http.ResponseWriter, _ *http.Request, _ string, _ any) error {
	return nil
}
func (stubSession) TakeMessages(_ http.ResponseWriter, _ *http.Request) (map[string]any, error) {
	return nil, nil
}

func TestNew_RejectsMultipleVersionSources(t *testing.T) {
	cases := []Config{
		{Session: stubSession{}, Version: "a", VersionFunc: func(*http.Request) string { return "b" }},
		{Session: stubSession{}, Version: "a", VersionFromFS: fstest.MapFS{}},
		{Session: stubSession{}, VersionFunc: func(*http.Request) string { return "b" }, VersionFromFS: fstest.MapFS{}},
	}
	for i, c := range cases {
		if _, err := New(c); !errors.Is(err, ErrConflictingVersion) {
			t.Errorf("case %d: expected ErrConflictingVersion, got %v", i, err)
		}
	}
}
