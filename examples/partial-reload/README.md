# inertia-go: partial-reload

Demonstrates the `Always`, `Optional`, and `Defer` prop wrappers. Each
load function logs to stderr when it evaluates, so curl + log
observation makes the partial-reload semantics tangible.

## Run

```bash
go run .
```

Then in a second terminal:

```bash
# 1. Full HTML load.
#    stderr: currentUser, loadExpensiveStats (NOT loadActivity — deferred)
curl -s http://localhost:8080/ -o /dev/null

# 2. Inertia full load (JSON).
#    stderr: same as #1 (currentUser + loadExpensiveStats)
curl -s -H 'X-Inertia: true' -H 'X-Inertia-Version: demo-v1' \
     http://localhost:8080/ -o /dev/null

# 3. Partial: ask only for "stats".
#    stderr: currentUser (Always survives partials) + loadExpensiveStats
#    activity is skipped.
curl -s -H 'X-Inertia: true' -H 'X-Inertia-Version: demo-v1' \
     -H 'X-Inertia-Partial-Component: Dashboard' \
     -H 'X-Inertia-Partial-Data: stats' \
     http://localhost:8080/ -o /dev/null

# 4. Deferred follow-up: ask only for "activity".
#    stderr: currentUser + loadActivity (now evaluated)
curl -s -H 'X-Inertia: true' -H 'X-Inertia-Version: demo-v1' \
     -H 'X-Inertia-Partial-Component: Dashboard' \
     -H 'X-Inertia-Partial-Data: activity' \
     http://localhost:8080/ -o /dev/null
```

Watch the first terminal: each curl prints a different combination of
`evaluating currentUser` / `loadExpensiveStats` / `loadActivity` lines.
That is the partial-reload contract in action.

## What to look at

- `main.go:19-22` — `currentUser` is cheap and called inline so the
  *value* (not a function) reaches `Always(...)`.
- `main.go:24-28` and `main.go:30-38` — `loadExpensiveStats` and
  `loadActivity` match `func() (any, error)`, the signature `Optional`
  and `Defer` require.
- `main.go:62-64` — wrapper invocation showing the different intents
  for each prop.

## See also

- [Main package partial-reload docs (godoc)](https://pkg.go.dev/github.com/inertia-go/inertia-go#Always)
- [Inertia.js partial reload protocol](https://inertiajs.com/partial-reloads)
