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
