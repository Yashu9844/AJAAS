package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/recruitment/dto"
	"github.com/jaas/jaas/internal/recruitment/models"
	"github.com/jaas/jaas/internal/recruitment/pipeline"
	"github.com/jaas/jaas/internal/recruitment/validators"
	"gorm.io/gorm"
)

// OfferService owns offers (FR-OF).
type OfferService struct{ base }

// NewOfferService builds an OfferService.
func NewOfferService(d Deps) *OfferService { return &OfferService{base{d}} }

// Create is R17: one open offer per candidate (RC-006); an interview-stage candidate moves to offer.
func (s *OfferService) Create(ctx context.Context, a Actor, req dto.CreateOfferRequest) (*dto.OfferResponse, error) {
	candID, err := parseUUIDField("candidate_id", req.CandidateID)
	if err != nil {
		return nil, err
	}
	o, err := s.newOffer(a, candID, req)
	if err != nil {
		return nil, err
	}
	err = s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		c, err := s.lockCandidate(ctx, tx, a.TenantID, candID)
		if err != nil {
			return err
		}
		if c.Stage != pipeline.Interview && c.Stage != pipeline.Offer {
			return ErrInvalidStage
		}
		if err := s.ensureNoLiveOffer(ctx, tx, c); err != nil {
			return err
		}
		if err := s.Repos.Offers.Create(ctx, tx, o); err != nil {
			return err
		}
		if c.Stage == pipeline.Offer {
			return nil
		}
		return s.moveStage(ctx, tx, a, c, stageChange{to: pipeline.Offer})
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.offer.created", "recruitment_offer", o.ID.String(), map[string]string{"candidate_id": candID.String()}}, nil)
	out := mapOffer(o)
	return &out, nil
}

// newOffer validates the request fields (FR-OF001) into an unsaved offer.
func (s *OfferService) newOffer(a Actor, candID uuid.UUID, req dto.CreateOfferRequest) (*models.Offer, error) {
	if err := validators.ValidateCTC(req.OfferedCTC); err != nil {
		return nil, invalid("offered_ctc", err.Error())
	}
	joining, err := validators.ParseFutureDate(req.JoiningDate, s.today())
	if err != nil {
		return nil, invalid("joining_date", err.Error())
	}
	var expires *time.Time
	if req.ExpiresOn != nil {
		d, err := validators.ParseFutureDate(*req.ExpiresOn, s.today())
		if err != nil {
			return nil, invalid("expires_on", err.Error())
		}
		if d.After(joining) {
			return nil, invalid("expires_on", validators.ErrExpiryOrder.Error())
		}
		expires = &d
	}
	return &models.Offer{TenantID: a.TenantID, CandidateID: candID, OfferedCTC: req.OfferedCTC, JoiningDate: joining, ExpiresOn: expires,
		Status: models.OfferOffered, CreatedByUserID: a.UserID}, nil
}

// ensureNoLiveOffer rejects a new offer while one is open or already accepted.
func (s *OfferService) ensureNoLiveOffer(ctx context.Context, tx *gorm.DB, c *models.Candidate) error {
	for _, status := range []string{models.OfferOffered, models.OfferAccepted} {
		o, err := s.Repos.Offers.LatestByStatus(ctx, tx, c.TenantID, c.ID, status)
		if err != nil {
			return err
		}
		if o != nil {
			return ErrOfferExists
		}
	}
	return nil
}

// List is R18.
func (s *OfferService) List(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.OfferResponse, dto.PageMeta, error) {
	rows, total, err := s.Repos.Offers.ListByCandidate(ctx, s.Tx.DB(), tenantID, candidateID, page)
	if err != nil {
		return nil, dto.PageMeta{}, err
	}
	return mapList(rows, mapOffer), page.Meta(total), nil
}

// Decide is R19 (D6-05): accepted|declined on an open, unexpired offer.
func (s *OfferService) Decide(ctx context.Context, a Actor, id uuid.UUID, decision string) (*dto.OfferResponse, error) {
	return s.close(ctx, a, id, decision)
}

// Withdraw is R20.
func (s *OfferService) Withdraw(ctx context.Context, a Actor, id uuid.UUID) (*dto.OfferResponse, error) {
	return s.close(ctx, a, id, models.OfferWithdrawn)
}

func (s *OfferService) close(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.OfferResponse, error) {
	var o *models.Offer
	err := s.Tx.InTx(ctx, func(tx *gorm.DB) error {
		var err error
		if o, err = s.Repos.Offers.FindByID(ctx, tx, a.TenantID, id); err != nil || o == nil {
			return orNotFound(err)
		}
		if _, err := s.lockCandidate(ctx, tx, a.TenantID, o.CandidateID); err != nil {
			return err
		}
		if o.Status != models.OfferOffered || (to == models.OfferAccepted && o.ExpiresOn != nil && o.ExpiresOn.Before(s.today())) {
			return ErrOfferState
		}
		now := s.Now().UTC()
		o.Status, o.DecidedAt = to, &now
		return s.Repos.Offers.Update(ctx, tx, o)
	})
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, a, auditEntry{"recruitment.offer." + to, "recruitment_offer", o.ID.String(), map[string]string{"status": to}}, nil)
	out := mapOffer(o)
	return &out, nil
}
