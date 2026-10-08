package services

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/attendance/calc"
	"github.com/jaas/jaas/internal/attendance/dto"
	"github.com/jaas/jaas/internal/attendance/models"
	"github.com/jaas/jaas/internal/attendance/validators"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Shift defaults (FR-SH001).
const (
	defaultGraceMins = 15
	defaultBreakMins = 60
)

// ShiftService manages shift templates (FR-SH001..FR-SH006).
type ShiftService interface {
	Create(ctx context.Context, a Actor, req dto.CreateShiftRequest) (*dto.ShiftResponse, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ShiftResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.ShiftResponse, dto.PageMeta, error)
	Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateShiftRequest) (*dto.ShiftResponse, error)
	Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.ShiftResponse, error)
}

type shiftService struct{ base }

// NewShiftService builds a ShiftService.
func NewShiftService(d Deps) ShiftService { return &shiftService{base{d}} }

func (s *shiftService) Create(ctx context.Context, a Actor, req dto.CreateShiftRequest) (*dto.ShiftResponse, error) {
	shift := &models.Shift{Name: strings.TrimSpace(req.Name), Code: req.Code, GracePeriodMins: defaultGraceMins,
		BreakDurationMins: defaultBreakMins, FullDayMinutes: req.FullDayMinutes, HalfDayMinutes: req.HalfDayMinutes,
		Timezone: req.Timezone, Status: models.ShiftActive}
	shift.TenantID = a.TenantID
	if req.GracePeriodMins != nil {
		shift.GracePeriodMins = *req.GracePeriodMins
	}
	if req.BreakDurationMins != nil {
		shift.BreakDurationMins = *req.BreakDurationMins
	}
	if err := applyTimes(shift, &req.StartTime, &req.EndTime); err != nil {
		return nil, err
	}
	if err := s.validate(ctx, shift); err != nil {
		return nil, err
	}
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repos.Shifts.Create(ctx, tx, shift) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"shift.created", "shift", shift.ID.String(), map[string]string{"name": shift.Name}}, nil)
	return mapShift(shift), nil
}

// applyTimes parses HH:MM pointers onto the shift and derives is_night_shift (FR-SH003, AT-013).
func applyTimes(shift *models.Shift, start, end *string) error {
	if start != nil {
		m, err := calc.ParseHHMM(*start)
		if err != nil {
			return invalid("start_time", err.Error())
		}
		shift.StartMinute = m
	}
	if end != nil {
		m, err := calc.ParseHHMM(*end)
		if err != nil {
			return invalid("end_time", err.Error())
		}
		shift.EndMinute = m
	}
	if shift.StartMinute == shift.EndMinute {
		return invalid("end_time", "must differ from start_time")
	}
	shift.IsNightShift = calc.IsNightShift(shift.StartMinute, shift.EndMinute)
	return nil
}

// validate checks code, timezone, thresholds and per-tenant uniqueness (FR-SH002, AT-013).
func (s *shiftService) validate(ctx context.Context, shift *models.Shift) error {
	if shift.Code != nil {
		if err := validators.ValidateShiftCode(*shift.Code); err != nil {
			return invalid("code", err.Error())
		}
	}
	loc, err := validators.LoadTimezone(shift.Timezone)
	if err != nil {
		return invalid("timezone", err.Error())
	}
	shift.Timezone = loc.String()
	if err := validators.ValidateThresholds(shift.FullDayMinutes, shift.HalfDayMinutes); err != nil {
		return invalid("half_day_minutes", err.Error())
	}
	same, err := s.Repos.Shifts.FindByName(ctx, s.Tx.DB(), shift.TenantID, shift.Name)
	if err != nil {
		return err
	}
	if same != nil && same.ID != shift.ID {
		return ErrShiftNameTaken
	}
	if shift.Code != nil && shift.ID == uuid.Nil {
		dup, err := s.Repos.Shifts.FindByCode(ctx, s.Tx.DB(), shift.TenantID, *shift.Code)
		if err != nil {
			return err
		}
		if dup != nil {
			return ErrShiftCodeTaken
		}
	}
	return nil
}

func (s *shiftService) load(ctx context.Context, tenantID, id uuid.UUID) (*models.Shift, error) {
	shift, err := s.Repos.Shifts.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return shift, nil
}

func (s *shiftService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ShiftResponse, error) {
	shift, err := s.load(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return mapShift(shift), nil
}

func (s *shiftService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.ShiftResponse, dto.PageMeta, error) {
	if status != "" && status != models.ShiftActive && status != models.ShiftInactive {
		return nil, dto.PageMeta{}, invalid("status", "must be active or inactive")
	}
	rows, total, err := s.Repos.Shifts.List(ctx, s.Tx.DB(), tenantID, status, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	out := make([]dto.ShiftResponse, len(rows))
	for i := range rows {
		out[i] = *mapShift(&rows[i])
	}
	return out, page.Meta(total), nil
}

// Update applies PATCH fields; code is immutable (FR-SH005); existing records are untouched (AT-021).
func (s *shiftService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateShiftRequest) (*dto.ShiftResponse, error) {
	shift, err := s.load(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		shift.Name = strings.TrimSpace(*req.Name)
	}
	if req.GracePeriodMins != nil {
		shift.GracePeriodMins = *req.GracePeriodMins
	}
	if req.BreakDurationMins != nil {
		shift.BreakDurationMins = *req.BreakDurationMins
	}
	if req.FullDayMinutes != nil {
		shift.FullDayMinutes = req.FullDayMinutes
	}
	if req.HalfDayMinutes != nil {
		shift.HalfDayMinutes = req.HalfDayMinutes
	}
	if req.Timezone != nil {
		shift.Timezone = *req.Timezone
	}
	if err := applyTimes(shift, req.StartTime, req.EndTime); err != nil {
		return nil, err
	}
	if err := s.validate(ctx, shift); err != nil {
		return nil, err
	}
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repos.Shifts.Update(ctx, tx, shift) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"shift.updated", "shift", shift.ID.String(), map[string]string{"name": shift.Name}}, nil)
	return mapShift(shift), nil
}

// Deactivate is blocked while assignments cover today (shift zone) or later (AT-012).
func (s *shiftService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.ShiftResponse, error) {
	shift, err := s.load(ctx, a.TenantID, id)
	if err != nil {
		return nil, err
	}
	if shift.Status == models.ShiftInactive {
		return nil, ErrShiftAlreadyInactive
	}
	local := s.Now().In(specFor(shift).Location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		n, err := s.Repos.Assignments.CountCoveringFrom(ctx, tx, a.TenantID, id, today)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrShiftInUse
		}
		shift.Status = models.ShiftInactive
		return s.Repos.Shifts.Update(ctx, tx, shift)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"shift.deactivated", "shift", shift.ID.String(), nil}, nil)
	return mapShift(shift), nil
}
