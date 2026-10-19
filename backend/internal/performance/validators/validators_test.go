package validators

import (
	"errors"
	"strings"
	"testing"

	"github.com/jaas/jaas/internal/performance/models"
	"github.com/jaas/jaas/internal/performance/score"
)

func TestCriteria_PM001(t *testing.T) {
	got, err := Criteria(nil)
	if err != nil || strings.Join(got, ",") != "Quality,Delivery,Collaboration,Ownership" {
		t.Fatalf("defaults: %v %v", got, err)
	}
	got, err = Criteria([]string{"  Craft ", "Impact"})
	if err != nil || got[0] != "Craft" || got[1] != "Impact" {
		t.Fatalf("trim/order: %v %v", got, err)
	}
	long := strings.Repeat("x", 51)
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = string(rune('a' + i))
	}
	for name, in := range map[string][]string{"dup ignoring case": {"Impact", "impact"}, "blank": {"  "}, "too long": {long}, "too many": eleven} {
		if _, err := Criteria(in); !errors.Is(err, ErrCriteria) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Criteria([]string{strings.Repeat("é", 50)}); err != nil {
		t.Fatal("length counts characters, not bytes")
	}
}

func TestDatesWeightsProgressKR(t *testing.T) {
	s, e, err := DateRange("2026-01-01", "2026-03-31")
	if err != nil || s.Month() != 1 || e.Day() != 31 {
		t.Fatal("range")
	}
	if _, _, err := DateRange("2026-04-01", "2026-03-31"); !errors.Is(err, ErrDateOrder) {
		t.Fatal("order")
	}
	if _, _, err := DateRange("x", "2026-03-31"); !errors.Is(err, ErrDate) {
		t.Fatal("bad start")
	}
	if _, _, err := DateRange("2026-01-01", "y"); !errors.Is(err, ErrDate) {
		t.Fatal("bad end")
	}
	for v, ok := range map[score.Fixed]bool{0: false, 1: true, score.Hundred: true, score.Hundred + 1: false, -1: false} {
		if (Weight(v) == nil) != ok {
			t.Errorf("Weight(%s)", v)
		}
	}
	for v, ok := range map[score.Fixed]bool{0: true, score.Hundred: true, score.Hundred + 1: false, -1: false} {
		if (Progress(v) == nil) != ok {
			t.Errorf("Progress(%s)", v)
		}
	}
	if KeyResult(score.FromInt(1), score.FromInt(1)) == nil || KeyResult(0, score.FromInt(5)) != nil {
		t.Fatal("KeyResult")
	}
}

// TestRatings_PM012 — exact criteria once each, integer 1–5; potential only (and always) on manager reviews.
func TestRatings_PM012(t *testing.T) {
	crit := []string{"Quality", "Delivery"}
	r := func(pairs ...interface{}) models.Ratings {
		var out models.Ratings
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, models.Rating{Criterion: pairs[i].(string), Score: pairs[i+1].(int)})
		}
		return out
	}
	if err := Ratings(crit, r("Delivery", 3, "quality", 5)); err != nil {
		t.Fatalf("any order, case-insensitive: %v", err)
	}
	for name, bad := range map[string]models.Ratings{
		"missing": r("Quality", 3), "extra": r("Quality", 3, "Delivery", 3, "Speed", 3), "dup": r("Quality", 3, "quality", 4),
		"zero": r("Quality", 0, "Delivery", 3), "six": r("Quality", 6, "Delivery", 3), "unknown": r("Quality", 3, "Speed", 3),
	} {
		if err := Ratings(crit, bad); !errors.Is(err, ErrRatings) {
			t.Errorf("%s: %v", name, err)
		}
	}
	hi, bogus, smuggled := "high", "stellar", "low|medium"
	cases := []struct {
		typ string
		p   *string
		ok  bool
	}{{models.ReviewManager, &hi, true}, {models.ReviewManager, nil, false}, {models.ReviewManager, &bogus, false}, {models.ReviewManager, &smuggled, false},
		{models.ReviewSelf, nil, true}, {models.ReviewSelf, &hi, false}, {models.ReviewPeer, &hi, false}}
	for _, c := range cases {
		if err := Potential(c.typ, c.p); (err == nil) != c.ok {
			t.Errorf("Potential(%s, %v) = %v", c.typ, c.p, err)
		}
	}
	if Canonical(crit, r("delivery", 3, "QUALITY", 5))[0].Criterion != "Quality" {
		t.Fatal("Canonical must reorder to the cycle's criteria and spelling")
	}
}
