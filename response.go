package inertia

import (
	"net/http"
	"strings"
)

// Redirect issues a redirect appropriate for both Inertia and non-Inertia
// requests. Unsafe methods (PUT/PATCH/DELETE) are promoted to 303 per
// v3 protocol. Fragment URLs return 409 + X-Inertia-Redirect for Inertia
// requests. Any errors or flash messages accumulated in the request's
// collectors are persisted to the session before the response is written.
func (i *Inertia) Redirect(w http.ResponseWriter, r *http.Request, url string) {
	i.persistCollectors(w, r)
	if strings.Contains(url, "#") && FromRequest(r).IsInertia {
		w.Header().Set("X-Inertia-Redirect", url)
		w.WriteHeader(http.StatusConflict)
		return
	}
	status := http.StatusFound
	switch r.Method {
	case http.MethodPut, http.MethodPatch, http.MethodDelete:
		status = http.StatusSeeOther
	}
	http.Redirect(w, r, url, status)
}

// Location triggers a full browser navigation (e.g. external redirect).
// For Inertia requests this returns 409 + X-Inertia-Location; otherwise
// it degrades to a standard http.Redirect (302). As with Redirect, any
// collected errors or flash messages are persisted to the session.
func (i *Inertia) Location(w http.ResponseWriter, r *http.Request, url string) {
	i.persistCollectors(w, r)
	if FromRequest(r).IsInertia {
		w.Header().Set("X-Inertia-Location", url)
		w.WriteHeader(http.StatusConflict)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// Back redirects to the request's Referer header (or "/" if absent).
func (i *Inertia) Back(w http.ResponseWriter, r *http.Request) {
	url := r.Referer()
	if url == "" {
		url = "/"
	}
	i.Redirect(w, r, url)
}

// persistCollectors transfers any errors and flash messages accumulated
// in the request-scoped collectors (populated via ValidationErrors and
// Flash helpers) to the configured Session store. It is a no-op when the
// collectors are absent (i.e. the request did not go through Middleware)
// or empty (no writes were made).
func (i *Inertia) persistCollectors(w http.ResponseWriter, r *http.Request) {
	if eb, ok := r.Context().Value(ctxKeyErrorBag).(*ErrorBagCollector); ok && eb.dirty {
		eb.mu.Lock()
		for bag, errs := range eb.entries {
			name := bag
			if name == "" {
				name = i.cfg.DefaultErrorBag
			}
			if err := i.cfg.Session.FlashErrors(w, r, name, errs); err != nil {
				i.logger.WarnContext(r.Context(), "inertia: FlashErrors failed", "err", err)
			}
		}
		eb.mu.Unlock()
	}
	if fb, ok := r.Context().Value(ctxKeyFlashBag).(*FlashCollector); ok && fb.dirty {
		fb.mu.Lock()
		for k, v := range fb.entries {
			if err := i.cfg.Session.FlashMessage(w, r, k, v); err != nil {
				i.logger.WarnContext(r.Context(), "inertia: FlashMessage failed", "err", err)
			}
		}
		fb.mu.Unlock()
	}
}
