package services

import (
	"context"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // FR-L001: IANA zones validate the same on every host

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/organization/dto"
	"github.com/jaas/jaas/internal/hr/organization/events"
	"github.com/jaas/jaas/internal/hr/organization/models"
	"github.com/jaas/jaas/internal/hr/organization/repositories"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"github.com/jaas/jaas/internal/shared/queue"
	"gorm.io/gorm"
)

// Location defaults and formats (FR-L001).
const (
	defaultCountry  = "IN"
	defaultTimeZone = "Asia/Kolkata"
)

var (
	locationCode = regexp.MustCompile(`^[A-Z0-9_-]{2,32}$`)
	stateCode    = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)
	countryCode  = regexp.MustCompile(`^[A-Z]{2}$`)
)

// LocationService handles work-site lifecycle (FR-L001..FR-L007). Audit rows come from the route's audit trail.
type LocationService interface {
	CreateLocation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateLocationRequest, correlationID uuid.UUID) (*dto.LocationResponse, error)
	GetLocation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.LocationResponse, error)
	ListLocations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f dto.LocationFilter, page, perPage int) (*dto.LocationListResponse, error)
	UpdateLocation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateLocationRequest, correlationID uuid.UUID) (*dto.LocationResponse, error)
	SetLocationStatus(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, status string, correlationID uuid.UUID) (*dto.LocationResponse, error)
}

type locationService struct {
	repo        repositories.LocationRepository
	mappingRepo repositories.MappingRepository
	publisher   queue.EventPublisher
}

// NewLocationService creates a LocationService.
func NewLocationService(repo repositories.LocationRepository, mappingRepo repositories.MappingRepository, publisher queue.EventPublisher) LocationService {
	return &locationService{repo: repo, mappingRepo: mappingRepo, publisher: publisher}
}

func mapLocation(l *models.Location) *dto.LocationResponse {
	return &dto.LocationResponse{
		ID: l.ID.String(), TenantID: l.TenantID.String(), Code: l.Code, Name: l.Name, AddressLine1: l.AddressLine1, AddressLine2: l.AddressLine2,
		City: l.City, StateCode: l.StateCode, CountryCode: l.CountryCode, PostalCode: l.PostalCode, TimeZone: l.TimeZone, Status: l.Status,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

func invalidLocation(msg string) error {
	return &sharedErrors.AppError{Code: "VALIDATION_ERROR", Message: msg, StatusCode: 400}
}

func (s *locationService) CreateLocation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateLocationRequest, correlationID uuid.UUID) (*dto.LocationResponse, error) {
	l := &models.Location{Code: strings.ToUpper(strings.TrimSpace(req.Code)), Name: req.Name, AddressLine1: req.AddressLine1, AddressLine2: req.AddressLine2,
		City: req.City, StateCode: req.StateCode, CountryCode: defaultCountry, PostalCode: req.PostalCode, TimeZone: defaultTimeZone, Status: "active"}
	l.TenantID = tenantID
	if req.CountryCode != nil {
		l.CountryCode = *req.CountryCode
	}
	if req.TimeZone != nil {
		l.TimeZone = *req.TimeZone
	}
	if !locationCode.MatchString(l.Code) {
		return nil, invalidLocation("code must be 2–32 characters of A–Z, 0–9, '_' or '-'")
	}
	if err := normalizeLocation(l); err != nil {
		return nil, err
	}
	if err := s.unique(ctx, tx, l); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, tx, l); err != nil {
		return nil, err
	}
	s.emit(ctx, tx, events.TypeLocationCreated, events.RoutingKeyLocationCreated, l, correlationID)
	return mapLocation(l), nil
}

// normalizeLocation trims and checks the editable fields (FR-L001, FR-L003).
func normalizeLocation(l *models.Location) error {
	l.Name, l.City = strings.TrimSpace(l.Name), strings.TrimSpace(l.City)
	l.StateCode, l.CountryCode = strings.ToUpper(strings.TrimSpace(l.StateCode)), strings.ToUpper(strings.TrimSpace(l.CountryCode))
	l.TimeZone = strings.TrimSpace(l.TimeZone)
	switch {
	case l.Name == "" || len([]rune(l.Name)) > 150:
		return invalidLocation("name must be 1–150 characters")
	case l.City == "" || len([]rune(l.City)) > 100:
		return invalidLocation("city must be 1–100 characters")
	case !stateCode.MatchString(l.StateCode):
		return invalidLocation("state_code must be 1–10 letters or digits (e.g. KA)")
	case !countryCode.MatchString(l.CountryCode):
		return invalidLocation("country_code must be an ISO 3166-1 alpha-2 code (e.g. IN)")
	}
	if l.TimeZone == "" || l.TimeZone == "Local" || !strings.Contains(l.TimeZone, "/") && l.TimeZone != "UTC" {
		return invalidLocation("time_zone must be an IANA zone (e.g. Asia/Kolkata)")
	}
	if _, err := time.LoadLocation(l.TimeZone); err != nil {
		return invalidLocation("time_zone must be an IANA zone (e.g. Asia/Kolkata)")
	}
	return nil
}

// unique enforces per-tenant unique code and case-insensitive name (FR-L001).
func (s *locationService) unique(ctx context.Context, db *gorm.DB, l *models.Location) error {
	byCode, err := s.repo.FindByCode(ctx, db, l.TenantID, l.Code)
	if err != nil {
		return err
	}
	byName, err := s.repo.FindByName(ctx, db, l.TenantID, l.Name)
	if err != nil {
		return err
	}
	if (byCode != nil && byCode.ID != l.ID) || (byName != nil && byName.ID != l.ID) {
		return &sharedErrors.AppError{Code: "CONFLICT", Message: "a location with this code or name already exists", StatusCode: 409}
	}
	return nil
}

func (s *locationService) GetLocation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.LocationResponse, error) {
	l, err := s.find(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	return mapLocation(l), nil
}

func (s *locationService) ListLocations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f dto.LocationFilter, page, perPage int) (*dto.LocationListResponse, error) {
	if f.Status != "" && f.Status != "active" && f.Status != "inactive" {
		return nil, invalidLocation("status must be active or inactive")
	}
	f.StateCode = strings.ToUpper(strings.TrimSpace(f.StateCode))
	f.Search = strings.TrimSpace(f.Search)
	rows, total, err := s.repo.FindAll(ctx, db, tenantID, f, page, perPage)
	if err != nil {
		return nil, err
	}
	data := make([]dto.LocationResponse, len(rows))
	for i := range rows {
		data[i] = *mapLocation(&rows[i])
	}
	return &dto.LocationListResponse{Data: data, Meta: dto.NewPaginationMeta(page, perPage, total)}, nil
}

func (s *locationService) UpdateLocation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateLocationRequest, correlationID uuid.UUID) (*dto.LocationResponse, error) {
	l, err := s.find(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if req.Code != nil && strings.ToUpper(strings.TrimSpace(*req.Code)) != l.Code {
		return nil, invalidLocation("code is immutable")
	}
	for dst, src := range map[*string]*string{&l.Name: req.Name, &l.City: req.City, &l.StateCode: req.StateCode, &l.CountryCode: req.CountryCode, &l.TimeZone: req.TimeZone} {
		if src != nil {
			*dst = *src
		}
	}
	for dst, src := range map[**string]*string{&l.AddressLine1: req.AddressLine1, &l.AddressLine2: req.AddressLine2, &l.PostalCode: req.PostalCode} {
		if src != nil {
			v := strings.TrimSpace(*src)
			*dst = &v
			if v == "" {
				*dst = nil
			}
		}
	}
	if err := normalizeLocation(l); err != nil {
		return nil, err
	}
	if err := s.unique(ctx, tx, l); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, tx, l); err != nil {
		return nil, err
	}
	s.emit(ctx, tx, events.TypeLocationUpdated, events.RoutingKeyLocationUpdated, l, correlationID)
	return mapLocation(l), nil
}

// SetLocationStatus deactivates (blocked while active mappings reference the location) or reactivates (FR-L004).
func (s *locationService) SetLocationStatus(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, status string, correlationID uuid.UUID) (*dto.LocationResponse, error) {
	l, err := s.find(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if l.Status == status {
		return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "location is already " + status, StatusCode: 409}
	}
	if status == "inactive" {
		n, err := s.mappingRepo.CountActiveByLocation(ctx, tx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, &sharedErrors.AppError{Code: "CONFLICT", Message: "location has active mappings; move them first", StatusCode: 409}
		}
	}
	l.Status = status
	if err := s.repo.Update(ctx, tx, l); err != nil {
		return nil, err
	}
	typ, key := events.TypeLocationActivated, events.RoutingKeyLocationActivated
	if status == "inactive" {
		typ, key = events.TypeLocationDeactivated, events.RoutingKeyLocationDeactivated
	}
	s.emit(ctx, tx, typ, key, l, correlationID)
	return mapLocation(l), nil
}

func (s *locationService) find(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Location, error) {
	l, err := s.repo.FindByID(ctx, db, tenantID, id)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, sharedErrors.ErrNotFound
	}
	return l, nil
}

func (s *locationService) emit(ctx context.Context, tx *gorm.DB, typ, key string, l *models.Location, correlationID uuid.UUID) {
	publishOrOutbox(ctx, tx, s.publisher, events.Exchange, events.NewEvent(typ, key, l.TenantID, correlationID,
		events.LocationPayload{LocationID: l.ID, Code: l.Code, StateCode: l.StateCode, CountryCode: l.CountryCode, Status: l.Status}))
}
