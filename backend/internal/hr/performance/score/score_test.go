package score

import (
	"encoding/json"
	"testing"
)

func f(s string) Fixed {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestParseAndCodec(t *testing.T) {
	for in, want := range map[string]string{"1": "1.00", "1.5": "1.50", "-2.25": "-2.25", "0.01": "0.01", " 100 ": "100.00"} {
		if got := f(in).String(); got != want {
			t.Errorf("Parse(%q) = %s", in, got)
		}
	}
	for _, bad := range []string{"", "x", "1.", "1.234", "--1", "1e3", "10000000000000"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) must fail", bad)
		}
	}
	var v Fixed
	if err := json.Unmarshal([]byte(`"12.5"`), &v); err != nil || v != 1250 {
		t.Fatal("json string")
	}
	if err := json.Unmarshal([]byte(`7`), &v); err != nil || v != 700 {
		t.Fatal("json number")
	}
	if json.Unmarshal([]byte(`1.005`), &v) == nil {
		t.Fatal("3 decimals")
	}
	b, _ := json.Marshal(Fixed(-5))
	if string(b) != "-0.05" {
		t.Fatalf("marshal %s", b)
	}
	for _, c := range []struct {
		src  interface{}
		want Fixed
	}{{"3.10", 310}, {[]byte("0.5"), 50}, {float64(2.345), 235}, {int64(4), 400}, {nil, 0}} {
		var s Fixed
		if err := s.Scan(c.src); err != nil || s != c.want {
			t.Errorf("Scan(%v) = %v %v", c.src, s, err)
		}
	}
	var s Fixed
	if s.Scan(true) == nil || s.Scan("bad") == nil {
		t.Fatal("scan errors")
	}
	if dv, _ := Fixed(150).Value(); dv != "1.50" {
		t.Fatal("Value")
	}
	if FromInt(3) != 300 || Fixed(250).Float() != 2.5 {
		t.Fatal("FromInt/Float")
	}
}

// TestGolden_G3_ScoreMath — PM-006, PM-009, PM-012, PM-015.
func TestGolden_G3_ScoreMath(t *testing.T) {
	kr := []struct{ start, target, current, want string }{
		{"0", "100", "40", "40.00"},
		{"0", "3", "1", "33.33"},    // 33.333 → 33.33
		{"0", "3", "2", "66.67"},    // 66.666 → 66.67 (half away from zero)
		{"50", "10", "30", "50.00"}, // decreasing target (EC-02)
		{"50", "10", "60", "0.00"},  // worse than start clamps to 0
		{"0", "10", "15", "100.00"}, // beyond target clamps to 100
		{"10", "20", "10", "0.00"},
		{"-5", "5", "0", "50.00"},
		{"0", "0.08", "0.01", "12.50"},
	}
	for _, c := range kr {
		if got := KRProgress(f(c.start), f(c.target), f(c.current)).String(); got != c.want {
			t.Errorf("KRProgress(%s→%s @%s) = %s, want %s", c.start, c.target, c.current, got, c.want)
		}
	}
	if KRProgress(f("5"), f("5"), f("5")) != 0 {
		t.Fatal("degenerate KR is 0 (validators reject it upstream)")
	}
	if got := Mean([]Fixed{f("33.33"), f("66.67"), f("100")}).String(); got != "66.67" {
		t.Fatalf("Mean = %s", got)
	}
	if got := Mean([]Fixed{f("0.01"), f("0.02")}).String(); got != "0.02" { // 0.015 → 0.02
		t.Fatalf("Mean half-away = %s", got)
	}
	if Mean(nil) != 0 {
		t.Fatal("Mean(nil)")
	}
	// Goal score: weights 60/40, progress 50/100 → 70.00; weights 1/2, progress 100/0 → 33.33.
	if got := WeightedScore([]Fixed{f("60"), f("40")}, []Fixed{f("50"), f("100")}).String(); got != "70.00" {
		t.Fatalf("WeightedScore = %s", got)
	}
	if got := WeightedScore([]Fixed{f("1"), f("2")}, []Fixed{f("100"), f("0")}).String(); got != "33.33" {
		t.Fatalf("WeightedScore = %s", got)
	}
	if WeightedScore(nil, nil) != 0 {
		t.Fatal("WeightedScore(nil)")
	}
	// Overall review score: mean of integer ratings.
	for ratings, want := range map[[3]int]string{{5, 4, 4}: "4.33", {3, 4, 4}: "3.67", {1, 1, 2}: "1.33", {5, 5, 5}: "5.00"} {
		if got := Overall(ratings[:]).String(); got != want {
			t.Errorf("Overall(%v) = %s", ratings, got)
		}
	}
	if Overall(nil) != 0 {
		t.Fatal("Overall(nil)")
	}
	for s, want := range map[string]string{"1.00": "low", "2.49": "low", "2.50": "medium", "3.49": "medium", "3.50": "high", "5.00": "high"} {
		if got := Band(f(s)); got != want {
			t.Errorf("Band(%s) = %s", s, got)
		}
	}
	if divRound(-7, 2) != -4 || divRound(7, -2) != -4 || divRound(5, 0) != 0 {
		t.Fatal("divRound sign handling")
	}
}
