package inertia

func setOf(s []string) map[string]bool {
	if len(s) == 0 {
		return nil
	}
	m := make(map[string]bool, len(s))
	for _, k := range s {
		m[k] = true
	}
	return m
}

func alwaysIncluded(v any) bool {
	if b, ok := asBuilder(v); ok {
		return b.always
	}
	return false
}
