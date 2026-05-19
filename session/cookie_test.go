package session

import (
	"crypto/rand"
	"encoding/base64"
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
		// Decode the base64 value, flip a byte in the middle of the ciphertext
		// (well away from the GCM tag), then re-encode.  Corrupting at the raw
		// byte level guarantees the tamper is never a no-op, unlike flipping the
		// last base64 character which may encode only padding bits (~25% no-op).
		raw, err := base64.RawURLEncoding.DecodeString(c.Value)
		if err != nil || len(raw) < nonceSize+2 {
			t.Fatalf("could not decode cookie value: %v", err)
		}
		raw[nonceSize] ^= 0xFF // first byte of ciphertext
		c.Value = base64.RawURLEncoding.EncodeToString(raw)
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

func TestNewCookie_RequiresKeys(t *testing.T) {
	if _, err := NewCookie(CookieOptions{}); err == nil {
		t.Fatal("expected error when Keys is empty")
	}
}

func TestNewCookie_RejectsWrongKeySize(t *testing.T) {
	short := make([]byte, 16)
	if _, err := NewCookie(CookieOptions{Keys: [][]byte{short}}); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}

func TestCookieStore_TakeAllBagsIssuesDeleteCookie(t *testing.T) {
	s := newCookieStore(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.FlashErrors(w, r, "default", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("FlashErrors: %v", err)
	}

	// Carry the cookie into the next request.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}

	// Drain the only bag; the response should issue a delete cookie
	// (MaxAge < 0) because the payload becomes empty.
	w2 := httptest.NewRecorder()
	if _, err := s.TakeErrors(w2, r2, "default"); err != nil {
		t.Fatalf("TakeErrors: %v", err)
	}
	cookies := w2.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a Set-Cookie header on drain")
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("expected delete cookie (MaxAge < 0), got MaxAge=%d", cookies[0].MaxAge)
	}
}
