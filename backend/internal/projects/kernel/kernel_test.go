package kernel

import (
	"math/rand"
	"sort"
	"testing"
)

func TestRankBetweenKeepsOrder(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	ranks := []string{RankBetween("", "")}
	for i := 0; i < 5000; i++ {
		pos := r.Intn(len(ranks) + 1)
		lo, hi := "", ""
		if pos > 0 {
			lo = ranks[pos-1]
		}
		if pos < len(ranks) {
			hi = ranks[pos]
		}
		got := RankBetween(lo, hi)
		if !ValidRank(got) || (lo != "" && got <= lo) || (hi != "" && got >= hi) {
			t.Fatalf("RankBetween(%q,%q)=%q", lo, hi, got)
		}
		ranks = append(ranks[:pos], append([]string{got}, ranks[pos:]...)...)
	}
	if !sort.StringsAreSorted(ranks) {
		t.Fatal("ranks must stay sorted after random insertions")
	}
}

func TestRankEdgesAndSpread(t *testing.T) {
	if RankBetween("b", "a") != "" || RankBetween("a", "a") != "" {
		t.Fatal("an empty interval has no rank")
	}
	appended := ""
	for i := 0; i < 200; i++ {
		next := RankBetween(appended, "")
		if next <= appended {
			t.Fatalf("append %d: %q <= %q", i, next, appended)
		}
		appended = next
	}
	prepended := "V"
	for i := 0; i < 200; i++ {
		next := RankBetween("", prepended)
		if next >= prepended || !ValidRank(next) {
			t.Fatalf("prepend %d: %q >= %q", i, next, prepended)
		}
		prepended = next
	}
	s := Spread(40)
	if len(s) != 40 || !sort.StringsAreSorted(s) {
		t.Fatalf("spread: %v", s)
	}
	if ValidRank("") || ValidRank("a0") || ValidRank("a-") || !ValidRank("aV") {
		t.Fatal("ValidRank")
	}
}

func TestCursorRoundTripAndLimits(t *testing.T) {
	type pos struct {
		ID string `json:"id"`
		N  int    `json:"n"`
	}
	var got pos
	if err := DecodeCursor(EncodeCursor(pos{"x", 4}), &got); err != nil || got.ID != "x" || got.N != 4 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err := DecodeCursor("", &got); err != nil {
		t.Fatal("empty cursor is the first page")
	}
	if err := DecodeCursor("!!!", &got); err == nil {
		t.Fatal("garbage cursor must be a validation error")
	}
	for in, want := range map[string]int{"": DefaultLimit, "x": DefaultLimit, "0": DefaultLimit, "7": 7, "500": MaxLimit} {
		if ClampLimit(in) != want {
			t.Errorf("ClampLimit(%q)", in)
		}
	}
	rows, more := Trim([]int{1, 2, 3}, 2)
	if len(rows) != 2 || !more {
		t.Fatal("trim")
	}
	if _, more = Trim([]int{1}, 2); more {
		t.Fatal("no extra row")
	}
}

func TestErrorsAndVersionConflict(t *testing.T) {
	if NotFound("task").StatusCode != 404 || Forbidden("task.update").StatusCode != 403 || Conflict("X", "y").StatusCode != 409 || Invalid("f", "m").StatusCode != 400 {
		t.Fatal("statuses")
	}
	v, ok := AsVersionConflict(&VersionConflict{Current: 4})
	if !ok || v.Current != 4 || v.Error() == "" {
		t.Fatal("version conflict")
	}
	if _, ok := AsVersionConflict(NotFound("x")); ok {
		t.Fatal("not a version conflict")
	}
}
