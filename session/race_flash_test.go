package session_test

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/inertia-go/inertia-go/session"
)

func newStore(t *testing.T) *session.CookieStore {
	t.Helper()
	var key [32]byte
	rand.Read(key[:])
	s, err := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestRaceConcurrentFlash exercises concurrent Flash* calls on the same
// ResponseWriter to confirm no data race under the race detector.
func TestRaceConcurrentFlash(t *testing.T) {
	s := newStore(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_ = s.FlashMessage(w, r, fmt.Sprintf("k%d", i), i)
		}()
	}
	wg.Wait()
	if err := s.FlushResponse(w); err != nil {
		t.Fatal(err)
	}
}
