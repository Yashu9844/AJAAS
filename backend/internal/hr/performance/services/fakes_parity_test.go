package services

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/models"
	"github.com/jaas/jaas/internal/hr/performance/repositories"
	"gorm.io/gorm"
)

// --- calibrations ---

type fakeCalibrations struct{ st *store }

func (f fakeCalibrations) Create(_ context.Context, _ *gorm.DB, c *models.Calibration) error {
	if err := f.st.err("cals.create"); err != nil {
		return err
	}
	ensureID(&c.ID)
	c.CreatedAt = f.st.stamp()
	f.st.cals = append(f.st.cals, *c)
	return nil
}

// newest returns the cycle's calibrations newest first.
func (f fakeCalibrations) newest(tenantID, cycleID uuid.UUID) []models.Calibration {
	var out []models.Calibration
	for i := len(f.st.cals) - 1; i >= 0; i-- {
		if c := f.st.cals[i]; c.TenantID == tenantID && c.CycleID == cycleID {
			out = append(out, c)
		}
	}
	return out
}

func (f fakeCalibrations) Latest(_ context.Context, _ *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*models.Calibration, error) {
	if err := f.st.err("cals.latest"); err != nil {
		return nil, err
	}
	for _, c := range f.newest(tenantID, cycleID) {
		if c.EmployeeID == employeeID {
			return &c, nil
		}
	}
	return nil, nil
}
func (f fakeCalibrations) LatestPerEmployee(_ context.Context, _ *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Calibration, error) {
	if err := f.st.err("cals.perEmployee"); err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	var out []models.Calibration
	for _, c := range f.newest(tenantID, cycleID) {
		if !seen[c.EmployeeID] {
			seen[c.EmployeeID] = true
			out = append(out, c)
		}
	}
	return out, nil
}
func (f fakeCalibrations) List(_ context.Context, _ *gorm.DB, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, p dto.Page) ([]models.Calibration, int64, error) {
	if err := f.st.err("cals.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Calibration
	for _, c := range f.newest(tenantID, cycleID) {
		if employeeID == nil || c.EmployeeID == *employeeID {
			out = append(out, c)
		}
	}
	rows, n := pageOf(out, p)
	return rows, n, nil
}

// --- feedback ---

type fakeFeedback struct{ st *store }

func (f fakeFeedback) Create(_ context.Context, _ *gorm.DB, fb *models.Feedback) error {
	if err := f.st.err("feedback.create"); err != nil {
		return err
	}
	ensureID(&fb.ID)
	fb.CreatedAt = f.st.stamp()
	f.st.feedback = append(f.st.feedback, *fb)
	return nil
}
func (f fakeFeedback) List(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, flt repositories.FeedbackFilter, p dto.Page) ([]models.Feedback, int64, error) {
	if err := f.st.err("feedback.list"); err != nil {
		return nil, 0, err
	}
	var out []models.Feedback
	for i := len(f.st.feedback) - 1; i >= 0; i-- {
		fb := f.st.feedback[i]
		if fb.TenantID == tenantID && (flt.ToID == nil || fb.ToEmployeeID == *flt.ToID) && (flt.FromID == nil || fb.FromEmployeeID == *flt.FromID) &&
			(!flt.PublicOnly || fb.Visibility == models.VisibilityPublic) {
			out = append(out, fb)
		}
	}
	rows, n := pageOf(out, p)
	return rows, n, nil
}
func (f fakeFeedback) CreateRequest(_ context.Context, _ *gorm.DB, r *models.FeedbackRequest) error {
	if err := f.st.err("requests.create"); err != nil {
		return err
	}
	ensureID(&r.ID)
	r.CreatedAt = f.st.stamp()
	f.st.requests[r.ID] = *r
	return nil
}
func (f fakeFeedback) UpdateRequest(_ context.Context, _ *gorm.DB, r *models.FeedbackRequest) error {
	if err := f.st.err("requests.update"); err != nil {
		return err
	}
	f.st.requests[r.ID] = *r
	return nil
}
func (f fakeFeedback) LockRequest(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*models.FeedbackRequest, error) {
	if err := f.st.err("requests.lock"); err != nil {
		return nil, err
	}
	if r, ok := f.st.requests[id]; ok && r.TenantID == tenantID {
		return &r, nil
	}
	return nil, nil
}
func (f fakeFeedback) PendingExists(_ context.Context, _ *gorm.DB, tenantID, requester, responder, about uuid.UUID) (bool, error) {
	if err := f.st.err("requests.exists"); err != nil {
		return false, err
	}
	for _, r := range f.st.requests {
		if r.TenantID == tenantID && r.RequesterEmployeeID == requester && r.ResponderEmployeeID == responder && r.AboutEmployeeID == about &&
			r.Status == models.RequestPending {
			return true, nil
		}
	}
	return false, nil
}
func (f fakeFeedback) ListRequests(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, flt repositories.RequestFilter, p dto.Page) ([]models.FeedbackRequest, int64, error) {
	if err := f.st.err("requests.list"); err != nil {
		return nil, 0, err
	}
	var out []models.FeedbackRequest
	for _, r := range f.st.requests {
		if r.TenantID == tenantID && (flt.ResponderID == nil || r.ResponderEmployeeID == *flt.ResponderID) &&
			(flt.RequesterID == nil || r.RequesterEmployeeID == *flt.RequesterID) && (flt.Status == "" || r.Status == flt.Status) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	rows, n := pageOf(out, p)
	return rows, n, nil
}
