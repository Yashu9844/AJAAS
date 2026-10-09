package dto

import "testing"

// TestParsePage_G14 — hostile paging is clamped.
func TestParsePage_G14(t *testing.T) {
	for _, c := range []struct {
		p, pp string
		want  Page
	}{{"", "", Page{1, 20}}, {"0", "0", Page{1, 20}}, {"-3", "x", Page{1, 20}}, {"2", "500", Page{2, 100}}, {"4", "10", Page{4, 10}}} {
		if got := ParsePage(c.p, c.pp); got != c.want {
			t.Errorf("ParsePage(%q,%q) = %+v", c.p, c.pp, got)
		}
	}
	if p := (Page{3, 10}); p.Offset() != 20 || p.Meta(21).TotalPages != 3 {
		t.Fatal("offset/meta")
	}
}
