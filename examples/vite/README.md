# inertia-go: vite

Demonstrates the `vite` sub-package: loading a Vite manifest in production
mode and switching to a Vite dev server via an environment variable.

## Run

Production (loads `build/manifest.json`):

```bash
go run .
```

Dev (points at <http://localhost:5173>):

```bash
INERTIA_VITE_DEV=1 go run .
```

Then open <http://localhost:8080> and view source. You'll see
`<script type="module" src="/assets/app-DvPpZ7K3.js"></script>` (prod)
or `<script type="module" src="http://localhost:5173/resources/js/app.tsx"></script>`
(dev). The actual asset files do not exist — this example demonstrates
the Go-side helper plumbing, not a running frontend build.

## What to look at

- `main.go:28-35` — environment-switched `vite.Dev` vs `vite.MustLoad`.
- `main.go:37-43` — `Config.Vite` injection.
- `views/app.html:6-7` — `{{ vite "..." }}` and `{{ viteCSS "..." }}`
  template helpers.
- `build/manifest.json` — fixture mirroring real Vite output shape.

## See also

- [Main package Vite section](../../README.md#vite-manifest)
- [`vite` sub-package docs](https://pkg.go.dev/github.com/inertia-go/inertia-go/vite)
