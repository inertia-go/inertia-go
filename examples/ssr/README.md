# inertia-go: ssr

Demonstrates the `ssr` sub-package: pointing `Config.SSR` at a Node SSR
service and contrasting the default fail-soft behaviour against
`SSRRequired=true`.

## Run

```bash
go run .
```

Without a Node service running on `127.0.0.1:13714`:

- <http://localhost:8080/> serves an HTML page with the CSR fallback
  (`<div id="app" data-page='...'>`). The stderr log contains one
  `slog.Warn` line per request: `inertia: ssr render failed; falling
  back to CSR`.
- <http://localhost:8080/strict> returns HTTP 500. The default
  `ErrorHandler` logs `slog.Error` with `errors.Is(err,
  ErrSSRUnavailable) == true`.

## Add a real SSR backend

The official Node SSR runner is `@inertiajs/server`. After
`npm install`, start it before launching this example:

```bash
node -e 'require("@inertiajs/server").createServer((page) => ({
  head: ["<title>SSR title</title>"],
  body: "<div id=\"app\">Rendered server-side</div>",
}))'
```

Then revisit the routes above — `/` will now contain the rendered head
and body; `/strict` will return 200 too.

## What to look at

- `main.go:32` — `ssr.NewHTTP("http://127.0.0.1:13714")` — the default
  loopback URL with `/render` and `/health` paths.
- `main.go:34-43` and `main.go:45-55` — two `*Inertia` instances,
  differing only in `SSRRequired`.
- `main.go:72-73` — per-instance Middleware wrapping (each `*Inertia`
  carries its own request-scope state).

## See also

- [Main package SSR section](../../README.md#ssr)
- [`ssr` sub-package docs](https://pkg.go.dev/github.com/inertia-go/inertia-go/ssr)
