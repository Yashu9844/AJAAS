package plus

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	coreAPI "github.com/jaas/jaas/internal/hr/core/api"
	employeeDTO "github.com/jaas/jaas/internal/hr/employee/dto"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/kernel"
	"github.com/jaas/jaas/internal/hr/recruitment/models"
	"github.com/jaas/jaas/internal/hr/recruitment/pipeline"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

func appErr(status int, code, msg string) *sharedErrors.AppError {
	return &sharedErrors.AppError{Code: code, Message: msg, StatusCode: status}
}

func invalid(field, msg string) *sharedErrors.AppError {
	return appErr(http.StatusBadRequest, sharedErrors.ErrValidation.Code, field+": "+msg)
}

// Errors (specification.md, Parity).
var (
	ErrNotFound       = appErr(http.StatusNotFound, sharedErrors.ErrNotFound.Code, "resource not found")
	ErrInactive       = appErr(http.StatusConflict, "INACTIVE_REFERENCE", "the position or scorecard must be active")
	ErrNoVacancy      = appErr(http.StatusConflict, "NO_VACANCY", "the position has fewer open seats than requested")
	ErrReqState       = appErr(http.StatusConflict, "REQUISITION_STATE", "the requisition's status does not allow this")
	ErrSelfApproval   = appErr(http.StatusForbidden, "RECRUITMENT_SELF_APPROVAL", "you cannot decide your own request")
	ErrHireCompleted  = appErr(http.StatusConflict, "HIRE_COMPLETED", "the candidate already has an employee profile")
	ErrAlreadyDecided = appErr(http.StatusConflict, "ALREADY_DECIDED", "you already decided this offer")
	ErrOfferState     = appErr(http.StatusConflict, "OFFER_STATE", "the offer is not waiting for approval")
	ErrNoScorecard    = appErr(http.StatusConflict, "NO_SCORECARD", "the candidate's job has no scorecard")
	ErrJobNotOpen     = appErr(http.StatusConflict, "JOB_NOT_OPEN", "the job must be draft or open")
	ErrInterviewState = appErr(http.StatusConflict, "INTERVIEW_STATE", "scores need a scheduled or completed interview")
)

// PositionInfo and Positions are what a requisition needs from core (RP-02).
type (
	PositionInfo = coreAPI.PositionInfo
	Positions    = coreAPI.Positions
)

// Directory is plus' view of HR/Employee.
type Directory interface {
	GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)
}

// Ports are the collaborators plus consumes.
type Ports struct {
	Positions Positions
	Employees Directory
	Audit     identityport.Auditor
}

// Actor is the caller.
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
}

// Service implements RP-01..RP-07.
type Service struct {
	store Store
	p     Ports
	now   func() time.Time
}

// NewService builds the service.
func NewService(store Store, p Ports, now func() time.Time) *Service {
	return &Service{store: store, p: p, now: now}
}

func orNotFound(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}

func text(field, v string, max int) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" || utf8.RuneCountInString(t) > max {
		return "", invalid(field, "is required and too long values are refused")
	}
	return t, nil
}

func (s *Service) log(ctx context.Context, a Actor, action, resource, id string) {
	_ = s.p.Audit.Log(ctx, s.store.DB(), a.TenantID.String(), a.UserID.String(), action, resource, id, nil, a.IP, a.UserAgent)
}

// --- seat release (RP-01) ---

// ReleaseHire gives back the seat a stuck hire reserved (RP-01); it is idempotent.
func (s *Service) ReleaseHire(ctx context.Context, a Actor, candidateID uuid.UUID) error {
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.store.Candidate(ctx, tx, a.TenantID, candidateID, true)
		if err != nil || c == nil {
			return orNotFound(err)
		}
		if c.HiredEmployeeID != nil {
			return ErrHireCompleted
		}
		if !c.SeatReserved {
			return nil
		}
		job, err := s.store.Job(ctx, tx, a.TenantID, c.JobID, true)
		if err != nil || job == nil {
			return orNotFound(err)
		}
		if job.ReservedCount > 0 {
			job.ReservedCount--
		}
		c.SeatReserved = false
		job.UpdatedAt, c.UpdatedAt = s.now().UTC(), s.now().UTC()
		if err := s.store.SaveJob(ctx, tx, job); err != nil {
			return err
		}
		return s.store.SaveCandidate(ctx, tx, c)
	})
	if err == nil {
		s.log(ctx, a, "recruitment.hire_released", "recruitment_candidate", candidateID.String())
	}
	return err
}

// --- requisitions (RP-02, RP-03) ---

// RequisitionInput creates a requisition.
type RequisitionInput struct {
	PositionID    string `json:"position_id"`
	Headcount     int    `json:"headcount"`
	Justification string `json:"justification"`
}

// checkSeats applies RP-02: the position is active and has enough open seats after other requisitions.
func (s *Service) checkSeats(ctx context.Context, db *gorm.DB, tenantID, position uuid.UUID, headcount int, except uuid.UUID) (PositionInfo, error) {
	info, err := s.p.Positions.PositionInfo(ctx, tenantID, position)
	if err != nil {
		return info, err
	}
	if !info.Exists {
		return info, ErrNotFound
	}
	if !info.Active {
		return info, ErrInactive
	}
	held, err := s.store.RequestedSeats(ctx, db, tenantID, position, except)
	if err != nil {
		return info, err
	}
	if headcount > info.OpenSeats-held {
		return info, ErrNoVacancy
	}
	return info, nil
}

// CreateRequisition is RP-02 (draft).
func (s *Service) CreateRequisition(ctx context.Context, a Actor, in RequisitionInput) (*Requisition, error) {
	position, err := uuid.Parse(in.PositionID)
	if err != nil {
		return nil, invalid("position_id", "must be a UUID")
	}
	if in.Headcount < 1 || in.Headcount > 100 {
		return nil, invalid("headcount", "must be 1Ã¢â‚¬â€œ100")
	}
	just, err := text("justification", in.Justification, 1000)
	if err != nil {
		return nil, err
	}
	info, err := s.checkSeats(ctx, s.store.DB(), a.TenantID, position, in.Headcount, uuid.Nil)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	r := &Requisition{TenantID: a.TenantID, PositionID: position, Title: info.Title, DepartmentID: info.DepartmentID, Headcount: in.Headcount,
		Justification: just, Status: ReqDraft, RequestedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, r) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.requisition_created", "recruitment_requisition", r.ID.String())
	return r, nil
}

func (s *Service) moveRequisition(ctx context.Context, a Actor, id uuid.UUID, verb string, fn func(tx *gorm.DB, r *Requisition) error) (*Requisition, error) {
	var r *Requisition
	err := s.store.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if r, err = s.store.Requisition(ctx, tx, a.TenantID, id, true); err != nil || r == nil {
			return orNotFound(err)
		}
		if err := fn(tx, r); err != nil {
			return err
		}
		r.UpdatedAt = s.now().UTC()
		return s.store.Save(ctx, tx, r)
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.requisition_"+verb, "recruitment_requisition", id.String())
	return r, nil
}

// SubmitRequisition is RP-03: the requester sends a draft for approval.
func (s *Service) SubmitRequisition(ctx context.Context, a Actor, id uuid.UUID) (*Requisition, error) {
	return s.moveRequisition(ctx, a, id, "submitted", func(tx *gorm.DB, r *Requisition) error {
		if r.RequestedByUserID != a.UserID {
			return ErrNotFound
		}
		if r.Status != ReqDraft {
			return ErrReqState
		}
		if _, err := s.checkSeats(ctx, tx, a.TenantID, r.PositionID, r.Headcount, r.ID); err != nil {
			return err
		}
		r.Status = ReqSubmitted
		return nil
	})
}

// CancelRequisition withdraws a draft or submitted requisition (the requester).
func (s *Service) CancelRequisition(ctx context.Context, a Actor, id uuid.UUID) (*Requisition, error) {
	return s.moveRequisition(ctx, a, id, "cancelled", func(_ *gorm.DB, r *Requisition) error {
		if r.RequestedByUserID != a.UserID {
			return ErrNotFound
		}
		if r.Status != ReqDraft && r.Status != ReqSubmitted {
			return ErrReqState
		}
		r.Status = ReqCancelled
		return nil
	})
}

// ApproveRequisition is RP-03: re-check the seats and create the linked draft job.
func (s *Service) ApproveRequisition(ctx context.Context, a Actor, id uuid.UUID, comment *string) (*Requisition, error) {
	return s.moveRequisition(ctx, a, id, "approved", func(tx *gorm.DB, r *Requisition) error {
		if r.Status != ReqSubmitted {
			return ErrReqState
		}
		if r.RequestedByUserID == a.UserID {
			return ErrSelfApproval
		}
		if _, err := s.checkSeats(ctx, tx, a.TenantID, r.PositionID, r.Headcount, r.ID); err != nil {
			return err
		}
		now := s.now().UTC()
		job := &models.Job{TenantID: a.TenantID, Title: r.Title, DepartmentID: &r.DepartmentID, Headcount: r.Headcount, EmploymentType: "full_time",
			Description: r.Justification, Status: pipeline.JobDraft, CreatedByUserID: r.RequestedByUserID, CreatedAt: now, UpdatedAt: now}
		if err := s.store.CreateJob(ctx, tx, job); err != nil {
			return err
		}
		r.Status, r.JobID, r.DecidedByUserID, r.DecidedAt = ReqApproved, &job.ID, &a.UserID, &now
		if c, err := optionalComment(comment); err != nil {
			return err
		} else {
			r.DecisionComment = c
		}
		return s.store.Event(ctx, tx, a.TenantID, "recruitment.requisition.approved", map[string]string{"requisition_id": r.ID.String(), "job_id": job.ID.String()})
	})
}

// RejectRequisition is RP-03 (comment required).
func (s *Service) RejectRequisition(ctx context.Context, a Actor, id uuid.UUID, comment *string) (*Requisition, error) {
	c, err := optionalComment(comment)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, invalid("comment", "is required when rejecting")
	}
	return s.moveRequisition(ctx, a, id, "rejected", func(_ *gorm.DB, r *Requisition) error {
		if r.Status != ReqSubmitted {
			return ErrReqState
		}
		if r.RequestedByUserID == a.UserID {
			return ErrSelfApproval
		}
		now := s.now().UTC()
		r.Status, r.DecidedByUserID, r.DecidedAt, r.DecisionComment = ReqRejected, &a.UserID, &now, c
		return nil
	})
}

func optionalComment(v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := text("comment", *v, 500)
	return &t, err
}

// GetRequisition returns one requisition.
func (s *Service) GetRequisition(ctx context.Context, a Actor, id uuid.UUID) (*Requisition, error) {
	r, err := s.store.Requisition(ctx, s.store.DB(), a.TenantID, id, false)
	if err != nil || r == nil {
		return nil, orNotFound(err)
	}
	return r, nil
}

// Requisitions lists requisitions.
func (s *Service) Requisitions(ctx context.Context, a Actor, status, position string) ([]Requisition, error) {
	switch status {
	case "", ReqDraft, ReqSubmitted, ReqApproved, ReqRejected, ReqCancelled:
	default:
		return nil, invalid("status", "must be draft, submitted, approved, rejected or cancelled")
	}
	f := ReqFilter{Status: status}
	if position != "" {
		id, err := uuid.Parse(position)
		if err != nil {
			return nil, invalid("position_id", "must be a UUID")
		}
		f.PositionID = &id
	}
	rows, err := s.store.Requisitions(ctx, s.store.DB(), a.TenantID, f, 200)
	if rows == nil {
		rows = []Requisition{}
	}
	return rows, err
}

// --- scorecards (RP-04, RP-05) ---

// ScorecardInput creates or edits a scorecard.
type ScorecardInput struct {
	Name         *string       `json:"name"`
	Competencies *Competencies `json:"competencies"`
	Status       *string       `json:"status"`
}

func (in ScorecardInput) apply(sc *Scorecard) error {
	if in.Name != nil {
		n, err := text("name", *in.Name, 150)
		if err != nil {
			return err
		}
		sc.Name = n
	}
	if in.Status != nil {
		if *in.Status != ScorecardActive && *in.Status != ScorecardArchived {
			return invalid("status", "must be active or archived")
		}
		sc.Status = *in.Status
	}
	if in.Competencies != nil {
		cs := *in.Competencies
		if len(cs) < 1 || len(cs) > 10 {
			return appErr(http.StatusBadRequest, "SCORECARD_INVALID", "a scorecard has 1Ã¢â‚¬â€œ10 competencies")
		}
		seen, total := map[string]bool{}, kernel.Fixed(0)
		for i := range cs {
			cs[i].Name = strings.TrimSpace(cs[i].Name)
			key := strings.ToLower(cs[i].Name)
			if cs[i].Name == "" || utf8.RuneCountInString(cs[i].Name) > 100 || seen[key] || cs[i].WeightPct <= 0 {
				return appErr(http.StatusBadRequest, "SCORECARD_INVALID", "competency names are distinct, 1Ã¢â‚¬â€œ100 characters, with a positive weight")
			}
			seen[key] = true
			total += cs[i].WeightPct
		}
		if total != 10000 {
			return appErr(http.StatusBadRequest, "SCORECARD_INVALID", "weights must add up to exactly 100.00")
		}
		sc.Competencies = cs
	}
	return nil
}

// CreateScorecard is RP-04.
func (s *Service) CreateScorecard(ctx context.Context, a Actor, in ScorecardInput) (*Scorecard, error) {
	if in.Name == nil || in.Competencies == nil {
		return nil, invalid("body", "name and competencies are required")
	}
	now := s.now().UTC()
	sc := &Scorecard{TenantID: a.TenantID, Status: ScorecardActive, CreatedByUserID: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := in.apply(sc); err != nil {
		return nil, err
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Create(ctx, tx, sc) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.scorecard_created", "recruitment_scorecard", sc.ID.String())
	return sc, nil
}

// UpdateScorecard edits a scorecard (RP-04).
func (s *Service) UpdateScorecard(ctx context.Context, a Actor, id uuid.UUID, in ScorecardInput) (*Scorecard, error) {
	sc, err := s.store.Scorecard(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || sc == nil {
		return nil, orNotFound(err)
	}
	if err := in.apply(sc); err != nil {
		return nil, err
	}
	sc.UpdatedAt = s.now().UTC()
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.Save(ctx, tx, sc) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.scorecard_updated", "recruitment_scorecard", id.String())
	return sc, nil
}

// GetScorecard returns one scorecard.
func (s *Service) GetScorecard(ctx context.Context, a Actor, id uuid.UUID) (*Scorecard, error) {
	sc, err := s.store.Scorecard(ctx, s.store.DB(), a.TenantID, id)
	if err != nil || sc == nil {
		return nil, orNotFound(err)
	}
	return sc, nil
}

// Scorecards lists scorecards.
func (s *Service) Scorecards(ctx context.Context, a Actor, status string) ([]Scorecard, error) {
	if status != "" && status != ScorecardActive && status != ScorecardArchived {
		return nil, invalid("status", "must be active or archived")
	}
	rows, err := s.store.Scorecards(ctx, s.store.DB(), a.TenantID, status)
	if rows == nil {
		rows = []Scorecard{}
	}
	return rows, err
}

// AssignScorecard is RP-04: attach an active scorecard to a draft or open job.
func (s *Service) AssignScorecard(ctx context.Context, a Actor, jobID uuid.UUID, rawCard string) (*JobScorecard, error) {
	cardID, err := uuid.Parse(rawCard)
	if err != nil {
		return nil, invalid("scorecard_id", "must be a UUID")
	}
	job, err := s.store.Job(ctx, s.store.DB(), a.TenantID, jobID, false)
	if err != nil || job == nil {
		return nil, orNotFound(err)
	}
	if job.Status != pipeline.JobDraft && job.Status != pipeline.JobOpen {
		return nil, ErrJobNotOpen
	}
	sc, err := s.store.Scorecard(ctx, s.store.DB(), a.TenantID, cardID)
	if err != nil || sc == nil {
		return nil, orNotFound(err)
	}
	if sc.Status != ScorecardActive {
		return nil, ErrInactive
	}
	row := &JobScorecard{TenantID: a.TenantID, JobID: jobID, ScorecardID: cardID}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveJobScorecard(ctx, tx, row) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.job_scorecard_assigned", "recruitment_job", jobID.String())
	return row, nil
}

// ScoreInput is one competency score.
type ScoreInput struct {
	Competency string  `json:"competency"`
	Score      int     `json:"score"`
	Note       *string `json:"note"`
}

// SubmitScores is RP-05: the assigned interviewer scores every competency of the job scorecard once.
func (s *Service) SubmitScores(ctx context.Context, a Actor, interviewID uuid.UUID, in []ScoreInput) ([]InterviewScore, error) {
	me, err := s.p.Employees.GetByUserID(ctx, a.TenantID, a.UserID)
	if errors.Is(err, sharedErrors.ErrNotFound) || (err == nil && me == nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	iv, err := s.store.Interview(ctx, s.store.DB(), a.TenantID, interviewID)
	if err != nil || iv == nil || iv.InterviewerEmployeeID != me.ID {
		return nil, orNotFound(err)
	}
	if iv.Status != models.InterviewScheduled && iv.Status != models.InterviewCompleted {
		return nil, ErrInterviewState
	}
	cand, err := s.store.Candidate(ctx, s.store.DB(), a.TenantID, iv.CandidateID, false)
	if err != nil || cand == nil {
		return nil, orNotFound(err)
	}
	card, err := s.cardOf(ctx, a.TenantID, cand.JobID)
	if err != nil {
		return nil, err
	}
	want := map[string]string{}
	for _, c := range card.Competencies {
		want[strings.ToLower(c.Name)] = c.Name
	}
	rows, seen, now := make([]InterviewScore, 0, len(in)), map[string]bool{}, s.now().UTC()
	for _, x := range in {
		name, ok := want[strings.ToLower(strings.TrimSpace(x.Competency))]
		if !ok || seen[name] {
			return nil, invalid("scores", "each competency of the scorecard exactly once, and only those")
		}
		if x.Score < 1 || x.Score > 5 {
			return nil, invalid("score", "must be 1Ã¢â‚¬â€œ5")
		}
		note, err := optionalComment(x.Note)
		if err != nil {
			return nil, err
		}
		seen[name] = true
		rows = append(rows, InterviewScore{TenantID: a.TenantID, InterviewID: interviewID, Competency: name, Score: x.Score, Note: note, CreatedAt: now})
	}
	if len(rows) != len(want) {
		return nil, invalid("scores", "every competency of the scorecard needs a score")
	}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.ReplaceScores(ctx, tx, a.TenantID, interviewID, rows) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.interview_scored", "recruitment_interview", interviewID.String())
	return rows, nil
}

// cardOf returns the scorecard of a job, or ErrNoScorecard.
func (s *Service) cardOf(ctx context.Context, tenantID, jobID uuid.UUID) (*Scorecard, error) {
	link, err := s.store.JobScorecard(ctx, s.store.DB(), tenantID, jobID)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, ErrNoScorecard
	}
	card, err := s.store.Scorecard(ctx, s.store.DB(), tenantID, link.ScorecardID)
	if err != nil || card == nil {
		return nil, orNotFound(err)
	}
	return card, nil
}

// CompetencyResult is one competency average (hundredths of a point).
type CompetencyResult struct {
	Competency string       `json:"competency"`
	WeightPct  kernel.Fixed `json:"weight_pct"`
	Average    int64        `json:"average"`
	Scores     int          `json:"scores"`
}

// CandidateScorecard is RP-05's aggregate.
type CandidateScorecard struct {
	CandidateID uuid.UUID          `json:"candidate_id"`
	Interviews  int                `json:"interviews"`
	Overall     int64              `json:"overall"`
	Competences []CompetencyResult `json:"competencies"`
}

// Scorecard aggregates the submitted scores of a candidate (RP-05).
func (s *Service) Scorecard(ctx context.Context, a Actor, candidateID uuid.UUID) (*CandidateScorecard, error) {
	cand, err := s.store.Candidate(ctx, s.store.DB(), a.TenantID, candidateID, false)
	if err != nil || cand == nil {
		return nil, orNotFound(err)
	}
	card, err := s.cardOf(ctx, a.TenantID, cand.JobID)
	if err != nil {
		return nil, err
	}
	scored, err := s.store.CandidateScores(ctx, s.store.DB(), a.TenantID, candidateID)
	if err != nil {
		return nil, err
	}
	sum, n, interviews := map[string]int64{}, map[string]int{}, map[uuid.UUID]bool{}
	for _, r := range scored {
		sum[r.Competency] += int64(r.Score)
		n[r.Competency]++
		interviews[r.InterviewID] = true
	}
	out := &CandidateScorecard{CandidateID: candidateID, Interviews: len(interviews), Competences: []CompetencyResult{}}
	var weighted int64
	for _, c := range card.Competencies {
		res := CompetencyResult{Competency: c.Name, WeightPct: c.WeightPct, Scores: n[c.Name]}
		if res.Scores > 0 {
			res.Average = int64(kernel.DivRound(sum[c.Name]*100, int64(res.Scores)))
			weighted += res.Average * int64(c.WeightPct)
		}
		out.Competences = append(out.Competences, res)
	}
	out.Overall = int64(kernel.DivRound(weighted, 10000))
	return out, nil
}

// --- offer approval (RP-06, RP-07) ---

// PolicyInput replaces the offer-approval policy.
type PolicyInput struct {
	RequiredApprovals int          `json:"required_approvals"`
	ThresholdCTC      kernel.Money `json:"threshold_ctc"`
}

// GetOfferPolicy returns the policy, or the default (no approvals).
func (s *Service) GetOfferPolicy(ctx context.Context, a Actor) (*OfferPolicy, error) {
	p, err := s.store.OfferPolicy(ctx, s.store.DB(), a.TenantID)
	if err != nil || p != nil {
		return p, err
	}
	return &OfferPolicy{TenantID: a.TenantID}, nil
}

// SetOfferPolicy is RP-06.
func (s *Service) SetOfferPolicy(ctx context.Context, a Actor, in PolicyInput) (*OfferPolicy, error) {
	if in.RequiredApprovals < 0 || in.RequiredApprovals > 3 {
		return nil, invalid("required_approvals", "must be 0Ã¢â‚¬â€œ3")
	}
	if in.ThresholdCTC < 0 {
		return nil, invalid("threshold_ctc", "must be zero or more")
	}
	p := &OfferPolicy{TenantID: a.TenantID, RequiredApprovals: in.RequiredApprovals, ThresholdCTC: in.ThresholdCTC, UpdatedByUserID: a.UserID, UpdatedAt: s.now().UTC()}
	if err := s.store.InTx(ctx, func(tx *gorm.DB) error { return s.store.SaveOfferPolicy(ctx, tx, p) }); err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.offer_policy_saved", "recruitment_offer_policy", a.TenantID.String())
	return p, nil
}

// StartingStatus implements the recruitment offer gate (RP-06): pending approval when the policy requires it.
func (s *Service) StartingStatus(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, ctc kernel.Money) (string, error) {
	p, err := s.store.OfferPolicy(ctx, tx, tenantID)
	if err != nil {
		return "", err
	}
	if p != nil && p.RequiredApprovals > 0 && ctc > p.ThresholdCTC {
		return StatusPendingApproval, nil
	}
	return StatusOffered, nil
}

// DecideOffer is RP-07: approve (the last distinct approval releases the offer) or reject (withdraws it).
func (s *Service) DecideOffer(ctx context.Context, a Actor, offerID uuid.UUID, decision string, comment *string) (*models.Offer, error) {
	c, err := optionalComment(comment)
	if err != nil {
		return nil, err
	}
	if decision == DecisionRejected && c == nil {
		return nil, invalid("comment", "is required when rejecting")
	}
	var o *models.Offer
	err = s.store.InTx(ctx, func(tx *gorm.DB) error {
		if o, err = s.store.Offer(ctx, tx, a.TenantID, offerID, true); err != nil || o == nil {
			return orNotFound(err)
		}
		if o.Status != StatusPendingApproval {
			return ErrOfferState
		}
		if o.CreatedByUserID == a.UserID {
			return ErrSelfApproval
		}
		existing, err := s.store.Approvals(ctx, tx, a.TenantID, offerID)
		if err != nil {
			return err
		}
		approved := 0
		for _, x := range existing {
			if x.ApproverUserID == a.UserID {
				return ErrAlreadyDecided
			}
			if x.Decision == DecisionApproved {
				approved++
			}
		}
		now := s.now().UTC()
		if err := s.store.Create(ctx, tx, &OfferApproval{TenantID: a.TenantID, OfferID: offerID, ApproverUserID: a.UserID, Decision: decision, Comment: c, DecidedAt: now}); err != nil {
			return err
		}
		o.UpdatedAt = now
		if decision == DecisionRejected {
			o.Status, o.DecidedAt = StatusWithdrawn, &now
		} else {
			policy, err := s.GetOfferPolicy(ctx, a)
			if err != nil {
				return err
			}
			if approved+1 >= max(policy.RequiredApprovals, 1) {
				o.Status = StatusOffered
			}
		}
		if err := s.store.SaveOffer(ctx, tx, o); err != nil {
			return err
		}
		return s.store.Event(ctx, tx, a.TenantID, "recruitment.offer."+decision, map[string]string{"offer_id": o.ID.String(), "candidate_id": o.CandidateID.String()})
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, a, "recruitment.offer_"+decision, "recruitment_offer", offerID.String())
	return o, nil
}

// OfferApprovals lists the decisions of an offer (RP-07).
func (s *Service) OfferApprovals(ctx context.Context, a Actor, offerID uuid.UUID) ([]OfferApproval, error) {
	o, err := s.store.Offer(ctx, s.store.DB(), a.TenantID, offerID, false)
	if err != nil || o == nil {
		return nil, orNotFound(err)
	}
	rows, err := s.store.Approvals(ctx, s.store.DB(), a.TenantID, offerID)
	if rows == nil {
		rows = []OfferApproval{}
	}
	return rows, err
}
