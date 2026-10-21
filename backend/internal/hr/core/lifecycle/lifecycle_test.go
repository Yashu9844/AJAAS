package lifecycle

import "testing"

// CG2 — every allowed and forbidden action transition.
func TestCanMove(t *testing.T) {
	all := []string{Draft, Submitted, Completed, Rejected, Cancelled, Rescinded}
	allowed := map[[2]string]bool{
		{Draft, Submitted}: true, {Draft, Cancelled}: true,
		{Submitted, Completed}: true, {Submitted, Rejected}: true, {Submitted, Cancelled}: true,
		{Completed, Rescinded}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanMove(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("%s → %s = %v", from, to, got)
			}
		}
		if !ValidStatus(from) {
			t.Errorf("%s must be valid", from)
		}
	}
	if ValidStatus("approved") || CanMove("x", Draft) {
		t.Fatal("unknown statuses")
	}
}

func TestTypes(t *testing.T) {
	for _, tp := range []string{Hire, Promote, Demote, Transfer, ChangeJob, ChangeManager, ChangeLocation, ProbationConfirm, Terminate, Rehire} {
		if !ValidType(tp) {
			t.Errorf("%s", tp)
		}
		if Starts(tp) == Changes(tp) && tp != Terminate {
			t.Errorf("%s must be exactly one of starts/changes", tp)
		}
	}
	if ValidType("fire") || Starts(Terminate) || Changes(Terminate) {
		t.Fatal("terminate is neither start nor change")
	}
}

func TestCanMovePosition(t *testing.T) {
	cases := map[[2]string]bool{
		{"active", "frozen"}: true, {"active", "closed"}: true, {"frozen", "active"}: true, {"frozen", "closed"}: true,
		{"active", "active"}: false, {"closed", "active"}: false, {"closed", "frozen"}: false, {"frozen", "frozen"}: false,
	}
	for c, want := range cases {
		if got := CanMovePosition(c[0], c[1]); got != want {
			t.Errorf("%v = %v", c, got)
		}
	}
}
