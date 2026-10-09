package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/models"
)

func TestStructureService(t *testing.T) {
	h := newHarness()
	admin, ctx := h.actor(uuid.New()), context.Background()
	svc := NewStructureService(h.deps)
	id := h.standardStructure(t, admin)
	got, err := svc.Get(ctx, h.tenant, id)
	if err != nil || len(got.Components) != 3 || got.Components[0].Code != "BASIC" || !got.PFEnabled || !got.Components[1].Taxable {
		t.Fatalf("get: %+v %v", got, err)
	}
	bad := []dto.CreateStructureRequest{
		{Name: "No basic", Components: []dto.ComponentRequest{{Code: "HRA", Name: "H", Kind: "earning", Calc: "fixed", Value: 100}}},
		{Name: got.Name, Components: []dto.ComponentRequest{{Code: "BASIC", Name: "B", Kind: "earning", Calc: "fixed", Value: 100}}},
	}
	if _, err := svc.Create(ctx, admin, bad[0]); codeOf(err) != "VALIDATION_ERROR" || !strings.Contains(err.Error(), "BASIC") {
		t.Fatalf("invalid structure: %v", err)
	}
	if _, err := svc.Create(ctx, admin, bad[1]); codeOf(err) != "CONFLICT" {
		t.Fatalf("duplicate name: %v", err)
	}
	upd, err := svc.Update(ctx, admin, id, dto.UpdateStructureRequest{Name: strp("Renamed"), Description: strp("d"), PFEnabled: boolp(false),
		ESIEnabled: boolp(false), PTEnabled: boolp(false), TDSEnabled: boolp(false)})
	if err != nil || upd.Name != "Renamed" || upd.PFEnabled || upd.TDSEnabled || *upd.Description != "d" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	other, _ := svc.Create(ctx, admin, dto.CreateStructureRequest{Name: "Other", Components: []dto.ComponentRequest{{Code: "BASIC", Name: "B", Kind: "earning", Calc: "fixed", Value: 100}}})
	if _, err := svc.Update(ctx, admin, other.ID, dto.UpdateStructureRequest{Name: strp("renamed")}); codeOf(err) != "CONFLICT" {
		t.Fatalf("rename collision: %v", err)
	}
	if r, err := svc.Deactivate(ctx, admin, id); err != nil || r.Status != models.StructureInactive {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.Deactivate(ctx, admin, id); codeOf(err) != "CONFLICT" {
		t.Fatalf("deactivate twice: %v", err)
	}
	if _, err := svc.Get(ctx, uuid.New(), id); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cross-tenant: %v", err)
	}
	if _, err := svc.Update(ctx, admin, uuid.New(), dto.UpdateStructureRequest{}); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("update unknown: %v", err)
	}
	list, meta, err := svc.List(ctx, h.tenant, models.StructureActive, dto.Page{Page: 1, PerPage: 20})
	if err != nil || len(list) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list active: %d %v", len(list), err)
	}
}

func TestAssignmentService(t *testing.T) {
	h := newHarness()
	admin, ctx := h.actor(uuid.New()), context.Background()
	svc := NewAssignmentService(h.deps)
	sid := h.standardStructure(t, admin)
	user, emp := h.employee("E1", d("2025-01-01"), nil)
	h.assign(t, admin, emp.ID, sid, calc.Rupees(1200000), "2026-01-01")
	h.assign(t, admin, emp.ID, sid, calc.Rupees(1500000), "2026-07-01")
	hist, meta, err := svc.List(ctx, h.tenant, emp.ID.String(), dto.Page{Page: 1, PerPage: 20})
	if err != nil || meta.TotalItems != 2 || hist[0].EffectiveTo != nil || *hist[1].EffectiveTo != "2026-06-30" {
		t.Fatalf("PY-014 history: %+v %v", hist, err)
	}
	mine, err := svc.Mine(ctx, h.actor(user))
	if err != nil || mine.Assignment.AnnualCTC != calc.Rupees(1500000) || mine.Breakdown.MonthlyCTC != calc.Rupees(125000) || mine.Breakdown.Net <= 0 {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	prev, err := svc.Preview(ctx, h.tenant, dto.PreviewRequest{StructureID: sid.String(), AnnualCTC: calc.Rupees(240000)})
	if err != nil || prev.Gross != calc.Rupees(20000) || len(prev.Lines) < 4 {
		t.Fatalf("preview (ESI applies at 20k): %+v %v", prev, err)
	}
	fixed, _ := NewStructureService(h.deps).Create(ctx, admin, dto.CreateStructureRequest{Name: "Fixed", Components: []dto.ComponentRequest{
		{Code: "BASIC", Name: "B", Kind: "earning", Calc: "fixed", Value: calc.Rupees(120000)}}})
	inactive, _ := NewStructureService(h.deps).Create(ctx, admin, dto.CreateStructureRequest{Name: "Gone", Components: []dto.ComponentRequest{
		{Code: "BASIC", Name: "B", Kind: "earning", Calc: "percent_of_ctc", Value: 5000}}})
	_, _ = NewStructureService(h.deps).Deactivate(ctx, admin, inactive.ID)
	cases := []struct {
		req  dto.AssignRequest
		code string
	}{
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: sid.String(), AnnualCTC: calc.Rupees(1), EffectiveFrom: "2026-03-01"}, "ASSIGNMENT_BACKDATED"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: fixed.ID.String(), AnnualCTC: calc.Rupees(600000), EffectiveFrom: "2026-09-01"}, "STRUCTURE_OVERFLOW"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: inactive.ID.String(), AnnualCTC: calc.Rupees(600000), EffectiveFrom: "2026-09-01"}, "STRUCTURE_INACTIVE"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: uuid.NewString(), AnnualCTC: calc.Rupees(600000), EffectiveFrom: "2026-09-01"}, "NOT_FOUND"},
		{dto.AssignRequest{EmployeeID: uuid.NewString(), StructureID: sid.String(), AnnualCTC: calc.Rupees(600000), EffectiveFrom: "2026-09-01"}, "EMPLOYEE_NOT_FOUND"},
		{dto.AssignRequest{EmployeeID: "x", StructureID: sid.String(), AnnualCTC: calc.Rupees(1), EffectiveFrom: "2026-09-01"}, "VALIDATION_ERROR"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: "x", AnnualCTC: calc.Rupees(1), EffectiveFrom: "2026-09-01"}, "VALIDATION_ERROR"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: sid.String(), AnnualCTC: 0, EffectiveFrom: "2026-09-01"}, "VALIDATION_ERROR"},
		{dto.AssignRequest{EmployeeID: emp.ID.String(), StructureID: sid.String(), AnnualCTC: calc.Rupees(1), EffectiveFrom: "bad"}, "VALIDATION_ERROR"},
	}
	for _, c := range cases {
		if _, err := svc.Assign(ctx, admin, c.req); codeOf(err) != c.code {
			t.Errorf("assign %+v: %q want %q", c.req, codeOf(err), c.code)
		}
	}
	if _, err := svc.Preview(ctx, h.tenant, dto.PreviewRequest{StructureID: fixed.ID.String(), AnnualCTC: calc.Rupees(600000)}); codeOf(err) != "STRUCTURE_OVERFLOW" {
		t.Fatalf("preview overflow: %v", err)
	}
	for _, r := range []dto.PreviewRequest{{StructureID: "x", AnnualCTC: 1}, {StructureID: sid.String()}} {
		if _, err := svc.Preview(ctx, h.tenant, r); codeOf(err) != "VALIDATION_ERROR" {
			t.Errorf("preview %+v: %v", r, err)
		}
	}
	if _, _, err := svc.List(ctx, h.tenant, "x", dto.Page{Page: 1, PerPage: 20}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("list bad id: %v", err)
	}
	if _, err := svc.Mine(ctx, h.actor(uuid.New())); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("mine without profile: %v", err)
	}
	other, _ := h.employee("E2", d("2025-01-01"), nil)
	if _, err := svc.Mine(ctx, h.actor(other)); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("mine without assignment: %v", err)
	}
}

func TestRunCreateAndQueries(t *testing.T) {
	l := setupSeptember(t)
	svc, ctx := NewRunService(l.h.deps), context.Background()
	if _, err := svc.Create(ctx, l.maker, dto.CreateRunRequest{Month: 9, Year: 2026}); codeOf(err) != "RUN_EXISTS" {
		t.Fatalf("duplicate period: %v", err)
	}
	if _, err := svc.Create(ctx, l.maker, dto.CreateRunRequest{Month: 11, Year: 2026}); codeOf(err) != "INVALID_PERIOD" {
		t.Fatalf("future period: %v", err)
	}
	if r, err := svc.Get(ctx, l.h.tenant, l.run.ID); err != nil || r.Status != models.RunDraft || r.Warnings == nil {
		t.Fatalf("get: %+v %v", r, err)
	}
	if _, err := svc.Get(ctx, uuid.New(), l.run.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cross-tenant run: %v", err)
	}
	if list, meta, err := svc.List(ctx, l.h.tenant, "", dto.Page{Page: 1, PerPage: 20}); err != nil || len(list) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list: %v", err)
	}
}
