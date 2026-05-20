package inertia

import (
	"testing"
	"time"
)

func TestBuilder_BaseConstructors(t *testing.T) {
	if Always(1).kind != kindEager || !Always(1).always {
		t.Error("Always: eager + always")
	}
	if Optional(func() (any, error) { return 1, nil }).kind != kindOptional {
		t.Error("Optional: kindOptional")
	}
	d := Defer(func() (any, error) { return 1, nil }, "feed")
	if d.kind != kindDeferred || d.defGrp != "feed" {
		t.Errorf("Defer: %+v", d)
	}
	if !Merge(1).merge || !DeepMerge(1).deepMerge {
		t.Error("Merge/DeepMerge flags")
	}
	if !Once(func() (any, error) { return 1, nil }).once {
		t.Error("Once flag")
	}
}

func TestBuilder_MergeChaining(t *testing.T) {
	b := Merge([]int{1}).Prepend("messages").MatchOn(map[string]string{"messages": "id"})
	if len(b.prependPath) != 1 || b.prependPath[0] != "messages" {
		t.Errorf("prependPath: %v", b.prependPath)
	}
	if b.matchOn["messages"] != "id" {
		t.Errorf("matchOn: %v", b.matchOn)
	}
	if Merge([]int{1}).Prepend().prependPath[0] != "" {
		t.Error("root prepend must store empty path")
	}
}

func TestBuilder_DeferDeepMergeComposition(t *testing.T) {
	b := Defer(func() (any, error) { return 1, nil }, "feed").DeepMerge()
	if b.kind != kindDeferred || !b.deepMerge || b.defGrp != "feed" {
		t.Errorf("defer+deepMerge: %+v", b)
	}
}

func TestBuilder_MergeOnce(t *testing.T) {
	b := Merge(func() (any, error) { return 1, nil }).Once()
	if !b.merge || !b.once || b.fn == nil {
		t.Errorf("merge+once with fn: %+v", b)
	}
}

func TestBuilder_OnceAdvanced(t *testing.T) {
	b := Once(func() (any, error) { return 1, nil }).ExpiresIn(time.Hour).As("plans").Fresh()
	if b.onceTTL != time.Hour || b.onceKey != "plans" || !b.onceFresh {
		t.Errorf("once advanced: %+v", b)
	}
}

func TestBuilder_DeferRescue(t *testing.T) {
	b := Defer(func() (any, error) { return 1, nil }).Rescue()
	if !b.rescue {
		t.Error("Rescue must set rescue")
	}
}

func TestBuilder_ConflictPanics(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"Prepend on Always", func() { Always(1).Prepend() }},
		{"MatchOn on Optional", func() { Optional(func() (any, error) { return 1, nil }).MatchOn(map[string]string{"a": "b"}) }},
		{"As without Once", func() { Merge(1).As("x") }},
		{"Fresh without Once", func() { Merge(1).Fresh() }},
		{"Rescue without Defer", func() { Merge(1).Rescue() }},
		{"empty MatchOn", func() { Merge(1).MatchOn(map[string]string{}) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%s must panic", c.name)
				}
			}()
			c.fn()
		})
	}
}

func TestScroll_Unchanged(t *testing.T) {
	next := 2
	s := Scroll([]int{1, 2, 3}, ScrollConfig{CurrentPage: 1, NextPage: &next})
	if scrollConfigOf(s) == nil || scrollConfigOf(s).PageName != "page" {
		t.Error("Scroll config / default page name")
	}
}
