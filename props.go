package inertia

import "time"

// propWrapper is the internal interface every prop wrapper satisfies.
// Bare values (non-wrapper) are handled by the evaluator directly.
type propWrapper interface {
	// evaluate returns the prop's value, invoking any callback.
	evaluate() (any, error)
	// evaluateEager reports whether the prop is evaluated in the full-response
	// (non-partial) path. Always/Merge/DeepMerge/Prepend/MatchOn return true;
	// Optional/Defer return false.
	evaluateEager() bool
	// alwaysInclude reports whether the prop must be included even on partial
	// reloads when not explicitly requested (i.e. Always).
	alwaysInclude() bool
	// isMerge reports whether mergeProps should include this key.
	isMerge() bool
	// isDeepMerge reports whether deepMergeProps should include this key.
	isDeepMerge() bool
	// isPrepend reports whether prependProps should include this key.
	isPrepend() bool
	// matchOnKeys returns the dotted key paths used for matchPropsOn
	// reconciliation, or nil if this is not a MatchOn wrapper.
	matchOnKeys() []string
	// deferGroup returns the Defer group, or "" if not a Defer wrapper.
	deferGroup() string
	// rescueOnError reports whether a Defer wrapper marked .Rescue() should
	// be dropped (and listed in rescuedProps) instead of failing the response.
	rescueOnError() bool
	// isOnce reports whether this is an Once wrapper (onceProps + skip-on-cached).
	isOnce() bool
	// onceTTL returns the Once expiry duration, or 0 for never-expires.
	onceTTL() time.Duration
	// scrollConfig returns the infinite-scroll config, or nil if not a Scroll wrapper.
	scrollConfig() *ScrollConfig
}

// The five exported constructors below (Always, Optional, Defer, Merge,
// DeepMerge) return values that implement propWrapper. The interface is
// internal: callers should treat the return value as an opaque handle to
// put into a Props map, and never attempt to assert it back to a concrete
// type or call its methods directly. The renderer extracts the behaviour
// via the partial.go and render.go helpers.

// alwaysWrap forces inclusion on every response and on every partial reload.
type alwaysWrap struct{ v any }

// Always wraps a value so it is included in every Inertia response,
// regardless of partial-reload selectors.
func Always(v any) propWrapper { return alwaysWrap{v: v} }

func (a alwaysWrap) evaluate() (any, error)    { return a.v, nil }
func (alwaysWrap) evaluateEager() bool         { return true }
func (alwaysWrap) alwaysInclude() bool         { return true }
func (alwaysWrap) isMerge() bool               { return false }
func (alwaysWrap) isDeepMerge() bool           { return false }
func (alwaysWrap) isPrepend() bool             { return false }
func (alwaysWrap) matchOnKeys() []string       { return nil }
func (alwaysWrap) deferGroup() string          { return "" }
func (alwaysWrap) rescueOnError() bool         { return false }
func (alwaysWrap) isOnce() bool                { return false }
func (alwaysWrap) onceTTL() time.Duration      { return 0 }
func (alwaysWrap) scrollConfig() *ScrollConfig { return nil }

// optionalWrap is excluded from full responses and from unrequested partial
// reloads. Included only when explicitly requested via X-Inertia-Partial-Data.
type optionalWrap struct{ fn func() (any, error) }

// Optional wraps a callback that is only evaluated when the client asks for
// this prop on a partial reload.
func Optional(fn func() (any, error)) propWrapper { return optionalWrap{fn: fn} }

func (o optionalWrap) evaluate() (any, error)    { return o.fn() }
func (optionalWrap) evaluateEager() bool         { return false }
func (optionalWrap) alwaysInclude() bool         { return false }
func (optionalWrap) isMerge() bool               { return false }
func (optionalWrap) isDeepMerge() bool           { return false }
func (optionalWrap) isPrepend() bool             { return false }
func (optionalWrap) matchOnKeys() []string       { return nil }
func (optionalWrap) deferGroup() string          { return "" }
func (optionalWrap) rescueOnError() bool         { return false }
func (optionalWrap) isOnce() bool                { return false }
func (optionalWrap) onceTTL() time.Duration      { return 0 }
func (optionalWrap) scrollConfig() *ScrollConfig { return nil }

// deferWrap is like optional but also surfaces in the PageObject's
// deferredProps map, telling the client to fetch the value automatically.
type deferWrap struct {
	fn    func() (any, error)
	group string
}

// Defer wraps a callback that is excluded from the initial response and
// listed in PageObject.deferredProps so the client fetches it post-mount.
// At most one group label may be passed (default "default"); passing more
// than one panics. The group batches deferred props that should be fetched
// in the same partial-reload request.
func Defer(fn func() (any, error), group ...string) propWrapper {
	if len(group) > 1 {
		panic("inertia.Defer: at most one group label is allowed")
	}
	g := "default"
	if len(group) == 1 && group[0] != "" {
		g = group[0]
	}
	return deferWrap{fn: fn, group: g}
}

func (d deferWrap) evaluate() (any, error)    { return d.fn() }
func (deferWrap) evaluateEager() bool         { return false }
func (deferWrap) alwaysInclude() bool         { return false }
func (deferWrap) isMerge() bool               { return false }
func (deferWrap) isDeepMerge() bool           { return false }
func (deferWrap) isPrepend() bool             { return false }
func (deferWrap) matchOnKeys() []string       { return nil }
func (d deferWrap) deferGroup() string        { return d.group }
func (deferWrap) rescueOnError() bool         { return false }
func (deferWrap) isOnce() bool                { return false }
func (deferWrap) onceTTL() time.Duration      { return 0 }
func (deferWrap) scrollConfig() *ScrollConfig { return nil }

// mergeWrap marks a prop value for client-side array/object merging on
// subsequent partial reloads (top-level merge).
type mergeWrap struct{ v any }

// Merge wraps a value that should be merged into the existing client-side
// value on partial reloads (shallow array/object append).
func Merge(v any) propWrapper { return mergeWrap{v: v} }

func (m mergeWrap) evaluate() (any, error)    { return m.v, nil }
func (mergeWrap) evaluateEager() bool         { return true }
func (mergeWrap) alwaysInclude() bool         { return false }
func (mergeWrap) isMerge() bool               { return true }
func (mergeWrap) isDeepMerge() bool           { return false }
func (mergeWrap) isPrepend() bool             { return false }
func (mergeWrap) matchOnKeys() []string       { return nil }
func (mergeWrap) deferGroup() string          { return "" }
func (mergeWrap) rescueOnError() bool         { return false }
func (mergeWrap) isOnce() bool                { return false }
func (mergeWrap) onceTTL() time.Duration      { return 0 }
func (mergeWrap) scrollConfig() *ScrollConfig { return nil }

// deepMergeWrap is like mergeWrap but signals recursive merging client-side.
type deepMergeWrap struct{ v any }

// DeepMerge is like Merge but signals the client to recursively merge nested
// objects instead of replacing at the top level.
func DeepMerge(v any) propWrapper { return deepMergeWrap{v: v} }

func (d deepMergeWrap) evaluate() (any, error)    { return d.v, nil }
func (deepMergeWrap) evaluateEager() bool         { return true }
func (deepMergeWrap) alwaysInclude() bool         { return false }
func (deepMergeWrap) isMerge() bool               { return false }
func (deepMergeWrap) isDeepMerge() bool           { return true }
func (deepMergeWrap) isPrepend() bool             { return false }
func (deepMergeWrap) matchOnKeys() []string       { return nil }
func (deepMergeWrap) deferGroup() string          { return "" }
func (deepMergeWrap) rescueOnError() bool         { return false }
func (deepMergeWrap) isOnce() bool                { return false }
func (deepMergeWrap) onceTTL() time.Duration      { return 0 }
func (deepMergeWrap) scrollConfig() *ScrollConfig { return nil }

// prependWrap marks a value for client-side prepend on partial reloads.
type prependWrap struct{ v any }

// Prepend wraps a value that the client should prepend (instead of
// append) to the existing client-side value on partial reloads.
func Prepend(v any) propWrapper { return prependWrap{v: v} }

func (p prependWrap) evaluate() (any, error)    { return p.v, nil }
func (prependWrap) evaluateEager() bool         { return true }
func (prependWrap) alwaysInclude() bool         { return false }
func (prependWrap) isMerge() bool               { return false }
func (prependWrap) isDeepMerge() bool           { return false }
func (prependWrap) isPrepend() bool             { return true }
func (prependWrap) matchOnKeys() []string       { return nil }
func (prependWrap) deferGroup() string          { return "" }
func (prependWrap) rescueOnError() bool         { return false }
func (prependWrap) isOnce() bool                { return false }
func (prependWrap) onceTTL() time.Duration      { return 0 }
func (prependWrap) scrollConfig() *ScrollConfig { return nil }

// matchOnWrap declares one or more dotted key paths used by the client
// to reconcile list items across partial reloads.
type matchOnWrap struct {
	v    any
	keys []string
}

// MatchOn wraps a value (typically a list) and declares one or more
// dotted key paths used by the client to reconcile items across partial
// reloads (e.g. matching by "id" or "uuid"). Panics if no keys are given.
func MatchOn(v any, keys ...string) propWrapper {
	if len(keys) == 0 {
		panic("inertia.MatchOn: at least one key path is required")
	}
	cp := make([]string, len(keys))
	copy(cp, keys)
	return matchOnWrap{v: v, keys: cp}
}

func (m matchOnWrap) evaluate() (any, error)    { return m.v, nil }
func (matchOnWrap) evaluateEager() bool         { return true }
func (matchOnWrap) alwaysInclude() bool         { return false }
func (matchOnWrap) isMerge() bool               { return false }
func (matchOnWrap) isDeepMerge() bool           { return false }
func (matchOnWrap) isPrepend() bool             { return false }
func (m matchOnWrap) matchOnKeys() []string     { return m.keys }
func (matchOnWrap) deferGroup() string          { return "" }
func (matchOnWrap) rescueOnError() bool         { return false }
func (matchOnWrap) isOnce() bool                { return false }
func (matchOnWrap) onceTTL() time.Duration      { return 0 }
func (matchOnWrap) scrollConfig() *ScrollConfig { return nil }

// asWrapper returns w as a propWrapper if it is one, plus ok=true.
func asWrapper(v any) (propWrapper, bool) {
	w, ok := v.(propWrapper)
	return w, ok
}
