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
	scrollMu.RLock()
	adapters := scrollAdapters
	scrollMu.RUnlock()
	for i := len(adapters) - 1; i >= 0; i-- {
		if adapters[i].Match(meta) {
			return adapters[i].Derive(meta, o)
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
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

func asIntPtr(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case *int:
		return n
	default:
		return nil
	}
}
