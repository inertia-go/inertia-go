package session

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
)

const memorySessionCookie = "inertia_session"

// MemoryStore is a process-local store keyed by a session-id cookie.
// Concurrent-safe, but lacks expiry. Intended for tests and local dev only.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]*memEntry
}

type memEntry struct {
	errors   map[string]map[string]string // bag -> field -> message
	messages map[string]any
}

// NewMemory returns an empty MemoryStore.
func NewMemory() *MemoryStore {
	return &MemoryStore{data: map[string]*memEntry{}}
}

func (m *MemoryStore) sessionID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(memorySessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback"
	}
	id := hex.EncodeToString(b[:])
	http.SetCookie(w, &http.Cookie{
		Name:     memorySessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

func (m *MemoryStore) entry(id string) *memEntry {
	if e, ok := m.data[id]; ok {
		return e
	}
	e := &memEntry{
		errors:   map[string]map[string]string{},
		messages: map[string]any{},
	}
	m.data[id] = e
	return e
}

// FlashErrors implements Store.
func (m *MemoryStore) FlashErrors(w http.ResponseWriter, r *http.Request, bag string, errs map[string]string) error {
	if len(errs) == 0 {
		return nil
	}
	id := m.sessionID(w, r)
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entry(id)
	e.errors[bag] = errs
	return nil
}

// TakeErrors implements Store.
func (m *MemoryStore) TakeErrors(_ http.ResponseWriter, r *http.Request, bag string) (map[string]string, error) {
	c, err := r.Cookie(memorySessionCookie)
	if err != nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[c.Value]
	if !ok {
		return nil, nil
	}
	out := e.errors[bag]
	delete(e.errors, bag)
	return out, nil
}

// FlashMessage implements Store.
func (m *MemoryStore) FlashMessage(w http.ResponseWriter, r *http.Request, key string, val any) error {
	id := m.sessionID(w, r)
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entry(id)
	e.messages[key] = val
	return nil
}

// TakeMessages implements Store.
func (m *MemoryStore) TakeMessages(_ http.ResponseWriter, r *http.Request) (map[string]any, error) {
	c, err := r.Cookie(memorySessionCookie)
	if err != nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[c.Value]
	if !ok {
		return nil, nil
	}
	out := e.messages
	e.messages = map[string]any{}
	return out, nil
}
