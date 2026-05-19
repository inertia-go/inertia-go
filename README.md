# inertia-go

A Go server-side adapter for [Inertia.js v3](https://inertiajs.com).

- Built on `net/http`
- Zero external dependencies
- Instance-method API, no global state
- Framework adapters (Gin, Echo, Fiber) live in separate repositories

## Install

```bash
go get github.com/inertia-go/inertia-go
```

Requires Go 1.23 or newer.

## Quick Start

```go
package main

import (
    "crypto/rand"
    "net/http"
    "os"

    "github.com/inertia-go/inertia-go"
    "github.com/inertia-go/inertia-go/session"
)

func main() {
    var key [32]byte
    _, _ = rand.Read(key[:])
    store, _ := session.NewCookie(session.CookieOptions{Keys: [][]byte{key[:]}})

    i, _ := inertia.New(inertia.Config{
        RootView:   "app.html",
        TemplateFS: os.DirFS("views"),
        Version:    "v1",
        Session:    store,
    })

    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        i.Render(w, r, "Home", inertia.Props{
            "greeting": "Hello from Go!",
        })
    })

    http.ListenAndServe(":8080", i.Middleware(mux))
}
```

## API Surface

- `inertia.New(Config) (*Inertia, error)`
- `(*Inertia).Middleware(http.Handler) http.Handler`
- `(*Inertia).Render(w, r, component, props)`
- `(*Inertia).Redirect(w, r, url)` / `Location` / `Back`
- `(*Inertia).Share(key, fn)` / `ShareValue(key, value)`
- Prop wrappers: `inertia.Always`, `Optional`, `Defer`, `Merge`, `DeepMerge`
- Helpers: `inertia.ValidationErrors(r)`, `inertia.Flash(r)`, `inertia.FromRequest(r)`
- Sessions: `session.NewCookie`, `session.NewMemory`, `session.NewNoop`

## Protocol Conformance

This package targets [Inertia v3](https://inertiajs.com/docs/v3/core-concepts/the-protocol)
without backward compatibility for v1 or v2.

| Feature | Supported |
|---|---|
| `X-Inertia` request/response header | ✅ |
| `X-Inertia-Version` + 409 mismatch | ✅ |
| `X-Inertia-Partial-Data` / `-Partial-Component` / `-Partial-Except` | ✅ |
| `X-Inertia-Reset` | Reserved (reaches handler, not used internally yet) |
| `X-Inertia-Location` (external redirect) | ✅ |
| `X-Inertia-Redirect` (fragment redirect) | ✅ |
| `Vary: X-Inertia` | ✅ |
| 302→303 conversion for PUT/PATCH/DELETE | ✅ |
| `encryptHistory` / `clearHistory` page meta | ✅ |
| `mergeProps` / `deepMergeProps` / `deferredProps` | ✅ |
| SSR HTTP client | v0.3.0 |
| Vite manifest helper | v0.2.0 |
| Precognition | Out of scope |

## Vite Manifest

The optional `vite` sub-package provides a `*Manifest` type that satisfies
the main package's `ViteHelper` interface.

```go
import (
    "github.com/inertia-go/inertia-go"
    "github.com/inertia-go/inertia-go/vite"
)

// Production: load from manifest.json
m := vite.MustLoad("public/build/manifest.json")

// Development: point at the Vite dev server
// m := vite.Dev("http://localhost:5173")

i, _ := inertia.New(inertia.Config{
    RootView:   "app.html",
    TemplateFS: os.DirFS("views"),
    Vite:       m,
    Session:    store,
})
```

Inside your root template, four helper functions are available:

| Helper | Output |
|---|---|
| `{{ vite "entry.tsx" }}` | `<script>` + `modulepreload` links + `stylesheet` links |
| `{{ viteCSS "entry.tsx" }}` | only `<link rel="stylesheet">` tags |
| `{{ viteAsset "path/file.png" }}` | a single resolved URL |
| `{{ viteReactRefresh }}` | React Refresh runtime (dev only; empty in prod) |

If `Config.Vite` is nil the helpers become no-ops that log a single
warning, so templates referencing them still parse.

## Framework Adapters

- [inertia-go-gin](https://github.com/inertia-go/inertia-go-gin) — Gin
- [inertia-go-echo](https://github.com/inertia-go/inertia-go-echo) — Echo
- [inertia-go-fiber](https://github.com/inertia-go/inertia-go-fiber) — Fiber

## License

MIT
