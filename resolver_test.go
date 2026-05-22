package inertia

import "testing"

func TestMatchers(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]string, string) bool
		set  []string
		path string
		want bool
	}{
		{"matchesOnly exact", matchesPath, []string{"auth"}, "auth", true},
		{"matchesOnly descend", matchesPath, []string{"auth"}, "auth.user", true},
		{"matchesOnly no match", matchesPath, []string{"auth"}, "posts", false},
		{"matchesOnly prefix-not-segment", matchesPath, []string{"auth"}, "authority", false},
		{"leadsToOnly ancestor", leadsToPath, []string{"auth.user"}, "auth", true},
		{"leadsToOnly not ancestor", leadsToPath, []string{"auth.user"}, "posts", false},
		{"leadsToOnly equal is not ancestor", leadsToPath, []string{"auth"}, "auth", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.fn(c.set, c.path); got != c.want {
				t.Errorf("%s(%v, %q) = %v, want %v", c.name, c.set, c.path, got, c.want)
			}
		})
	}
}

func TestUnpackDotProps(t *testing.T) {
	props := Props{
		"auth.user":  "alice",
		"auth.token": "xyz",
		"plain":      1,
	}
	unpackDotProps(props)

	if _, ok := props["auth.user"]; ok {
		t.Error("flat dot key auth.user must be removed")
	}
	auth, ok := props["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth must be a nested map, got %T", props["auth"])
	}
	if auth["user"] != "alice" || auth["token"] != "xyz" {
		t.Errorf("auth = %v, want {user:alice, token:xyz}", auth)
	}
	if props["plain"] != 1 {
		t.Errorf("plain key must be untouched: %v", props["plain"])
	}
}

func TestUnpackDotProps_MergesIntoExistingMap(t *testing.T) {
	props := Props{
		"auth":      map[string]any{"id": 7},
		"auth.user": "alice",
	}
	unpackDotProps(props)
	auth := props["auth"].(map[string]any)
	if auth["id"] != 7 || auth["user"] != "alice" {
		t.Errorf("auth = %v, want {id:7, user:alice}", auth)
	}
}

func TestResolve_PlainNestedPreserved(t *testing.T) {
	pr := &propsResolver{markers: newMarkers()}
	out, err := pr.resolve(Props{
		"auth": map[string]any{"user": "alice", "roles": []string{"admin"}},
		"n":    42,
	})
	if err != nil {
		t.Fatal(err)
	}
	auth := out["auth"].(map[string]any)
	if auth["user"] != "alice" {
		t.Errorf("nested leaf preserved: %v", auth)
	}
	if out["n"] != 42 {
		t.Errorf("scalar preserved: %v", out["n"])
	}
}

func TestResolve_NestedOptionalOnPartial(t *testing.T) {
	mk := func(only []string) map[string]any {
		pr := &propsResolver{
			isPartial: true,
			only:      only,
			markers:   newMarkers(),
		}
		out, _ := pr.resolve(Props{
			"auth": map[string]any{
				"user":  Optional(func() (any, error) { return "alice", nil }),
				"token": Optional(func() (any, error) { return "xyz", nil }),
			},
		})
		return out
	}
	out := mk([]string{"auth.user"})
	auth, _ := out["auth"].(map[string]any)
	if auth == nil || auth["user"] != "alice" {
		t.Errorf("auth.user must be included: %v", out)
	}
	if _, ok := auth["token"]; ok {
		t.Errorf("auth.token must be omitted: %v", auth)
	}
}

func TestResolve_OptionalNotRequested_Omitted(t *testing.T) {
	pr := &propsResolver{
		isPartial: true,
		only:      []string{"other"},
		markers:   newMarkers(),
	}
	out, _ := pr.resolve(Props{
		"feed":  Optional(func() (any, error) { return map[string]any{"items": []int{1}}, nil }),
		"other": "x",
	})
	if _, ok := out["feed"]; ok {
		t.Errorf("feed not in only; must be omitted: %v", out)
	}
}

func TestResolve_ParentWasResolvedBypass(t *testing.T) {
	// only=["feed.shown"] makes "feed" reachable solely via leadsToPath (it is
	// an ancestor of the requested leaf), NOT via matchesPath. A closure at
	// "feed" returns a map, so its children are marked parentWasResolved and
	// bypass the partial filter: "feed.extra" is included even though it is
	// neither requested nor a descendant of the selector. This isolates the
	// bypass — without it, "feed.extra" would be filtered (see the static case).
	pr := &propsResolver{
		isPartial: true,
		only:      []string{"feed.shown"},
		markers:   newMarkers(),
	}
	out, err := pr.resolve(Props{
		"feed": Optional(func() (any, error) {
			return map[string]any{"shown": "a", "extra": "b"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	feed, ok := out["feed"].(map[string]any)
	if !ok {
		t.Fatalf("feed must resolve to a nested map: %v", out)
	}
	if feed["shown"] != "a" {
		t.Errorf("requested leaf feed.shown must be included: %v", feed)
	}
	if feed["extra"] != "b" {
		t.Errorf("closure-returned sibling must bypass the partial filter: %v", feed)
	}
}

func TestResolve_StaticNestedMap_ChildrenStayFiltered(t *testing.T) {
	// Same selector as the bypass test, but "feed" is a STATIC map (no closure),
	// so its children are NOT marked parentWasResolved. "feed.shown" is included
	// (matchesPath), but "feed.extra" — neither requested nor a descendant of the
	// selector — stays filtered. The contrast with the closure case is the whole
	// point of parentWasResolved.
	pr := &propsResolver{
		isPartial: true,
		only:      []string{"feed.shown"},
		markers:   newMarkers(),
	}
	out, _ := pr.resolve(Props{
		"feed": map[string]any{
			"shown": "a",
			"extra": Optional(func() (any, error) { return "b", nil }),
		},
	})
	feed, _ := out["feed"].(map[string]any)
	if feed["shown"] != "a" {
		t.Errorf("requested leaf feed.shown must be included: %v", feed)
	}
	if _, ok := feed["extra"]; ok {
		t.Errorf("static-map sibling not requested must stay filtered: %v", feed)
	}
}

func TestResolve_NestedMergeEmitsDottedKey(t *testing.T) {
	pr := &propsResolver{markers: newMarkers()}
	_, err := pr.resolve(Props{
		"auth": map[string]any{"notifications": Merge([]int{1, 2})},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range pr.markers.mergeKeys {
		if k == "auth.notifications" {
			found = true
		}
	}
	if !found {
		t.Errorf("nested Merge must emit dotted mergeProps key: %v", pr.markers.mergeKeys)
	}
}

func hasKey(list []string, want string) bool {
	for _, k := range list {
		if k == want {
			return true
		}
	}
	return false
}

// deferMerge builds a Deferred prop that is also mergeable. The package has no
// chainable .Merge() on a deferred builder, so the field is set directly to
// model the official Defer()->shouldMerge state.
func deferMerge(fn func() (any, error)) *propBuilder {
	b := Defer(fn)
	b.merge = true
	return b
}

func deferOnce(fn func() (any, error)) *propBuilder {
	b := Defer(fn)
	b.once = true
	return b
}

func optionalMerge(fn func() (any, error)) *propBuilder {
	b := Optional(fn)
	b.merge = true
	return b
}

// TestResolve_DeferMergeMetadataOnInitial asserts that a deferred mergeable prop
// excluded from the initial response still emits its merge metadata (and the
// deferred group), matching the official excludeIgnoredProp behavior.
func TestResolve_DeferMergeMetadataOnInitial(t *testing.T) {
	pr := &propsResolver{markers: newMarkers(), deferred: map[string][]string{}}
	out, err := pr.resolve(Props{
		"feed": deferMerge(func() (any, error) { return []int{1}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["feed"]; ok {
		t.Errorf("deferred prop must be excluded from initial output: %v", out)
	}
	if !hasKey(pr.markers.mergeKeys, "feed") {
		t.Errorf("Defer().Merge() must still emit feed in mergeKeys: %v", pr.markers.mergeKeys)
	}
	if !hasKey(pr.deferred["default"], "feed") {
		t.Errorf("Defer().Merge() must still emit feed in deferred group default: %v", pr.deferred)
	}
}

// TestResolve_DeferOnceMetadataOnInitial asserts that a deferred once prop
// excluded from the initial response still emits its once metadata.
func TestResolve_DeferOnceMetadataOnInitial(t *testing.T) {
	pr := &propsResolver{markers: newMarkers(), deferred: map[string][]string{}}
	out, err := pr.resolve(Props{
		"feed": deferOnce(func() (any, error) { return []int{1}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["feed"]; ok {
		t.Errorf("deferred prop must be excluded from initial output: %v", out)
	}
	if got, ok := pr.markers.onceProps["feed"]; !ok || got.Prop != "feed" {
		t.Errorf("Defer().Once() must still emit onceProps[feed]: %+v", pr.markers.onceProps)
	}
}

// TestResolve_OptionalMergeMetadataOnInitial asserts that an optional mergeable
// prop excluded from the initial response still emits its merge metadata.
func TestResolve_OptionalMergeMetadataOnInitial(t *testing.T) {
	pr := &propsResolver{markers: newMarkers(), deferred: map[string][]string{}}
	out, err := pr.resolve(Props{
		"items": optionalMerge(func() (any, error) { return []int{1}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["items"]; ok {
		t.Errorf("optional prop must be excluded from initial output: %v", out)
	}
	if !hasKey(pr.markers.mergeKeys, "items") {
		t.Errorf("Optional().Merge() must still emit items in mergeKeys: %v", pr.markers.mergeKeys)
	}
}

// TestResolve_ArrayPartialOptionalResolved ports the official PropsResolverTest
// case: a partial only=["foos"] over an indexed array of maps, where each
// element holds an Optional field, resolves the Optional inside each element
// (foos.0.bar) because foos matchesOnly and descendants are included.
func TestResolve_ArrayPartialOptionalResolved(t *testing.T) {
	pr := &propsResolver{
		isPartial: true,
		only:      []string{"foos"},
		markers:   newMarkers(),
	}
	out, err := pr.resolve(Props{
		"foos": []any{
			map[string]any{"name": "First", "bar": Optional(func() (any, error) { return "b1", nil })},
			map[string]any{"name": "Second", "bar": Optional(func() (any, error) { return "b2", nil })},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	foos, ok := out["foos"].([]any)
	if !ok || len(foos) != 2 {
		t.Fatalf("foos must be a 2-element slice: %#v", out["foos"])
	}
	e0, _ := foos[0].(map[string]any)
	if e0 == nil || e0["name"] != "First" || e0["bar"] != "b1" {
		t.Errorf("foos[0] must include name and resolved Optional bar: %#v", e0)
	}
	e1, _ := foos[1].(map[string]any)
	if e1 == nil || e1["name"] != "Second" || e1["bar"] != "b2" {
		t.Errorf("foos[1] must include name and resolved Optional bar: %#v", e1)
	}
}

// TestResolve_ArrayInitialOptionalExcluded ports the official case: on an
// initial (non-partial) load, an Optional field inside an array element is
// excluded and its closure is NOT invoked, while sibling static fields remain.
func TestResolve_ArrayInitialOptionalExcluded(t *testing.T) {
	invoked := false
	pr := &propsResolver{markers: newMarkers(), deferred: map[string][]string{}}
	out, err := pr.resolve(Props{
		"foos": []any{
			map[string]any{
				"name": "First",
				"bar": Optional(func() (any, error) {
					invoked = true
					return "b1", nil
				}),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if invoked {
		t.Error("Optional closure inside array element must NOT be invoked on initial load")
	}
	foos, ok := out["foos"].([]any)
	if !ok || len(foos) != 1 {
		t.Fatalf("foos must be a 1-element slice: %#v", out["foos"])
	}
	e0, _ := foos[0].(map[string]any)
	if e0 == nil || e0["name"] != "First" {
		t.Errorf("foos[0].name must be present: %#v", e0)
	}
	if _, ok := e0["bar"]; ok {
		t.Errorf("foos[0].bar (Optional) must be absent on initial load: %#v", e0)
	}
}

// TestResolve_ArrayOfMapsNormalized verifies []map[string]any input is
// normalized and recursed just like []any, resolving nested Optional fields.
func TestResolve_ArrayOfMapsNormalized(t *testing.T) {
	pr := &propsResolver{
		isPartial: true,
		only:      []string{"foos"},
		markers:   newMarkers(),
	}
	out, err := pr.resolve(Props{
		"foos": []map[string]any{
			{"name": "First", "bar": Optional(func() (any, error) { return "b1", nil })},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	foos, ok := out["foos"].([]any)
	if !ok || len(foos) != 1 {
		t.Fatalf("foos must normalize to a 1-element []any: %#v", out["foos"])
	}
	e0, _ := foos[0].(map[string]any)
	if e0 == nil || e0["bar"] != "b1" {
		t.Errorf("nested Optional in []map element must resolve: %#v", e0)
	}
}

// TestResolve_ArrayScalarsPreserved verifies a plain scalar array is passed
// through unchanged and in order.
func TestResolve_ArrayScalarsPreserved(t *testing.T) {
	pr := &propsResolver{markers: newMarkers(), deferred: map[string][]string{}}
	out, err := pr.resolve(Props{"nums": []any{3, 1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := out["nums"].([]any)
	if !ok || len(got) != 3 || got[0] != 3 || got[1] != 1 || got[2] != 2 {
		t.Errorf("scalar array must be preserved in order: %#v", out["nums"])
	}
}
