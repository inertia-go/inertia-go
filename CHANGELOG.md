# Changelog

All notable changes to this project will be documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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
