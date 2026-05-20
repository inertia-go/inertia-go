package inertia

import (
	"fmt"
	"sync"
)

// ScrollAdapter converts a paginator/metadata object into a ScrollConfig.
// Match reports whether this adapter handles meta; Derive normalizes it.
// Register custom adapters with RegisterScrollAdapter.
type ScrollAdapter interface {
	Match(meta any) bool
	Derive(meta any, o ScrollOptions) ScrollConfig
}

// ScrollOptions carries per-call overrides into ScrollAdapter.Derive.
type ScrollOptions struct {
	// PageName overrides the adapter-derived page query-param name.
	// Empty means the adapter decides.
	PageName string
}

var (
	scrollMu       sync.RWMutex
	scrollAdapters = builtinScrollAdapters()
)

// builtinScrollAdapters returns the default registry: the identity adapter
// (ScrollConfig passthrough) followed by the map adapter. User adapters are
// appended after these and, by reverse-order matching, take precedence.
func builtinScrollAdapters() []ScrollAdapter {
	return []ScrollAdapter{identityAdapter{}, mapAdapter{}}
}

// RegisterScrollAdapter appends a to the global registry. Adapters are
// matched in REVERSE registration order (last registered wins), so a user
// adapter overrides a built-in one for an overlapping type. Safe for
// concurrent use; intended for init-time registration.
func RegisterScrollAdapter(a ScrollAdapter) {
	scrollMu.Lock()
	defer scrollMu.Unlock()
	scrollAdapters = append(scrollAdapters, a)
}

// resetScrollAdapters restores the registry to just the built-ins. Tests
// call it (via t.Cleanup) to undo RegisterScrollAdapter side effects.
func resetScrollAdapters() {
	scrollMu.Lock()
	defer scrollMu.Unlock()
	scrollAdapters = builtinScrollAdapters()
}

// deriveScroll walks the registry in reverse order and returns the first
// matching adapter's ScrollConfig. Panics if no adapter matches (fail-fast,
// consistent with prop-construction misuse elsewhere).
func deriveScroll(meta any, o ScrollOptions) ScrollConfig {
	// The RLock is held across every adapter Match/Derive call to avoid
	// racing with RegisterScrollAdapter's in-place append. Consequently an
	// adapter implementation must NOT call RegisterScrollAdapter from within
	// Match/Derive (it would deadlock on scrollMu).
	scrollMu.RLock()
	defer scrollMu.RUnlock()
	for i := len(scrollAdapters) - 1; i >= 0; i-- {
		if scrollAdapters[i].Match(meta) {
			return scrollAdapters[i].Derive(meta, o)
		}
	}
	panic(fmt.Sprintf("inertia: no ScrollAdapter matched %T", meta))
}

// identityAdapter handles a ScrollConfig passed directly, subsuming the
// pre-v0.7 manual path. PageName override still applies.
type identityAdapter struct{}

func (identityAdapter) Match(meta any) bool { _, ok := meta.(ScrollConfig); return ok }
func (identityAdapter) Derive(meta any, o ScrollOptions) ScrollConfig {
	cfg := meta.(ScrollConfig)
	if cfg.PageName == "" {
		cfg.PageName = "page"
	}
	if o.PageName != "" {
		cfg.PageName = o.PageName
	}
	return cfg
}

// mapAdapter is the "metadata hash" escape hatch for users without a
// supported paginator. Accepts map[string]any with keys pageName,
// currentPage, previousPage, nextPage (all optional).
type mapAdapter struct{}

func (mapAdapter) Match(meta any) bool { _, ok := meta.(map[string]any); return ok }
func (mapAdapter) Derive(meta any, o ScrollOptions) ScrollConfig {
	m := meta.(map[string]any)
	cfg := ScrollConfig{
		PageName:     asString(m["pageName"], "page"),
		CurrentPage:  asInt(m["currentPage"]),
		PreviousPage: asIntPtr(m["previousPage"]),
		NextPage:     asIntPtr(m["nextPage"]),
	}
	if o.PageName != "" {
		cfg.PageName = o.PageName
	}
	return cfg
}

func asString(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

func asIntPtr(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case *int:
		return n
	case float64:
		i := int(n)
		return &i
	default:
		return nil
	}
}

// scrollProp is the prop type produced by Scroll. metadata is resolved
// through the adapter registry at render time; dataFn is evaluated lazily
// only if the prop survives partial-reload filtering. wrapper is the
// nesting key for the data (default "data"); pageName overrides the
// adapter-derived page query-param name.
type scrollProp struct {
	metadata any
	dataFn   func() any
	wrapper  string
	pageName string
}

// Scroll wraps a page of infinite-scroll data. metadata is resolved via the
// ScrollAdapter registry into a ScrollConfig; data is a lazy callback run
// only when the prop is included. The resolved data nests at
// props.<key>.<wrapper> (wrapper default "data"), <key>.<wrapper> is listed
// in mergeProps, and the derived config is emitted under scrollProps.<key>.
func Scroll(metadata any, data func() any, opts ...ScrollOption) *scrollProp {
	if data == nil {
		panic("inertia.Scroll: data callback must not be nil")
	}
	sp := &scrollProp{metadata: metadata, dataFn: data, wrapper: "data"}
	for _, o := range opts {
		o(sp)
	}
	return sp
}

// ScrollOption is a functional option for Scroll.
type ScrollOption func(*scrollProp)

// WithPageName overrides the page query-param name. Multiple scroll
// containers on one page need distinct names to avoid URL conflicts.
func WithPageName(name string) ScrollOption {
	return func(sp *scrollProp) { sp.pageName = name }
}

// WithWrapper sets the nesting key for the data (default "data"). Only
// props.<key>.<wrapper> is merged; sibling keys in the same object are
// preserved across infinite-scroll page loads.
func WithWrapper(key string) ScrollOption {
	return func(sp *scrollProp) { sp.wrapper = key }
}

// scrollConfig resolves the prop's metadata through the registry, applying
// the per-prop pageName override.
func (sp *scrollProp) scrollConfig() ScrollConfig {
	return deriveScroll(sp.metadata, ScrollOptions{PageName: sp.pageName})
}
