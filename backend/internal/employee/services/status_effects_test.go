package services

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
)

func TestApplyStatusEffects(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	detail := func() *models.EmploymentDetail { return &models.EmploymentDetail{NoticePeriodDays: 30} }

	// confirmation
	d := detail()
	if ev := applyStatusEffects(d, dto.TransitionStatusRequest{}, "probation", "active", now); ev != "confirmed" || d.ConfirmationDate == nil || !d.ConfirmationDate.Equal(now) {
		t.Fatalf("confirmation: %s %+v", ev, d)
	}
	// explicit confirmation date wins
	explicit := now.AddDate(0, 0, -3)
	d = detail()
	applyStatusEffects(d, dto.TransitionStatusRequest{ConfirmationDate: &explicit}, "probation", "active", now)
	if !d.ConfirmationDate.Equal(explicit) {
		t.Fatal("explicit confirmation date must be kept")
	}
	// active -> active elsewhere is not a confirmation
	if ev := applyStatusEffects(detail(), dto.TransitionStatusRequest{}, "on_leave", "active", now); ev != "status_changed:active" {
		t.Fatalf("on_leave->active: %s", ev)
	}

	// notice: tentative exit = resignation + notice period
	d = detail()
	if ev := applyStatusEffects(d, dto.TransitionStatusRequest{Notes: "x"}, "active", "notice", now); ev != "status_changed:notice" {
		t.Fatalf("notice event: %s", ev)
	}
	if d.ResignationDate == nil || d.ExitDate == nil || !d.ExitDate.Equal(now.AddDate(0, 0, 30)) {
		t.Fatalf("tentative exit not derived: %+v", d)
	}

	// final exit keeps explicit values and records the reason
	exit := now.AddDate(0, 1, 0)
	res := now.AddDate(0, 0, -1)
	d = detail()
	ev := applyStatusEffects(d, dto.TransitionStatusRequest{ExitDate: &exit, ResignationDate: &res, ExitReason: "offer"}, "notice", "resigned", now)
	if ev != "resigned" || !d.ExitDate.Equal(exit) || !d.ResignationDate.Equal(res) || d.ExitReason != "offer" {
		t.Fatalf("resigned: %s %+v", ev, d)
	}

	// termination stamps the exit date when missing
	d = detail()
	if ev := applyStatusEffects(d, dto.TransitionStatusRequest{}, "active", "terminated", now); ev != "terminated" || d.ExitDate == nil || !d.ExitDate.Equal(now) {
		t.Fatalf("terminated: %s %+v", ev, d)
	}

	// no employment detail loaded
	if ev := applyStatusEffects(nil, dto.TransitionStatusRequest{}, "active", "terminated", now); ev != "terminated" {
		t.Fatalf("nil detail terminated: %s", ev)
	}
	if ev := applyStatusEffects(nil, dto.TransitionStatusRequest{}, "active", "on_leave", now); ev != "status_changed:on_leave" {
		t.Fatalf("nil detail on_leave: %s", ev)
	}
}
