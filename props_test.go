package inertia

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestAlways_AlwaysIncluded(t *testing.T) {
	p := Always("hello")
	got, err := p.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %v", got)
	}
	if !p.alwaysInclude() {
		t.Error("Always should report alwaysInclude")
	}
}

func TestOptional_NotEvaluatedUntilRequested(t *testing.T) {
	calls := 0
	p := Optional(func() (any, error) {
		calls++
		return "v", nil
	})
	if p.evaluateEager() {
		t.Error("Optional must not evaluate eagerly")
	}
	if calls != 0 {
		t.Error("Optional callback ran during construction")
	}
	got, err := p.evaluate()
	if err != nil || got != "v" {
		t.Errorf("evaluate: %v %v", got, err)
	}
}

func TestDefer_HasGroupAndDoesNotEvaluateEager(t *testing.T) {
	p := Defer(func() (any, error) { return 1, nil }, "groupA")
	if p.evaluateEager() {
		t.Error("Defer must not evaluate eagerly")
	}
	if p.deferGroup() != "groupA" {
		t.Errorf("group: %s", p.deferGroup())
	}
}

func TestDefer_DefaultGroup(t *testing.T) {
	p := Defer(func() (any, error) { return 1, nil })
	if g := p.deferGroup(); g != "default" {
		t.Errorf("group: %q", g)
	}
}

func TestMerge_EvaluatesEagerAndMarksMerge(t *testing.T) {
	p := Merge([]int{1, 2, 3})
	if !p.evaluateEager() {
		t.Error("Merge should evaluate eagerly")
	}
	if !p.isMerge() {
		t.Error("Merge should report isMerge")
	}
	got, _ := p.evaluate()
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("got %v", got)
	}
}

func TestDeepMerge_MarksDeepMerge(t *testing.T) {
	p := DeepMerge(map[string]int{"a": 1})
	if !p.isDeepMerge() {
		t.Error("DeepMerge should report isDeepMerge")
	}
}

func TestEvaluate_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	p := Optional(func() (any, error) { return nil, want })
	_, err := p.evaluate()
	if !errors.Is(err, want) {
		t.Errorf("got %v", err)
	}
}

func TestDefer_EmptyStringGroupDefaults(t *testing.T) {
	p := Defer(func() (any, error) { return 1, nil }, "")
	if g := p.deferGroup(); g != "default" {
		t.Errorf("empty string group should default, got %q", g)
	}
}

func TestDefer_TooManyGroupsPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for multiple group labels")
		}
	}()
	_ = Defer(func() (any, error) { return 1, nil }, "a", "b")
}

func TestDefer_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	p := Defer(func() (any, error) { return nil, want })
	_, err := p.evaluate()
	if !errors.Is(err, want) {
		t.Errorf("got %v", err)
	}
}

func TestPrepend_EvaluatesAndMarks(t *testing.T) {
	p := Prepend([]int{1, 2})
	got, err := p.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("evaluate: %v", got)
	}
	if !p.evaluateEager() {
		t.Error("Prepend must be eager")
	}
	if p.alwaysInclude() {
		t.Error("Prepend must not alwaysInclude")
	}
	if !p.isPrepend() {
		t.Error("Prepend must report isPrepend")
	}
	if p.isMerge() || p.isDeepMerge() {
		t.Error("Prepend must not report merge/deepMerge")
	}
	if p.matchOnKeys() != nil {
		t.Error("Prepend must not return matchOnKeys")
	}
	if p.deferGroup() != "" {
		t.Error("Prepend must not have a defer group")
	}
}

func TestMatchOn_EvaluatesAndExposesKeys(t *testing.T) {
	m := MatchOn([]int{1}, "id", "slug")
	got, err := m.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("evaluate: %v", got)
	}
	if !m.evaluateEager() {
		t.Error("MatchOn must be eager")
	}
	if m.alwaysInclude() {
		t.Error("MatchOn must not alwaysInclude")
	}
	if !reflect.DeepEqual(m.matchOnKeys(), []string{"id", "slug"}) {
		t.Errorf("matchOnKeys: %v", m.matchOnKeys())
	}
}

func TestMatchOn_PanicsWithNoKeys(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MatchOn(...) with no keys must panic")
		}
	}()
	_ = MatchOn("x")
}

func TestMatchOn_CopiesCallerKeys(t *testing.T) {
	keys := []string{"id"}
	m := MatchOn([]int{1}, keys...)
	keys[0] = "MUTATED"
	if m.matchOnKeys()[0] != "id" {
		t.Errorf("MatchOn must copy caller's keys; got %v", m.matchOnKeys())
	}
}

func TestDefer_Rescue_MarksWrapper(t *testing.T) {
	d := Defer(func() (any, error) { return 1, nil }).Rescue()
	if !d.rescueOnError() {
		t.Error("Rescue() must set rescueOnError")
	}
	if d.deferGroup() != "default" {
		t.Errorf("Rescue() must preserve group: %q", d.deferGroup())
	}
	g := Defer(func() (any, error) { return 1, nil }, "feed").Rescue()
	if g.deferGroup() != "feed" {
		t.Errorf("group lost: %q", g.deferGroup())
	}
	if Defer(func() (any, error) { return 1, nil }).rescueOnError() {
		t.Error("plain Defer must not rescue")
	}
}

func TestPrepend_AppearsInPageObject(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Feed", Props{"items": Prepend([]int{1, 2})})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page.PrependProps, []string{"items"}) {
		t.Errorf("prependProps: %v", page.PrependProps)
	}
}

func TestOnce_WrapperBehavior(t *testing.T) {
	o := Once(func() (any, error) { return []int{1}, nil })
	v, err := o.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, []int{1}) {
		t.Errorf("evaluate: %v", v)
	}
	if !o.isOnce() {
		t.Error("Once must report isOnce")
	}
	if !o.evaluateEager() {
		t.Error("Once must be eager (sent on first load)")
	}
	if o.onceTTL() != 0 {
		t.Errorf("default TTL must be 0; got %v", o.onceTTL())
	}
	withTTL := Once(func() (any, error) { return 1, nil }).ExpiresIn(time.Hour)
	if withTTL.onceTTL() != time.Hour {
		t.Errorf("ExpiresIn TTL: %v", withTTL.onceTTL())
	}
}

func TestMatchOn_AppearsInPageObject(t *testing.T) {
	i := newTestInertia(t)
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i.Render(w, r, "Feed", Props{
			"feed": MatchOn([]map[string]any{{"id": 1}}, "id", "slug"),
		})
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var page PageObject
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	want := []string{"feed.id", "feed.slug"}
	if !reflect.DeepEqual(page.MatchPropsOn, want) {
		t.Errorf("matchPropsOn: got %v, want %v", page.MatchPropsOn, want)
	}
}

func TestScroll_WrapperBehavior(t *testing.T) {
	next := 2
	s := Scroll([]int{1, 2, 3}, ScrollConfig{CurrentPage: 1, NextPage: &next})
	cfg := s.scrollConfig()
	if cfg == nil {
		t.Fatal("scrollConfig must be non-nil")
	}
	if cfg.PageName != "page" {
		t.Errorf("empty PageName must default to \"page\"; got %q", cfg.PageName)
	}
	if cfg.CurrentPage != 1 || cfg.NextPage == nil || *cfg.NextPage != 2 {
		t.Errorf("config: %+v", cfg)
	}
	if !s.evaluateEager() {
		t.Error("Scroll must be eager")
	}
	v, err := s.evaluate()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, []int{1, 2, 3}) {
		t.Errorf("evaluate must return raw data: %v", v)
	}
}
