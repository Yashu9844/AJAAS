package dto

import "testing"

// TestParsePage_G13 freezes golden G13: hostile pagination input clamps, never errors or panics.
func TestParsePage_G13(t *testing.T) {
	cases := []struct {
		page, perPage string
		want          Page
	}{
		{"", "", Page{1, 20}},
		{"0", "0", Page{1, 20}},
		{"-1", "-5", Page{1, 20}},
		{"abc", "xyz", Page{1, 20}},
		{"3", "1000", Page{3, 100}},
		{"2", "50", Page{2, 50}},
	}
	for _, tc := range cases {
		if got := ParsePage(tc.page, tc.perPage); got != tc.want {
			t.Errorf("ParsePage(%q,%q) = %+v, want %+v", tc.page, tc.perPage, got, tc.want)
		}
	}
}

func TestPageOffsetAndMeta(t *testing.T) {
	p := Page{Page: 3, PerPage: 20}
	if p.Offset() != 40 {
		t.Fatalf("offset = %d", p.Offset())
	}
	for total, pages := range map[int64]int{0: 0, 1: 1, 20: 1, 21: 2, 100: 5} {
		if m := p.Meta(total); m.TotalPages != pages || m.TotalItems != total || m.Page != 3 || m.PerPage != 20 {
			t.Errorf("Meta(%d) = %+v, want %d pages", total, m, pages)
		}
	}
}
