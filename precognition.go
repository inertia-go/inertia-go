package inertia

import (
	"encoding/json"
	"net/http"
)

// Precognition handles an Inertia/Laravel precognitive validation request.
//
// On a precognitive request (Precognition: true), it inspects the
// validation errors collected via ValidationErrors(r) for the request's
// error bag, filtered by Precognition-Validate-Only when present, and:
//   - writes 204 No Content with Precognition-Success: true when there are
//     no (filtered) errors;
//   - writes 422 with a JSON body {<ErrorsPropKey>: {field: message}}
//     otherwise.
//
// It returns true once it has written the response, so the handler should
// return immediately. On a non-precognitive request it writes nothing and
// returns false, letting the handler proceed to its real action.
//
// Unlike Laravel's middleware, this does NOT auto-run validation or skip
// the handler body: the handler runs its own validation (via
// ValidationErrors(r).Add) and calls this helper explicitly.
func (i *Inertia) Precognition(w http.ResponseWriter, r *http.Request) bool {
	info := FromRequest(r)
	if !info.IsPrecognition {
		return false
	}

	bag := info.ErrorBag
	if bag == "" {
		bag = i.cfg.DefaultErrorBag
	}

	var errs map[string]string
	if eb, ok := r.Context().Value(ctxKeyErrorBag).(*ErrorBagCollector); ok {
		// ValidationErrors(r).Add writes to the unnamed bag "". Map the
		// logical default bag back to "" (mirroring persistCollectors).
		key := bag
		if bag == i.cfg.DefaultErrorBag {
			key = ""
		}
		errs = eb.snapshot(key)
	}
	if len(info.ValidateOnly) > 0 {
		errs = filterErrors(errs, info.ValidateOnly)
	}

	w.Header().Set("Precognition", "true")
	if len(errs) == 0 {
		w.Header().Set("Precognition-Success", "true")
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]any{i.cfg.ErrorsPropKey: errs})
	return true
}

// HandlePrecognition is a convenience wrapper over Precognition that removes
// the validate-then-check boilerplate. It runs validate (which should record
// field errors via ValidationErrors(r).Add) and then delegates to
// Precognition.
//
// On a precognitive request it returns true after writing the 204/422
// response, so the handler returns. On a non-precognitive request it returns
// false — but validate has still run, so the errors it recorded remain in the
// request error bag for the handler's normal redirect-flash path.
//
//	func submit(w http.ResponseWriter, r *http.Request) {
//	    if i.HandlePrecognition(w, r, func(r *http.Request) {
//	        if name == "" { inertia.ValidationErrors(r).Add("name", "required") }
//	    }) {
//	        return
//	    }
//	    // not precognitive: perform the real write, then redirect/flash
//	}
//
// validate must be side-effect-free beyond recording errors: it runs on every
// request (precognitive or not), so it must not perform the action itself.
func (i *Inertia) HandlePrecognition(w http.ResponseWriter, r *http.Request, validate func(*http.Request)) bool {
	validate(r)
	return i.Precognition(w, r)
}

// filterErrors returns the subset of errs whose keys appear in only.
func filterErrors(errs map[string]string, only []string) map[string]string {
	keep := make(map[string]bool, len(only))
	for _, f := range only {
		keep[f] = true
	}
	out := make(map[string]string, len(errs))
	for field, msg := range errs {
		if keep[field] {
			out[field] = msg
		}
	}
	return out
}
