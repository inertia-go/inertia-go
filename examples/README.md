# inertia-go examples

Each subdirectory is a standalone Go module showing one slice of the
package. Pick one and `cd` into it.

| Example | What it shows |
|---|---|
| [basic/](basic/) | Cookie session, shared props, redirect + flash |
| [vite/](vite/) | Vite manifest helper — prod (`Load`) and dev (`Dev`) |
| [ssr/](ssr/) | SSR HTTP client — fail-soft default and `SSRRequired=true` |
| [partial-reload/](partial-reload/) | Always / Optional / Defer prop wrappers |

## Running

Each example has its own `README.md` with run instructions. All four
listen on `:8080` by default — only run one at a time, or override the
port in `main.go`.

## Module isolation

Examples are independent Go modules so they can pull external
dependencies (none currently do) without leaking into the main package.
Each `go.mod` includes `replace github.com/inertia-go/inertia-go =>
../..` so local edits to the package are picked up immediately.
