package plus

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"gorm.io/gorm"
)

// RuleInput sets a leave type's encashment rule (LP-12).
type RuleInput struct {
	MaxDaysPerYear  calc.Days `json:"max_days_per_year"`
	MinBalanceAfter calc.Days `json:"min_balance_after"`
}

// SetRule is LP-12.
func (s *Service) SetRule(ctx context.Context, a Actor, typeID string, in RuleInput) (*EncashmentRule, error) {
	id, err := uuid.Parse(typeID)
	if err != nil {
		return nil, invalid("leave_type_id", "must be a UUID")
	}
	if in.MaxDaysPerYear < 0 || in.MaxDaysPerYear > 36500 || in.MaxDaysPerYear%50 != 0 || in.MinBalanceAfter < 0 || in.MinBalanceAfter > 36500 {
		return nil, invalid("rule", "max_days_per_year is 0–365 in half days and min_balance_after is ≥ 0")
	}
	if exists, _, err := s.store.TypeState(ctx, s.store.DB(), a.TenantID, id); err != nil || !exists {
		if err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	r := &EncashmentRule{TenantID: a.TenantID, LeaveTypeID: id, MaxDaysPerYear: in.MaxDaysPerYear, MinBalanceAfter: in.MinBalanceAfter,
		UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveEncashmentRule(ctx, tx, r) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.encashment_rule_saved", "leave_encashment_rule", id.String(), nil)
	return r, nil
}

// Rules lists the rules.
func (s *Service) Rules(ctx context.Context, a Actor) ([]EncashmentRule, error) {
	rows, err := s.store.EncashmentRules(ctx, s.store.DB(), a.TenantID)
	if rows == nil {
		rows = []EncashmentRule{}
	}
	return rows, err
}

// EncashInput requests an encashment (LP-13).
type EncashInput struct {
	LeaveTypeID string    `json:"leave_type_id"`
	Days        calc.Days `json:"days"`
	Reason      *string   `json:"reason"`
}

// check applies LP-13's limits for `days` of one worker and type; pendingOthers is the days already reserved by other
// pending requests of the same worker and type that the check must respect.
func (s *Service) check(ctx context.Context, tx *gorm.DB, a Actor, employeeID, typeID uuid.UUID, year int, days calc.Days, includePending bool) error {
	rule, err := s.store.EncashmentRule(ctx, tx, a.TenantID, typeID)
	if err != nil {
		return err
	}
	if rule == nil {
		return ErrNotEncashable
	}
	statuses := []string{StatusApproved}
	if includePending {
		statuses = append(statuses, StatusPending)
	}
	used, err := s.store.EncashedDays(ctx, tx, a.TenantID, employeeID, typeID, year, statuses...)
	if err != nil {
		return err
	}
	if used+days > rule.MaxDaysPerYear {
		return ErrEncashLimit
	}
	avail, err := s.p.Mover.AvailableTx(ctx, tx, a.leaveActor(), employeeID, typeID)
	if err != nil {
		return err
	}
	if avail-days < rule.MinBalanceAfter {
		return ErrInsufficient
	}
	return nil
}

// RequestEncashment is LP-13.
func (s *Service) RequestEncashment(ctx context.Context, a Actor, in EncashInput) (*Encashment, error) {
	me, err := s.worker(ctx, a)
	if err != nil {
		return nil, err
	}
	typeID, err := uuid.Parse(in.LeaveTypeID)
	if err != nil {
		return nil, invalid("leave_type_id", "must be a UUID")
	}
	if in.Days <= 0 || in.Days > 36500 || in.Days%50 != 0 {
		return nil, invalid("days", "must be a positive multiple of 0.5")
	}
	reason, err := optionalText("reason", in.Reason, 500)
	if err != nil {
		return nil, err
	}
	exists, active, err := s.store.TypeState(ctx, s.store.DB(), a.TenantID, typeID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if !active {
		return nil, ErrInactiveType
	}
	now := s.now().UTC()
	e := &Encashment{TenantID: a.TenantID, EmployeeProfileID: me.ID, LeaveTypeID: typeID, Days: in.Days, Reason: reason, Status: StatusPending,
		RequestedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if e.Year, err = s.yearOfToday(ctx, tx, a.TenantID); err != nil {
			return err
		}
		if dup, err := s.store.PendingEncashment(ctx, tx, a.TenantID, me.ID, typeID); err != nil || dup != nil {
			if err != nil {
				return err
			}
			return ErrConflict
		}
		if err := s.check(ctx, tx, a, me.ID, typeID, e.Year, in.Days, true); err != nil {
			return err
		}
		return s.store.Create(ctx, tx, e)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.encashment_requested", "leave_encashment", e.ID.String(), map[string]string{"leave_type_id": typeID.String()})
	return e, nil
}

func (s *Service) yearOfToday(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) (int, error) {
	m, err := s.cal.YearStartMonth(ctx, tx, tenantID)
	if err != nil {
		return 0, err
	}
	return calc.YearOf(s.today(), m), nil
}

// DecideEncashment is LP-14: approve (re-check and debit), reject (comment required) or cancel (own pending).
func (s *Service) DecideEncashment(ctx context.Context, a Actor, id uuid.UUID, to string, comment *string) (*Encashment, error) {
	cm, err := optionalText("comment", comment, 500)
	if err != nil {
		return nil, err
	}
	if to == StatusRejected && cm == nil {
		return nil, invalid("comment", "is required when rejecting")
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	var e *Encashment
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if e, err = s.store.Encashment(ctx, tx, a.TenantID, id, true); err != nil || e == nil {
			return orNotFound(err)
		}
		own := e.RequestedByUserID == a.UserID || (me != nil && me.ID == e.EmployeeProfileID)
		if to == StatusCancelled {
			if !own {
				return ErrNotFound
			}
		} else {
			ok, err := s.mayDecide(ctx, a, e.EmployeeProfileID)
			if err != nil {
				return err
			}
			if !ok && !own {
				return ErrNotFound
			}
			if own {
				return ErrSelfApproval
			}
		}
		if e.Status != StatusPending {
			return ErrNotPending
		}
		now := s.now().UTC()
		e.Status, e.UpdatedAt = to, now
		if to != StatusCancelled {
			e.DecidedByUserID, e.DecisionComment, e.DecidedAt = &a.UserID, cm, &now
		}
		if to == StatusApproved {
			if err := s.check(ctx, tx, a, e.EmployeeProfileID, e.LeaveTypeID, e.Year, e.Days, false); err != nil {
				return err
			}
			if _, err := s.p.Mover.AdjustTx(ctx, tx, a.leaveActor(), e.EmployeeProfileID, e.LeaveTypeID, -e.Days, "encashment"); err != nil {
				return err
			}
			if err := s.store.Event(ctx, tx, a.TenantID, "leave.encashment_approved", map[string]string{"encashment_id": e.ID.String(),
				"employee_id": e.EmployeeProfileID.String(), "days": e.Days.String()}); err != nil {
				return err
			}
		}
		return s.store.Save(ctx, tx, e)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.encashment_"+to, "leave_encashment", id.String(), map[string]string{"employee_id": e.EmployeeProfileID.String()})
	return e, nil
}

// MyEncashments lists the caller's requests.
func (s *Service) MyEncashments(ctx context.Context, a Actor) ([]Encashment, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, ErrEmployeeNotFound
	}
	return s.Encashments(ctx, a, &me.ID, nil, "")
}

// Encashments lists requests (leave:read).
func (s *Service) Encashments(ctx context.Context, a Actor, employeeID, typeID *uuid.UUID, status string) ([]Encashment, error) {
	if status != "" && status != StatusPending && status != StatusApproved && status != StatusRejected && status != StatusCancelled {
		return nil, invalid("status", "must be pending, approved, rejected or cancelled")
	}
	rows, err := s.store.Encashments(ctx, s.store.DB(), a.TenantID, EncashFilter{EmployeeID: employeeID, LeaveTypeID: typeID, Status: status}, maxList)
	if rows == nil {
		rows = []Encashment{}
	}
	return rows, err
}
