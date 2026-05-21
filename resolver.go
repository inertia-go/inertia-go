package inertia

import "strings"

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
