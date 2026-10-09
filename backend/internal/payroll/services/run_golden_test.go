package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	leavecalc "github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
)

type lifecycle struct {
	h              *harness
	maker, checker Actor
	run            *dto.RunResponse
	userA, userD   uuid.UUID
	a, b, c, dEmp  uuid.UUID
	structure      uuid.UUID
}

// setupSeptember builds September 2026 (30 days): A full month, B joins 16th, C exits 10th, D has 2 LOP days.
func setupSeptember(t *testing.T) *lifecycle {
	t.Helper()
	h := newHarness()
	l := &lifecycle{h: h, maker: h.actor(uuid.New()), checker: h.actor(uuid.New())}
	l.structure = h.standardStructure(t, l.maker)
	exit := d("2026-09-10")
	userA, empA := h.employee("A1", d("2025-01-01"), nil)
	_, empB := h.employee("B1", d("2026-09-16"), nil)
	_, empC := h.employee("C1", d("2025-01-01"), &exit)
	userD, empD := h.employee("D1", d("2025-01-01"), nil)
	l.userA, l.userD, l.a, l.b, l.c, l.dEmp = userA, userD, empA.ID, empB.ID, empC.ID, empD.ID
	for _, e := range []uuid.UUID{empA.ID, empC.ID, empD.ID} {
		h.assign(t, l.maker, e, l.structure, calc.Rupees(1200000), "2026-01-01")
	}
	h.assign(t, l.maker, empB.ID, l.structure, calc.Rupees(1200000), "2026-09-16")
	h.lop.days[empD.ID] = leavecalc.Days(200)
	run, err := NewRunService(h.deps).Create(context.Background(), l.maker, dto.CreateRunRequest{Month: 9, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	l.run = run
	return l
}

func (l *lifecycle) slips() map[string]models.Payslip {
	out := map[string]models.Payslip{}
	for _, p := range l.h.st.payslips {
		out[p.EmployeeCode] = p
	}
	return out
}

func lineAmt(p models.Payslip, code string) calc.Money {
	for _, l := range p.Lines {
		if l.Code == code {
			return l.Amount
		}
	}
	return -1
}

// TestGolden_Calculate — PY-001..PY-009 end to end, incl. G10 (LOP from Module 4) and window clipping.
func TestGolden_Calculate(t *testing.T) {
	l := setupSeptember(t)
	run, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID)
	if err != nil || run.Status != models.RunCalculated || run.EmployeeCount != 4 || len(run.Warnings) != 0 {
		t.Fatalf("calculate: %+v %v", run, err)
	}
	s := l.slips()
	if a := s["A1"]; a.Gross != calc.Rupees(100000) || lineAmt(a, "PF") != calc.Rupees(1800) || lineAmt(a, "PT") != calc.Rupees(200) ||
		lineAmt(a, "TDS") != -1 || a.Net != calc.Rupees(98000) || a.PayableDays != 3000 {
		t.Fatalf("A (full month, taxable 11.25L → no TDS): %+v", a)
	}
	if b := s["B1"]; b.Gross != calc.Rupees(50000) || b.PayableDays != 1500 {
		t.Fatalf("B joins 16th → 15/30: %+v", b)
	}
	if c := s["C1"]; c.Gross != calc.Money(3333333) || c.PayableDays != 1000 {
		t.Fatalf("C exits 10th → 10/30: gross %s", c.Gross)
	}
	if dd := s["D1"]; dd.LopDays != 200 || dd.PayableDays != 2800 || dd.Gross != calc.Money(9333333) {
		t.Fatalf("G10 D LOP 2 → 28/30: %+v", dd)
	}
	var gross calc.Money
	for _, p := range s {
		gross += p.Gross
	}
	if run.GrossTotal != gross || run.NetTotal != run.GrossTotal-run.DeductionTotal {
		t.Fatalf("totals %s vs %s", run.GrossTotal, gross)
	}
}

// TestGolden_G11_G12_StateMachine — maker-checker and invalid transitions.
func TestGolden_G11_G12_StateMachine(t *testing.T) {
	l := setupSeptember(t)
	svc, ctx := NewRunService(l.h.deps), context.Background()
	if _, err := svc.Finalize(ctx, l.checker, l.run.ID); codeOf(err) != "RUN_STATE" {
		t.Fatalf("G12 finalize draft: %v", err)
	}
	if _, err := svc.Approve(ctx, l.checker, l.run.ID); codeOf(err) != "RUN_STATE" {
		t.Fatalf("G12 approve draft: %v", err)
	}
	_, _ = svc.Calculate(ctx, l.maker, l.run.ID)
	if _, err := svc.Approve(ctx, l.maker, l.run.ID); codeOf(err) != "SELF_APPROVAL_FORBIDDEN" {
		t.Fatalf("G11 maker approves own calculation: %v", err)
	}
	if r, err := svc.Approve(ctx, l.checker, l.run.ID); err != nil || r.Status != models.RunApproved || *r.ApprovedByUserID != l.checker.UserID {
		t.Fatalf("checker approves: %+v %v", r, err)
	}
	if _, err := svc.Calculate(ctx, l.maker, l.run.ID); codeOf(err) != "RUN_STATE" {
		t.Fatalf("G12 recalc after approve: %v", err)
	}
	if r, err := svc.Finalize(ctx, l.checker, l.run.ID); err != nil || r.Status != models.RunFinalized || r.FinalizedAt == nil {
		t.Fatalf("finalize: %+v %v", r, err)
	}
	if _, err := svc.Finalize(ctx, l.checker, l.run.ID); codeOf(err) != "RUN_STATE" {
		t.Fatalf("finalize twice: %v", err)
	}
	if _, err := svc.Calculate(ctx, l.maker, uuid.New()); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("unknown run: %v", err)
	}
}

// TestGolden_G15_Recalculate — a new assignment changes figures; payslips are replaced, not duplicated.
func TestGolden_G15_Recalculate(t *testing.T) {
	l := setupSeptember(t)
	svc, ctx := NewRunService(l.h.deps), context.Background()
	first, _ := svc.Calculate(ctx, l.maker, l.run.ID)
	l.h.assign(t, l.maker, l.a, l.structure, calc.Rupees(2400000), "2026-09-01")
	second, err := svc.Calculate(ctx, l.maker, l.run.ID)
	if err != nil || len(l.h.st.payslips) != 4 || second.GrossTotal != first.GrossTotal+calc.Rupees(100000) {
		t.Fatalf("recalc: %d slips, gross %s → %s, %v", len(l.h.st.payslips), first.GrossTotal, second.GrossTotal, err)
	}
	if tds := lineAmt(l.slips()["A1"], "TDS"); tds <= 0 {
		t.Fatalf("A at 24L must pay TDS, got %s", tds)
	}
}

// TestGolden_SelfVisibilityAndPrivacy — PY-013 and NFR-SEC003 (unit level of G13/G14).
func TestGolden_SelfVisibilityAndPrivacy(t *testing.T) {
	l := setupSeptember(t)
	runs, slips, ctx := NewRunService(l.h.deps), NewPayslipService(l.h.deps), context.Background()
	_, _ = runs.Calculate(ctx, l.maker, l.run.ID)
	page := dto.Page{Page: 1, PerPage: 20}
	if mine, _, _ := slips.Mine(ctx, l.h.actor(l.userA), page); len(mine) != 0 {
		t.Fatal("no payslips before finalize")
	}
	aSlip := l.slips()["A1"]
	if _, err := slips.MineOne(ctx, l.h.actor(l.userA), aSlip.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("unfinalized own payslip must be 404: %v", err)
	}
	_, _ = runs.Approve(ctx, l.checker, l.run.ID)
	_, _ = runs.Finalize(ctx, l.checker, l.run.ID)
	mine, meta, err := slips.Mine(ctx, l.h.actor(l.userA), page)
	if err != nil || len(mine) != 1 || meta.TotalItems != 1 || mine[0].Month != 9 || mine[0].EmployeeCode != "A1" {
		t.Fatalf("mine after finalize: %+v %v", mine, err)
	}
	if one, err := slips.MineOne(ctx, l.h.actor(l.userA), aSlip.ID); err != nil || one.Net != aSlip.Net {
		t.Fatalf("own payslip: %v", err)
	}
	if _, err := slips.MineOne(ctx, l.h.actor(l.userD), aSlip.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("PS-T1 another employee's payslip: %v", err)
	}
	for _, e := range l.h.audit.entries {
		meta, _ := e["meta"].(map[string]string)
		for k, v := range meta {
			if strings.Contains(k, "ctc") || strings.Contains(k, "net") || strings.Contains(v, "100000") {
				t.Fatalf("NFR-SEC003 audit leaks salary: %v", e)
			}
		}
	}
	for _, o := range l.h.st.outbox {
		if strings.Contains(o.Payload, l.a.String()) || strings.Contains(o.Payload, "A1") {
			t.Fatalf("FR-EV002 event names an employee: %s", o.Payload)
		}
	}
	if len(l.h.pub.keys) != 4 {
		t.Fatalf("events published after commit: %v", l.h.pub.keys)
	}
}
