// Example: SSR HTTP client integration.
//
// Run `go run .` and visit:
//   - http://localhost:8080/        — fail-soft default (CSR fallback)
//   - http://localhost:8080/strict  — fail-hard (HTTP 500 if SSR is unavailable)
//
// To see SSR actually succeed, run a Node SSR service on
// 127.0.0.1:13714 (e.g. @inertiajs/server) before starting this example.
package main

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"

	inertia "github.com/inertia-go/inertia-go"
	"github.com/inertia-go/inertia-go/session"
	"github.com/inertia-go/inertia-go/ssr"
)

func main() {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		log.Fatal(err)
	}
	store, err := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})
	if err != nil {
		log.Fatal(err)
	}

	ssrClient := ssr.NewHTTP("http://127.0.0.1:13714")

	iSoft, err := inertia.New(inertia.Config{
		RootView:   "app.html",
		TemplateFS: os.DirFS("views"),
		Version:    "demo-v1",
		Session:    store,
		SSR:        ssrClient,
	})
	if err != nil {
		log.Fatal(err)
	}

	iHard, err := inertia.New(inertia.Config{
		RootView:    "app.html",
		TemplateFS:  os.DirFS("views"),
		Version:     "demo-v1",
		Session:     store,
		SSR:         ssrClient,
		SSRRequired: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	softMux := http.NewServeMux()
	softMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		iSoft.Render(w, r, "Home", inertia.Props{
			"message": "Default: fail-soft — falls back to CSR if SSR is down",
		})
	})

	hardMux := http.NewServeMux()
	hardMux.HandleFunc("/strict", func(w http.ResponseWriter, r *http.Request) {
		iHard.Render(w, r, "Strict", inertia.Props{
			"message": "SSRRequired=true — returns 500 if SSR is down",
		})
	})

	top := http.NewServeMux()
	top.Handle("/strict", iHard.Middleware(hardMux))
	top.Handle("/", iSoft.Middleware(softMux))

	addr := ":8080"
	log.Printf("inertia-go ssr example listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, top))
}
