# Changelog

All notable changes to this project will be documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.7.0] — 2026-05-20

### BREAKING

- **`Scroll` signature change for full parity.** `Scroll(data any, cfg
  ScrollConfig)` becomes `Scroll(metadata any, data func() any, opts
  ...ScrollOption)`. `metadata` resolves through the new `ScrollAdapter`
  registry (a `ScrollConfig` or `map[string]any` works built-in); `data`
  is now a lazy callback evaluated only when the prop is included. Migrate
  `Scroll(rows, cfg)` → `Scroll(cfg, func() any { return rows })`.
  A nil data callback panics at construction (fail-fast).

### Added

- `ScrollAdapter` interface + `RegisterScrollAdapter` — pluggable
  pagination-metadata derivation, matched in reverse registration order
  (custom adapters override built-ins). Built-ins: identity (`ScrollConfig`)
  and map (`map[string]any` metadata hash, also accepting JSON-decoded
  `float64` numbers).
- `WithWrapper(key)` — nest scroll data under a custom key (default
  `"data"`); only `<prop>.<wrapper>` is merged, sibling metadata preserved.
- `WithPageName(name)` — distinct page query-param per scroll container.
- `Scroll` data callback is lazy: skipped entirely when partial-reload
  filtering excludes the prop.

## [0.6.0] — 2026-05-20

### BREAKING

- **Composable prop modifiers replace wrapper types.** The 9 internal
  prop-wrapper types and the `propWrapper` interface are replaced by a
  single composable `*propBuilder`. The standalone `Prepend(v)` and
  `MatchOn(v, keys...)` constructors are **removed** — use
  `Merge(v).Prepend(path)` and `Merge(v).MatchOn(map[string]string{...})`.
  Modifiers now compose: `Defer(fn).DeepMerge()`, `Merge(fn).Once()`,
  `Once(fn).As(key).Fresh()`. Conflicting combinations panic at
  construct time.

### Added

- Chainable modifiers: `.Prepend(path...)`, `.Append(path...)`,
  `.MatchOn(map)`, `.DeepMerge()`, `.Once()`, `.ExpiresIn(d)`,
  `.As(key)`, `.Fresh()`, `.Rescue()`.
- Nested merge/prepend paths emit dotted metadata
  (`prependProps: ["chat.messages"]`, `matchPropsOn: ["chat.data.id"]`).
- `Once` advanced API: `.As(key)` for a custom onceProps key, `.Fresh()`
  to force server-side re-resolution.

### Fixed

- `once` props re-resolve on an explicit `X-Inertia-Partial-Data`
  request even when the client reports them cached.
- `once` props using `.As(alias)` now honor the cache-skip via the alias
  key (previously always re-resolved).
- `Purpose: prefetch` is parsed and exposed via `FromRequest(r).IsPrefetch`.

## [0.5.0] — 2026-05-20

### BREAKING

- **PageObject type corrections**: `scrollProps` and `onceProps` change
  from `map[string]map[string]any` to typed `map[string]ScrollConfig` /
  `map[string]OnceConfig`. These were reserved-but-unpopulated in v0.4,
  so no runtime behavior changes unless code asserted the old map types.
- **propWrapper interface** gained four methods (`rescueOnError`,
  `isOnce`, `onceTTL`, `scrollConfig`). Internal interface; user code
  unaffected unless asserting concrete wrapper types (unsupported).

### Added

- `Defer(...).Rescue()` — a deferred prop whose callback errors is dropped
  and its key listed in `rescuedProps` instead of failing the response.
- `Once(fn)` / `.ExpiresIn(d)` — props the client caches once and reuses;
  honors the `X-Inertia-Except-Once-Props` request header.
- `Scroll(data, ScrollConfig{})` — infinite-scroll pagination; nests data
  at `props.<key>.data`, lists `<key>.data` in `mergeProps`, emits
  `scrollProps`. Merge direction exposed via `RequestInfo.ScrollMergeIntent`
  (`X-Inertia-Infinite-Scroll-Merge-Intent`).
- `Config.PreserveFragment` + `SetPreserveFragment(r, bool)` — page-object
  `preserveFragment` with a bidirectional per-request override.
- Examples now mount a real Inertia v3 client (esm.sh importmap + Vue 3)
  instead of dead skeleton templates.

### Fixed

- `CookieStore` flush now runs before the response headers are committed
  (on first WriteHeader/Write), so flash/errors are no longer dropped
  under a real net/http server. Previously the deferred flush ran after
  the handler had already sent headers.
- `ResponseController.Flush()` / `.Hijack()` drain the session before
  committing headers or hijacking, closing the streaming/WebSocket
  data-loss path.

## [0.4.0] — 2026-05-20

### BREAKING

- **HTML output shape**: initial HTML now emits
  `<script data-page="app" type="application/json">…</script><div id="app"></div>`
  per Inertia v3 protocol. v2 clients will no longer boot. Templates
  using `{{ .InertiaBody }}` need no change.
- **CookieStore requires `inertia.Middleware`** to flush flash/error
  payloads. Without the middleware, flashes are silently dropped.
- **propWrapper interface** gained two methods (`isPrepend`,
  `matchOnKeys`). Internal interface; user code unaffected unless
  asserting concrete wrapper types (unsupported).

### Added

- New prop wrappers: `Prepend(v)`, `MatchOn(v, keys...)`.
- `PageObject` fields: `prependProps`, `matchPropsOn`, `sharedProps`,
  plus reserved-for-v0.5 `scrollProps`, `onceProps`, `rescuedProps`
  (always empty in v0.4).
- `sharedProps` auto-derived from `Share` / `ShareValue` registrations.
- `SessionFlusher` interface — session stores implementing it receive a
  per-request flush hook via `inertia.Middleware`.

### Fixed

- Initial HTML response now includes `deferredProps` metadata so v3
  clients automatically issue follow-up partial reloads.
- `X-Inertia-Partial-Except` without `X-Inertia-Partial-Data` now
  returns all eager props minus the excluded set (previously dropped
  most props).
- `CookieStore` no longer overwrites prior flashes when multiple
  `FlashErrors`/`FlashMessage` calls occur in the same response —
  payload accumulates and emits a single `Set-Cookie`.

## [0.3.1] — 2026-05-19

### Fixed

- `ssr.HTTPClient.Render` / `Ping` now explicitly discard the
  `resp.Body.Close` error to satisfy `errcheck` under
  `golangci-lint` v2. Runtime behaviour is unchanged.

## [0.3.0] — 2026-05-19

### Added

- `ssr` sub-package providing `HTTPClient` (with `NewHTTP` constructor,
  `Render`, `Ping` methods) that speaks the Inertia.js SSR HTTP
  protocol. Stdlib-only, no external dependencies.
- Main package: `SSRClient` interface (parallel to `ViteHelper` /
  `SessionStore`). Returns stdlib types only — sub-package does not
  import the main package.
- `writeHTML` invokes `Config.SSR.Render` for the initial HTML
  response when configured, injecting head fragments and body into
  `RootData.InertiaHead` / `InertiaBody`. Inertia XHR requests skip
  SSR. Default behaviour on SSR error: log a warning and fall back to
  CSR. With `Config.SSRRequired=true`, errors route through
  `Config.ErrorHandler` wrapped as `ErrSSRUnavailable`.

### Changed

- `Config.SSR` type from `any` to `SSRClient`. v0.1.0 and v0.2.0 docs
  declared the field reserved and ignored at runtime, so no working
  user code is affected.
- `Config.SSRRequired` documentation activated: the field's behavior
  is now wired through `writeHTML`.

## [0.2.0] — 2026-05-19

### Added

- `vite` sub-package providing `Manifest` (with `Load`, `MustLoad`, `Dev`
  constructors) and four template helpers: `Tag`, `Asset`, `CSS`,
  `ReactRefresh`. Pure-stdlib implementation, no external dependencies.
- `ErrManifestNotFound` sentinel in `vite` package.
- Main package: `ViteHelper` interface (parallel to `SessionStore`).
- Root template FuncMap now exposes `vite`, `viteAsset`, `viteCSS`,
  `viteReactRefresh`. When `Config.Vite` is nil each helper logs once and
  emits empty content, so templates always parse.

### Changed

- `Config.Vite` type from `any` to `ViteHelper`. The v0.1.0 docs declared
  this field reserved and ignored at runtime, so no working user code
  depended on the previous type.

## [0.1.0] — 2026-05-19

### Added

- Core `*Inertia` type, `Config`, and `New` constructor with validation.
- `Middleware` that parses Inertia request headers, sets `Vary: X-Inertia`,
  and exposes per-request errors/flash collectors via context.
- `Render` implementing the full Inertia v3 protocol: shared-props merge,
  partial-reload filtering (`Partial-Data` / `Partial-Component` /
  `Partial-Except`), version negotiation with 409, prop wrapper evaluation,
  PageObject construction (`mergeProps`, `deepMergeProps`, `deferredProps`,
  `encryptHistory`, `clearHistory`), and HTML vs JSON dispatch.
- Prop wrappers: `Always`, `Optional`, `Defer`, `Merge`, `DeepMerge`.
- Shared props: `Share`, `ShareValue`, `ShareEval`.
- Redirect helpers: `Redirect` (302/303), `Location` (409 +
  `X-Inertia-Location`), `Back`, plus fragment-redirect handling.
- `session.Store` interface plus `CookieStore` (AES-GCM, key rotation,
  size limit), `MemoryStore` (test-only), and `Noop`.
- Default `html/template` root template renderer with custom `RootRender`
  override and minimal fallback template.
- Version sources: static string, function, `VersionFromFS` (directory hash).
- Sentinel errors: `ErrSessionRequired`, `ErrTemplateNotFound`,
  `ErrCookieTooLarge`, `ErrSSRUnavailable`, `ErrPropEvaluationFailed`,
  `ErrConflictingVersion`.
- Protocol conformance test suite (`protocol_test.go`) verifying header,
  status-code, and PageObject behaviour against the v3 spec.
- `examples/basic` end-to-end demo.

### Deferred to later releases

- Vite manifest helper (`vite/`) — landed in v0.2.0.
- SSR HTTP client (`ssr/`) — planned for v0.3.0.
