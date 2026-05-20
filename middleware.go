package inertia

import (
	"context"
	"net/http"
	"strings"
)

// RequestInfo captures the Inertia-specific request state.
// Retrieve it inside a handler via FromRequest(r).
type RequestInfo struct {
	IsInertia        bool
	Version          string
	PartialData      []string
	PartialComponent string
	PartialExcept    []string
	Reset            []string
	ErrorBag         string
}

type ctxKey int

const (
	ctxKeyRequestInfo ctxKey = iota
	ctxKeyErrorBag
	ctxKeyFlashBag
	ctxKeySessionErrors
	ctxKeySessionFlash
)

// Middleware returns an http.Handler that wraps next, populating Inertia
// request context (RequestInfo, errors/flash collectors, session-loaded
// errors and messages) and setting Vary: X-Inertia on the response.
func (i *Inertia) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer i.flushSession(w)
		info := parseRequestInfo(r)

		// Pull errors and messages from the session (read-and-clear).
		// Errors are fetched for the request's error bag (or "default").
		bag := info.ErrorBag
		if bag == "" {
			bag = i.cfg.DefaultErrorBag
		}
		sessErrors, err := i.cfg.Session.TakeErrors(w, r, bag)
		if err != nil {
			i.logger.WarnContext(r.Context(), "inertia: session.TakeErrors failed",
				"err", err)
		}
		sessFlash, err := i.cfg.Session.TakeMessages(w, r)
		if err != nil {
			i.logger.WarnContext(r.Context(), "inertia: session.TakeMessages failed",
				"err", err)
		}

		// Handler-local collectors (filled by ValidationErrors/Flash helpers).
		errBag := newErrorBag()
		flBag := newFlashBag()

		ctx := r.Context()
		ctx = context.WithValue(ctx, ctxKeyRequestInfo, info)
		ctx = context.WithValue(ctx, ctxKeyErrorBag, errBag)
		ctx = context.WithValue(ctx, ctxKeyFlashBag, flBag)
		ctx = context.WithValue(ctx, ctxKeySessionErrors, sessErrors)
		ctx = context.WithValue(ctx, ctxKeySessionFlash, sessFlash)

		w.Header().Add("Vary", "X-Inertia")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseRequestInfo(r *http.Request) RequestInfo {
	return RequestInfo{
		IsInertia:        r.Header.Get("X-Inertia") == "true",
		Version:          r.Header.Get("X-Inertia-Version"),
		PartialData:      splitCSV(r.Header.Get("X-Inertia-Partial-Data")),
		PartialComponent: r.Header.Get("X-Inertia-Partial-Component"),
		PartialExcept:    splitCSV(r.Header.Get("X-Inertia-Partial-Except")),
		Reset:            splitCSV(r.Header.Get("X-Inertia-Reset")),
		ErrorBag:         r.Header.Get("X-Inertia-Error-Bag"),
	}
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	// Reuse parts' backing array; safe because strings.Split allocates a fresh slice here.
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// FromRequest returns the RequestInfo previously installed by Middleware.
// Zero value if not present.
func FromRequest(r *http.Request) RequestInfo {
	info, _ := r.Context().Value(ctxKeyRequestInfo).(RequestInfo)
	return info
}
