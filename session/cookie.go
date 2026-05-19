package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
)

// ErrCookieTooLarge is returned when an encoded cookie payload exceeds
// the configured maximum size (default 3.5 KB).
var ErrCookieTooLarge = errors.New("session: cookie payload exceeds size limit")

const (
	defaultCookieName = "inertia_flash"
	defaultMaxBytes   = 3584 // 3.5 KB
	nonceSize         = 12
)

// CookieOptions configures a CookieStore.
type CookieOptions struct {
	// Name is the cookie name. Default "inertia_flash".
	Name string
	// Keys is the rotation slice (32 bytes each). Index 0 is the active key.
	Keys [][]byte
	// Path defaults to "/".
	Path string
	// Domain is optional.
	Domain string
	// Secure defaults to true.
	Secure bool
	// HTTPOnly defaults to true.
	HTTPOnly bool
	// SameSite defaults to http.SameSiteLaxMode.
	SameSite http.SameSite
	// MaxAge in seconds. Default 120 (flash data is short-lived).
	MaxAge int
	// MaxBytes is the encoded cookie size limit. Default 3584.
	MaxBytes int
}

// CookieStore is a stateless session store that encodes flash data inside
// an AES-GCM-encrypted cookie. Concurrent-safe.
type CookieStore struct {
	opts CookieOptions
	aead []cipher.AEAD // index aligned with opts.Keys
}

type cookiePayload struct {
	Errors   map[string]map[string]string `json:"e,omitempty"`
	Messages map[string]any               `json:"m,omitempty"`
}

// NewCookie constructs a CookieStore. Returns an error if Keys is empty or
// any key is not 32 bytes.
func NewCookie(opts CookieOptions) (*CookieStore, error) {
	if len(opts.Keys) == 0 {
		return nil, errors.New("session: CookieOptions.Keys is required")
	}
	if opts.Name == "" {
		opts.Name = defaultCookieName
	}
	if opts.Path == "" {
		opts.Path = "/"
	}
	if opts.MaxAge == 0 {
		opts.MaxAge = 120
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	if opts.SameSite == 0 {
		opts.SameSite = http.SameSiteLaxMode
	}

	aeads := make([]cipher.AEAD, 0, len(opts.Keys))
	for i, k := range opts.Keys {
		if len(k) != 32 {
			return nil, errors.New("session: each key must be 32 bytes")
		}
		block, err := aes.NewCipher(k)
		if err != nil {
			return nil, err
		}
		a, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		_ = i
		aeads = append(aeads, a)
	}
	return &CookieStore{opts: opts, aead: aeads}, nil
}

func (s *CookieStore) read(r *http.Request) cookiePayload {
	c, err := r.Cookie(s.opts.Name)
	if err != nil || c.Value == "" {
		return cookiePayload{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) < nonceSize+1 {
		return cookiePayload{}
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	aad := []byte(s.opts.Name)
	for _, a := range s.aead {
		pt, err := a.Open(nil, nonce, ct, aad)
		if err != nil {
			continue
		}
		var p cookiePayload
		if err := json.Unmarshal(pt, &p); err != nil {
			return cookiePayload{}
		}
		return p
	}
	return cookiePayload{}
}

func (s *CookieStore) write(w http.ResponseWriter, p cookiePayload) error {
	if isEmpty(p) {
		// Issue a delete cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     s.opts.Name,
			Value:    "",
			Path:     s.opts.Path,
			Domain:   s.opts.Domain,
			MaxAge:   -1,
			Secure:   s.opts.Secure,
			HttpOnly: s.opts.HTTPOnly,
			SameSite: s.opts.SameSite,
		})
		return nil
	}
	pt, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	ct := s.aead[0].Seal(nil, nonce[:], pt, []byte(s.opts.Name))
	out := make([]byte, 0, nonceSize+len(ct))
	out = append(out, nonce[:]...)
	out = append(out, ct...)
	encoded := base64.RawURLEncoding.EncodeToString(out)
	if len(encoded) > s.opts.MaxBytes {
		return ErrCookieTooLarge
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.opts.Name,
		Value:    encoded,
		Path:     s.opts.Path,
		Domain:   s.opts.Domain,
		MaxAge:   s.opts.MaxAge,
		Secure:   s.opts.Secure,
		HttpOnly: s.opts.HTTPOnly,
		SameSite: s.opts.SameSite,
	})
	return nil
}

func isEmpty(p cookiePayload) bool {
	return len(p.Errors) == 0 && len(p.Messages) == 0
}

// FlashErrors implements Store.
func (s *CookieStore) FlashErrors(w http.ResponseWriter, r *http.Request, bag string, errs map[string]string) error {
	if len(errs) == 0 {
		return nil
	}
	p := s.read(r)
	if p.Errors == nil {
		p.Errors = map[string]map[string]string{}
	}
	p.Errors[bag] = errs
	return s.write(w, p)
}

// TakeErrors implements Store.
func (s *CookieStore) TakeErrors(w http.ResponseWriter, r *http.Request, bag string) (map[string]string, error) {
	p := s.read(r)
	if p.Errors == nil {
		return nil, nil
	}
	out := p.Errors[bag]
	delete(p.Errors, bag)
	if err := s.write(w, p); err != nil {
		return out, err
	}
	return out, nil
}

// FlashMessage implements Store.
func (s *CookieStore) FlashMessage(w http.ResponseWriter, r *http.Request, key string, val any) error {
	p := s.read(r)
	if p.Messages == nil {
		p.Messages = map[string]any{}
	}
	p.Messages[key] = val
	return s.write(w, p)
}

// TakeMessages implements Store.
func (s *CookieStore) TakeMessages(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	p := s.read(r)
	if p.Messages == nil {
		return nil, nil
	}
	out := p.Messages
	p.Messages = nil
	if err := s.write(w, p); err != nil {
		return out, err
	}
	return out, nil
}
