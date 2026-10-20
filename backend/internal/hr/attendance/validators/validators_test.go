package validators

import (
	"testing"
	"time"
)

func TestValidateShiftCode(t *testing.T) {
	for _, ok := range []string{"day", "night-1", "g2"} {
		if err := ValidateShiftCode(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a", "Day", "has space", "x!", "this-code-is-way-too-long-for-shift"} {
		if err := ValidateShiftCode(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestLoadTimezone(t *testing.T) {
	loc, err := LoadTimezone("")
	if err != nil || loc != time.UTC {
		t.Fatalf("empty tz must default to UTC, got %v %v", loc, err)
	}
	if loc, err := LoadTimezone("Asia/Kolkata"); err != nil || loc.String() != "Asia/Kolkata" {
		t.Fatalf("Asia/Kolkata: %v %v", loc, err)
	}
	for _, bad := range []string{"Local", "Mars/Olympus", "../etc/passwd"} {
		if _, err := LoadTimezone(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestParseDate(t *testing.T) {
	d, err := ParseDate("2026-10-08")
	if err != nil || !d.Equal(time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v %v", d, err)
	}
	for _, bad := range []string{"", "2026-13-01", "08-10-2026", "2026/10/08"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestDateRange(t *testing.T) {
	today := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	from, to, err := DateRange("", "", today, 62)
	if err != nil || !from.Equal(today.AddDate(0, 0, -6)) || !to.Equal(today) {
		t.Fatalf("default range = %v..%v %v", from, to, err)
	}
	if _, _, err := DateRange("2026-10-09", "2026-10-01", today, 62); err == nil {
		t.Error("from > to must fail")
	}
	if _, _, err := DateRange("2026-01-01", "2026-10-01", today, 62); err == nil {
		t.Error("range over max must fail")
	}
	if _, _, err := DateRange("bad", "", today, 62); err == nil {
		t.Error("bad from must fail")
	}
	if _, _, err := DateRange("", "bad", today, 62); err == nil {
		t.Error("bad to must fail")
	}
	if f, tt, err := DateRange("2026-10-01", "2026-10-05", today, 62); err != nil || f.Day() != 1 || tt.Day() != 5 {
		t.Errorf("explicit range = %v..%v %v", f, tt, err)
	}
}

func TestThresholdPair(t *testing.T) {
	ten, five := 10, 5
	if err := ValidateThresholds(&ten, &five); err != nil {
		t.Errorf("full 10 half 5: %v", err)
	}
	if err := ValidateThresholds(&five, &ten); err == nil {
		t.Error("half > full must fail")
	}
	if err := ValidateThresholds(nil, &ten); err != nil {
		t.Error("single threshold is fine")
	}
}
