package inertia

import (
	"net/http"
	"strings"
)

// Redirect issues a redirect appropriate for both Inertia and non-Inertia
// requests. Unsafe methods (PUT/PATCH/DELETE) are promoted to 303 per
// v3 protocol. Fragment URLs return 409 + X-Inertia-Redirect for Inertia
// requests.
func (i *Inertia) Redirect(w http.ResponseWriter, r *http.Request, url string) {
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
// it degrades to a standard http.Redirect (302).
func (i *Inertia) Location(w http.ResponseWriter, r *http.Request, url string) {
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
