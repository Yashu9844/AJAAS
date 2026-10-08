package validators

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/leave/calc"
)

func TestNormalizeTypeCode(t *testing.T) {
	for in, want := range map[string]string{"el": "EL", " sl ": "SL", "LOP_1": "LOP_1"} {
		if got, err := NormalizeTypeCode(in); err != nil || got != want {
			t.Errorf("NormalizeTypeCode(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "a", "TOO_LONG_CODE", "e-l", "é"} {
		if _, err := NormalizeTypeCode(bad); err == nil {
			t.Errorf("NormalizeTypeCode(%q) must fail", bad)
		}
	}
}

func TestAmounts(t *testing.T) {
	for _, ok := range []int64{0, 150, 36500} {
		if ValidateAllowance(daysOf(ok)) != nil {
			t.Errorf("allowance %d should pass", ok)
		}
	}
	for _, bad := range []int64{-1, 36501} {
		if ValidateAllowance(daysOf(bad)) == nil {
			t.Errorf("allowance %d should fail", bad)
		}
	}
	for _, ok := range []int64{-36500, -50, 50, 36500} {
		if ValidateAdjustment(daysOf(ok)) != nil {
			t.Errorf("adjustment %d should pass", ok)
		}
	}
	for _, bad := range []int64{0, 36501, -36501} {
		if ValidateAdjustment(daysOf(bad)) == nil {
			t.Errorf("adjustment %d should fail", bad)
		}
	}
}

func TestParseDateAndYear(t *testing.T) {
	if d, err := ParseDate("2026-10-09"); err != nil || d.Day() != 9 || d.Location() != time.UTC {
		t.Fatalf("ParseDate: %v %v", d, err)
	}
	if _, err := ParseDate("09-10-2026"); err != ErrDate {
		t.Fatal("bad date must fail")
	}
	if y, err := ParseYear("", 2026); err != nil || y != 2026 {
		t.Fatal("default year")
	}
	if y, err := ParseYear("2027", 2026); err != nil || y != 2027 {
		t.Fatal("explicit year")
	}
	for _, bad := range []string{"abc", "1999", "2201"} {
		if _, err := ParseYear(bad, 2026); err != ErrYear {
			t.Errorf("ParseYear(%q) must fail", bad)
		}
	}
}

func TestGenderApplies(t *testing.T) {
	cases := []struct {
		applicable, gender string
		want               bool
	}{{"all", "", true}, {"female", "Female", true}, {"female", "male", false}, {"male", " MALE ", true}, {"male", "", false}}
	for _, tc := range cases {
		if got := GenderApplies(tc.applicable, tc.gender); got != tc.want {
			t.Errorf("GenderApplies(%q,%q) = %v", tc.applicable, tc.gender, got)
		}
	}
}

// LV-005 notice and backdating window.
func TestNoticeOK(t *testing.T) {
	today := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		start  time.Time
		notice int
		want   bool
	}{
		{today.AddDate(0, 0, 7), 7, true},
		{today.AddDate(0, 0, 6), 7, false},
		{today.AddDate(0, 0, -30), 0, true},
		{today.AddDate(0, 0, -31), 0, false},
		{today, 0, true},
	}
	for _, tc := range cases {
		if got := NoticeOK(tc.start, today, tc.notice); got != tc.want {
			t.Errorf("NoticeOK(%s, notice %d) = %v", tc.start.Format(DateLayout), tc.notice, got)
		}
	}
}

func daysOf(v int64) calc.Days { return calc.Days(v) }
