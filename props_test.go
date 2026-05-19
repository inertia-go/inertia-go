package inertia

import (
	"errors"
	"reflect"
	"testing"
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
