package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/calc"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
)

func TestTypeService_Lifecycle(t *testing.T) {
	h := newHarness()
	svc, ctx, admin := NewTypeService(h.deps), context.Background(), h.actor(uuid.New())
	el, err := svc.Create(ctx, admin, dto.CreateLeaveTypeRequest{Name: "Earned", Code: " el ", AnnualAllowance: 1800, CarryForwardLimit: 500})
	if err != nil || el.Code != "EL" || !el.IsPaid || el.Accrual != "annual" || !el.AllowHalfDay || el.ApplicableGender != "all" || el.Status != "active" {
		t.Fatalf("create defaults: %+v %v", el, err)
	}
	dup := []struct {
		req  dto.CreateLeaveTypeRequest
		code string
	}{
		{dto.CreateLeaveTypeRequest{Name: "earned", Code: "EL2"}, "CONFLICT"},
		{dto.CreateLeaveTypeRequest{Name: "Other", Code: "el"}, "CONFLICT"},
		{dto.CreateLeaveTypeRequest{Name: "Bad", Code: "e-l"}, "VALIDATION_ERROR"},
		{dto.CreateLeaveTypeRequest{Name: "Big", Code: "BIG", AnnualAllowance: 36600}, "VALIDATION_ERROR"},
		{dto.CreateLeaveTypeRequest{Name: "Neg", Code: "NEG", CarryForwardLimit: -1}, "VALIDATION_ERROR"},
	}
	for _, d := range dup {
		if _, err := svc.Create(ctx, admin, d.req); codeOf(err) != d.code {
			t.Errorf("create %+v: %v", d.req, err)
		}
	}
	upd, err := svc.Update(ctx, admin, el.ID, dto.UpdateLeaveTypeRequest{Name: strp("Earned Leave"), IsPaid: boolp(false),
		Accrual: strp("monthly"), ApplicableGender: strp("female"), AllowHalfDay: boolp(false), SandwichRule: boolp(true),
		MinNoticeDays: intp(3), MaxConsecutiveDays: intp(10), AnnualAllowance: daysp(1200), CarryForwardLimit: daysp(0)})
	if err != nil || upd.Name != "Earned Leave" || upd.IsPaid || upd.Accrual != "monthly" || *upd.MaxConsecutiveDays != 10 || upd.Code != "EL" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if upd, _ = svc.Update(ctx, admin, el.ID, dto.UpdateLeaveTypeRequest{MaxConsecutiveDays: intp(0)}); upd.MaxConsecutiveDays != nil {
		t.Fatal("max 0 clears the limit")
	}
	other, _ := svc.Create(ctx, admin, dto.CreateLeaveTypeRequest{Name: "Sick", Code: "SL"})
	if _, err := svc.Update(ctx, admin, other.ID, dto.UpdateLeaveTypeRequest{Name: strp("earned leave")}); codeOf(err) != "CONFLICT" {
		t.Fatalf("rename onto existing: %v", err)
	}
	if _, err := svc.Update(ctx, admin, other.ID, dto.UpdateLeaveTypeRequest{AnnualAllowance: daysp(-5)}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad allowance update: %v", err)
	}
	if _, err := svc.Update(ctx, admin, uuid.New(), dto.UpdateLeaveTypeRequest{}); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("update unknown: %v", err)
	}
	if res, err := svc.Deactivate(ctx, admin, el.ID); err != nil || res.Status != "inactive" {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.Deactivate(ctx, admin, el.ID); codeOf(err) != "CONFLICT" {
		t.Fatalf("deactivate twice: %v", err)
	}
	if _, err := svc.Deactivate(ctx, admin, uuid.New()); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("deactivate unknown: %v", err)
	}
	if got, err := svc.Get(ctx, h.tenant, other.ID); err != nil || got.Code != "SL" {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Get(ctx, uuid.New(), other.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("cross-tenant get: %v", err)
	}
	list, meta, err := svc.List(ctx, h.tenant, "active", dto.Page{Page: 1, PerPage: 20})
	if err != nil || len(list) != 1 || meta.TotalItems != 1 {
		t.Fatalf("list active: %d %v", len(list), err)
	}
}

func TestHolidayService(t *testing.T) {
	h := newHarness()
	svc, ctx, admin := NewHolidayService(h.deps), context.Background(), h.actor(uuid.New())
	hol, err := svc.Create(ctx, admin, dto.CreateHolidayRequest{Date: "2026-12-25", Name: "Christmas"})
	if err != nil || hol.Date != "2026-12-25" {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Create(ctx, admin, dto.CreateHolidayRequest{Date: "2026-12-25", Name: "Dup"}); codeOf(err) != "CONFLICT" {
		t.Fatalf("dup: %v", err)
	}
	if _, err := svc.Create(ctx, admin, dto.CreateHolidayRequest{Date: "25-12", Name: "Bad"}); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad date: %v", err)
	}
	_, _ = svc.Create(ctx, admin, dto.CreateHolidayRequest{Date: "2027-01-01", Name: "NY"})
	if list, err := svc.List(ctx, h.tenant, ""); err != nil || len(list) != 1 {
		t.Fatalf("default year list: %d %v", len(list), err)
	}
	if list, _ := svc.List(ctx, h.tenant, "2027"); len(list) != 1 || list[0].Name != "NY" {
		t.Fatalf("2027 list: %+v", list)
	}
	if _, err := svc.List(ctx, h.tenant, "abc"); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad year: %v", err)
	}
	if _, err := svc.Delete(ctx, admin, hol.ID); err != nil || len(h.st.holidays) != 1 {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Delete(ctx, admin, hol.ID); codeOf(err) != "NOT_FOUND" {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestBalanceService(t *testing.T) {
	h := newHarness()
	user, emp := h.employee()
	el := h.leaveType("EL", 1200, func(t *models.LeaveType) { t.CarryForwardLimit = 500 })
	h.leaveType("SL", 1800, func(t *models.LeaveType) { t.Accrual = models.AccrualMonthly })
	prev := models.Balance{ID: uuid.New(), TenantID: h.tenant, EmployeeProfileID: emp.ID, LeaveTypeID: el.ID, Year: 2025, Accrued: 800}
	h.st.balances[prev.ID] = prev
	svc, ctx := NewBalanceService(h.deps), context.Background()
	mine, err := svc.Mine(ctx, h.actor(user))
	if err != nil || len(mine) != 2 {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	byCode := map[string]dto.BalanceResponse{mine[0].LeaveTypeCode: mine[0], mine[1].LeaveTypeCode: mine[1]}
	if b := byCode["EL"]; b.Opening != 500 || b.Accrued != 1200 || b.Available != 1700 {
		t.Fatalf("EL carry-forward G4 + annual accrual: %+v", b)
	}
	if b := byCode["SL"]; b.Accrued != 1500 || b.Opening != 0 {
		t.Fatalf("SL monthly 18 through October = 15: %+v", b)
	}
	_, _ = svc.Mine(ctx, h.actor(user))
	if k := ledgerKinds(h); k[models.KindAccrual] != 2700 || k[models.KindCarryForward] != 500 {
		t.Fatalf("materialization is idempotent: %+v", k)
	}
	admin := h.actor(uuid.New())
	if _, err := svc.ForEmployee(ctx, admin, emp.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ForEmployee(ctx, admin, uuid.NewString()); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("unknown employee: %v", err)
	}
	if _, err := svc.ForEmployee(ctx, admin, "nope"); codeOf(err) != "VALIDATION_ERROR" {
		t.Fatalf("bad id: %v", err)
	}
	adj, err := svc.Adjust(ctx, admin, dto.AdjustBalanceRequest{EmployeeID: emp.ID.String(), LeaveTypeID: el.ID.String(), Days: -250, Reason: "correction"})
	if err != nil || adj.Adjusted != -250 || adj.Available != 1450 {
		t.Fatalf("adjust: %+v %v", adj, err)
	}
	for _, bad := range []dto.AdjustBalanceRequest{
		{EmployeeID: emp.ID.String(), LeaveTypeID: el.ID.String(), Days: 0},
		{EmployeeID: emp.ID.String(), LeaveTypeID: uuid.NewString(), Days: 100},
		{EmployeeID: uuid.NewString(), LeaveTypeID: el.ID.String(), Days: 100},
	} {
		if _, err := svc.Adjust(ctx, admin, bad); err == nil {
			t.Errorf("adjust %+v must fail", bad)
		}
	}
	rows, meta, err := svc.Ledger(ctx, h.tenant, LedgerQuery{EmployeeID: emp.ID.String(), LeaveTypeID: el.ID.String()}, dto.Page{Page: 1, PerPage: 20})
	if err != nil || meta.TotalItems != 3 || len(rows) != 3 {
		t.Fatalf("ledger EL: %d %v", meta.TotalItems, err)
	}
	for _, q := range []LedgerQuery{{EmployeeID: "x"}, {EmployeeID: emp.ID.String(), LeaveTypeID: "y"}} {
		if _, _, err := svc.Ledger(ctx, h.tenant, q, dto.Page{Page: 1, PerPage: 20}); codeOf(err) != "VALIDATION_ERROR" {
			t.Errorf("ledger %+v: %v", q, err)
		}
	}
	if _, err := svc.Mine(ctx, h.actor(uuid.New())); codeOf(err) != "EMPLOYEE_NOT_FOUND" {
		t.Fatalf("mine unknown: %v", err)
	}
}

func daysp(d calc.Days) *calc.Days { return &d }
