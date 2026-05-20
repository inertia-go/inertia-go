package inertia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// PageObject is the JSON shape Inertia sends to the client.
type PageObject struct {
	Component      string              `json:"component"`
	Props          map[string]any      `json:"props"`
	URL            string              `json:"url"`
	Version        string              `json:"version"`
	EncryptHistory bool                `json:"encryptHistory,omitempty"`
	ClearHistory   bool                `json:"clearHistory,omitempty"`
	MergeProps     []string            `json:"mergeProps,omitempty"`
	DeepMergeProps []string            `json:"deepMergeProps,omitempty"`
	DeferredProps  map[string][]string `json:"deferredProps,omitempty"`

	// v0.4 — added with their populating code paths.
	PrependProps []string `json:"prependProps,omitempty"`
	MatchPropsOn []string `json:"matchPropsOn,omitempty"`
	SharedProps  []string `json:"sharedProps,omitempty"`

	// v0.5 — Inertia v3 page-object features.
	ScrollProps      map[string]ScrollConfig `json:"scrollProps,omitempty"`
	OnceProps        map[string]OnceConfig   `json:"onceProps,omitempty"`
	RescuedProps     []string                `json:"rescuedProps,omitempty"`
	PreserveFragment bool                    `json:"preserveFragment,omitempty"`
}

// ScrollConfig is the per-key infinite-scroll metadata emitted under
// PageObject.scrollProps. Pointers are nil when there is no adjacent page.
type ScrollConfig struct {
	PageName     string `json:"pageName"`
	PreviousPage *int   `json:"previousPage"`
	NextPage     *int   `json:"nextPage"`
	CurrentPage  int    `json:"currentPage"`
}

// OnceConfig is the per-key once-prop metadata emitted under
// PageObject.onceProps. ExpiresAt is a Unix-millisecond timestamp, or nil
// for "never expires".
type OnceConfig struct {
	Prop      string `json:"prop"`
	ExpiresAt *int64 `json:"expiresAt"`
}

// Render writes an Inertia response for the given component and props.
// If the request is an Inertia AJAX request (X-Inertia: true), the
// response is JSON; otherwise it is the initial HTML document with the
// PageObject embedded in a <script data-page="app"
// type="application/json"> element followed by an empty <div id="app">
// mount node (Inertia v3 shape).
func (i *Inertia) Render(w http.ResponseWriter, r *http.Request, component string, props Props) {
	info := FromRequest(r)
	currentVer := i.currentVersion(r)

	// Version negotiation: GET + Inertia + mismatch → 409 + Location.
	if info.IsInertia && r.Method == http.MethodGet &&
		info.Version != "" && info.Version != currentVer {
		w.Header().Set("X-Inertia-Location", r.URL.String())
		w.WriteHeader(http.StatusConflict)
		return
	}

	merged := i.mergeAllProps(r, props)
	keep := filterKeys(merged, info.PartialComponent, component, info.PartialData, info.PartialExcept)

	isPartial := info.PartialComponent != "" && info.PartialComponent == component
	resolved, err := i.evaluatePropsFor(r, merged, keep, isPartial)
	if err != nil {
		i.cfg.ErrorHandler(w, r,
			fmt.Errorf("%w: %w", ErrPropEvaluationFailed, err))
		return
	}

	page := PageObject{
		Component:        component,
		Props:            resolved.values,
		URL:              r.URL.RequestURI(),
		Version:          currentVer,
		EncryptHistory:   i.cfg.EncryptHistory,
		ClearHistory:     i.cfg.ClearHistory,
		MergeProps:       resolved.mergeKeys,
		DeepMergeProps:   resolved.deepMergeKeys,
		DeferredProps:    resolved.deferred,
		PrependProps:     resolved.prependKeys,
		MatchPropsOn:     resolved.matchPropsOn,
		SharedProps:      i.sharedKeysSnapshot(),
		RescuedProps:     resolved.rescued,
		PreserveFragment: i.resolvePreserveFragment(r),
	}

	if currentVer != "" {
		w.Header().Set("X-Inertia-Version", currentVer)
	}

	if info.IsInertia {
		i.writeJSON(w, r, page)
		return
	}
	i.writeHTML(w, r, page)
}

// resolvePreserveFragment returns the per-request override if one was set
// via SetPreserveFragment, otherwise the Config default.
func (i *Inertia) resolvePreserveFragment(r *http.Request) bool {
	if h, ok := r.Context().Value(ctxKeyPreserveFragment).(*preserveFragmentHolder); ok {
		h.mu.Lock()
		v := h.val
		h.mu.Unlock()
		if v != nil {
			return *v
		}
	}
	return i.cfg.PreserveFragment
}

// sharedKeysSnapshot returns the sorted, deduplicated keys of values
// registered via Share / ShareValue. errors/flash injected by
// mergeAllProps are intentionally excluded: v3 reserves the
// shared-props notion for explicitly registered keys.
func (i *Inertia) sharedKeysSnapshot() []string {
	i.sharedMu.RLock()
	defer i.sharedMu.RUnlock()

	seen := make(map[string]bool, len(i.sharedStatic)+len(i.sharedFuncs))
	for k := range i.sharedStatic {
		seen[k] = true
	}
	for k := range i.sharedFuncs {
		seen[k] = true
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (i *Inertia) mergeAllProps(r *http.Request, user Props) Props {
	out := Props{}

	func() {
		i.sharedMu.RLock()
		defer i.sharedMu.RUnlock()
		for k, v := range i.sharedStatic {
			out[k] = v
		}
		for k, fn := range i.sharedFuncs {
			out[k] = fn(r)
		}
	}()

	// session errors / messages
	if errs, _ := r.Context().Value(ctxKeySessionErrors).(map[string]string); len(errs) > 0 {
		out[i.cfg.ErrorsPropKey] = errs
	} else {
		out[i.cfg.ErrorsPropKey] = map[string]string{} // always present per protocol
	}
	if msgs, _ := r.Context().Value(ctxKeySessionFlash).(map[string]any); len(msgs) > 0 {
		out[i.cfg.FlashPropKey] = msgs
	}

	for k, v := range user {
		out[k] = v
	}
	return out
}

// resolvedProps bundles the evaluated prop values with the per-marker key
// lists that populate the PageObject. Returned by evaluatePropsFor.
type resolvedProps struct {
	values        map[string]any
	mergeKeys     []string
	deepMergeKeys []string
	prependKeys   []string
	matchPropsOn  []string
	deferred      map[string][]string
	rescued       []string
}

// propMarkers accumulates the per-wrapper key lists that populate the
// PageObject (mergeProps, deepMergeProps, prependProps, matchPropsOn).
// Collected synchronously in the keep-loop before evaluation fans out.
type propMarkers struct {
	mergeKeys   []string
	deepKeys    []string
	prependKeys []string
	matchOn     []string
}

// collect classifies a single wrapped prop, appending its key to the
// relevant marker lists. Returns whether the prop should be rescued on
// evaluation error.
func (m *propMarkers) collect(key string, wrap propWrapper) (rescue bool) {
	if wrap.isMerge() {
		m.mergeKeys = append(m.mergeKeys, key)
	}
	if wrap.isDeepMerge() {
		m.deepKeys = append(m.deepKeys, key)
	}
	if wrap.isPrepend() {
		m.prependKeys = append(m.prependKeys, key)
	}
	for _, mk := range wrap.matchOnKeys() {
		m.matchOn = append(m.matchOn, key+"."+mk)
	}
	return wrap.rescueOnError()
}

// evaluatePropsFor evaluates the subset of props identified by keep and
// returns the final value map alongside the per-marker key lists that
// populate the PageObject.
//
// Deferred metadata is collected from the entire merged-props map on
// non-partial responses (so the v3 client sees deferredProps on initial
// HTML) but is left empty on partial responses (the client uses metadata
// from the initial response, not subsequent partials).
func (i *Inertia) evaluatePropsFor(r *http.Request, all Props, keep []string, isPartial bool) (resolvedProps, error) {
	_ = r // reserved for future Defer-with-context evaluation.

	out := make(map[string]any, len(keep))
	var (
		mu       sync.Mutex
		firstErr error
		rescued  []string
		wg       sync.WaitGroup
	)

	deferredMap := map[string][]string{}
	if !isPartial {
		for k, v := range all {
			if w, ok := asWrapper(v); ok {
				if g := w.deferGroup(); g != "" {
					deferredMap[g] = append(deferredMap[g], k)
				}
			}
		}
		for g := range deferredMap {
			sort.Strings(deferredMap[g])
		}
	}

	markers := &propMarkers{}
	for _, k := range keep {
		v, ok := all[k]
		if !ok {
			continue
		}
		rescue := false
		if wrap, isWrap := asWrapper(v); isWrap {
			rescue = markers.collect(k, wrap)
		}
		wg.Add(1)
		go func(key string, raw any, rescuable bool) {
			defer wg.Done()
			val, err := evaluateOne(raw)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if rescuable {
					rescued = append(rescued, key)
					return
				}
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[key] = val
		}(k, v, rescue)
	}
	wg.Wait()
	if firstErr != nil {
		return resolvedProps{}, firstErr
	}
	sort.Strings(markers.mergeKeys)
	sort.Strings(markers.deepKeys)
	sort.Strings(markers.prependKeys)
	sort.Strings(markers.matchOn)
	sort.Strings(rescued)
	if len(deferredMap) == 0 {
		deferredMap = nil
	}
	return resolvedProps{
		values:        out,
		mergeKeys:     markers.mergeKeys,
		deepMergeKeys: markers.deepKeys,
		prependKeys:   markers.prependKeys,
		matchPropsOn:  markers.matchOn,
		deferred:      deferredMap,
		rescued:       rescued,
	}, nil
}

func evaluateOne(v any) (any, error) {
	if w, ok := asWrapper(v); ok {
		return w.evaluate()
	}
	return v, nil
}

func (i *Inertia) writeJSON(w http.ResponseWriter, r *http.Request, page PageObject) {
	body, err := json.Marshal(page)
	if err != nil {
		i.cfg.ErrorHandler(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Inertia", "true")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

var (
	closeScriptToken   = []byte("</script")
	closeScriptEscaped = []byte(`\u003c/script`)
)

// renderScriptSafe returns the v3 initial-HTML body: a <script
// data-page="app" type="application/json"> element carrying the
// PageObject JSON, followed by the empty <div id="app"> mount node.
// Any literal </script byte sequence in the payload is rewritten to the
// JSON-legal unicode escape </script so a hostile prop value cannot
// terminate the script block early. (json.Marshal already escapes < to
// <, so this is defense-in-depth for callers that pass raw bytes.)
func renderScriptSafe(body []byte) string {
	safe := bytes.ReplaceAll(body, closeScriptToken, closeScriptEscaped)
	return `<script data-page="app" type="application/json">` + string(safe) +
		`</script>` + `<div id="app"></div>`
}

func (i *Inertia) writeHTML(w http.ResponseWriter, r *http.Request, page PageObject) {
	body, err := json.Marshal(page)
	if err != nil {
		i.cfg.ErrorHandler(w, r, err)
		return
	}
	data := RootData{
		InertiaHead: "",
		InertiaBody: template.HTML(renderScriptSafe(body)),
		Component:   page.Component,
		URL:         page.URL,
		Version:     page.Version,
		PageJSON:    template.JS(body),
	}
	if i.cfg.SSR != nil {
		head, ssrBody, err := i.cfg.SSR.Render(r.Context(), json.RawMessage(body))
		switch {
		case err == nil:
			data.InertiaHead = template.HTML(strings.Join(head, "\n"))
			data.InertiaBody = template.HTML(ssrBody)
		case i.cfg.SSRRequired:
			i.cfg.ErrorHandler(w, r, fmt.Errorf("%w: %v", ErrSSRUnavailable, err))
			return
		default:
			i.logger.Warn("inertia: ssr render failed; falling back to CSR",
				"err", err, "url", r.URL.Path)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := i.renderRoot(w, data); err != nil {
		i.cfg.ErrorHandler(w, r, err)
	}
}
