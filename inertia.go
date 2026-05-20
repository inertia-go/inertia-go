// Package inertia is a Go server-side adapter for Inertia.js v3.
// It implements the Inertia protocol on top of net/http and is intended
// to be used directly or wrapped by framework-specific adapter packages
// (e.g. inertia-go-gin).
//
// The package exposes an *Inertia value with no global state. Construct
// one with New(Config{...}), install its Middleware, then call Render
// from your HTTP handlers.
package inertia

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Props is the user-facing alias for prop maps passed to Render.
type Props = map[string]any

// SessionStore is the interface the package needs from a session backend.
// The real definition lives in package session; we declare it here as a
// local interface so the core package does not depend on the sub-package.
// Any *session.CookieStore or *session.MemoryStore implements it.
type SessionStore interface {
	FlashErrors(w http.ResponseWriter, r *http.Request, bag string, errs map[string]string) error
	TakeErrors(w http.ResponseWriter, r *http.Request, bag string) (map[string]string, error)
	FlashMessage(w http.ResponseWriter, r *http.Request, key string, val any) error
	TakeMessages(w http.ResponseWriter, r *http.Request) (map[string]any, error)
}

// SessionFlusher is an optional capability a SessionStore may implement
// to defer writing the response cookie until the end of the HTTP
// request. inertia.Middleware calls FlushResponse via a deferred hook
// so multiple Flash*/Take* operations in a single response accumulate
// into one Set-Cookie. Stores that don't implement this interface
// continue to write eagerly per call.
type SessionFlusher interface {
	FlushResponse(w http.ResponseWriter) error
}

// RootData is passed to the root template (default or RootRender hook).
type RootData struct {
	InertiaHead template.HTML
	InertiaBody template.HTML
	Component   string
	URL         string
	Version     string
	PageJSON    template.JS
}

// ViteHelper is the contract the core package consumes from any
// Vite-style asset resolver. The vite sub-package's *Manifest satisfies
// this interface; users may also supply custom implementations (e.g.
// a webpack-manifest resolver). A nil ViteHelper is permitted: each
// template helper degrades to a no-op that logs once.
type ViteHelper interface {
	Tag(entry string) template.HTML
	Asset(entry string) string
	CSS(entry string) template.HTML
	ReactRefresh() template.HTML
}

// SSRClient is the contract the core package consumes from any
// server-side renderer. The ssr sub-package's *HTTPClient satisfies
// this interface; users may supply alternatives (gRPC, in-process
// renderer, pool with retries, ...). A nil SSRClient disables SSR
// entirely.
//
// Render is invoked once per initial HTML response — Inertia XHR
// requests skip SSR. The page argument is the already-serialized
// PageObject, identical to the bytes that would otherwise be embedded
// inside the <script data-page="app" type="application/json"> element.
//
// The returned head and body are injected verbatim into the root
// template as template.HTML, bypassing html/template auto-escaping.
// Use only a trusted SSR service: a hostile renderer could return
// fragments that script the page. The reference *ssr.HTTPClient
// assumes a developer-controlled loopback Node service.
//
// Implementations must respect ctx cancellation: the request's
// context is forwarded so client disconnect aborts the SSR call.
type SSRClient interface {
	Render(ctx context.Context, page json.RawMessage) (head []string, body string, err error)
}

// Config configures an *Inertia instance. Required: Session.
type Config struct {
	// RootView is the template name (relative to TemplateFS) used for the
	// initial HTML response. If empty, a minimal fallback template is used.
	RootView string

	// TemplateFS is the filesystem from which RootView is loaded.
	// If nil, RootView lookups fall back to the minimal template.
	TemplateFS fs.FS

	// RootRender overrides html/template entirely when non-nil.
	RootRender func(w io.Writer, data RootData) error

	// HotReload causes the root template to be reparsed on every request
	// (useful for development).
	HotReload bool

	// Version is the static asset version string.
	// At most one of Version / VersionFunc / VersionFromFS may be set.
	Version       string
	VersionFunc   func(r *http.Request) string
	VersionFromFS fs.FS

	// EncryptHistory / ClearHistory set page-level meta in the PageObject.
	EncryptHistory bool
	ClearHistory   bool

	// PreserveFragment is the default for the page object's preserveFragment
	// flag. Override per-request with inertia.SetPreserveFragment(r, bool).
	PreserveFragment bool

	// Session is required when using errors or flash (which the package
	// auto-injects on every Render). Pass session.NewNoop() to opt out.
	Session SessionStore

	// FlashPropKey / ErrorsPropKey / DefaultErrorBag override defaults.
	FlashPropKey    string // default "flash"
	ErrorsPropKey   string // default "errors"
	DefaultErrorBag string // default "default"

	// SSR, if non-nil, enables server-side pre-rendering for the
	// initial HTML response. Inertia XHR requests skip SSR. On error
	// the default behaviour is to log a warning and fall back to CSR;
	// set SSRRequired=true to convert SSR errors into 500s via
	// ErrorHandler.
	SSR SSRClient

	// Vite, if non-nil, registers four template helpers in the root
	// template: vite, viteAsset, viteCSS, viteReactRefresh. See the
	// ViteHelper interface above; the vite sub-package provides a
	// reference implementation via *vite.Manifest.
	Vite ViteHelper

	// SSRRequired, when true, converts SSR errors into HTTP 500
	// responses routed through ErrorHandler with the underlying error
	// wrapped as ErrSSRUnavailable. Default: log + fall back to CSR.
	SSRRequired bool

	// ErrorHandler handles unrecoverable runtime errors (prop evaluation
	// failure, template rendering failure). Default: slog.Error + 500.
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// Logger is used for all package logs. Default: slog.Default().
	Logger *slog.Logger
}

// Inertia is the package's main type. Construct via New.
type Inertia struct {
	cfg Config

	sharedMu     sync.RWMutex
	sharedStatic map[string]any
	sharedFuncs  map[string]func(r *http.Request) any

	rootTplMu sync.RWMutex
	rootTpl   *template.Template

	logger *slog.Logger

	fsVerOnce sync.Once
	fsVer     string

	// nowFn is overridable in tests.
	nowFn func() time.Time
}

// New constructs an *Inertia from a Config. Returns ErrSessionRequired
// if Session is nil (use session.NewNoop() to suppress).
func New(cfg Config) (*Inertia, error) {
	if cfg.Session == nil {
		return nil, ErrSessionRequired
	}
	versionSources := 0
	if cfg.Version != "" {
		versionSources++
	}
	if cfg.VersionFunc != nil {
		versionSources++
	}
	if cfg.VersionFromFS != nil {
		versionSources++
	}
	if versionSources > 1 {
		return nil, ErrConflictingVersion
	}
	if cfg.FlashPropKey == "" {
		cfg.FlashPropKey = "flash"
	}
	if cfg.ErrorsPropKey == "" {
		cfg.ErrorsPropKey = "errors"
	}
	if cfg.DefaultErrorBag == "" {
		cfg.DefaultErrorBag = "default"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultErrorHandler(cfg.Logger)
	}
	return &Inertia{
		cfg:          cfg,
		sharedStatic: map[string]any{},
		sharedFuncs:  map[string]func(r *http.Request) any{},
		logger:       cfg.Logger,
		nowFn:        time.Now,
	}, nil
}

func defaultErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger.ErrorContext(r.Context(), "inertia: unhandled error",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("err", err),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Share registers a per-request prop function whose result is merged into
// every Render call's props. Callbacks run on every request unless the
// partial-reload filter excludes them.
func (i *Inertia) Share(key string, fn func(r *http.Request) any) {
	i.sharedMu.Lock()
	defer i.sharedMu.Unlock()
	i.sharedFuncs[key] = fn
}

// ShareValue registers a static value that is merged into every Render call.
func (i *Inertia) ShareValue(key string, v any) {
	i.sharedMu.Lock()
	defer i.sharedMu.Unlock()
	i.sharedStatic[key] = v
}

// ShareEval is an alias for Share kept for parity with the design spec.
func (i *Inertia) ShareEval(key string, fn func(r *http.Request) any) {
	i.Share(key, fn)
}

// flushSession invokes the session store's FlushResponse hook if the
// store implements SessionFlusher. Called via defer at the end of every
// request handled by Middleware so accumulators can emit their writes.
func (i *Inertia) flushSession(w http.ResponseWriter) {
	fl, ok := i.cfg.Session.(SessionFlusher)
	if !ok {
		return
	}
	if err := fl.FlushResponse(w); err != nil {
		i.logger.Warn("inertia: session flush failed", "err", err)
	}
}
