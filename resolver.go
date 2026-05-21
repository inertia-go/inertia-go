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
