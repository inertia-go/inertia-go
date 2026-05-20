package session

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	if err := s.FlushResponse(w); err != nil {
		t.Fatalf("FlushResponse: %v", err)
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
	_ = s.FlushResponse(w)

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
	_ = old.FlushResponse(w)

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
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.FlashMessage(w, r, "big", huge); err != nil {
		t.Fatalf("FlashMessage should buffer, not write: %v", err)
	}
	err := s.FlushResponse(w)
	if !errors.Is(err, ErrCookieTooLarge) {
		t.Fatalf("expected ErrCookieTooLarge on flush, got %v", err)
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
	if err := s.FlushResponse(w); err != nil {
		t.Fatalf("FlushResponse: %v", err)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}

	w2 := httptest.NewRecorder()
	if _, err := s.TakeErrors(w2, r2, "default"); err != nil {
		t.Fatalf("TakeErrors: %v", err)
	}
	if err := s.FlushResponse(w2); err != nil {
		t.Fatalf("FlushResponse: %v", err)
	}
	cookies := w2.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a Set-Cookie header on drain")
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("expected delete cookie (MaxAge < 0), got MaxAge=%d", cookies[0].MaxAge)
	}
}

// decryptPayload is a test helper that decodes and decrypts a cookie
// produced by CookieStore.FlushResponse.
func decryptPayload(t *testing.T, s *CookieStore, value string) cookiePayload {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < nonceSize+1 {
		t.Fatalf("cookie too short: %d bytes", len(raw))
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	pt, err := s.aead[0].Open(nil, nonce, ct, []byte(s.opts.Name))
	if err != nil {
		t.Fatal(err)
	}
	var p cookiePayload
	if err := json.Unmarshal(pt, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCookieStore_MultiFlashSameResponse(t *testing.T) {
	s := newCookieStore(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := s.FlashErrors(w, r, "default", map[string]string{"email": "required"}); err != nil {
		t.Fatalf("FlashErrors: %v", err)
	}
	if err := s.FlashMessage(w, r, "success", "saved"); err != nil {
		t.Fatalf("FlashMessage: %v", err)
	}
	if err := s.FlushResponse(w); err != nil {
		t.Fatalf("FlushResponse: %v", err)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected exactly one Set-Cookie, got %d", len(cookies))
	}
	p := decryptPayload(t, s, cookies[0].Value)
	if got := p.Errors["default"]["email"]; got != "required" {
		t.Errorf("errors[default][email] = %q, want %q", got, "required")
	}
	if got := p.Messages["success"]; got != "saved" {
		t.Errorf("messages[success] = %v, want %q", got, "saved")
	}
}

func TestCookieStore_FlushNoOpOnIdleRequest(t *testing.T) {
	s := newCookieStore(t)
	w := httptest.NewRecorder()
	if err := s.FlushResponse(w); err != nil {
		t.Fatalf("FlushResponse: %v", err)
	}
	if cookies := w.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("idle request should not emit Set-Cookie, got %v", cookies)
	}
}

func TestCookieStore_EnsurePendingSeedsFromRequest(t *testing.T) {
	s := newCookieStore(t)

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.FlashErrors(w1, r1, "default", map[string]string{"a": "1"}); err != nil {
		t.Fatalf("seed FlashErrors: %v", err)
	}
	if err := s.FlushResponse(w1); err != nil {
		t.Fatalf("seed FlushResponse: %v", err)
	}
	seedCookies := w1.Result().Cookies()
	if len(seedCookies) != 1 {
		t.Fatalf("seed produced %d cookies", len(seedCookies))
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range seedCookies {
		r2.AddCookie(c)
	}
	if err := s.FlashMessage(w2, r2, "ok", "yes"); err != nil {
		t.Fatalf("FlashMessage: %v", err)
	}
	if err := s.FlushResponse(w2); err != nil {
		t.Fatalf("FlushResponse: %v", err)
	}
	cookies := w2.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	p := decryptPayload(t, s, cookies[0].Value)
	if got := p.Errors["default"]["a"]; got != "1" {
		t.Errorf("seed errors lost: got %q", got)
	}
	if got := p.Messages["ok"]; got != "yes" {
		t.Errorf("new message lost: got %v", got)
	}
}
