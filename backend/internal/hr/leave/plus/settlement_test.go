package plus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"gorm.io/gorm"
)

// The fake store gains the three settlement queries (payroll final settlement, PP-06).

func (f *fakeStore) LatestBalances(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) ([]TypeDays, error) {
	return f.typeDays, f.fail["latest"]
}

func (f *fakeStore) UnpaidEncashments(_ context.Context, _ *gorm.DB, _, emp uuid.UUID) ([]Encashment, error) {
	var out []Encashment
	for _, e := range f.encash {
		if e.EmployeeProfileID == emp && e.Status == StatusApproved && e.PaidOutAt == nil {
			out = append(out, e)
		}
	}
	return out, f.fail["unpaid"]
}

func (f *fakeStore) MarkPaid(_ context.Context, _ *gorm.DB, _ uuid.UUID, ids []uuid.UUID, at time.Time) (int64, error) {
	if err := f.fail["markpaid"]; err != nil {
		return 0, err
	}
	var n int64
	for _, id := range ids {
		if e, ok := f.encash[id]; ok && e.Status == StatusApproved && e.PaidOutAt == nil {
			e.PaidOutAt = &at
			f.encash[id] = e
			n++
		}
	}
	return n, nil
}

func TestSettlementPort(t *testing.T) {
	e := setup(t)
	port := SettlementPort(e.store)
	e.store.typeDays = []TypeDays{{LeaveTypeID: e.annualType, Days: calc.FromWhole(4)}, {LeaveTypeID: e.compType, Days: 0}, {LeaveTypeID: uuid.New(), Days: -50}}
	bal, err := port.EncashableBalances(ctx(), nil, e.tenant, e.alice)
	if err != nil || len(bal) != 1 || bal[0].LeaveTypeID != e.annualType || bal[0].Days != calc.FromWhole(4) {
		t.Fatalf("only positive balances of encashable types: %+v %v", bal, err)
	}
	e.store.fail["latest"] = errBoom
	if _, err := port.EncashableBalances(ctx(), nil, e.tenant, e.alice); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "latest")

	approved := Encashment{ID: uuid.New(), EmployeeProfileID: e.alice, LeaveTypeID: e.annualType, Days: 150, Status: StatusApproved}
	pending := Encashment{ID: uuid.New(), EmployeeProfileID: e.alice, LeaveTypeID: e.annualType, Days: 50, Status: StatusPending}
	e.store.encash[approved.ID], e.store.encash[pending.ID] = approved, pending
	unpaid, err := port.UnpaidEncashments(ctx(), nil, e.tenant, e.alice)
	if err != nil || len(unpaid) != 1 || unpaid[0].ID != approved.ID || unpaid[0].Days != 150 {
		t.Fatalf("only approved, unpaid encashments: %+v %v", unpaid, err)
	}
	e.store.fail["unpaid"] = errBoom
	if _, err := port.UnpaidEncashments(ctx(), nil, e.tenant, e.alice); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	delete(e.store.fail, "unpaid")

	if err := port.MarkEncashmentsPaid(ctx(), nil, e.tenant, nil, now); err != nil {
		t.Fatal("nothing to mark is fine")
	}
	if err := port.MarkEncashmentsPaid(ctx(), nil, e.tenant, []uuid.UUID{pending.ID}, now); !errors.Is(err, ErrNotPayable) {
		t.Fatalf("a pending encashment cannot be paid: %v", err)
	}
	if err := port.MarkEncashmentsPaid(ctx(), nil, e.tenant, []uuid.UUID{approved.ID}, now); err != nil || e.store.encash[approved.ID].PaidOutAt == nil {
		t.Fatal("approved encashment is stamped", err)
	}
	if err := port.MarkEncashmentsPaid(ctx(), nil, e.tenant, []uuid.UUID{approved.ID}, now); !errors.Is(err, ErrNotPayable) {
		t.Fatalf("paying twice is refused: %v", err)
	}
	e.store.fail["markpaid"] = errBoom
	if err := port.MarkEncashmentsPaid(ctx(), nil, e.tenant, []uuid.UUID{approved.ID}, now); !errors.Is(err, errBoom) {
		t.Fatal(err)
	}
	if m := (&Module{svc: e.svc, cal: e.svc.cal}); m.Settlement() == nil {
		t.Fatal("module exposes the port")
	}
}
