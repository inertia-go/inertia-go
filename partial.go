package inertia

// filterKeys returns the subset of props keys that must be included in the
// current response, applying Inertia v3 partial-reload rules.
//
// Parameters:
//   - props:            the full prop map.
//   - reqComponent:     X-Inertia-Partial-Component (empty for non-partial).
//   - currentComponent: the component being rendered (Render's `component` arg).
//   - partialData:      X-Inertia-Partial-Data values.
//   - partialExcept:    X-Inertia-Partial-Except values.
//
// Rules:
//   - When reqComponent != currentComponent (or is empty), the request is
//     treated as a full response: every prop whose evaluateEager() returns
//     true is kept. Bare values count as eager.
//   - Otherwise, keep = (partialData ∪ alwaysInclude) − partialExcept.
//     Keys in partialData that do not exist in props are silently ignored.
func filterKeys(props Props, reqComponent, currentComponent string,
	partialData, partialExcept []string) []string {

	isPartial := reqComponent != "" && reqComponent == currentComponent

	if !isPartial {
		out := make([]string, 0, len(props))
		for k, v := range props {
			if isEagerEvaluated(v) {
				out = append(out, k)
			}
		}
		return out
	}

	requested := make(map[string]bool, len(partialData))
	for _, k := range partialData {
		requested[k] = true
	}
	excluded := make(map[string]bool, len(partialExcept))
	for _, k := range partialExcept {
		excluded[k] = true
	}

	out := make([]string, 0, len(props))
	for k, v := range props {
		if excluded[k] {
			continue
		}
		if requested[k] || alwaysIncluded(v) {
			if _, exists := props[k]; exists {
				out = append(out, k)
			}
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
