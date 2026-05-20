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

## Examples

Runnable Go modules under [`examples/`](examples/):

| Example | What it shows |
|---|---|
| [basic/](examples/basic/) | Cookie session, shared props, redirect + flash |
| [vite/](examples/vite/) | Vite manifest helper — prod (`Load`) and dev (`Dev`) |
| [ssr/](examples/ssr/) | SSR HTTP client — fail-soft default and `SSRRequired=true` |
| [partial-reload/](examples/partial-reload/) | `Always` / `Optional` / `Defer` prop wrappers |

Each example is a standalone Go module; `cd` into one and run `go run .`.

## API Surface

- `inertia.New(Config) (*Inertia, error)`
- `(*Inertia).Middleware(http.Handler) http.Handler`
- `(*Inertia).Render(w, r, component, props)`
- `(*Inertia).Redirect(w, r, url)` / `Location` / `Back`
- `(*Inertia).Share(key, fn)` / `ShareValue(key, value)`
- Prop types: `inertia.Always`, `Optional`, `Defer`, `Merge`, `DeepMerge`, `Once`, `Scroll`
- Chainable modifiers (compose on Merge/DeepMerge/Defer/Once): `.Prepend(path...)`, `.Append(path...)`, `.MatchOn(map)`, `.DeepMerge()`, `.Once()`, `.ExpiresIn(d)`, `.As(key)`, `.Fresh()`, `.Rescue()` — e.g. `Merge(rows).Prepend("messages").MatchOn(map[string]string{"messages": "id"})`, `Defer(fn).DeepMerge()`, `Once(fn).As("plans").Fresh()`
- Infinite scroll: `inertia.Scroll(metadata, func() any { ... }, inertia.WithWrapper("items"), inertia.WithPageName("orders"))` — `metadata` resolves through the `ScrollAdapter` registry (`inertia.RegisterScrollAdapter`); a bare `map[string]any{"pageName","currentPage","previousPage","nextPage"}` or a `ScrollConfig` works out of the box. The data callback is lazy (run only when the prop is included).
- Page meta: `Config.PreserveFragment`, `inertia.SetPreserveFragment(r, bool)`
- Helpers: `inertia.ValidationErrors(r)`, `inertia.Flash(r)`, `inertia.FromRequest(r)`
- Sessions: `session.NewCookie`, `session.NewMemory`, `session.NewNoop`
- Vite: `vite.Load`, `vite.MustLoad`, `vite.Dev` (satisfies `inertia.ViteHelper`)
- SSR: `ssr.NewHTTP` (satisfies `inertia.SSRClient`)

## Protocol Conformance

This package targets [Inertia v3](https://inertiajs.com/docs/v3/core-concepts/the-protocol)
without backward compatibility for v1 or v2.

| Feature | Supported |
|---|---|
| `X-Inertia` request/response header | ✅ |
| `X-Inertia-Version` + 409 mismatch | ✅ |
| `X-Inertia-Partial-Data` / `-Partial-Component` / `-Partial-Except` | ✅ |
| `X-Inertia-Reset` | Parsed; exposed via `FromRequest(r).Reset` (client-only directive — no mandatory server behavior) |
| `Purpose: prefetch` | Parsed; exposed via `FromRequest(r).IsPrefetch` |
| `X-Inertia-Location` (external redirect) | ✅ |
| `X-Inertia-Redirect` (fragment redirect) | ✅ |
| `Vary: X-Inertia` | ✅ |
| 302→303 conversion for PUT/PATCH/DELETE | ✅ |
| `encryptHistory` / `clearHistory` page meta | ✅ |
| `mergeProps` / `deepMergeProps` / `deferredProps` | ✅ |
| `prependProps` (v3 prepend) | ✅ |
| `matchPropsOn` (v3 list reconciliation) | ✅ |
| `sharedProps` (v3 shared-keys metadata) | ✅ |
| `scrollProps` (v3 infinite scroll, adapter-derived) | ✅ |
| `onceProps` (v3 once props) | ✅ |
| `rescuedProps` (v3 deferred rescue) | ✅ |
| `preserveFragment` page meta | ✅ |
| SSR HTTP client | ✅ |
| Vite manifest helper | ✅ |
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

## SSR

The optional `ssr` sub-package speaks the Inertia.js SSR HTTP
protocol. The main package's `SSRClient` interface is satisfied by
`*ssr.HTTPClient`; users may supply other implementations.

```go
import (
    "github.com/inertia-go/inertia-go"
    "github.com/inertia-go/inertia-go/ssr"
)

i, _ := inertia.New(inertia.Config{
    RootView:   "app.html",
    TemplateFS: os.DirFS("views"),
    Session:    store,
    SSR:        ssr.NewHTTP("http://127.0.0.1:13714"),
    // SSRRequired: true,  // fail-hard instead of CSR fallback
})
```

Include `{{ .InertiaHead }}` in your root template's `<head>` to
receive the SSR-emitted tags. SSR runs only on the initial HTML
response — Inertia XHR navigations skip it.

On error the package logs an `slog.Warn` and falls back to
client-side rendering. Set `Config.SSRRequired = true` to route
errors through `Config.ErrorHandler` (default: 500) instead.
Process management for the Node SSR service is out of scope — run
it under systemd, supervisord, a k8s sidecar, or whatever fits your
stack.

## Cookie session lifecycle

`session.CookieStore` buffers flash/error writes per HTTP request and
emits a single `Set-Cookie` at the end. The buffer is drained by
`inertia.Middleware` via a deferred hook, so apps using `CookieStore`
MUST mount the middleware:

```go
i, _ := inertia.New(inertia.Config{
    RootView:   "app.html",
    TemplateFS: os.DirFS("views"),
    Session:    store,
})

http.ListenAndServe(":8080", i.Middleware(mux))  // required for CookieStore
```

`session.MemoryStore` and `session.NewNoop()` are not affected; they
write eagerly per call.

## Framework Adapters

- [inertia-go-gin](https://github.com/inertia-go/inertia-go-gin) — Gin
- [inertia-go-echo](https://github.com/inertia-go/inertia-go-echo) — Echo
- [inertia-go-fiber](https://github.com/inertia-go/inertia-go-fiber) — Fiber

## License

MIT
