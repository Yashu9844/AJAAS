package ops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	attModels "github.com/jaas/jaas/internal/hr/attendance/models"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// sweepInterval is how often the background sweep looks for tenants whose previous day is not yet marked.
const sweepInterval = time.Hour

// AutoAbsentResult is one run's counts (AP-08).
type AutoAbsentResult struct {
	Date    Date `json:"date"`
	Absent  int  `json:"absent"`
	WeekOff int  `json:"week_off"`
	Holiday int  `json:"holiday"`
	Skipped int  `json:"skipped"`
}

// working statuses a worker may be marked absent in (AT-002).
var workingStatuses = []string{"active", "probation", "notice"}

// RunAutoAbsent is AP-07: every working worker without a record for the date gets a system record. Each worker commits
// on its own, so the run is idempotent and safe to repeat or resume.
func (s *Service) RunAutoAbsent(ctx context.Context, tenantID uuid.UUID, date time.Time, by *uuid.UUID) (*AutoAbsentResult, error) {
	p, err := s.policy(ctx, s.store.DB(), tenantID)
	if err != nil {
		return nil, err
	}
	status := attModels.StatusAbsent
	if p.weekOff(date) {
		status = attModels.StatusWeekOff
	}
	if h, err := s.p.Holidays.IsHoliday(ctx, s.store.DB(), tenantID, date); err != nil {
		return nil, err
	} else if h {
		status = attModels.StatusHoliday
	}
	res := &AutoAbsentResult{Date: Date{date}}
	for _, st := range workingStatuses {
		for page := 1; ; page++ {
			emps, total, err := s.p.Employees.List(ctx, tenantID, employeeDTO.EmployeeFilter{Status: st, Page: page, PerPage: 100})
			if err != nil {
				return nil, err
			}
			for _, e := range emps {
				if !workedOn(e, date) {
					res.Skipped++
					continue
				}
				var created bool
				err := s.store.InTx(ctx, func(tx *gorm.DB) (err error) {
					created, err = s.p.Writer.MarkDay(ctx, tx, tenantID, e.ID, date, status)
					return err
				})
				if err != nil {
					return nil, err
				}
				switch {
				case !created:
					res.Skipped++
				case status == attModels.StatusAbsent:
					res.Absent++
				case status == attModels.StatusWeekOff:
					res.WeekOff++
				default:
					res.Holiday++
				}
			}
			if len(emps) == 0 || int64(page*100) >= total {
				break
			}
		}
	}
	run := &JobRun{TenantID: tenantID, Kind: jobAutoAbsent, RunDate: Date{date}, Absent: res.Absent, WeekOff: res.WeekOff, Holiday: res.Holiday,
		Skipped: res.Skipped, TriggeredByUserID: by, RanAt: s.now().UTC()}
	return res, s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, run) })
}

// workedOn reports whether the worker was employed on the date (joined by then, not yet exited).
func workedOn(e employeeDTO.EmployeeResponse, date time.Time) bool {
	if e.Employment == nil {
		return true
	}
	if !e.Employment.JoiningDate.IsZero() && day(e.Employment.JoiningDate).After(date) {
		return false
	}
	return e.Employment.ExitDate == nil || !day(*e.Employment.ExitDate).Before(date)
}

// AutoAbsent is AP-08 for the caller's tenant.
func (s *Service) AutoAbsent(ctx context.Context, a Actor, rawDate string) (*AutoAbsentResult, error) {
	date := s.today().AddDate(0, 0, -1)
	if strings.TrimSpace(rawDate) != "" {
		d, err := parseDate("date", rawDate)
		if err != nil {
			return nil, err
		}
		date = d
	}
	if !date.Before(s.today()) || date.Before(s.today().AddDate(0, 0, -jobWindowDays)) {
		return nil, ErrJobWindow
	}
	res, err := s.RunAutoAbsent(ctx, a.TenantID, date, &a.UserID)
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "attendance.auto_absent_run", "attendance_job", a.TenantID.String(), map[string]string{"date": date.Format(DateLayout)})
	return res, nil
}

// Runs lists recent job runs.
func (s *Service) Runs(ctx context.Context, a Actor) ([]JobRun, error) {
	rows, err := s.store.JobRuns(ctx, s.store.DB(), a.TenantID, 50)
	if rows == nil {
		rows = []JobRun{}
	}
	return rows, err
}

// Sweep marks yesterday for every tenant with auto-absent enabled that has not been run for it (AP-08). It returns the
// number of tenants processed.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	tenants, err := s.store.AutoAbsentTenants(ctx, s.store.DB())
	if err != nil {
		return 0, err
	}
	date, done := s.today().AddDate(0, 0, -1), 0
	for _, t := range tenants {
		ran, err := s.store.JobRan(ctx, s.store.DB(), t, jobAutoAbsent, date)
		if err != nil {
			return done, err
		}
		if ran {
			continue
		}
		if _, err := s.RunAutoAbsent(ctx, t, date, nil); err != nil {
			if errors.Is(err, sharedErrors.ErrNotFound) {
				continue
			}
			return done, err
		}
		done++
	}
	return done, nil
}

// Run ticks Sweep every interval until ctx is cancelled (also once at start).
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		interval = sweepInterval
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
