package plus

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReqFilter narrows requisition lists.
type ReqFilter struct {
	Status     string
	PositionID *uuid.UUID
}

// Store is parity data access (tenant-scoped; `tx` passed per call).
type Store interface {
	InTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	DB() *gorm.DB
	Create(ctx context.Context, tx *gorm.DB, row interface{}) error
	Save(ctx context.Context, tx *gorm.DB, row interface{}) error

	Requisition(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*Requisition, error)
	Requisitions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ReqFilter, limit int) ([]Requisition, error)
	// RequestedSeats sums the seats other requisitions still hold on the position (RP-02).
	RequestedSeats(ctx context.Context, db *gorm.DB, tenantID, positionID uuid.UUID, except uuid.UUID) (int, error)
	CreateJob(ctx context.Context, tx *gorm.DB, job *models.Job) error
	Job(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*models.Job, error)
	SaveJob(ctx context.Context, tx *gorm.DB, job *models.Job) error

	Scorecard(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*Scorecard, error)
	Scorecards(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string) ([]Scorecard, error)
	JobScorecard(ctx context.Context, db *gorm.DB, tenantID, jobID uuid.UUID) (*JobScorecard, error)
	SaveJobScorecard(ctx context.Context, tx *gorm.DB, row *JobScorecard) error

	Interview(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Interview, error)
	Candidate(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*models.Candidate, error)
	SaveCandidate(ctx context.Context, tx *gorm.DB, c *models.Candidate) error
	ReplaceScores(ctx context.Context, tx *gorm.DB, tenantID, interviewID uuid.UUID, rows []InterviewScore) error
	CandidateScores(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID) ([]ScoredInterview, error)

	OfferPolicy(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (*OfferPolicy, error)
	SaveOfferPolicy(ctx context.Context, tx *gorm.DB, p *OfferPolicy) error
	Offer(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID, lock bool) (*models.Offer, error)
	SaveOffer(ctx context.Context, tx *gorm.DB, o *models.Offer) error
	Approvals(ctx context.Context, db *gorm.DB, tenantID, offerID uuid.UUID) ([]OfferApproval, error)

	// Event stores an outbox event for the recruitment relay (ids only).
	Event(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, key string, payload map[string]string) error
}

// ScoredInterview is one interview with its scores, for the candidate scorecard (RP-05).
type ScoredInterview struct {
	InterviewID uuid.UUID
	Competency  string
	Score       int
}

type gormStore struct{ db *gorm.DB }

// NewStore builds the gorm-backed Store.
func NewStore(db *gorm.DB) Store { return gormStore{db: db} }

func (s gormStore) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}
func (s gormStore) DB() *gorm.DB { return s.db }
func (s gormStore) Create(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (s gormStore) Save(ctx context.Context, tx *gorm.DB, row interface{}) error {
	return tx.WithContext(ctx).Save(row).Error
}

func take[T any](q *gorm.DB) (*T, error) {
	var out T
	err := q.Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func scoped(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) *gorm.DB {
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q.Where("tenant_id = ? AND id = ?", t, id)
}

func (s gormStore) Requisition(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*Requisition, error) {
	return take[Requisition](scoped(ctx, db, t, id, lock))
}
func (s gormStore) Requisitions(ctx context.Context, db *gorm.DB, t uuid.UUID, f ReqFilter, limit int) ([]Requisition, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.PositionID != nil {
		q = q.Where("position_id = ?", *f.PositionID)
	}
	var out []Requisition
	return out, q.Order("created_at DESC, id").Limit(limit).Find(&out).Error
}
func (s gormStore) RequestedSeats(ctx context.Context, db *gorm.DB, t, position, except uuid.UUID) (int, error) {
	var n int
	err := db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(r.headcount), 0) FROM recruitment_requisitions r
		LEFT JOIN recruitment_jobs j ON j.id = r.job_id
		WHERE r.tenant_id = ? AND r.position_id = ? AND r.id <> ?
		AND (r.status = 'submitted' OR (r.status = 'approved' AND (j.id IS NULL OR j.status IN ('draft', 'open', 'on_hold'))))`,
		t, position, except).Row().Scan(&n)
	return n, err
}
func (s gormStore) CreateJob(ctx context.Context, tx *gorm.DB, job *models.Job) error {
	return tx.WithContext(ctx).Create(job).Error
}
func (s gormStore) Job(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*models.Job, error) {
	return take[models.Job](scoped(ctx, db, t, id, lock))
}
func (s gormStore) SaveJob(ctx context.Context, tx *gorm.DB, job *models.Job) error {
	return tx.WithContext(ctx).Save(job).Error
}

func (s gormStore) Scorecard(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*Scorecard, error) {
	return take[Scorecard](scoped(ctx, db, t, id, false))
}
func (s gormStore) Scorecards(ctx context.Context, db *gorm.DB, t uuid.UUID, status string) ([]Scorecard, error) {
	q := db.WithContext(ctx).Where("tenant_id = ?", t)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []Scorecard
	return out, q.Order("lower(name), id").Find(&out).Error
}
func (s gormStore) JobScorecard(ctx context.Context, db *gorm.DB, t, jobID uuid.UUID) (*JobScorecard, error) {
	return take[JobScorecard](db.WithContext(ctx).Where("tenant_id = ? AND job_id = ?", t, jobID))
}
func (s gormStore) SaveJobScorecard(ctx context.Context, tx *gorm.DB, row *JobScorecard) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

func (s gormStore) Interview(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Interview, error) {
	return take[models.Interview](scoped(ctx, db, t, id, false))
}
func (s gormStore) Candidate(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*models.Candidate, error) {
	return take[models.Candidate](scoped(ctx, db, t, id, lock))
}
func (s gormStore) SaveCandidate(ctx context.Context, tx *gorm.DB, c *models.Candidate) error {
	return tx.WithContext(ctx).Save(c).Error
}
func (s gormStore) ReplaceScores(ctx context.Context, tx *gorm.DB, t, interview uuid.UUID, rows []InterviewScore) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND interview_id = ?", t, interview).Delete(&InterviewScore{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Create(&rows).Error
}
func (s gormStore) CandidateScores(ctx context.Context, db *gorm.DB, t, candidate uuid.UUID) ([]ScoredInterview, error) {
	var out []ScoredInterview
	err := db.WithContext(ctx).Raw(`SELECT sc.interview_id AS interview_id, sc.competency AS competency, sc.score AS score
		FROM recruitment_interview_scores sc JOIN recruitment_interviews i ON i.id = sc.interview_id
		WHERE sc.tenant_id = ? AND i.candidate_id = ? ORDER BY sc.interview_id, sc.competency`, t, candidate).Scan(&out).Error
	return out, err
}

func (s gormStore) OfferPolicy(ctx context.Context, db *gorm.DB, t uuid.UUID) (*OfferPolicy, error) {
	return take[OfferPolicy](db.WithContext(ctx).Where("tenant_id = ?", t))
}
func (s gormStore) SaveOfferPolicy(ctx context.Context, tx *gorm.DB, p *OfferPolicy) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(p).Error
}
func (s gormStore) Offer(ctx context.Context, db *gorm.DB, t, id uuid.UUID, lock bool) (*models.Offer, error) {
	return take[models.Offer](scoped(ctx, db, t, id, lock))
}
func (s gormStore) SaveOffer(ctx context.Context, tx *gorm.DB, o *models.Offer) error {
	return tx.WithContext(ctx).Save(o).Error
}
func (s gormStore) Approvals(ctx context.Context, db *gorm.DB, t, offer uuid.UUID) ([]OfferApproval, error) {
	var out []OfferApproval
	return out, db.WithContext(ctx).Where("tenant_id = ? AND offer_id = ?", t, offer).Order("decided_at, id").Find(&out).Error
}

func (s gormStore) Event(ctx context.Context, tx *gorm.DB, t uuid.UUID, key string, payload map[string]string) error {
	id := uuid.New()
	raw, err := json.Marshal(map[string]interface{}{"event_id": id, "event_type": key, "routing_key": key, "version": "1.0", "event_version": 1,
		"tenant_id": t, "occurred_at": time.Now().UTC(), "producer": "recruitment-service", "payload": payload})
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`INSERT INTO recruitment_events_outbox (id, tenant_id, event_id, event_type, routing_key, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), t, id, key, key, string(raw)).Error
}
