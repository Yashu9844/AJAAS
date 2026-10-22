package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	attModels "github.com/jaas/jaas/internal/hr/attendance/models"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

const (
	maxImportRows   = 500
	importWindow    = 90
	maxSessionHours = 20
)

// ImportRow is one worker-day to import (AP-10).
type ImportRow struct {
	EmployeeCode string `json:"employee_code"`
	Date         string `json:"date"`
	PunchIn      string `json:"punch_in"`
	PunchOut     string `json:"punch_out"`
}

// ImportInput is the batch.
type ImportInput struct {
	DryRun    bool        `json:"dry_run"`
	Overwrite bool        `json:"overwrite"`
	Rows      []ImportRow `json:"rows"`
}

// RowError explains why a row cannot be imported.
type RowError struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ImportResult is the outcome (AP-11): nothing is written unless Errors is empty and DryRun is false.
type ImportResult struct {
	DryRun  bool       `json:"dry_run"`
	Total   int        `json:"total"`
	Applied int        `json:"applied"`
	Errors  []RowError `json:"errors"`
}

type planned struct {
	employee uuid.UUID
	date     time.Time
	in, out  time.Time
}

// Import is AP-10/AP-11: validate every row, then apply all of them in one transaction or none.
func (s *Service) Import(ctx context.Context, a Actor, in ImportInput) (*ImportResult, error) {
	if len(in.Rows) == 0 || len(in.Rows) > maxImportRows {
		return nil, ErrImportTooLarge
	}
	res := &ImportResult{DryRun: in.DryRun, Total: len(in.Rows), Errors: []RowError{}}
	fail := func(i int, code, msg string) {
		res.Errors = append(res.Errors, RowError{Row: i + 1, Code: code, Message: msg})
	}
	codes, seen, plan := map[string]*employeeDTO.EmployeeResponse{}, map[string]bool{}, []planned{}
	today := s.today()
	for i, r := range in.Rows {
		code := strings.TrimSpace(r.EmployeeCode)
		date, derr := time.Parse(DateLayout, strings.TrimSpace(r.Date))
		pin, ierr := time.Parse(time.RFC3339, strings.TrimSpace(r.PunchIn))
		pout, oerr := time.Parse(time.RFC3339, strings.TrimSpace(r.PunchOut))
		switch {
		case code == "":
			fail(i, "VALIDATION_ERROR", "employee_code is required")
			continue
		case derr != nil:
			fail(i, "VALIDATION_ERROR", "date must be YYYY-MM-DD")
			continue
		case ierr != nil || oerr != nil:
			fail(i, "VALIDATION_ERROR", "punch_in and punch_out must be RFC 3339 instants")
			continue
		case date.After(today) || date.Before(today.AddDate(0, 0, -importWindow)):
			fail(i, "DATE_WINDOW", "the date must be within the last 90 days and not in the future")
			continue
		case !pout.After(pin) || pout.Sub(pin) > maxSessionHours*time.Hour:
			fail(i, "VALIDATION_ERROR", "punch_out must be after punch_in, at most 20 hours later")
			continue
		case pin.Before(date.Add(-14*time.Hour)) || !pin.Before(date.Add(38*time.Hour)):
			fail(i, "DATE_MISMATCH", "punch_in must fall on the date (within a day, timezone-safe)")
			continue
		}
		emp, ok := codes[strings.ToLower(code)]
		if !ok {
			var err error
			if emp, err = s.byCode(ctx, a.TenantID, code); err != nil {
				return nil, err
			}
			codes[strings.ToLower(code)] = emp
		}
		if emp == nil {
			fail(i, "EMPLOYEE_NOT_FOUND", "no employee with this code")
			continue
		}
		key := emp.ID.String() + date.Format(DateLayout)
		if seen[key] {
			fail(i, "DUPLICATE_ROW", "the batch already has a row for this worker and date")
			continue
		}
		seen[key] = true
		exists, punched, err := s.p.Writer.HasRecord(ctx, s.store.DB(), a.TenantID, emp.ID, date)
		if err != nil {
			return nil, err
		}
		if exists && punched && !in.Overwrite {
			fail(i, "EXISTS", "the day already has punches; set overwrite to replace them")
			continue
		}
		plan = append(plan, planned{employee: emp.ID, date: date, in: pin.UTC(), out: pout.UTC()})
	}
	if len(res.Errors) > 0 || in.DryRun {
		return res, nil
	}
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		for _, p := range plan {
			if err := s.p.Writer.SetWindow(ctx, tx, a.TenantID, p.employee, p.date, p.in, p.out, attModels.SourceImport); err != nil {
				return err
			}
		}
		return s.store.Create(ctx, tx, &ImportLog{TenantID: a.TenantID, RowsTotal: len(in.Rows), RowsApplied: len(plan), Overwrite: in.Overwrite,
			CreatedByUserID: a.UserID, CreatedAt: s.now().UTC()})
	})
	if err != nil {
		return nil, err
	}
	res.Applied = len(plan)
	s.log(ctx, a, "attendance.import_applied", "attendance_import", a.TenantID.String(), map[string]string{"rows": strconv.Itoa(len(plan))})
	return res, nil
}

// byCode finds a worker by exact employee code (case-insensitive), or nil.
func (s *Service) byCode(ctx context.Context, tenantID uuid.UUID, code string) (*employeeDTO.EmployeeResponse, error) {
	rows, _, err := s.p.Employees.List(ctx, tenantID, employeeDTO.EmployeeFilter{Search: code, Page: 1, PerPage: 20})
	if errors.Is(err, sharedErrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if strings.EqualFold(rows[i].EmployeeCode, code) {
			return &rows[i], nil
		}
	}
	return nil, nil
}

// Imports lists recent applied imports.
func (s *Service) Imports(ctx context.Context, a Actor) ([]ImportLog, error) {
	rows, err := s.store.Imports(ctx, s.store.DB(), a.TenantID, 50)
	if rows == nil {
		rows = []ImportLog{}
	}
	return rows, err
}
