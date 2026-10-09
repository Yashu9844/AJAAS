package calc

import (
	"encoding/json"
	"testing"
)

func TestParseMoney(t *testing.T) {
	ok := map[string]Money{"0": 0, "1": 100, "41666.67": 4166667, "0.05": 5, "-2.5": -250, " 10 ": 1000}
	for in, want := range ok {
		if got, err := ParseMoney(in); err != nil || got != want {
			t.Errorf("ParseMoney(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "x", "1.234", "1.", "--1", "1e3", "999999999999999999"} {
		if _, err := ParseMoney(bad); err == nil {
			t.Errorf("ParseMoney(%q) must fail", bad)
		}
	}
}

func TestMoney_StringSQLJSON(t *testing.T) {
	if Money(4166667).String() != "41666.67" || Money(-5).String() != "-0.05" || Rupees(3).String() != "3.00" {
		t.Fatal("String")
	}
	if v, err := Money(150).Value(); err != nil || v != "1.50" {
		t.Fatalf("Value %v %v", v, err)
	}
	var m Money
	for _, tc := range []struct {
		src  interface{}
		want Money
	}{{"4.50", 450}, {[]byte("0.25"), 25}, {float64(1.5), 150}, {int64(2), 200}, {nil, 0}} {
		if err := m.Scan(tc.src); err != nil || m != tc.want {
			t.Errorf("Scan(%v) = %d %v", tc.src, m, err)
		}
	}
	if m.Scan(true) == nil || m.Scan("x") == nil {
		t.Fatal("bad scans must fail")
	}
	raw, _ := json.Marshal(struct {
		M Money `json:"m"`
	}{4166667})
	if string(raw) != `{"m":41666.67}` {
		t.Fatalf("marshal %s", raw)
	}
	var in struct {
		M Money `json:"m"`
	}
	if json.Unmarshal([]byte(`{"m":1200000}`), &in) != nil || in.M != 120000000 {
		t.Fatalf("unmarshal %d", in.M)
	}
	if json.Unmarshal([]byte(`{"m":"12.5"}`), &in) != nil || in.M != 1250 {
		t.Fatal("quoted unmarshal")
	}
	if json.Unmarshal([]byte(`{"m":1.234}`), &in) == nil {
		t.Fatal("3 decimals must fail")
	}
}

// Rounding is half away from zero at paise; statutory rounding at rupees (D5-02).
func TestMoney_Arithmetic(t *testing.T) {
	if got := Money(10000000).Percent(4000); got != 4000000 { // 100,000.00 × 40.00%
		t.Fatalf("Percent = %d", got)
	}
	if got := Money(333).Percent(5000); got != 167 { // 3.33 × 50% = 1.665 → 1.67
		t.Fatalf("Percent half-up = %d", got)
	}
	if got := Money(-333).Percent(5000); got != -167 {
		t.Fatalf("Percent negative = %d", got)
	}
	if got := Money(10000000).Prorate(15, 30); got != 5000000 {
		t.Fatalf("Prorate = %d", got)
	}
	if got := Money(10000).Prorate(1, 3); got != 3333 {
		t.Fatalf("Prorate 1/3 = %d", got)
	}
	if got := Money(10000).Prorate(5, 0); got != 0 {
		t.Fatal("Prorate with zero denominator must be 0")
	}
	for in, want := range map[Money]Money{15049: 15000, 15050: 15100, -15050: -15100, 0: 0} {
		if got := in.RoundRupee(); got != want {
			t.Errorf("RoundRupee(%d) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[Money]Money{15001: 15100, 15000: 15000, 1: 100, 0: 0} {
		if got := in.CeilRupee(); got != want {
			t.Errorf("CeilRupee(%d) = %d, want %d", in, got, want)
		}
	}
	if Min(3, 5) != 3 || Min(7, 2) != 2 {
		t.Fatal("Min")
	}
}
