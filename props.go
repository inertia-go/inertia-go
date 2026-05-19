package inertia

// propWrapper is the internal interface every prop wrapper satisfies.
// Bare values (non-wrapper) are handled by the evaluator directly.
type propWrapper interface {
	// evaluate returns the prop's value, invoking any callback.
	evaluate() (any, error)
	// evaluateEager reports whether the prop is evaluated in the full-response
	// (non-partial) path. Always/Merge/DeepMerge return true; Optional/Defer
	// return false.
	evaluateEager() bool
	// alwaysInclude reports whether the prop must be included even on partial
	// reloads when not explicitly requested (i.e. Always).
	alwaysInclude() bool
	// isMerge reports whether mergeProps should include this key.
	isMerge() bool
	// isDeepMerge reports whether deepMergeProps should include this key.
	isDeepMerge() bool
	// deferGroup returns the Defer group, or "" if not a Defer wrapper.
	deferGroup() string
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

func (a alwaysWrap) evaluate() (any, error) { return a.v, nil }
func (alwaysWrap) evaluateEager() bool      { return true }
func (alwaysWrap) alwaysInclude() bool      { return true }
func (alwaysWrap) isMerge() bool            { return false }
func (alwaysWrap) isDeepMerge() bool        { return false }
func (alwaysWrap) deferGroup() string       { return "" }

// optionalWrap is excluded from full responses and from unrequested partial
// reloads. Included only when explicitly requested via X-Inertia-Partial-Data.
type optionalWrap struct{ fn func() (any, error) }

// Optional wraps a callback that is only evaluated when the client asks for
// this prop on a partial reload.
func Optional(fn func() (any, error)) propWrapper { return optionalWrap{fn: fn} }

func (o optionalWrap) evaluate() (any, error) { return o.fn() }
func (optionalWrap) evaluateEager() bool      { return false }
func (optionalWrap) alwaysInclude() bool      { return false }
func (optionalWrap) isMerge() bool            { return false }
func (optionalWrap) isDeepMerge() bool        { return false }
func (optionalWrap) deferGroup() string       { return "" }

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

func (d deferWrap) evaluate() (any, error) { return d.fn() }
func (deferWrap) evaluateEager() bool      { return false }
func (deferWrap) alwaysInclude() bool      { return false }
func (deferWrap) isMerge() bool            { return false }
func (deferWrap) isDeepMerge() bool        { return false }
func (d deferWrap) deferGroup() string     { return d.group }

// mergeWrap marks a prop value for client-side array/object merging on
// subsequent partial reloads (top-level merge).
type mergeWrap struct{ v any }

// Merge wraps a value that should be merged into the existing client-side
// value on partial reloads (shallow array/object append).
func Merge(v any) propWrapper { return mergeWrap{v: v} }

func (m mergeWrap) evaluate() (any, error) { return m.v, nil }
func (mergeWrap) evaluateEager() bool      { return true }
func (mergeWrap) alwaysInclude() bool      { return false }
func (mergeWrap) isMerge() bool            { return true }
func (mergeWrap) isDeepMerge() bool        { return false }
func (mergeWrap) deferGroup() string       { return "" }

// deepMergeWrap is like mergeWrap but signals recursive merging client-side.
type deepMergeWrap struct{ v any }

// DeepMerge is like Merge but signals the client to recursively merge nested
// objects instead of replacing at the top level.
func DeepMerge(v any) propWrapper { return deepMergeWrap{v: v} }

func (d deepMergeWrap) evaluate() (any, error) { return d.v, nil }
func (deepMergeWrap) evaluateEager() bool      { return true }
func (deepMergeWrap) alwaysInclude() bool      { return false }
func (deepMergeWrap) isMerge() bool            { return false }
func (deepMergeWrap) isDeepMerge() bool        { return true }
func (deepMergeWrap) deferGroup() string       { return "" }

// asWrapper returns w as a propWrapper if it is one, plus ok=true.
func asWrapper(v any) (propWrapper, bool) {
	w, ok := v.(propWrapper)
	return w, ok
}
