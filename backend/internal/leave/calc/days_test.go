package calc

import (
	"encoding/json"
	"testing"
)

func TestParseDays(t *testing.T) {
	ok := map[string]Days{"0": 0, "1": 100, "1.5": 150, "0.25": 25, "12.00": 1200, "-2.5": -250, "365": 36500, " 3.1 ": 310}
	for in, want := range ok {
		got, err := ParseDays(in)
		if err != nil || got != want {
			t.Errorf("ParseDays(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1.234", "1.", ".5x", "--1", "1e2", "99999999999999999999"} {
		if _, err := ParseDays(bad); err == nil {
			t.Errorf("ParseDays(%q) must fail", bad)
		}
	}
}

func TestDays_StringAndFloat(t *testing.T) {
	for d, want := range map[Days]string{0: "0.00", 150: "1.50", 5: "0.05", -250: "-2.50", -5: "-0.05", 36500: "365.00"} {
		if got := d.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", d, got, want)
		}
	}
	if Days(450).Float() != 4.5 {
		t.Error("Float conversion")
	}
	if FromWhole(3) != 300 {
		t.Error("FromWhole")
	}
}

func TestDays_SQLValueScan(t *testing.T) {
	v, err := Days(450).Value()
	if err != nil || v != "4.50" {
		t.Fatalf("Value = %v, %v", v, err)
	}
	var d Days
	for _, tc := range []struct {
		src  interface{}
		want Days
	}{{"4.50", 450}, {[]byte("0.25"), 25}, {float64(1.5), 150}, {int64(2), 200}} {
		if err := d.Scan(tc.src); err != nil || d != tc.want {
			t.Errorf("Scan(%v) = %d, %v; want %d", tc.src, d, err, tc.want)
		}
	}
	if err := d.Scan(nil); err != nil || d != 0 {
		t.Errorf("Scan(nil) = %d, %v", d, err)
	}
	if err := d.Scan(true); err == nil {
		t.Error("Scan(bool) must fail")
	}
	if err := d.Scan("x"); err == nil {
		t.Error("Scan(bad string) must fail")
	}
}

func TestDays_JSON(t *testing.T) {
	raw, err := json.Marshal(struct {
		D Days `json:"d"`
	}{150})
	if err != nil || string(raw) != `{"d":1.50}` {
		t.Fatalf("marshal = %s, %v", raw, err)
	}
	var in struct {
		D Days `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"d":2.5}`), &in); err != nil || in.D != 250 {
		t.Fatalf("unmarshal = %d, %v", in.D, err)
	}
	if err := json.Unmarshal([]byte(`{"d":"2.5"}`), &in); err != nil || in.D != 250 {
		t.Fatalf("unmarshal quoted = %d, %v", in.D, err)
	}
	if err := json.Unmarshal([]byte(`{"d":0.125}`), &in); err == nil {
		t.Fatal("more than 2 decimals must fail")
	}
}
