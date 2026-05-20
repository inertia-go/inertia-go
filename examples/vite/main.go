// Example: Vite manifest integration.
//
// Run `go run .` to use the manifest fixture (prod mode), or
// `INERTIA_VITE_DEV=1 go run .` to point at a Vite dev server.
package main

import (
	"crypto/rand"
	"log"
	"net/http"
	"os"

	"github.com/inertia-go/inertia-go"
	"github.com/inertia-go/inertia-go/session"
	"github.com/inertia-go/inertia-go/vite"
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

	var viteHelper inertia.ViteHelper
	if os.Getenv("INERTIA_VITE_DEV") == "1" {
		log.Println("vite: dev mode — http://localhost:5173")
		viteHelper = vite.Dev("http://localhost:5173")
	} else {
		log.Println("vite: prod mode — build/manifest.json")
		viteHelper = vite.MustLoad("build/manifest.json")
	}

	i, err := inertia.New(inertia.Config{
		RootView:   "app.html",
		TemplateFS: os.DirFS("views"),
		Version:    "demo-v1",
		Session:    store,
		Vite:       viteHelper,
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Home", inertia.Props{
			"greeting": "Vite manifest demo",
		})
	})

	addr := ":8080"
	log.Printf("inertia-go vite example listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, i.Middleware(mux)))
}
