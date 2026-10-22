package plus

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/leave/calc"
	"gorm.io/gorm"
)

const (
	compOffWindowDays = 30
	maxList           = 200
	compOffSweep      = 24 * time.Hour
)

// CompOffInput requests a comp-off credit (LP-08).
type CompOffInput struct {
	WorkedDate string    `json:"worked_date"`
	Days       calc.Days `json:"days"`
	Reason     string    `json:"reason"`
}

// nonWorking reports whether the date is a holiday or weekly off for the employee (their location's calendar).
func (s *Service) nonWorking(ctx context.Context, empID, tenantID uuid.UUID, d time.Time) (bool, error) {
	tenant, err := s.store.TenantHolidays(ctx, s.store.DB(), tenantID, d, d)
	if err != nil {
		return false, err
	}
	for _, h := range tenant {
		if !h.IsOptional {
			return true, nil
		}
	}
	extra, weekOff, err := s.cal.For(ctx, s.store.DB(), tenantID, empID, d, d)
	if err != nil {
		return false, err
	}
	if len(extra) > 0 {
		return true, nil
	}
	if weekOff != nil {
		return weekOff(d), nil
	}
	return calc.IsWeekend(d), nil
}

// RequestCompOff is LP-08.
func (s *Service) RequestCompOff(ctx context.Context, a Actor, in CompOffInput) (*CompOff, error) {
	me, err := s.worker(ctx, a)
	if err != nil {
		return nil, err
	}
	d, err := parseDate("worked_date", in.WorkedDate)
	if err != nil {
		return nil, err
	}
	if in.Days != 50 && in.Days != 100 {
		return nil, invalid("days", "must be 0.5 or 1")
	}
	reason, err := text("reason", in.Reason, 500)
	if err != nil {
		return nil, err
	}
	if d.After(s.today()) || d.Before(s.today().AddDate(0, 0, -compOffWindowDays)) {
		return nil, invalid("worked_date", "must be within the last 30 days and not in the future")
	}
	pol, err := s.GetPolicy(ctx, a)
	if err != nil {
		return nil, err
	}
	if pol.CompOffLeaveTypeID == nil {
		return nil, ErrCompOffConfig
	}
	if off, err := s.nonWorking(ctx, me.ID, a.TenantID, d); err != nil || !off {
		if err != nil {
			return nil, err
		}
		return nil, ErrWorkingDay
	}
	if mins, err := s.p.Presence.WorkedMinutes(ctx, s.store.DB(), a.TenantID, me.ID, d); err != nil || mins <= 0 {
		if err != nil {
			return nil, err
		}
		return nil, ErrNoWork
	}
	now := s.now().UTC()
	c := &CompOff{TenantID: a.TenantID, EmployeeProfileID: me.ID, WorkedDate: Date{d}, Days: in.Days, Reason: reason, Status: StatusPending,
		RequestedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if dup, err := s.store.CompOffOnDay(ctx, tx, a.TenantID, me.ID, d); err != nil || dup != nil {
			if err != nil {
				return err
			}
			return ErrConflict
		}
		return s.store.Create(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.comp_off_requested", "leave_comp_off", c.ID.String(), map[string]string{"worked_date": in.WorkedDate})
	return c, nil
}

// DecideCompOff is LP-09: approve (credit the balance), reject (comment required) or cancel (own pending).
func (s *Service) DecideCompOff(ctx context.Context, a Actor, id uuid.UUID, to string, comment *string) (*CompOff, error) {
	cm, err := optionalText("comment", comment, 500)
	if err != nil {
		return nil, err
	}
	if to == StatusRejected && cm == nil {
		return nil, invalid("comment", "is required when rejecting")
	}
	pol, err := s.GetPolicy(ctx, a)
	if err != nil {
		return nil, err
	}
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	var c *CompOff
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if c, err = s.store.CompOff(ctx, tx, a.TenantID, id, true); err != nil || c == nil {
			return orNotFound(err)
		}
		own := c.RequestedByUserID == a.UserID || (me != nil && me.ID == c.EmployeeProfileID)
		if to == StatusCancelled {
			if !own {
				return ErrNotFound
			}
		} else {
			ok, err := s.mayDecide(ctx, a, c.EmployeeProfileID)
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
		if c.Status != StatusPending {
			return ErrNotPending
		}
		now := s.now().UTC()
		c.Status, c.UpdatedAt = to, now
		if to != StatusCancelled {
			c.DecidedByUserID, c.DecisionComment, c.DecidedAt = &a.UserID, cm, &now
		}
		if to == StatusApproved {
			if pol.CompOffLeaveTypeID == nil {
				return ErrCompOffConfig
			}
			exp := Date{c.WorkedDate.AddDate(0, 0, pol.CompOffExpiryDays)}
			c.ExpiresOn = &exp
			if _, err := s.p.Mover.AdjustTx(ctx, tx, a.leaveActor(), c.EmployeeProfileID, *pol.CompOffLeaveTypeID, c.Days, "comp-off"); err != nil {
				return err
			}
			if err := s.store.Event(ctx, tx, a.TenantID, "leave.comp_off_credited", map[string]string{"comp_off_id": c.ID.String(),
				"employee_id": c.EmployeeProfileID.String(), "days": c.Days.String()}); err != nil {
				return err
			}
		}
		return s.store.Save(ctx, tx, c)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.comp_off_"+to, "leave_comp_off", id.String(), map[string]string{"employee_id": c.EmployeeProfileID.String()})
	return c, nil
}

// MyCompOffs lists the caller's requests.
func (s *Service) MyCompOffs(ctx context.Context, a Actor) ([]CompOff, error) {
	me, err := s.self(ctx, a)
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, ErrEmployeeNotFound
	}
	return s.CompOffs(ctx, a, &me.ID, "")
}

// CompOffs lists requests (leave:read).
func (s *Service) CompOffs(ctx context.Context, a Actor, employeeID *uuid.UUID, status string) ([]CompOff, error) {
	if status != "" && status != StatusPending && status != StatusApproved && status != StatusRejected && status != StatusCancelled {
		return nil, invalid("status", "must be pending, approved, rejected or cancelled")
	}
	rows, err := s.store.CompOffs(ctx, s.store.DB(), a.TenantID, CompOffFilter{EmployeeID: employeeID, Status: status}, maxList)
	if rows == nil {
		rows = []CompOff{}
	}
	return rows, err
}

// ExpireResult counts what one expiry run did.
type ExpireResult struct {
	Expired int       `json:"expired"`
	Days    calc.Days `json:"days_debited"`
}

// ExpireCompOffs is LP-10 for one tenant: each overdue credit is debited by min(credit, available) (never below
// zero) and marked, so a second run finds nothing.
func (s *Service) ExpireCompOffs(ctx context.Context, tenantID, by uuid.UUID) (*ExpireResult, error) {
	pol, err := s.store.Policy(ctx, s.store.DB(), tenantID)
	if err != nil || pol == nil || pol.CompOffLeaveTypeID == nil {
		return &ExpireResult{}, err
	}
	rows, err := s.store.ExpiredCompOffs(ctx, s.store.DB(), tenantID, s.today())
	if err != nil {
		return nil, err
	}
	res := &ExpireResult{}
	actor := Actor{TenantID: tenantID, UserID: by}.leaveActor()
	for _, row := range rows {
		err := s.store.InTx(ctx, func(tx *gorm.DB) error {
			c, err := s.store.CompOff(ctx, tx, tenantID, row.ID, true)
			if err != nil || c == nil || c.ExpiredAt != nil {
				return err
			}
			avail, err := s.p.Mover.AvailableTx(ctx, tx, actor, c.EmployeeProfileID, *pol.CompOffLeaveTypeID)
			if err != nil {
				return err
			}
			debit := min(c.Days, max(avail, 0))
			if debit > 0 {
				if _, err := s.p.Mover.AdjustTx(ctx, tx, actor, c.EmployeeProfileID, *pol.CompOffLeaveTypeID, -debit, "comp-off expired"); err != nil {
					return err
				}
			}
			now := s.now().UTC()
			c.ExpiredDays, c.ExpiredAt, c.UpdatedAt = &debit, &now, now
			res.Expired++
			res.Days += debit
			return s.store.Save(ctx, tx, c)
		})
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// Expire is LP-10 for the caller's tenant.
func (s *Service) Expire(ctx context.Context, a Actor) (*ExpireResult, error) {
	res, err := s.ExpireCompOffs(ctx, a.TenantID, a.UserID)
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "leave.comp_off_expiry_run", "leave_comp_off", a.TenantID.String(), map[string]string{"expired": strconv.Itoa(res.Expired)})
	return res, nil
}

// Sweep expires comp-off credits for every tenant that configured a comp-off type.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	tenants, err := s.store.CompOffTenants(ctx, s.store.DB())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tenants {
		res, err := s.ExpireCompOffs(ctx, t, uuid.Nil)
		if err != nil {
			return n, err
		}
		n += res.Expired
	}
	return n, nil
}

// Run ticks Sweep every interval until ctx is cancelled (also once at start).
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = compOffSweep
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if _, err := s.Sweep(ctx); err != nil && onError != nil && ctx.Err() == nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
