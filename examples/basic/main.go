package main

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"

	"github.com/inertia-go/inertia-go"
	"github.com/inertia-go/inertia-go/session"
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

	i, err := inertia.New(inertia.Config{
		RootView:   "app.html",
		TemplateFS: os.DirFS("views"),
		Version:    "demo-v1",
		Session:    store,
	})
	if err != nil {
		log.Fatal(err)
	}
	i.ShareValue("appName", "inertia-go basic")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Home", inertia.Props{
			"greeting": "Hello from Go!",
		})
	})
	mux.HandleFunc("/redirect-demo", func(w http.ResponseWriter, r *http.Request) {
		inertia.Flash(r).Set("notice", "You were redirected")
		i.Redirect(w, r, "/")
	})

	addr := ":8080"
	log.Printf("inertia-go basic example listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, i.Middleware(mux)))
}
