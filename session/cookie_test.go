package session

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newCookieStore(t *testing.T) *CookieStore {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	s, err := NewCookie(CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCookieStore_ErrorsRoundTrip(t *testing.T) {
	s := newCookieStore(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	in := map[string]string{"email": "invalid", "name": "required"}
	if err := s.FlashErrors(w, r, "default", in); err != nil {
		t.Fatalf("FlashErrors: %v", err)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}
	got, err := s.TakeErrors(httptest.NewRecorder(), r2, "default")
	if err != nil {
		t.Fatalf("TakeErrors: %v", err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %v, want %v", got, in)
	}
}

func TestCookieStore_TamperedCookieFailsSilently(t *testing.T) {
	s := newCookieStore(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_ = s.FlashErrors(w, r, "default", map[string]string{"a": "b"})

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		// Flip a byte inside the value.
		c.Value = c.Value[:len(c.Value)-1] + "A"
		r2.AddCookie(c)
	}
	got, err := s.TakeErrors(httptest.NewRecorder(), r2, "default")
	if err != nil {
		t.Fatalf("TakeErrors with tampered cookie returned err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("tampered cookie should yield empty, got %v", got)
	}
}

func TestCookieStore_KeyRotation(t *testing.T) {
	var keyA, keyB [32]byte
	if _, err := rand.Read(keyA[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(keyB[:]); err != nil {
		t.Fatal(err)
	}
	old, err := NewCookie(CookieOptions{Keys: [][]byte{keyA[:]}})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewCookie(CookieOptions{Keys: [][]byte{keyB[:], keyA[:]}})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	_ = old.FlashErrors(w, httptest.NewRequest(http.MethodGet, "/", nil), "default",
		map[string]string{"a": "b"})

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}
	got, _ := rotated.TakeErrors(httptest.NewRecorder(), r2, "default")
	if got["a"] != "b" {
		t.Errorf("rotated store failed to read old key: %v", got)
	}
}

func TestCookieStore_OversizePayloadReturnsError(t *testing.T) {
	s := newCookieStore(t)
	huge := strings.Repeat("x", 8000)
	err := s.FlashMessage(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
		"big", huge)
	if !errors.Is(err, ErrCookieTooLarge) {
		t.Fatalf("expected ErrCookieTooLarge, got %v", err)
	}
}
