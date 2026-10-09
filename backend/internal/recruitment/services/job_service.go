package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/events"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/pipeline"
	"github.com/jaas/jaas/internal/recruitment/repositories"
	"gorm.io/gorm"
)

// JobService owns job openings (FR-JB).
type JobService struct{ base }

// NewJobService builds a JobService.
func NewJobService(d Deps) *JobService { return &JobService{base{d}} }

// Create is R1: a draft job with Module 1 references validated.
func (s *JobService) Create(ctx context.Context, a Actor, req dto.CreateJobRequest) (*dto.JobResponse, error) {
	dept, desig, err := s.orgRefs(ctx, a.TenantID, req.DepartmentID, req.DesignationID)
	if err != nil {
		return nil, err
	}
	j := &models.Job{TenantID: a.TenantID, Title: req.Title, DepartmentID: dept, DesignationID: desig, Headcount: req.Headcount,
		Location: req.Location, EmploymentType: req.EmploymentType, Description: req.Description, Status: pipeline.JobDraft, CreatedByUserID: a.UserID}
	if req.MinExperienceYears != nil {
		j.MinExperienceYears = *req.MinExperienceYears
	}
	if err := s.Tx.InTx(ctx, func(tx *gorm.DB) error { return s.Repos.Jobs.Create(ctx, tx, j) }); err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.job.created", "recruitment_job", j.ID.String(), map[string]string{"status": j.Status}}, nil)
	out := mapJob(j)
	return &out, nil
}

// Get is R3.
func (s *JobService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.JobResponse, error) {
	j, err := s.Repos.Jobs.FindByID(ctx, s.Tx.DB(), tenantID, id)
	if err != nil || j == nil {
		return nil, orNotFound(err)
	}
	out := mapJob(j)
	return &out, nil
}

// List is R2.
func (s *JobService) List(ctx context.Context, tenantID uuid.UUID, f repositories.JobFilter, page dto.Page) ([]dto.JobResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Jobs.List(ctx, s.Tx.DB(), tenantID, f, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return mapList(rows, mapJob), page.Meta(total), nil
}

// Update is R4: any field while the job is not closed/filled (FR-JB002).
func (s *JobService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateJobRequest) (*dto.JobResponse, error) {
	dept, desig, err := s.orgRefs(ctx, a.TenantID, req.DepartmentID, req.DesignationID)
	if err != nil {
		return nil, err
	}
	var j *models.Job
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		if j, err = s.lockJob(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if !pipeline.Editable(j.Status) {
			return ErrJobState
		}
		if req.Headcount != nil && *req.Headcount < j.HiredCount {
			return invalid("headcount", "must not be below hired_count")
		}
		applyJobPatch(j, req, dept, desig)
		return s.Repos.Jobs.Update(ctx, tx, j)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.job.updated", "recruitment_job", j.ID.String(), nil}, nil)
	out := mapJob(j)
	return &out, nil
}

// SetStatus is R5 (FR-JB003); the first open publishes recruitment.job_published.
func (s *JobService) SetStatus(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.JobResponse, error) {
	var j *models.Job
	var row *models.OutboxEvent
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if j, err = s.lockJob(ctx, tx, a.TenantID, id); err != nil {
			return err
		}
		if !pipeline.CanTransitionJob(j.Status, to) {
			return ErrJobState
		}
		row, err = s.applyStatus(ctx, tx, a, j, to)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.job.status", "recruitment_job", j.ID.String(), map[string]string{"status": to}}, row)
	out := mapJob(j)
	return &out, nil
}

func (s *JobService) applyStatus(ctx context.Context, tx *gorm.DB, a Actor, j *models.Job, to string) (*models.OutboxEvent, error) {
	now := s.Now().UTC()
	firstOpen := to == pipeline.JobOpen && j.OpenedAt == nil
	j.Status = to
	if firstOpen {
		j.OpenedAt = &now
	}
	if to == pipeline.JobClosed {
		j.ClosedAt = &now
	}
	if err := s.Repos.Jobs.Update(ctx, tx, j); err != nil || !firstOpen {
		return nil, err
	}
	return s.writeOutbox(ctx, tx, a, events.JobPublished, events.Payload{JobID: j.ID, JobStatus: to})
}

func (s *JobService) lockJob(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error) {
	j, err := s.Repos.Jobs.LockByID(ctx, tx, tenantID, id)
	if err != nil || j == nil {
		return nil, orNotFound(err)
	}
	return j, nil
}

// orgRefs parses and validates optional department/designation ids against Module 1 (D6-03).
func (s *JobService) orgRefs(ctx context.Context, tenantID uuid.UUID, deptRaw, desigRaw *string) (*uuid.UUID, *uuid.UUID, error) {
	dept, err := s.orgRef(ctx, tenantID, "department_id", deptRaw)
	if err != nil {
		return nil, nil, err
	}
	desig, err := s.orgRef(ctx, tenantID, "designation_id", desigRaw)
	return dept, desig, err
}

func (s *JobService) orgRef(ctx context.Context, tenantID uuid.UUID, field string, raw *string) (*uuid.UUID, error) {
	id, err := parseOptionalUUID(field, raw)
	if err != nil || id == nil {
		return nil, err
	}
	exists := s.Org.DepartmentExists
	if field == "designation_id" {
		exists = s.Org.DesignationExists
	}
	ok, err := exists(ctx, tenantID, *id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, invalid(field, "not found in this tenant")
	}
	return id, nil
}

func applyJobPatch(j *models.Job, req dto.UpdateJobRequest, dept, desig *uuid.UUID) {
	setIf(&j.Title, req.Title)
	setIf(&j.Headcount, req.Headcount)
	setIf(&j.EmploymentType, req.EmploymentType)
	setIf(&j.MinExperienceYears, req.MinExperienceYears)
	setIf(&j.Description, req.Description)
	if req.Location != nil {
		j.Location = req.Location
	}
	if dept != nil {
		j.DepartmentID = dept
	}
	if desig != nil {
		j.DesignationID = desig
	}
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
