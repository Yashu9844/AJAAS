package services

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/repositories"
	"gorm.io/gorm"
)

// --- balances ---

type fakeBalances struct{ st *store }

func (f fakeBalances) LockEmployee(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) error {
	f.st.locks++
	return f.st.err("balance.lock")
}
func (f fakeBalances) FindOrCreate(ctx context.Context, db *gorm.DB, k repositories.BalanceKey) (*models.Balance, bool, error) {
	if err := f.st.err("balance.create"); err != nil {
		return nil, false, err
	}
	if b, _ := f.Find(ctx, db, k); b != nil {
		return b, false, nil
	}
	b := models.Balance{ID: uuid.New(), TenantID: k.TenantID, EmployeeProfileID: k.EmployeeID, LeaveTypeID: k.LeaveTypeID, Year: k.Year}
	f.st.balances[b.ID] = b
	return &b, true, nil
}
func (f fakeBalances) Find(_ context.Context, _ *gorm.DB, k repositories.BalanceKey) (*models.Balance, error) {
	if err := f.st.err("balance.find"); err != nil {
		return nil, err
	}
	for _, b := range f.st.balances {
		if b.TenantID == k.TenantID && b.EmployeeProfileID == k.EmployeeID && b.LeaveTypeID == k.LeaveTypeID && b.Year == k.Year {
			c := b
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeBalances) Update(_ context.Context, _ *gorm.DB, b *models.Balance) error {
	if err := f.st.err("balance.update"); err != nil {
		return err
	}
	f.st.balances[b.ID] = *b
	return nil
}

// --- requests ---

type fakeRequests struct{ st *store }

func (f fakeRequests) Create(_ context.Context, _ *gorm.DB, r *models.Request) error {
	if err := f.st.err("request.create"); err != nil {
		return err
	}
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	f.st.requests[r.ID] = *r
	return nil
}
func (f fakeRequests) Update(_ context.Context, _ *gorm.DB, r *models.Request) error {
	if err := f.st.err("request.update"); err != nil {
		return err
	}
	f.st.requests[r.ID] = *r
	return nil
}
func (f fakeRequests) FindByID(_ context.Context, _ *gorm.DB, tenant, id uuid.UUID) (*models.Request, error) {
	if err := f.st.err("request.find"); err != nil {
		return nil, err
	}
	if r, ok := f.st.requests[id]; ok && r.TenantID == tenant {
		return &r, nil
	}
	return nil, nil
}
func (f fakeRequests) Overlapping(_ context.Context, _ *gorm.DB, tenant, emp uuid.UUID, from, to time.Time) ([]models.Request, error) {
	if err := f.st.err("request.overlap"); err != nil {
		return nil, err
	}
	var out []models.Request
	for _, r := range f.st.requests {
		live := r.Status == models.StatusPending || r.Status == models.StatusApproved
		if r.TenantID == tenant && r.EmployeeProfileID == emp && live && !r.StartDate.After(to) && !r.EndDate.Before(from) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f fakeRequests) List(_ context.Context, _ *gorm.DB, tenant uuid.UUID, fl repositories.RequestFilter, p dto.Page) ([]models.Request, int64, error) {
	if err := f.st.err("request.list"); err != nil {
		return nil, 0, err
	}
	var rows []models.Request
	for _, r := range f.st.requests {
		ok := r.TenantID == tenant && (fl.EmployeeID == nil || r.EmployeeProfileID == *fl.EmployeeID) &&
			(fl.Status == "" || r.Status == fl.Status) && (fl.From == nil || !r.EndDate.Before(*fl.From)) &&
			(fl.To == nil || !r.StartDate.After(*fl.To))
		if ok {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].StartDate.After(rows[j].StartDate) })
	out, total := pageOf(rows, p)
	return out, total, nil
}

// --- ledger + outbox ---

type fakeLedger struct{ st *store }

func (f fakeLedger) Create(_ context.Context, _ *gorm.DB, e *models.LedgerEntry) error {
	if err := f.st.err("ledger.create"); err != nil {
		return err
	}
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	f.st.ledger = append(f.st.ledger, *e)
	return nil
}
func (f fakeLedger) List(_ context.Context, _ *gorm.DB, tenant uuid.UUID, fl repositories.LedgerFilter, p dto.Page) ([]models.LedgerEntry, int64, error) {
	if err := f.st.err("ledger.list"); err != nil {
		return nil, 0, err
	}
	var rows []models.LedgerEntry
	for _, e := range f.st.ledger {
		if e.TenantID == tenant && e.EmployeeProfileID == fl.EmployeeID && (fl.LeaveTypeID == nil || e.LeaveTypeID == *fl.LeaveTypeID) {
			rows = append(rows, e)
		}
	}
	out, total := pageOf(rows, p)
	return out, total, nil
}

type fakeOutbox struct{ st *store }

func (f fakeOutbox) Create(_ context.Context, _ *gorm.DB, e *models.OutboxEvent) error {
	if err := f.st.err("outbox.create"); err != nil {
		return err
	}
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	f.st.outbox[e.ID] = *e
	return nil
}
func (f fakeOutbox) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, at time.Time) error {
	if e, ok := f.st.outbox[id]; ok {
		e.Published, e.PublishedAt = true, &at
		f.st.outbox[id] = e
	}
	return f.st.err("outbox.mark")
}
func (f fakeOutbox) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	if e, ok := f.st.outbox[id]; ok {
		e.Attempts++
		e.LastError = &reason
		f.st.outbox[id] = e
	}
	return nil
}
func (f fakeOutbox) FetchUnpublished(_ context.Context, _ *gorm.DB, limit, max int) ([]models.OutboxEvent, error) {
	if err := f.st.err("outbox.fetch"); err != nil {
		return nil, err
	}
	var out []models.OutboxEvent
	for _, e := range f.st.outbox {
		if !e.Published && e.Attempts < max && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}
