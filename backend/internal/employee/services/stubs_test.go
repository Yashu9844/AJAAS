package services_test

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/repositories"
	identityDto "github.com/jaas/jaas/internal/identity/dto"
	"gorm.io/gorm"
)

type mockProfileRepo struct {
	byID     map[uuid.UUID]*models.EmployeeProfile
	byUserID map[uuid.UUID]*models.EmployeeProfile
	byCode   map[string]*models.EmployeeProfile
}

func newMockProfileRepo() *mockProfileRepo {
	return &mockProfileRepo{
		byID:     make(map[uuid.UUID]*models.EmployeeProfile),
		byUserID: make(map[uuid.UUID]*models.EmployeeProfile),
		byCode:   make(map[string]*models.EmployeeProfile),
	}
}

func (m *mockProfileRepo) WithTx(tx *gorm.DB) repositories.EmployeeProfileRepository {
	return m
}

func (m *mockProfileRepo) Create(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error {
	m.byID[profile.ID] = profile
	m.byUserID[profile.UserID] = profile
	m.byCode[strings.ToLower(profile.EmployeeCode)] = profile
	return nil
}

func (m *mockProfileRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.EmployeeProfile, error) {
	p, ok := m.byID[id]
	if !ok || p.TenantID != tenantID {
		return nil, nil
	}
	return p, nil
}

func (m *mockProfileRepo) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*models.EmployeeProfile, error) {
	p, ok := m.byUserID[userID]
	if !ok || p.TenantID != tenantID {
		return nil, nil
	}
	return p, nil
}

func (m *mockProfileRepo) GetByCode(ctx context.Context, tenantID uuid.UUID, code string) (*models.EmployeeProfile, error) {
	p, ok := m.byCode[strings.ToLower(code)]
	if !ok || p.TenantID != tenantID {
		return nil, nil
	}
	return p, nil
}

func (m *mockProfileRepo) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error) {
	var list []models.EmployeeProfile
	for _, p := range m.byID {
		if p.TenantID == tenantID {
			if filter.Status != "" && p.Status != filter.Status {
				continue
			}
			list = append(list, *p)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockProfileRepo) Update(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error {
	m.byID[profile.ID] = profile
	m.byUserID[profile.UserID] = profile
	m.byCode[strings.ToLower(profile.EmployeeCode)] = profile
	return nil
}

func (m *mockProfileRepo) UpdateEmploymentDetail(ctx context.Context, tx *gorm.DB, detail *models.EmploymentDetail) error {
	for _, p := range m.byID {
		if p.ID == detail.EmployeeProfileID {
			p.EmploymentDetail = detail
		}
	}
	return nil
}

func (m *mockProfileRepo) UpdateContact(ctx context.Context, tx *gorm.DB, contact *models.EmployeeContact) error {
	for _, p := range m.byID {
		if p.ID == contact.EmployeeProfileID {
			p.Contact = contact
		}
	}
	return nil
}

func (m *mockProfileRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	if p, ok := m.byID[id]; ok {
		delete(m.byUserID, p.UserID)
		delete(m.byCode, strings.ToLower(p.EmployeeCode))
		delete(m.byID, id)
	}
	return nil
}

type mockStatutoryRepo struct {
	byProfileID map[uuid.UUID]*models.EmployeeStatutory
}

func newMockStatutoryRepo() *mockStatutoryRepo {
	return &mockStatutoryRepo{
		byProfileID: make(map[uuid.UUID]*models.EmployeeStatutory),
	}
}

func (m *mockStatutoryRepo) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) (*models.EmployeeStatutory, error) {
	s, ok := m.byProfileID[profileID]
	if !ok || s.TenantID != tenantID {
		return nil, nil
	}
	return s, nil
}

func (m *mockStatutoryRepo) Upsert(ctx context.Context, tx *gorm.DB, statutory *models.EmployeeStatutory) error {
	m.byProfileID[statutory.EmployeeProfileID] = statutory
	return nil
}

type mockDocumentRepo struct {
	docs map[uuid.UUID]*models.EmployeeDocument
}

func newMockDocumentRepo() *mockDocumentRepo {
	return &mockDocumentRepo{
		docs: make(map[uuid.UUID]*models.EmployeeDocument),
	}
}

func (m *mockDocumentRepo) Create(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error {
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	m.docs[doc.ID] = doc
	return nil
}

func (m *mockDocumentRepo) GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*models.EmployeeDocument, error) {
	d, ok := m.docs[docID]
	if !ok || d.TenantID != tenantID {
		return nil, nil
	}
	return d, nil
}

func (m *mockDocumentRepo) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeDocument, error) {
	var list []models.EmployeeDocument
	for _, d := range m.docs {
		if d.TenantID == tenantID && d.EmployeeProfileID == profileID {
			list = append(list, *d)
		}
	}
	return list, nil
}

func (m *mockDocumentRepo) Update(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error {
	m.docs[doc.ID] = doc
	return nil
}

type mockTimelineRepo struct {
	timelines map[uuid.UUID][]models.EmployeeTimeline
}

func newMockTimelineRepo() *mockTimelineRepo {
	return &mockTimelineRepo{
		timelines: make(map[uuid.UUID][]models.EmployeeTimeline),
	}
}

func (m *mockTimelineRepo) Create(ctx context.Context, tx *gorm.DB, timeline *models.EmployeeTimeline) error {
	if timeline.ID == uuid.Nil {
		timeline.ID = uuid.New()
	}
	m.timelines[timeline.EmployeeProfileID] = append(m.timelines[timeline.EmployeeProfileID], *timeline)
	return nil
}

func (m *mockTimelineRepo) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeTimeline, error) {
	return m.timelines[profileID], nil
}

type mockUserService struct {
	users map[uuid.UUID]*identityDto.UserResponse
}

func newMockUserService() *mockUserService {
	return &mockUserService{
		users: make(map[uuid.UUID]*identityDto.UserResponse),
	}
}

func (m *mockUserService) CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req identityDto.CreateUserRequest, correlationID uuid.UUID) (*identityDto.UserResponse, error) {
	return nil, nil
}
func (m *mockUserService) InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req identityDto.InviteUserRequest, correlationID uuid.UUID) (*identityDto.UserResponse, error) {
	return nil, nil
}
func (m *mockUserService) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityDto.UserResponse, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}
func (m *mockUserService) ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*identityDto.UserListResponse, error) {
	return nil, nil
}
func (m *mockUserService) UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req identityDto.UpdateUserRequest) (*identityDto.UserResponse, error) {
	return nil, nil
}
func (m *mockUserService) DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error {
	return nil
}

type mockPublisher struct {
	events []string
}

func (m *mockPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error {
	m.events = append(m.events, routingKey)
	return nil
}

func (m *mockUserService) ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*identityDto.UserResponse, error) {
	return &identityDto.UserResponse{ID: id.String(), Status: "active"}, nil
}
