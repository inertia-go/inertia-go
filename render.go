package inertia

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
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
}

// Render writes an Inertia response for the given component and props.
// If the request is an Inertia AJAX request (X-Inertia: true), the
// response is JSON; otherwise it is the initial HTML document with the
// PageObject embedded in <div id="app" data-page="...">.
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

	evaluated, mergeKeys, deepMergeKeys, deferred, err := i.evaluatePropsFor(r, merged, keep)
	if err != nil {
		i.cfg.ErrorHandler(w, r,
			fmt.Errorf("%w: %w", ErrPropEvaluationFailed, err))
		return
	}

	page := PageObject{
		Component:      component,
		Props:          evaluated,
		URL:            r.URL.RequestURI(),
		Version:        currentVer,
		EncryptHistory: i.cfg.EncryptHistory,
		ClearHistory:   i.cfg.ClearHistory,
		MergeProps:     mergeKeys,
		DeepMergeProps: deepMergeKeys,
		DeferredProps:  deferred,
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

// evaluatePropsFor evaluates the subset of props identified by keep,
// returning the final value map plus the merge/deepMerge/deferred markers
// that go into the PageObject.
func (i *Inertia) evaluatePropsFor(r *http.Request, all Props, keep []string) (
	map[string]any, []string, []string, map[string][]string, error,
) {
	out := make(map[string]any, len(keep))
	var (
		mu          sync.Mutex
		firstErr    error
		mergeKeys   []string
		deepKeys    []string
		deferredMap = map[string][]string{}
		wg          sync.WaitGroup
	)
	_ = r // r reserved for future Defer-with-context evaluation.

	for _, k := range keep {
		v, ok := all[k]
		if !ok {
			continue
		}
		wrap, isWrap := asWrapper(v)
		if isWrap {
			if wrap.isMerge() {
				mergeKeys = append(mergeKeys, k)
			}
			if wrap.isDeepMerge() {
				deepKeys = append(deepKeys, k)
			}
			if g := wrap.deferGroup(); g != "" {
				deferredMap[g] = append(deferredMap[g], k)
			}
		}
		wg.Add(1)
		go func(key string, raw any) {
			defer wg.Done()
			val, err := evaluateOne(raw)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
				return
			}
			out[key] = val
		}(k, v)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, nil, nil, nil, firstErr
	}
	if len(deferredMap) == 0 {
		deferredMap = nil
	}
	return out, mergeKeys, deepKeys, deferredMap, nil
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

func (i *Inertia) writeHTML(w http.ResponseWriter, r *http.Request, page PageObject) {
	body, err := json.Marshal(page)
	if err != nil {
		i.cfg.ErrorHandler(w, r, err)
		return
	}
	data := RootData{
		InertiaHead: "",
		InertiaBody: template.HTML(fmt.Sprintf(`<div id="app" data-page='%s'></div>`, string(body))),
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
