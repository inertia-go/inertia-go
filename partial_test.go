package inertia

import (
	"reflect"
	"sort"
	"testing"
)

func TestFilterKeys_FullResponse_KeepsEagerEvaluatedOnly(t *testing.T) {
	props := Props{
		"users": []int{1, 2},                                     // bare, eager
		"stats": Optional(func() (any, error) { return 1, nil }), // lazy
		"perms": Defer(func() (any, error) { return 1, nil }),    // deferred
		"auth":  Always("u"),                                     // always
		"tags":  Merge([]int{1}),                                 // eager + merge
	}
	keep := filterKeys(props, "", "", nil, nil)
	sort.Strings(keep)
	want := []string{"auth", "tags", "users"}
	if !reflect.DeepEqual(keep, want) {
		t.Errorf("got %v, want %v", keep, want)
	}
}

func TestFilterKeys_PartialReload_KeepsRequestedAndAlways(t *testing.T) {
	props := Props{
		"users": []int{1, 2},
		"stats": Optional(func() (any, error) { return 1, nil }),
		"perms": Defer(func() (any, error) { return 1, nil }),
		"auth":  Always("u"),
		"tags":  Merge([]int{1}),
	}
	keep := filterKeys(props,
		"Users/Index", "Users/Index",
		[]string{"stats", "perms"}, nil)
	sort.Strings(keep)
	want := []string{"auth", "perms", "stats"}
	if !reflect.DeepEqual(keep, want) {
		t.Errorf("got %v, want %v", keep, want)
	}
}

func TestFilterKeys_PartialReload_Except(t *testing.T) {
	props := Props{
		"users": []int{1, 2},
		"stats": []int{3},
		"auth":  Always("u"),
	}
	keep := filterKeys(props,
		"Users/Index", "Users/Index",
		[]string{"users", "stats"}, []string{"stats"})
	sort.Strings(keep)
	want := []string{"auth", "users"}
	if !reflect.DeepEqual(keep, want) {
		t.Errorf("got %v, want %v", keep, want)
	}
}

func TestFilterKeys_PartialReload_ComponentMismatch_FallsBackToFull(t *testing.T) {
	props := Props{
		"a": "x",
		"b": Optional(func() (any, error) { return 1, nil }),
	}
	keep := filterKeys(props,
		"Other/Page", "Users/Index", []string{"b"}, nil)
	sort.Strings(keep)
	if !reflect.DeepEqual(keep, []string{"a"}) {
		t.Errorf("got %v", keep)
	}
}
