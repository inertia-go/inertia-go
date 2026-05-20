package inertia

// filterKeys returns the subset of props keys that must be included in the
// current response, applying Inertia v3 partial-reload rules.
//
// Rules:
//   - Non-partial (reqComponent empty or != currentComponent): keep all
//     keys whose evaluateEager() is true; partialExcept is ignored.
//   - Partial with non-empty partialData: keep (partialData ∪ alwaysInclude)
//     − partialExcept.
//   - Partial with empty partialData (only Partial-Except or neither
//     header set): keep (all eager ∪ alwaysInclude) − partialExcept.
func filterKeys(props Props, reqComponent, currentComponent string,
	partialData, partialExcept []string) []string {

	isPartial := reqComponent != "" && reqComponent == currentComponent
	excluded := setOf(partialExcept)

	if !isPartial {
		// partialExcept is intentionally ignored for non-partial responses.
		return collect(props, func(_ string, v any) bool {
			return isEagerEvaluated(v)
		})
	}

	if len(partialData) == 0 {
		return collect(props, func(k string, v any) bool {
			if excluded[k] {
				return false
			}
			return isEagerEvaluated(v) || alwaysIncluded(v)
		})
	}

	requested := setOf(partialData)
	return collect(props, func(k string, v any) bool {
		if excluded[k] {
			return false
		}
		return requested[k] || alwaysIncluded(v)
	})
}

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

func collect(props Props, pred func(k string, v any) bool) []string {
	out := make([]string, 0, len(props))
	for k, v := range props {
		if pred(k, v) {
			out = append(out, k)
		}
	}
	return out
}

func isEagerEvaluated(v any) bool {
	if w, ok := asWrapper(v); ok {
		return w.evaluateEager()
	}
	return true // bare values are always evaluated eagerly.
}

func alwaysIncluded(v any) bool {
	if w, ok := asWrapper(v); ok {
		return w.alwaysInclude()
	}
	return false
}
