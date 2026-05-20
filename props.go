package inertia

import "time"

type propKind int

const (
	kindEager propKind = iota
	kindOptional
	kindDeferred
)

// propBuilder is the single prop type. Base constructors return *propBuilder;
// chainable modifiers set fields and return the same pointer. Conflicting
// modifiers panic at construct time (fail-fast).
type propBuilder struct {
	value any
	fn    func() (any, error)
	kind  propKind

	always bool
	defGrp string
	rescue bool

	merge       bool
	deepMerge   bool
	prependPath []string
	appendPath  []string
	matchOn     map[string]string

	once      bool
	onceTTL   time.Duration
	onceKey   string
	onceFresh bool
}

func (b *propBuilder) resolve() (any, error) {
	if b.fn != nil {
		return b.fn()
	}
	return b.value, nil
}

// Always wraps a value so it is included in every Inertia response,
// regardless of partial-reload selectors.
func Always(v any) *propBuilder { return &propBuilder{value: v, kind: kindEager, always: true} }

// Optional wraps a callback that is only evaluated when the client asks for
// this prop on a partial reload.
func Optional(fn func() (any, error)) *propBuilder {
	return &propBuilder{fn: fn, kind: kindOptional}
}

// Defer wraps a callback that is excluded from the initial response and
// listed in PageObject.deferredProps so the client fetches it post-mount.
// At most one group label may be passed (default "default"); passing more
// than one panics.
func Defer(fn func() (any, error), group ...string) *propBuilder {
	if len(group) > 1 {
		panic("inertia.Defer: at most one group label is allowed")
	}
	g := "default"
	if len(group) == 1 && group[0] != "" {
		g = group[0]
	}
	return &propBuilder{fn: fn, kind: kindDeferred, defGrp: g}
}

// Merge wraps a value (or func() (any, error)) that should be merged into the
// existing client-side value on partial reloads (shallow array/object append).
func Merge(v any) *propBuilder {
	b := &propBuilder{kind: kindEager, merge: true}
	b.setValueOrFn(v)
	return b
}

// DeepMerge is like Merge but signals the client to recursively merge nested
// objects instead of replacing at the top level.
func DeepMerge(v any) *propBuilder {
	b := &propBuilder{kind: kindEager, deepMerge: true}
	b.setValueOrFn(v)
	return b
}

// Once wraps a callback whose result the client caches once and reuses on
// subsequent navigations. Chain .ExpiresIn(d) to set a TTL, .As(k) to set a
// cache key, or .Fresh() to always re-resolve.
func Once(fn func() (any, error)) *propBuilder {
	return &propBuilder{fn: fn, kind: kindEager, once: true}
}

func (b *propBuilder) setValueOrFn(v any) {
	if fn, ok := v.(func() (any, error)); ok {
		b.fn = fn
		return
	}
	b.value = v
}

func (b *propBuilder) requireMergeFamily(name string) {
	if !b.merge && !b.deepMerge {
		panic("inertia: ." + name + " requires Merge or DeepMerge")
	}
}

func (b *propBuilder) requireOnce(name string) {
	if !b.once {
		panic("inertia: ." + name + " requires Once")
	}
}

// Prepend marks the given dotted sub-paths (relative to the prop key) for
// client-side prepend on partial reloads. With no arguments it targets the
// prop root. Requires Merge or DeepMerge.
func (b *propBuilder) Prepend(paths ...string) *propBuilder {
	b.requireMergeFamily("Prepend")
	if len(paths) == 0 {
		b.prependPath = append(b.prependPath, "")
	} else {
		b.prependPath = append(b.prependPath, paths...)
	}
	return b
}

// Append marks the given dotted sub-paths (relative to the prop key) for
// client-side append on partial reloads. Requires Merge or DeepMerge.
// A no-arg Append is a no-op: root append is already the default merge
// behavior, so only non-empty sub-paths are emitted.
func (b *propBuilder) Append(paths ...string) *propBuilder {
	b.requireMergeFamily("Append")
	if len(paths) == 0 {
		b.appendPath = append(b.appendPath, "")
	} else {
		b.appendPath = append(b.appendPath, paths...)
	}
	return b
}

// MatchOn declares one or more path->field mappings used by the client to
// reconcile list items across partial reloads. Requires Merge or DeepMerge.
func (b *propBuilder) MatchOn(pathField map[string]string) *propBuilder {
	b.requireMergeFamily("MatchOn")
	if len(pathField) == 0 {
		panic("inertia.MatchOn: at least one path->field mapping is required")
	}
	if b.matchOn == nil {
		b.matchOn = make(map[string]string, len(pathField))
	}
	for p, f := range pathField {
		b.matchOn[p] = f
	}
	return b
}

// DeepMerge switches a Merge prop to recursive (deep) merging.
func (b *propBuilder) DeepMerge() *propBuilder {
	b.merge = false
	b.deepMerge = true
	return b
}

// Once marks the prop as a once-cached prop.
func (b *propBuilder) Once() *propBuilder {
	b.once = true
	return b
}

// ExpiresIn sets how long the client may reuse the cached value before
// re-fetching. Requires Once.
func (b *propBuilder) ExpiresIn(d time.Duration) *propBuilder {
	b.requireOnce("ExpiresIn")
	b.onceTTL = d
	return b
}

// As overrides the cache key for a once prop. Requires Once.
func (b *propBuilder) As(key string) *propBuilder {
	b.requireOnce("As")
	b.onceKey = key
	return b
}

// Fresh forces a once prop to re-resolve even when the client reports it
// cached. Requires Once.
func (b *propBuilder) Fresh() *propBuilder {
	b.requireOnce("Fresh")
	b.onceFresh = true
	return b
}

// Rescue marks a deferred prop so that, if its callback returns an error
// during resolution, the prop is dropped and its key is added to
// PageObject.rescuedProps instead of failing the whole response. Requires Defer.
func (b *propBuilder) Rescue() *propBuilder {
	if b.kind != kindDeferred {
		panic("inertia: .Rescue requires Defer")
	}
	b.rescue = true
	return b
}

func asBuilder(v any) (*propBuilder, bool) {
	b, ok := v.(*propBuilder)
	return b, ok
}

