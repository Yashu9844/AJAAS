package pipeline

import "testing"

// TestGolden_G2_StageGraph — RC-002: every manual move, allowed or not.
func TestGolden_G2_StageGraph(t *testing.T) {
	all := []string{Applied, Screening, Interview, Offer, Hired, Rejected, Withdrawn}
	allowed := map[[2]string]bool{
		{Applied, Screening}: true, {Applied, Rejected}: true, {Applied, Withdrawn}: true,
		{Screening, Interview}: true, {Screening, Rejected}: true, {Screening, Withdrawn}: true,
		{Interview, Rejected}: true, {Interview, Withdrawn}: true,
		{Offer, Rejected}: true, {Offer, Withdrawn}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanMove(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("CanMove(%s → %s) = %v", from, to, got)
			}
		}
	}
	for _, s := range []string{Hired, Rejected, Withdrawn} {
		if !IsTerminal(s) {
			t.Errorf("%s must be terminal", s)
		}
	}
	if IsTerminal(Offer) || !ValidStage(Screening) || ValidStage("onboarding") {
		t.Fatal("stage helpers")
	}
}

// TestGolden_G3_JobStatusGraph — FR-JB003.
func TestGolden_G3_JobStatusGraph(t *testing.T) {
	all := []string{JobDraft, JobOpen, JobOnHold, JobClosed, JobFilled}
	allowed := map[[2]string]bool{
		{JobDraft, JobOpen}: true, {JobDraft, JobClosed}: true,
		{JobOpen, JobOnHold}: true, {JobOpen, JobClosed}: true,
		{JobOnHold, JobOpen}: true, {JobOnHold, JobClosed}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanTransitionJob(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("CanTransitionJob(%s → %s) = %v", from, to, got)
			}
		}
	}
	if !Editable(JobDraft) || !Editable(JobOnHold) || Editable(JobClosed) || Editable(JobFilled) {
		t.Fatal("Editable")
	}
}
