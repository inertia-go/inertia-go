// Example: partial-reload prop wrappers.
//
// Demonstrates Always, Optional, and Defer by logging which prop loader
// functions evaluate on each request. Use curl with the X-Inertia-Partial-*
// headers (see README) to observe the different behaviours.
package main

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/inertia-go/inertia-go"
	"github.com/inertia-go/inertia-go/session"
)

func currentUser() map[string]any {
	log.Println("evaluating currentUser")
	return map[string]any{"id": 1, "name": "Ada"}
}

func loadExpensiveStats() (any, error) {
	log.Println("evaluating loadExpensiveStats")
	time.Sleep(50 * time.Millisecond)
	return map[string]any{"views": 12345, "signups": 67}, nil
}

func loadActivity() (any, error) {
	log.Println("evaluating loadActivity")
	time.Sleep(50 * time.Millisecond)
	return []map[string]any{
		{"at": "2026-05-20T10:00:00Z", "kind": "login"},
		{"at": "2026-05-20T09:55:12Z", "kind": "purchase"},
	}, nil
}

func main() {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		log.Fatal(err)
	}
	store, err := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		log.Fatal(err)
	}

	i, err := inertia.New(inertia.Config{
		RootView:   "app.html",
		TemplateFS: os.DirFS("views"),
		Version:    "demo-v1",
		Session:    store,
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Dashboard", inertia.Props{
			"user":     inertia.Always(currentUser()),
			"stats":    inertia.Optional(loadExpensiveStats),
			"activity": inertia.Defer(loadActivity, "feed"),
		})
	})

	addr := ":8080"
	log.Printf("inertia-go partial-reload example listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, i.Middleware(mux)))
}
