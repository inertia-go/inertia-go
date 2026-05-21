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
