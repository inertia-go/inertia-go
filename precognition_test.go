package inertia

import "testing"

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
