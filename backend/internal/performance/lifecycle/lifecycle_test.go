package lifecycle

import "testing"

// TestGolden_G2_CycleGraph — PM-002 plus the per-state permissions PM-003/PM-007/PM-011/PM-012.
func TestGolden_G2_CycleGraph(t *testing.T) {
	all := []string{CycleDraft, CycleActive, CycleReview, CycleCompleted}
	allowed := map[[2]string]bool{{CycleDraft, CycleActive}: true, {CycleActive, CycleReview}: true, {CycleReview, CycleCompleted}: true}
	for _, from := range all {
		for _, to := range all {
			if got := CanTransitionCycle(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("cycle %s → %s = %v", from, to, got)
			}
		}
	}
	want := map[string][4]bool{ // editable, goalsOpen, checkInsOpen, reviewsOpen
		CycleDraft:     {true, true, false, false},
		CycleActive:    {false, true, true, false},
		CycleReview:    {false, false, true, true},
		CycleCompleted: {false, false, false, false},
	}
	for s, w := range want {
		if got := [4]bool{CycleEditable(s), GoalsOpen(s), CheckInsOpen(s), ReviewsOpen(s)}; got != w {
			t.Errorf("%s flags = %v, want %v", s, got, w)
		}
	}
	if !ValidCycleStatus(CycleReview) || ValidCycleStatus("archived") {
		t.Fatal("ValidCycleStatus")
	}
}

// TestGolden_G2_GoalGraph — FR-GL004.
func TestGolden_G2_GoalGraph(t *testing.T) {
	all := []string{GoalActive, GoalCompleted, GoalCancelled}
	allowed := map[[2]string]bool{{GoalActive, GoalCompleted}: true, {GoalActive, GoalCancelled}: true, {GoalCompleted, GoalActive}: true}
	for _, from := range all {
		for _, to := range all {
			if got := CanTransitionGoal(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("goal %s → %s = %v", from, to, got)
			}
		}
	}
}
