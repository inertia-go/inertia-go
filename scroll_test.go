package inertia

import "testing"

func TestMapAdapter_DerivesAllKeys(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	prev, next := 1, 3
	meta := map[string]any{
		"pageName":     "users",
		"currentPage":  2,
		"previousPage": prev,
		"nextPage":     next,
	}
	cfg := deriveScroll(meta, ScrollOptions{})
	if cfg.PageName != "users" {
		t.Errorf("PageName = %q, want users", cfg.PageName)
	}
	if cfg.CurrentPage != 2 {
		t.Errorf("CurrentPage = %d, want 2", cfg.CurrentPage)
	}
	if cfg.PreviousPage == nil || *cfg.PreviousPage != 1 {
		t.Errorf("PreviousPage = %v, want 1", cfg.PreviousPage)
	}
	if cfg.NextPage == nil || *cfg.NextPage != 3 {
		t.Errorf("NextPage = %v, want 3", cfg.NextPage)
	}
}

func TestMapAdapter_Defaults(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	cfg := deriveScroll(map[string]any{}, ScrollOptions{})
	if cfg.PageName != "page" {
		t.Errorf("PageName default = %q, want page", cfg.PageName)
	}
	if cfg.CurrentPage != 0 {
		t.Errorf("CurrentPage default = %d, want 0", cfg.CurrentPage)
	}
	if cfg.PreviousPage != nil || cfg.NextPage != nil {
		t.Errorf("prev/next default should be nil, got %v/%v", cfg.PreviousPage, cfg.NextPage)
	}
}

func TestIdentityAdapter_PassesScrollConfigThrough(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	n := 5
	in := ScrollConfig{PageName: "feed", CurrentPage: 4, NextPage: &n}
	cfg := deriveScroll(in, ScrollOptions{})
	if cfg.PageName != "feed" || cfg.CurrentPage != 4 || cfg.NextPage == nil || *cfg.NextPage != 5 {
		t.Errorf("identity adapter mangled config: %+v", cfg)
	}
}

func TestScrollOptions_PageNameOverride(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	cfg := deriveScroll(map[string]any{"pageName": "page"}, ScrollOptions{PageName: "orders"})
	if cfg.PageName != "orders" {
		t.Errorf("PageName override = %q, want orders", cfg.PageName)
	}
}

type fakePaginator struct{ cur int }

type fakeAdapter struct{}

func (fakeAdapter) Match(meta any) bool { _, ok := meta.(fakePaginator); return ok }
func (fakeAdapter) Derive(meta any, o ScrollOptions) ScrollConfig {
	p := meta.(fakePaginator)
	name := "page"
	if o.PageName != "" {
		name = o.PageName
	}
	return ScrollConfig{PageName: name, CurrentPage: p.cur}
}

func TestRegisterScrollAdapter_CustomTypeMatched(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	RegisterScrollAdapter(fakeAdapter{})
	cfg := deriveScroll(fakePaginator{cur: 7}, ScrollOptions{})
	if cfg.CurrentPage != 7 {
		t.Errorf("custom adapter not used: CurrentPage = %d, want 7", cfg.CurrentPage)
	}
}

func TestRegisterScrollAdapter_ReverseOrderPrecedence(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	// Both adapters match map[string]any; the later registration must win.
	RegisterScrollAdapter(stampAdapter{stamp: "first"})
	RegisterScrollAdapter(stampAdapter{stamp: "second"})
	cfg := deriveScroll(map[string]any{}, ScrollOptions{})
	if cfg.PageName != "second" {
		t.Errorf("reverse-order precedence broken: PageName = %q, want second", cfg.PageName)
	}
}

type stampAdapter struct{ stamp string }

func (stampAdapter) Match(meta any) bool { _, ok := meta.(map[string]any); return ok }
func (a stampAdapter) Derive(_ any, _ ScrollOptions) ScrollConfig {
	return ScrollConfig{PageName: a.stamp}
}

func TestDeriveScroll_NoMatchPanics(t *testing.T) {
	t.Cleanup(resetScrollAdapters)
	defer func() {
		if recover() == nil {
			t.Error("deriveScroll with unmatched type must panic")
		}
	}()
	deriveScroll(42, ScrollOptions{})
}
