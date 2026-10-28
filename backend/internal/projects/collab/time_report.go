package collab

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Report grouping and limits (TM-02).
const (
	GroupByTask    = "task"
	GroupByUser    = "user"
	maxReportDays  = 366
	defaultDays    = 30
	maxReportRows  = 500
	reportDateForm = "2006-01-02"
)

// ReportQuery selects the range (inclusive days, UTC) and the grouping of a project time report.
type ReportQuery struct {
	From    string // YYYY-MM-DD; default 29 days before To
	To      string // YYYY-MM-DD; default today
	GroupBy string // task (default) or user
}

// ReportRow is the time logged by one task or one user. Number, Title and EstimateMinutes are set when grouping by task.
type ReportRow struct {
	Key             uuid.UUID `json:"key"`
	Number          *int      `json:"number,omitempty"`
	Title           string    `json:"title,omitempty"`
	EstimateMinutes *int      `json:"estimate_minutes,omitempty"`
	Minutes         int       `json:"minutes"`
	Entries         int       `json:"entries"`
}

// Report is the totals of a project for a date range plus the estimate of all its open and closed tasks.
type Report struct {
	From            string      `json:"from"`
	To              string      `json:"to"`
	GroupBy         string      `json:"group_by"`
	TotalMinutes    int         `json:"total_minutes"`
	Entries         int         `json:"entries"`
	EstimateMinutes int         `json:"estimate_minutes"`
	Rows            []ReportRow `json:"rows"`
}

type window struct{ from, to time.Time } // from inclusive, to exclusive

func (s *Service) parseWindow(q ReportQuery) (window, string, string, error) {
	to := s.now().UTC().Truncate(24 * time.Hour)
	var err error
	if q.To != "" {
		if to, err = time.Parse(reportDateForm, q.To); err != nil {
			return window{}, "", "", kernel.Invalid("to", "must be YYYY-MM-DD")
		}
	}
	from := to.AddDate(0, 0, -(defaultDays - 1))
	if q.From != "" {
		if from, err = time.Parse(reportDateForm, q.From); err != nil {
			return window{}, "", "", kernel.Invalid("from", "must be YYYY-MM-DD")
		}
	}
	if from.After(to) || to.Sub(from) > maxReportDays*24*time.Hour {
		return window{}, "", "", kernel.Invalid("from", "must not be after to, and the range is at most 366 days")
	}
	return window{from: from, to: to.AddDate(0, 0, 1)}, from.Format(reportDateForm), to.Format(reportDateForm), nil
}

// TimeReport totals the finished time of a project per task or per user, with estimate against logged (TM-02).
// Readers of the project's tasks may see it; running timers are not counted until they stop.
func (s *Service) TimeReport(ctx context.Context, a api.Actor, projectID uuid.UUID, q ReportQuery) (*Report, error) {
	acc, err := s.p.Authorizer.Access(ctx, s.db, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, err
	}
	if q.GroupBy == "" {
		q.GroupBy = GroupByTask
	}
	if q.GroupBy != GroupByTask && q.GroupBy != GroupByUser {
		return nil, kernel.Invalid("group_by", "must be task or user")
	}
	w, from, to, err := s.parseWindow(q)
	if err != nil {
		return nil, err
	}
	r := &Report{From: from, To: to, GroupBy: q.GroupBy}
	if err := s.reportRows(ctx, reportScope{tenantID: a.TenantID, projectID: projectID, w: w}, r); err != nil {
		return nil, err
	}
	if err := s.reportTotals(ctx, reportScope{tenantID: a.TenantID, projectID: projectID, w: w}, r); err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(estimate_minutes), 0) FROM tasks WHERE tenant_id = ? AND project_id = ? AND deleted_at IS NULL`,
		a.TenantID, projectID).Scan(&r.EstimateMinutes).Error
	return r, err
}

const reportTaskSQL = `SELECT t.id AS key, t.number AS number, t.title AS title, t.estimate_minutes AS estimate_minutes,
		SUM(te.minutes)::int AS minutes, COUNT(te.id)::int AS entries
	FROM time_entries te JOIN tasks t ON t.id = te.task_id AND t.tenant_id = te.tenant_id
	WHERE te.tenant_id = ? AND t.project_id = ? AND t.deleted_at IS NULL AND te.ended_at IS NOT NULL AND te.started_at >= ? AND te.started_at < ?
	GROUP BY t.id, t.number, t.title, t.estimate_minutes ORDER BY minutes DESC, t.number LIMIT ?`

const reportUserSQL = `SELECT te.user_id AS key, SUM(te.minutes)::int AS minutes, COUNT(te.id)::int AS entries
	FROM time_entries te JOIN tasks t ON t.id = te.task_id AND t.tenant_id = te.tenant_id
	WHERE te.tenant_id = ? AND t.project_id = ? AND t.deleted_at IS NULL AND te.ended_at IS NOT NULL AND te.started_at >= ? AND te.started_at < ?
	GROUP BY te.user_id ORDER BY minutes DESC, te.user_id LIMIT ?`

// reportScope is the tenant, project and date window a report covers.
type reportScope struct {
	tenantID, projectID uuid.UUID
	w                   window
}

func (s *Service) reportRows(ctx context.Context, sc reportScope, r *Report) error {
	sql := reportTaskSQL
	if r.GroupBy == GroupByUser {
		sql = reportUserSQL
	}
	r.Rows = []ReportRow{}
	return s.db.WithContext(ctx).Raw(sql, sc.tenantID, sc.projectID, sc.w.from, sc.w.to, maxReportRows).Scan(&r.Rows).Error
}

// reportTotals sums every finished entry in range, so the totals stay exact when the row list is capped.
func (s *Service) reportTotals(ctx context.Context, sc reportScope, r *Report) error {
	var t struct{ Minutes, Entries int }
	err := s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(te.minutes), 0)::int AS minutes, COUNT(te.id)::int AS entries
		FROM time_entries te JOIN tasks t ON t.id = te.task_id AND t.tenant_id = te.tenant_id
		WHERE te.tenant_id = ? AND t.project_id = ? AND t.deleted_at IS NULL AND te.ended_at IS NOT NULL AND te.started_at >= ? AND te.started_at < ?`,
		sc.tenantID, sc.projectID, sc.w.from, sc.w.to).Scan(&t).Error
	r.TotalMinutes, r.Entries = t.Minutes, t.Entries
	return err
}
