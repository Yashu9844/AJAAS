package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/leave/dto"
	"github.com/jaas/jaas/internal/leave/models"
	"github.com/jaas/jaas/internal/leave/validators"
	"gorm.io/gorm"
)

// HolidayService manages the tenant holiday calendar (FR-HD001..HD003).
type HolidayService interface {
	Create(ctx context.Context, a Actor, req dto.CreateHolidayRequest) (*dto.HolidayResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, year string) ([]dto.HolidayResponse, error)
	Delete(ctx context.Context, a Actor, id uuid.UUID) (*dto.HolidayResponse, error)
}

type holidayService struct{ base }

// NewHolidayService builds a HolidayService.
func NewHolidayService(d Deps) HolidayService { return &holidayService{base{d}} }

func (s *holidayService) Create(ctx context.Context, a Actor, req dto.CreateHolidayRequest) (*dto.HolidayResponse, error) {
	date, err := validators.ParseDate(req.Date)
	if err != nil {
		return nil, invalid("date", err.Error())
	}
	h := &models.Holiday{TenantID: a.TenantID, HolidayDate: date, Name: req.Name, IsOptional: req.IsOptional}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		existing, err := s.Repos.Holidays.FindByDate(ctx, tx, a.TenantID, date)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrHolidayTaken
		}
		return s.Repos.Holidays.Create(ctx, tx, h)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.holiday_created", "leave_holiday", h.ID.String(), map[string]string{"date": req.Date}}, nil)
	res := mapHoliday(h)
	return &res, nil
}

// List returns one year's holidays (default: current year).
func (s *holidayService) List(ctx context.Context, tenantID uuid.UUID, year string) ([]dto.HolidayResponse, error) {
	y, err := validators.ParseYear(year, s.today().Year())
	if err != nil {
		return nil, invalid("year", err.Error())
	}
	from := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows, err := s.Repos.Holidays.ListRange(ctx, s.Tx.DB(), tenantID, from, from.AddDate(1, 0, -1))
	if err != nil {
		return nil, err
	}
	out := make([]dto.HolidayResponse, len(rows))
	for i := range rows {
		out[i] = mapHoliday(&rows[i])
	}
	return out, nil
}

// Delete soft-deletes a holiday; existing requests keep their snapshot (LV-012).
func (s *holidayService) Delete(ctx context.Context, a Actor, id uuid.UUID) (*dto.HolidayResponse, error) {
	var h *models.Holiday
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if h, err = s.Repos.Holidays.FindByID(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if h == nil {
			return ErrNotFound
		}
		return s.Repos.Holidays.Delete(ctx, tx, h)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"leave.holiday_deleted", "leave_holiday", h.ID.String(), nil}, nil)
	res := mapHoliday(h)
	return &res, nil
}
