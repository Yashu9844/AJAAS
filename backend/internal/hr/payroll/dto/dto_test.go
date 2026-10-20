package dto

import "testing"

// TestParsePage_G16 — golden G16 helper.
func TestParsePage_G16(t *testing.T) {
	for _, c := range []struct {
		p, pp string
		want  Page
	}{{"", "", Page{1, 20}}, {"0", "0", Page{1, 20}}, {"-1", "abc", Page{1, 20}}, {"2", "1000", Page{2, 100}}, {"3", "15", Page{3, 15}}} {
		if got := ParsePage(c.p, c.pp); got != c.want {
			t.Errorf("ParsePage(%q,%q) = %+v", c.p, c.pp, got)
		}
	}
	p := Page{Page: 2, PerPage: 20}
	if p.Offset() != 20 || p.Meta(41).TotalPages != 3 || p.Meta(0).TotalPages != 0 {
		t.Fatal("offset/meta")
	}
}
