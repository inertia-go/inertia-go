package inertia

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// matchesPath reports whether path equals or descends from any entry in set.
// Used for both partial "only" matching and "except" matching: a path matches
// a selector when it IS the selector or is a dot-descendant of it.
func matchesPath(set []string, path string) bool {
	for _, s := range set {
		if path == s || strings.HasPrefix(path, s+".") {
			return true
		}
	}
	return false
}

// leadsToPath reports whether path is a strict dot-ancestor of any entry in
// set. A parent like "auth" must be traversable when "only" requests
// "auth.user", so the resolver can descend to the requested leaf.
func leadsToPath(set []string, path string) bool {
	for _, s := range set {
		if strings.HasPrefix(s, path+".") {
			return true
		}
	}
	return false
}

// unpackDotProps rewrites top-level keys containing a dot into nested maps,
// mutating props in place. "auth.user" => props["auth"]["user"]. An existing
// nested map at the parent path is preserved and extended; a non-map value at
// the parent path is left alone (the flat key stays, to avoid clobbering).
// Only top-level keys are unpacked (matching the official resolver).
func unpackDotProps(props Props) {
	var dotted []string
	for key := range props {
		if strings.Contains(key, ".") {
			dotted = append(dotted, key)
		}
	}
	for _, key := range dotted {
		if setNestedPath(props, strings.Split(key, "."), props[key]) {
			delete(props, key)
		}
	}
}

// setNestedPath walks/creates nested maps for segments[:len-1] and sets the
// final segment to val. Returns false if an intermediate segment holds a
// non-map value (cannot descend), leaving props unchanged for that key.
func setNestedPath(root map[string]any, segments []string, val any) bool {
	cur := root
	for _, seg := range segments[:len(segments)-1] {
		next, ok := cur[seg]
		if !ok {
			m := map[string]any{}
			cur[seg] = m
			cur = m
			continue
		}
		m, ok := next.(map[string]any)
		if !ok {
			return false
		}
		cur = m
	}
	cur[segments[len(segments)-1]] = val
	return true
}

func newMarkers() *propMarkers {
	return &propMarkers{
		onceProps:   map[string]OnceConfig{},
		scrollProps: map[string]ScrollConfig{},
	}
}

type propsResolver struct {
	isPartial     bool
	only          []string // nil when not a partial reload or no Partial-Data
	except        []string
	exceptOnce    map[string]bool
	scrollPrepend bool
	reset         map[string]bool
	markers       *propMarkers
	rescued       []string
	deferred      map[string][]string
}

// resolve unpacks dot keys then resolves the tree from the root.
func (pr *propsResolver) resolve(props Props) (map[string]any, error) {
	if pr.markers == nil {
		pr.markers = newMarkers()
	}
	if pr.deferred == nil {
		pr.deferred = map[string][]string{}
	}
	pr.markers.scrollPrepend = pr.scrollPrepend
	pr.markers.reset = pr.reset
	unpackDotProps(props)
	return pr.resolveProps(props, "", false)
}

func (pr *propsResolver) resolveProps(props map[string]any, prefix string, parentResolved bool) (map[string]any, error) {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(props))
	for _, key := range keys {
		path := joinPath(prefix, key)
		val, include, err := pr.resolveItem(props[key], path, parentResolved)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		out[key] = val
	}
	return out, nil
}

// resolveArray resolves the elements of an indexed array, recursing into each
// element by its numeric-index path (prefix.0, prefix.1, ...). Mirrors the
// official is_array recursion, which treats list arrays the same as maps: a
// prop-type inside an element is resolved or excluded just like a map field.
// Excluded elements (e.g. an Optional element on initial load) are omitted from
// the output slice; element order is otherwise preserved.
func (pr *propsResolver) resolveArray(arr []any, prefix string, parentResolved bool) ([]any, error) {
	out := make([]any, 0, len(arr))
	for i, elem := range arr {
		path := joinPath(prefix, strconv.Itoa(i))
		val, include, err := pr.resolveItem(elem, path, parentResolved)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		out = append(out, val)
	}
	return out, nil
}

// resolveItem resolves a single value at path (a map field value or an array
// element). It returns include=false when the value must be omitted (filtered
// out on a partial reload, skipped once-cache, excluded from the initial
// response, or rescued after a callback error). Shared by resolveProps and
// resolveArray so map fields and array elements follow identical rules.
func (pr *propsResolver) resolveItem(prop any, path string, parentResolved bool) (any, bool, error) {
	if !pr.shouldInclude(path, prop, parentResolved) {
		return nil, false, nil
	}
	// A once prop the client already has cached (and isn't Fresh) still emits
	// its metadata, but its value is skipped. Match the ordering: metadata
	// first, then skip.
	if pr.shouldSkipOnce(path, prop) {
		pr.collectMetadata(path, prop)
		return nil, false, nil
	}
	if !pr.isPartial && pr.excludeFromInitial(path, prop) {
		return nil, false, nil
	}

	val, err := pr.resolveValue(prop)
	if err != nil {
		if isRescuable(prop) {
			pr.rescued = append(pr.rescued, path)
			return nil, false, nil
		}
		return nil, false, err
	}

	// Unwrap one level: a closure that returned a prop-type.
	if isPropType(val) {
		prop = val
		if !pr.isPartial && pr.excludeFromInitial(path, prop) {
			return nil, false, nil
		}
		if val, err = pr.resolveValue(prop); err != nil {
			if isRescuable(prop) {
				pr.rescued = append(pr.rescued, path)
				return nil, false, nil
			}
			return nil, false, err
		}
	}

	pr.collectMetadata(path, prop)

	// Scroll values nest under their wrapper; do not recurse into them.
	if sp, ok := prop.(*scrollProp); ok {
		return map[string]any{sp.wrapper: val}, true, nil
	}
	val, err = pr.recurseInto(val, path, parentResolved || isClosureProp(prop))
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

// recurseInto descends into a resolved value when it is itself a map or an
// indexed array, matching the official is_array recursion. []map[string]any is
// normalized to []any first. Scalars and other types pass through unchanged.
func (pr *propsResolver) recurseInto(val any, path string, childResolved bool) (any, error) {
	switch v := val.(type) {
	case map[string]any:
		return pr.resolveProps(v, path, childResolved)
	case []any:
		return pr.resolveArray(v, path, childResolved)
	case []map[string]any:
		norm := make([]any, len(v))
		for i := range v {
			norm[i] = v[i]
		}
		return pr.resolveArray(norm, path, childResolved)
	default:
		return val, nil
	}
}

func (pr *propsResolver) shouldInclude(path string, prop any, parentResolved bool) bool {
	if !pr.isPartial || alwaysIncluded(prop) || parentResolved {
		return true
	}
	return pr.pathMatches(path)
}

func (pr *propsResolver) pathMatches(path string) bool {
	if pr.only != nil && !matchesPath(pr.only, path) && !leadsToPath(pr.only, path) {
		return false
	}
	if pr.except != nil && matchesPath(pr.except, path) {
		return false
	}
	return true
}

// includedInPartialMetadata reports whether a path is eligible to contribute
// merge/once metadata on a partial reload. Mirrors the official
// isIncludedInPartialMetadata: it uses matchesOnly (matchesPath) ONLY — NOT
// leadsToPath — so an ancestor path traversed solely to reach a requested leaf
// does not surface its own merge/once metadata.
func (pr *propsResolver) includedInPartialMetadata(path string) bool {
	if pr.only != nil && !matchesPath(pr.only, path) {
		return false
	}
	if pr.except != nil && matchesPath(pr.except, path) {
		return false
	}
	return true
}

// excludeFromInitial drops Optional and Deferred props from the initial
// (non-partial) response. Before discarding the value it collects the metadata
// the client needs for the deferred follow-up: deferred-group membership (for
// Deferred), plus merge and once metadata for either kind. Mirrors the official
// excludeIgnoredProp. Returns true when the prop must be skipped.
func (pr *propsResolver) excludeFromInitial(path string, prop any) bool {
	b, ok := asBuilder(prop)
	if !ok {
		return false
	}
	switch b.kind {
	case kindOptional:
		if b.shouldMerge() {
			pr.collectMergeMetadata(path, b)
		}
		pr.collectOnceMetadata(path, b)
		return true
	case kindDeferred:
		pr.deferred[b.defGrp] = append(pr.deferred[b.defGrp], path)
		if b.shouldMerge() {
			pr.collectMergeMetadata(path, b)
		}
		pr.collectOnceMetadata(path, b)
		return true
	}
	return false
}

func (pr *propsResolver) resolveValue(prop any) (any, error) {
	return evaluateOne(prop)
}

func isPropType(v any) bool {
	if _, ok := asBuilder(v); ok {
		return true
	}
	_, ok := v.(*scrollProp)
	return ok
}

func isClosureProp(prop any) bool {
	if b, ok := asBuilder(prop); ok {
		return b.fn != nil
	}
	if _, ok := prop.(*scrollProp); ok {
		return true
	}
	return false
}

func isRescuable(prop any) bool {
	b, ok := asBuilder(prop)
	return ok && b.rescue
}

// collectMetadata classifies a prop and appends its key (the full dot path) to
// the relevant marker lists. Preserves the v0.9 behavior exactly: reset
// suppression, scroll merge-intent + reset flag, nested-target handling, once
// alias/TTL. Moved from the old propMarkers.collect, re-keyed on dot path.
func (pr *propsResolver) collectMetadata(path string, prop any) {
	if sp, ok := prop.(*scrollProp); ok {
		cfg := sp.scrollConfig()
		cfg.Reset = pr.reset[path]
		pr.markers.scrollProps[path] = cfg
		if !pr.reset[path] {
			target := joinPath(path, sp.wrapper)
			if pr.scrollPrepend {
				pr.markers.prependKeys = append(pr.markers.prependKeys, target)
			} else {
				pr.markers.mergeKeys = append(pr.markers.mergeKeys, target)
			}
		}
		return
	}
	b, ok := asBuilder(prop)
	if !ok {
		return
	}
	pr.collectMergeMetadata(path, b)
	pr.collectOnceMetadata(path, b)
}

// collectMergeMetadata appends a prop's merge/deep/prepend/append/matchOn
// markers keyed on the full dot path. Reset suppresses all merge intent
// (official collectMergeableMetadata checks reset first). It is shared by
// collectMetadata and excludeFromInitial so deferred/optional props excluded
// from the initial response still surface their merge metadata.
func (pr *propsResolver) collectMergeMetadata(path string, b *propBuilder) {
	if pr.reset[path] {
		return
	}
	if pr.isPartial && !pr.includedInPartialMetadata(path) {
		return
	}
	nested := len(b.prependPath) > 0 || len(b.appendPath) > 0
	if b.merge && !nested {
		pr.markers.mergeKeys = append(pr.markers.mergeKeys, path)
	}
	if b.deepMerge {
		pr.markers.deepKeys = append(pr.markers.deepKeys, path)
	}
	for _, p := range b.prependPath {
		pr.markers.prependKeys = append(pr.markers.prependKeys, joinPath(path, p))
	}
	for _, p := range b.appendPath {
		if p != "" {
			pr.markers.mergeKeys = append(pr.markers.mergeKeys, joinPath(path, p))
		}
	}
	for sub, field := range b.matchOn {
		pr.markers.matchOn = append(pr.markers.matchOn, joinPath(joinPath(path, sub), field))
	}
}

// collectOnceMetadata records a once prop's cache key + TTL keyed on the alias
// (or full dot path). Shared by collectMetadata and excludeFromInitial.
func (pr *propsResolver) collectOnceMetadata(path string, b *propBuilder) {
	if !b.once {
		return
	}
	if pr.isPartial && !pr.includedInPartialMetadata(path) {
		return
	}
	onceKey := path
	if b.onceKey != "" {
		onceKey = b.onceKey
	}
	var exp *int64
	if b.onceTTL > 0 {
		ms := time.Now().Add(b.onceTTL).UnixMilli()
		exp = &ms
	}
	pr.markers.onceProps[onceKey] = OnceConfig{Prop: path, ExpiresAt: exp}
}

func (b *propBuilder) shouldMerge() bool {
	return b.merge || b.deepMerge || len(b.prependPath) > 0 || len(b.appendPath) > 0 || len(b.matchOn) > 0
}

// shouldSkipOnce reports whether a once prop is client-cached and must be
// skipped (value not resolved/included). Mirrors the official
// wasAlreadyLoadedByClient: skip when the alias (.As) or, falling back, the
// full dot path is reported in X-Inertia-Except-Once-Props AND the prop is not
// .Fresh(). There is no explicit-partial-data force-refresh shortcut in the
// official resolver; only .Fresh() forces re-resolution.
func (pr *propsResolver) shouldSkipOnce(path string, prop any) bool {
	b, ok := asBuilder(prop)
	if !ok || !b.once {
		return false
	}
	onceKey := path
	if b.onceKey != "" {
		onceKey = b.onceKey
	}
	return pr.exceptOnce[onceKey] && !b.onceFresh
}
