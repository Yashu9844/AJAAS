package dto

import "testing"

// TestParsePage_G15 — golden G15: hostile paging is clamped, never an error.
func TestParsePage_G15(t *testing.T) {
	cases := []struct {
		page, per string
		want      Page
	}{
		{"", "", Page{1, 20}},
		{"0", "0", Page{1, 20}},
		{"-1", "abc", Page{1, 20}},
		{"3", "100000", Page{3, 100}},
		{"2", "50", Page{2, 50}},
	}
	for _, tc := range cases {
		if got := ParsePage(tc.page, tc.per); got != tc.want {
			t.Errorf("ParsePage(%q,%q) = %+v, want %+v", tc.page, tc.per, got, tc.want)
		}
	}
}

func TestPage_OffsetAndMeta(t *testing.T) {
	p := Page{Page: 3, PerPage: 20}
	if p.Offset() != 40 {
		t.Fatalf("offset = %d", p.Offset())
	}
	if m := p.Meta(41); m.TotalPages != 3 || m.TotalItems != 41 || m.Page != 3 || m.PerPage != 20 {
		t.Fatalf("meta = %+v", m)
	}
	if m := p.Meta(0); m.TotalPages != 0 {
		t.Fatalf("empty meta = %+v", m)
	}
}
