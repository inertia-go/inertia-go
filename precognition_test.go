package inertia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorBag_Snapshot(t *testing.T) {
	eb := newErrorBag()
	eb.Add("name", "required")
	eb.Bag("signup").Add("email", "invalid") // different bag, must not leak

	got := eb.snapshot("") // default bag (empty name)
	if len(got) != 1 || got["name"] != "required" {
		t.Errorf("snapshot(default) = %v, want {name:required}", got)
	}

	// Mutating the snapshot must not affect the collector.
	got["name"] = "tampered"
	again := eb.snapshot("")
	if again["name"] != "required" {
		t.Errorf("snapshot must return a copy; collector mutated to %v", again)
	}

	sign := eb.snapshot("signup")
	if len(sign) != 1 || sign["email"] != "invalid" {
		t.Errorf("snapshot(signup) = %v, want {email:invalid}", sign)
	}
}

func TestPrecognition_SuccessNoErrors(t *testing.T) {
	i := newTestInertia(t)
	var handled bool
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled = i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !handled {
		t.Error("Precognition must return true on a precognitive request")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Precognition") != "true" {
		t.Errorf("missing Precognition: true header")
	}
	if rec.Header().Get("Precognition-Success") != "true" {
		t.Errorf("missing Precognition-Success: true header")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 body must be empty, got %q", rec.Body.String())
	}
}

func TestPrecognition_FailureWithErrors(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("name", "required")
		i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if rec.Header().Get("Precognition") != "true" {
		t.Errorf("missing Precognition: true header")
	}
	if rec.Header().Get("Precognition-Success") == "true" {
		t.Errorf("Precognition-Success must NOT be set on failure")
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	if body["errors"]["name"] != "required" {
		t.Errorf("body = %v, want errors.name=required", body)
	}
}

func TestPrecognition_ValidateOnlyFilter(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("name", "required")
		ValidationErrors(r).Add("email", "invalid")
		i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	req.Header.Set("Precognition-Validate-Only", "email")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body map[string]map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["errors"]["name"]; ok {
		t.Errorf("name must be filtered out by validate-only: %v", body)
	}
	if body["errors"]["email"] != "invalid" {
		t.Errorf("email must be reported: %v", body)
	}
}

func TestPrecognition_ValidateOnlyAllPass(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("name", "required")
		i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	req.Header.Set("Precognition-Validate-Only", "phone")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (filtered set has no errors)", rec.Code)
	}
	if rec.Header().Get("Precognition-Success") != "true" {
		t.Error("missing Precognition-Success on filtered-clean result")
	}
}

func TestPrecognition_NonPrecognitionPassesThrough(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if i.Precognition(w, r) {
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418 — Precognition must not write on non-precog", rec.Code)
	}
}

func TestPrecognition_NamedErrorBag(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Bag("signup").Add("email", "invalid")
		i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	req.Header.Set("X-Inertia-Error-Bag", "signup")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (errors in the named bag)", rec.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	if body["errors"]["email"] != "invalid" {
		t.Errorf("named-bag error not read back: %v", body)
	}
}

func TestPrecognition_CustomErrorsPropKey(t *testing.T) {
	i, err := New(Config{Session: stubSession{}, ErrorsPropKey: "validationErrors"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ValidationErrors(r).Add("name", "required")
		i.Precognition(w, r)
	}))
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set("Precognition", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rec.Body.String())
	}
	if _, ok := body["errors"]; ok {
		t.Errorf("422 body must use the custom ErrorsPropKey, not \"errors\": %v", body)
	}
	if body["validationErrors"]["name"] != "required" {
		t.Errorf("422 body = %v, want validationErrors.name=required", body)
	}
}
