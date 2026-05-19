# Changelog

All notable changes to this project will be documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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

- Vite manifest helper (`vite/`) — planned for v0.2.0.
- SSR HTTP client (`ssr/`) — planned for v0.3.0.
