package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/payroll/calc"
	"github.com/jaas/jaas/internal/hr/payroll/dto"
	"github.com/jaas/jaas/internal/hr/payroll/models"
)

func (l *lifecycle) calculate(t *testing.T) {
	t.Helper()
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err != nil {
		t.Fatal(err)
	}
}

// TestParityEmployerLines freezes PP-01 in a real run: employer lines are on the payslip but outside gross, deductions and net.
func TestParityEmployerLines(t *testing.T) {
	l := setupSeptember(t)
	l.calculate(t)
	s := l.slips()
	a := s["A1"]
	if a.Net != calc.Rupees(98000) || a.Gross != calc.Rupees(100000) {
		t.Fatalf("PP-01 employer lines must not touch net: %+v", a)
	}
	want := map[string]calc.Money{"EPF_ER": calc.Rupees(550), "EPS_ER": calc.Rupees(1250), "EDLI_ER": calc.Rupees(75), "PFADM_ER": calc.Rupees(75)}
	for code, amt := range want {
		if got := lineAmt(a, code); got != amt {
			t.Errorf("%s: got %s want %s", code, got, amt)
		}
	}
	if a.EmployerCost != calc.Rupees(1950) {
		t.Fatalf("employer cost %s", a.EmployerCost)
	}
	for _, ln := range a.Lines {
		if ln.Code == "EPF_ER" && ln.Kind != models.KindEmployer {
			t.Fatalf("kind %s", ln.Kind)
		}
	}
	var total calc.Money
	for _, p := range s {
		total += p.EmployerCost
	}
	run := l.h.st.runs[l.run.ID]
	if run.EmployerTotal != total || total == 0 {
		t.Fatalf("run employer total %s vs %s", run.EmployerTotal, total)
	}
	if len(a.Segments) != 1 || a.Segments[0].From != "2026-09-01" || a.Segments[0].To != "2026-09-30" {
		t.Fatalf("one segment recorded: %+v", a.Segments)
	}
}

// TestParityMidMonthChange freezes PG3 in a run: a CTC change on the 16th prices each half at its own CTC.
func TestParityMidMonthChange(t *testing.T) {
	l := setupSeptember(t)
	_, e := l.h.employee("E1", d("2025-01-01"), nil)
	l.h.assign(t, l.maker, e.ID, l.structure, calc.Rupees(1200000), "2026-01-01")
	l.h.assign(t, l.maker, e.ID, l.structure, calc.Rupees(2400000), "2026-09-16")
	l.calculate(t)
	p := l.slips()["E1"]
	if p.Gross != calc.Rupees(150000) || p.PayableDays != 3000 || p.AnnualCTC != calc.Rupees(2400000) {
		t.Fatalf("PG3 payslip: %+v", p)
	}
	if len(p.Segments) != 2 || p.Segments[0].To != "2026-09-15" || p.Segments[1].From != "2026-09-16" || p.Segments[0].AnnualCTC != calc.Rupees(1200000) {
		t.Fatalf("PG3 segments: %+v", p.Segments)
	}
	if lineAmt(p, "BASIC") != calc.Rupees(60000) || lineAmt(p, "PF") != calc.Rupees(1800) {
		t.Fatalf("PG3 merged basic/PF: %s %s", lineAmt(p, "BASIC"), lineAmt(p, "PF"))
	}
	// LOP is asked per segment range, so a worker with LOP only counts it in the segment it falls in.
	l.h.lop.days[e.ID] = 100
	l.calculate(t)
	if got := l.slips()["E1"]; got.LopDays != 200 || got.PayableDays != 2800 {
		t.Fatalf("LOP is summed across the two segments: %+v", got)
	}
}

// TestParityStatePT freezes PG2 in a run: the worker's state picks the slab, February overrides, tenants can replace slabs.
func TestParityStatePT(t *testing.T) {
	l := setupSeptember(t)
	pt := &fakePT{state: map[uuid.UUID]string{l.a: "KA", l.dEmp: "ZZ"}, slabs: map[string][]calc.PTSlab{}}
	l.h.deps.PT = pt
	l.calculate(t)
	s := l.slips()
	if got := lineAmt(s["A1"], "PT"); got != calc.Rupees(200) {
		t.Fatalf("KA in September: %s", got)
	}
	if got := lineAmt(s["D1"], "PT"); got != calc.Rupees(200) {
		t.Fatalf("an unknown state keeps the default slab: %s", got)
	}
	pt.slabs["KA"] = []calc.PTSlab{{MinGross: 0, Amount: calc.Rupees(50)}}
	l.calculate(t)
	if got := lineAmt(l.slips()["A1"], "PT"); got != calc.Rupees(50) {
		t.Fatalf("tenant slabs replace the defaults: %s", got)
	}
	delete(pt.slabs, "KA")

	// February: Karnataka charges 300.
	feb, err := NewRunService(l.h.deps).Create(context.Background(), l.maker, dto.CreateRunRequest{Month: 2, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, feb.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range l.h.st.payslips {
		if p.RunID == feb.ID && p.EmployeeCode == "A1" && lineAmt(p, "PT") != calc.Rupees(300) {
			t.Fatalf("PG2 February override: %s", lineAmt(p, "PT"))
		}
	}
	pt.err = errors.New("states down")
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err == nil {
		t.Fatal("a failing state lookup must fail the calculation")
	}
}

// TestParityBenefits freezes PP-05: benefit deductions come before statutory lines and reduce the net.
func TestParityBenefits(t *testing.T) {
	l := setupSeptember(t)
	l.h.deps.Benefits = &fakeBenefits{lines: map[uuid.UUID][]BenefitLine{l.a: {{Code: "med", Name: "Medical", Amount: calc.Rupees(500)}, {Code: "zero", Name: "None", Amount: 0}}}}
	l.calculate(t)
	a := l.slips()["A1"]
	if lineAmt(a, "BEN_MED") != calc.Rupees(500) || lineAmt(a, "BEN_ZERO") != -1 || a.Net != calc.Rupees(97500) {
		t.Fatalf("PP-05: %+v", a)
	}
	pos := map[string]int{}
	for _, ln := range a.Lines {
		pos[ln.Code] = ln.Position
	}
	if pos["BEN_MED"] > pos["PF"] {
		t.Fatalf("benefits before statutory: %v", pos)
	}
	l.h.deps.Benefits = &fakeBenefits{err: errors.New("benefits down")}
	if _, err := NewRunService(l.h.deps).Calculate(context.Background(), l.maker, l.run.ID); err == nil {
		t.Fatal("a failing benefits lookup must fail the calculation")
	}
}

// runSlips returns one run payslips by employee code.
func (l *lifecycle) runSlips(run uuid.UUID) map[string]models.Payslip {
	out := map[string]models.Payslip{}
	for _, p := range l.h.st.payslips {
		if p.RunID == run {
			out[p.EmployeeCode] = p
		}
	}
	return out
}

// finalize drives a run to finalized (maker calculates, checker approves, PY-012).
func (l *lifecycle) finalize(t *testing.T, run *dto.RunResponse) {
	t.Helper()
	ctx, svc := context.Background(), NewRunService(l.h.deps)
	if _, err := svc.Calculate(ctx, l.maker, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, l.checker, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Finalize(ctx, l.checker, run.ID); err != nil {
		t.Fatal(err)
	}
}

// TestParityArrears freezes PG4: a backdated CTC change is settled once, in the next run, and never doubled.
func TestParityArrears(t *testing.T) {
	l := setupSeptember(t)
	ctx, svc := context.Background(), NewRunService(l.h.deps)
	_, up := l.h.employee("UP1", d("2025-01-01"), nil)
	_, down := l.h.employee("DN1", d("2025-01-01"), nil)
	for _, e := range []uuid.UUID{up.ID, down.ID} {
		l.h.assign(t, l.maker, e, l.structure, calc.Rupees(1200000), "2026-01-01")
	}
	aug, err := svc.Create(ctx, l.maker, dto.CreateRunRequest{Month: 8, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	l.finalize(t, aug)

	// Backdated to 1 August: UP gets a raise, DN a cut.
	l.h.assign(t, l.maker, up.ID, l.structure, calc.Rupees(1320000), "2026-08-01")
	l.h.assign(t, l.maker, down.ID, l.structure, calc.Rupees(1080000), "2026-08-01")
	l.calculate(t)
	s := l.runSlips(l.run.ID)
	u, dn := s["UP1"], s["DN1"]
	if lineAmt(u, calc.ArrearsCode) != calc.Rupees(10000) || u.Gross != calc.Rupees(120000) {
		t.Fatalf("PG4 positive arrears: gross %s arrears %s", u.Gross, lineAmt(u, calc.ArrearsCode))
	}
	if lineAmt(dn, calc.ArrearsRecoveryCode) != calc.Rupees(10000) || dn.Gross != calc.Rupees(90000) {
		t.Fatalf("PG4 negative arrears: gross %s recovery %s", dn.Gross, lineAmt(dn, calc.ArrearsRecoveryCode))
	}
	if lineAmt(s["A1"], calc.ArrearsCode) != -1 {
		t.Fatalf("a worker with unchanged history gets none")
	}
	if len(l.h.st.arrears) != 2 {
		t.Fatalf("one row per settled source: %d", len(l.h.st.arrears))
	}
	// Recalculating replaces the run arrears; nothing doubles.
	l.calculate(t)
	if len(l.h.st.arrears) != 2 || lineAmt(l.runSlips(l.run.ID)["UP1"], calc.ArrearsCode) != calc.Rupees(10000) {
		t.Fatalf("recalculation must not double arrears: %d rows", len(l.h.st.arrears))
	}
	l.finalize(t, l.run)

	// The next run finds August already settled (and September unchanged): no further arrears.
	oct, err := svc.Create(ctx, l.maker, dto.CreateRunRequest{Month: 10, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Calculate(ctx, l.maker, oct.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range l.h.st.payslips {
		if p.RunID == oct.ID && (lineAmt(p, calc.ArrearsCode) != -1 || lineAmt(p, calc.ArrearsRecoveryCode) != -1) {
			t.Fatalf("PG4 settled exactly once, found arrears again on %s", p.EmployeeCode)
		}
	}

	// A failing arrears lookup fails the calculation and rolls back.
	l.h.st.fail["payslip.before"] = errors.New("db down")
	if _, err := svc.Calculate(ctx, l.maker, oct.ID); err == nil {
		t.Fatal("arrears lookup failure must surface")
	}
	delete(l.h.st.fail, "payslip.before")
	for _, key := range []string{"arrears.paid", "arrears.replace", "assignment.foremployee"} {
		l.h.st.fail[key] = errors.New("db down")
		if _, err := svc.Calculate(ctx, l.maker, oct.ID); err == nil {
			t.Fatalf("%s failure must surface", key)
		}
		delete(l.h.st.fail, key)
	}
}

// TestParityArrearsSkipsBrokenStructures keeps the run going and warns when an old month cannot be repriced.
func TestParityArrearsSkipsBrokenStructures(t *testing.T) {
	l := setupSeptember(t)
	ctx, svc := context.Background(), NewRunService(l.h.deps)
	_, e := l.h.employee("OV1", d("2025-01-01"), nil)
	l.h.assign(t, l.maker, e.ID, l.structure, calc.Rupees(1200000), "2026-01-01")
	aug, _ := svc.Create(ctx, l.maker, dto.CreateRunRequest{Month: 8, Year: 2026})
	l.finalize(t, aug)
	// A 1-rupee CTC cannot carry the fixed components: repricing August overflows.
	l.h.assign(t, l.maker, e.ID, l.structure, calc.Rupees(1200000), "2026-08-01")
	for id, as := range l.h.st.assignments {
		if as.EmployeeProfileID == e.ID && as.EffectiveTo == nil {
			as.AnnualCTC = 0
			l.h.st.assignments[id] = as
		}
	}
	run, err := svc.Calculate(ctx, l.maker, l.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != models.RunCalculated {
		t.Fatalf("%+v", run)
	}
}
