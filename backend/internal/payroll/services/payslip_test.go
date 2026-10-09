package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
)

func finalize(t *testing.T, l *lifecycle) {
	t.Helper()
	svc, ctx := NewRunService(l.h.deps), context.Background()
	for _, step := range []func() (*dto.RunResponse, error){
		func() (*dto.RunResponse, error) { return svc.Calculate(ctx, l.maker, l.run.ID) },
		func() (*dto.RunResponse, error) { return svc.Approve(ctx, l.checker, l.run.ID) },
		func() (*dto.RunResponse, error) { return svc.Finalize(ctx, l.checker, l.run.ID) },
	} {
		if _, err := step(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPayslipQueriesAndCSV(t *testing.T) {
	l := setupSeptember(t)
	svc, ctx, page := NewPayslipService(l.h.deps), context.Background(), dto.Page{Page: 1, PerPage: 20}
	_, _ = NewRunService(l.h.deps).Calculate(ctx, l.maker, l.run.ID)
	if _, err := svc.PayoutCSV(ctx, l.h.tenant, l.run.ID); codeOf(err) != "RUN_STATE" {
		t.Fatalf("CSV before finalize: %v", err)
	}
	_, _ = NewRunService(l.h.deps).Approve(ctx, l.checker, l.run.ID)
	_, _ = NewRunService(l.h.deps).Finalize(ctx, l.checker, l.run.ID)
	slips, meta, err := svc.ByRun(ctx, l.h.tenant, l.run.ID, page)
	if err != nil || meta.TotalItems != 4 || len(slips[0].Lines) == 0 {
		t.Fatalf("by run: %d %v", meta.TotalItems, err)
	}
	if one, err := svc.Get(ctx, l.h.tenant, slips[0].ID); err != nil || one.ID != slips[0].ID {
		t.Fatalf("get: %v", err)
	}
	for _, call := range []func() error{
		func() error { _, err := svc.Get(ctx, uuid.New(), slips[0].ID); return err },
		func() error { _, _, err := svc.ByRun(ctx, l.h.tenant, uuid.New(), page); return err },
		func() error { _, err := svc.PayoutCSV(ctx, l.h.tenant, uuid.New()); return err },
	} {
		if codeOf(call()) != "NOT_FOUND" {
			t.Error("unknown ids must be 404")
		}
	}
	// PS-T8: a hostile employee name is neutralized in the export
	p := l.h.st.payslips[slips[0].ID]
	p.EmployeeName = "=HYPERLINK(\"x\")"
	l.h.st.payslips[p.ID] = p
	csv, err := svc.PayoutCSV(ctx, l.h.tenant, l.run.ID)
	text := string(csv)
	if err != nil || !strings.HasPrefix(text, "employee_code,employee_name,net_pay\n") || strings.Count(text, "\n") != 5 || !strings.Contains(text, `'=HYPERLINK`) {
		t.Fatalf("csv: %q %v", text, err)
	}
	if _, _, err := svc.Mine(ctx, l.h.actor(uuid.New()), page); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("mine without profile: %v", err)
	}
	if _, err := svc.MineOne(ctx, l.h.actor(uuid.New()), slips[0].ID); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("mine one without profile: %v", err)
	}
}

// Warnings: missing employee and an overflowing assignment are skipped, not fatal; net cap is reported.
func TestCalculateWarnings(t *testing.T) {
	l := setupSeptember(t)
	fixed, _ := NewStructureService(l.h.deps).Create(context.Background(), l.maker, dto.CreateStructureRequest{Name: "Fixed", Components: []dto.ComponentRequest{
		{Code: "BASIC", Name: "B", Kind: "earning", Calc: "fixed", Value: calc.Rupees(10000)},
		{Code: "LOAN", Name: "Loan", Kind: "deduction", Calc: "fixed", Value: calc.Rupees(20000)}}})
	_, overflow := l.h.employee("O1", d("2025-01-01"), nil)
	_, capped := l.h.employee("K1", d("2025-01-01"), nil)
	ghost := uuid.New()
	for emp, ctc := range map[uuid.UUID]calc.Money{overflow.ID: calc.Rupees(60000), capped.ID: calc.Rupees(120000), ghost: calc.Rupees(120000)} {
		a := models.Assignment{ID: uuid.New(), TenantID: l.h.tenant, EmployeeProfileID: emp, StructureID: fixed.ID, AnnualCTC: ctc, EffectiveFrom: d("2026-01-01")}
		l.h.st.assignments[a.ID] = a
	}
	run, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID)
	if err != nil || run.EmployeeCount != 5 || len(run.Warnings) != 3 {
		t.Fatalf("warnings: %+v %v", run, err)
	}
	joined := strings.Join(run.Warnings, ",")
	for _, want := range []string{"STRUCTURE_OVERFLOW:O1", "NET_CAPPED:K1", "EMPLOYEE_MISSING:" + ghost.String()} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %s in %s", want, joined)
		}
	}
}

// Window: an employee who left before the month gets no payslip and no warning.
func TestCalculateSkipsOutsideWindow(t *testing.T) {
	l := setupSeptember(t)
	left := d("2026-08-31")
	_, emp := l.h.employee("X1", d("2025-01-01"), &left)
	a := models.Assignment{ID: uuid.New(), TenantID: l.h.tenant, EmployeeProfileID: emp.ID, StructureID: l.structure, AnnualCTC: calc.Rupees(1200000), EffectiveFrom: d("2026-01-01")}
	l.h.st.assignments[a.ID] = a
	run, _ := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID)
	if run.EmployeeCount != 4 || len(run.Warnings) != 0 {
		t.Fatalf("left employee must be skipped silently: %+v", run)
	}
}

// Every repository/port failure inside a command surfaces and rolls back.
func TestPayrollFaultInjection(t *testing.T) {
	ops := []string{"run.lock", "run.update", "outbox.create", "assignment.eligible", "payslip.delete", "payslip.create", "structure.components"}
	for _, op := range ops {
		l := setupSeptember(t)
		l.h.st.fail[op] = errors.New("boom:" + op)
		before := len(l.h.st.payslips)
		if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err == nil {
			t.Errorf("%s: calculate must fail", op)
		}
		if len(l.h.st.payslips) != before || l.h.st.runs[l.run.ID].Status != models.RunDraft {
			t.Errorf("%s: must roll back", op)
		}
	}
	l := setupSeptember(t)
	l.h.lop.err = errors.New("leave down")
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err == nil {
		t.Fatal("Module 4 failure must fail the calculation")
	}
	l.h.lop.err, l.h.emps.err = nil, errors.New("directory down")
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err == nil {
		t.Fatal("Module 2 failure must fail the calculation")
	}
}

func TestOutboxRelayAndRunner(t *testing.T) {
	l := setupSeptember(t)
	l.h.pub.fail = true
	finalize(t, l)
	relay := NewOutboxRelay(l.h.deps)
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("broker down: %d %v", n, err)
	}
	l.h.pub.fail = false
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 3 { // calculated, approved, finalized (run_initiated was published in setup)
		t.Fatalf("relay: %d %v", n, err)
	}
	l.h.st.fail["outbox.fetch"] = errors.New("db down")
	if _, err := relay.RelayOnce(context.Background()); err == nil {
		t.Fatal("fetch failure must surface")
	}
	relay.interval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	<-done
	if NewTxRunner(nil).DB() != nil {
		t.Fatal("runner DB passthrough")
	}
}
