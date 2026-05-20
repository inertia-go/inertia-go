# inertia-go: basic

The smallest possible inertia-go server: AES-GCM cookie session, one
shared prop, a `/` page, and a `/redirect-demo` route that flashes a
notice and bounces back to `/`.

## Run

```bash
go run .
```

Then open <http://localhost:8080>.

To see the redirect + flash flow, visit <http://localhost:8080/redirect-demo>;
the response redirects to `/` and the flash prop contains "You were redirected".

## What to look at

- `main.go:14-21` — cookie store setup using `session.NewCookie` with a
  random 32-byte key.
- `main.go:23-32` — `inertia.New` configuration and `ShareValue` for the
  global `appName` prop.
- `main.go:35-43` — minimal handlers, including the flash + redirect
  pattern.
- `views/app.html` — root template using `{{ .InertiaBody }}` and the
  Inertia.js + Vue CDN preamble.

## See also

- [Main package quick start](../../README.md#quick-start)
- [`session.CookieStore` docs](https://pkg.go.dev/github.com/inertia-go/inertia-go/session#CookieStore)
