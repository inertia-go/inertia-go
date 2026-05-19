package session

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMemoryStore_ErrorsRoundTrip(t *testing.T) {
	s := NewMemory()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	in := map[string]string{"email": "invalid"}
	if err := s.FlashErrors(w, r, "default", in); err != nil {
		t.Fatalf("FlashErrors: %v", err)
	}

	// Replay cookie set by FlashErrors onto the next request.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}
	w2 := httptest.NewRecorder()

	got, err := s.TakeErrors(w2, r2, "default")
	if err != nil {
		t.Fatalf("TakeErrors: %v", err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %v, want %v", got, in)
	}

	// Second take should yield empty (read-and-clear of the bag).
	// Reuse cookies from w (the original FlashErrors recorder) so r3 has
	// a valid session id and we actually exercise the delete path.
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r3.AddCookie(c)
	}
	got2, _ := s.TakeErrors(httptest.NewRecorder(), r3, "default")
	if len(got2) != 0 {
		t.Errorf("expected cleared, got %v", got2)
	}
}

func TestMemoryStore_FlashMessagesRoundTrip(t *testing.T) {
	s := NewMemory()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.FlashMessage(w, r, "success", "Saved"); err != nil {
		t.Fatalf("FlashMessage: %v", err)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}
	got, _ := s.TakeMessages(httptest.NewRecorder(), r2)
	if got["success"] != "Saved" {
		t.Errorf("got %v", got)
	}
}
