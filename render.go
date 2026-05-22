package inertia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
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
//
// Reset is a server-computed output flag: it is set to true by the renderer
// when the prop's key appears in the X-Inertia-Reset header. Values supplied
// by callers are overwritten during rendering.
type ScrollConfig struct {
	PageName     string `json:"pageName"`
	PreviousPage *int   `json:"previousPage"`
	NextPage     *int   `json:"nextPage"`
	CurrentPage  int    `json:"currentPage"`
	Reset        bool   `json:"reset"`
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
	isPartial := info.PartialComponent != "" && info.PartialComponent == component
	pr := &propsResolver{
		isPartial:     isPartial,
		except:        info.PartialExcept,
		exceptOnce:    setOf(info.ExceptOnceProps),
		scrollPrepend: info.ScrollMergeIntent == "prepend",
		reset:         setOf(info.Reset),
		markers:       newMarkers(),
		deferred:      map[string][]string{},
	}
	if isPartial && len(info.PartialData) > 0 {
		pr.only = info.PartialData
	}
	values, err := pr.resolve(merged)
	if err != nil {
		i.cfg.ErrorHandler(w, r,
			fmt.Errorf("%w: %w", ErrPropEvaluationFailed, err))
		return
	}
	sort.Strings(pr.markers.mergeKeys)
	sort.Strings(pr.markers.deepKeys)
	sort.Strings(pr.markers.prependKeys)
	sort.Strings(pr.markers.matchOn)
	sort.Strings(pr.rescued)
	for g := range pr.deferred {
		sort.Strings(pr.deferred[g])
	}
	deferredMap := pr.deferred
	if len(deferredMap) == 0 {
		deferredMap = nil
	}
	onceProps := pr.markers.onceProps
	if len(onceProps) == 0 {
		onceProps = nil
	}
	scrollProps := pr.markers.scrollProps
	if len(scrollProps) == 0 {
		scrollProps = nil
	}

	page := PageObject{
		Component:        component,
		Props:            values,
		URL:              r.URL.RequestURI(),
		Version:          currentVer,
		EncryptHistory:   i.cfg.EncryptHistory,
		ClearHistory:     i.cfg.ClearHistory,
		MergeProps:       pr.markers.mergeKeys,
		DeepMergeProps:   pr.markers.deepKeys,
		DeferredProps:    deferredMap,
		PrependProps:     pr.markers.prependKeys,
		MatchPropsOn:     pr.markers.matchOn,
		SharedProps:      i.sharedKeysSnapshot(),
		ScrollProps:      scrollProps,
		OnceProps:        onceProps,
		RescuedProps:     pr.rescued,
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

// propMarkers accumulates the per-prop key lists that populate the
// PageObject (mergeProps, deepMergeProps, prependProps, matchPropsOn,
// onceProps, scrollProps). Collected during recursive resolution by
// collectMetadata.
type propMarkers struct {
	mergeKeys   []string
	deepKeys    []string
	prependKeys []string
	matchOn     []string
	onceProps   map[string]OnceConfig
	scrollProps map[string]ScrollConfig
	// scrollPrepend is set when the client's
	// X-Inertia-Infinite-Scroll-Merge-Intent header is "prepend", switching
	// Scroll props from appending (mergeProps) to prepending (prependProps).
	scrollPrepend bool
	// reset is the set of prop keys from X-Inertia-Reset. A key in this set is
	// suppressed from merge/prepend/deepMerge metadata, and its scrollProps
	// entry gets reset=true.
	reset map[string]bool
}

func joinPath(key, sub string) string {
	if key == "" {
		return sub
	}
	if sub == "" {
		return key
	}
	return key + "." + sub
}

func evaluateOne(v any) (any, error) {
	if b, ok := asBuilder(v); ok {
		return b.resolve()
	}
	if sp, ok := v.(*scrollProp); ok {
		return sp.dataFn(), nil
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
