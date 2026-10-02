package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

type stubDeptRepo struct {
	byID     map[uuid.UUID]*models.Department
	byName   map[string]*models.Department
	children map[uuid.UUID][]models.Department
	roots    []models.Department
}

func (s *stubDeptRepo) Create(ctx context.Context, tx *gorm.DB, dept *models.Department) error {
	if dept.ID == uuid.Nil {
		dept.ID = uuid.New()
	}
	s.byID[dept.ID] = dept
	s.byName[strings.ToLower(dept.Name)] = dept
	return nil
}
func (s *stubDeptRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error) {
	return s.byID[id], nil
}
func (s *stubDeptRepo) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error) {
	return s.byName[strings.ToLower(name)], nil
}
func (s *stubDeptRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error) {
	out := make([]models.Department, 0, len(s.byID))
	for _, d := range s.byID {
		out = append(out, *d)
	}
	return out, int64(len(out)), nil
}
func (s *stubDeptRepo) FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error) {
	return s.children[parentID], nil
}
func (s *stubDeptRepo) FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error) {
	return s.roots, nil
}
func (s *stubDeptRepo) Update(ctx context.Context, tx *gorm.DB, dept *models.Department) error {
	s.byID[dept.ID] = dept
	return nil
}
func (s *stubDeptRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	delete(s.byID, id)
	return nil
}

type stubTeamRepo struct {
	byID map[uuid.UUID]*models.Team
}

func (s *stubTeamRepo) Create(ctx context.Context, tx *gorm.DB, team *models.Team) error {
	if team.ID == uuid.Nil {
		team.ID = uuid.New()
	}
	s.byID[team.ID] = team
	return nil
}
func (s *stubTeamRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error) {
	return s.byID[id], nil
}
func (s *stubTeamRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error) {
	return nil, 0, nil
}
func (s *stubTeamRepo) CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *stubTeamRepo) Update(ctx context.Context, tx *gorm.DB, team *models.Team) error {
	s.byID[team.ID] = team
	return nil
}
func (s *stubTeamRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	delete(s.byID, id)
	return nil
}

type stubMappingRepo struct {
	byID    map[uuid.UUID]*models.Mapping
	primary map[uuid.UUID]*models.Mapping
}

func (s *stubMappingRepo) Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	s.byID[m.ID] = m
	if m.IsPrimary {
		s.primary[m.UserID] = m
	}
	return nil
}
func (s *stubMappingRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error) {
	return s.byID[id], nil
}
func (s *stubMappingRepo) FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error) {
	return s.primary[userID], nil
}
func (s *stubMappingRepo) FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error) {
	out := make([]models.Mapping, 0)
	for _, m := range s.byID {
		if m.UserID == userID {
			out = append(out, *m)
		}
	}
	return out, int64(len(out)), nil
}
func (s *stubMappingRepo) FindReports(ctx context.Context, db *gorm.DB, tenantID, managerID uuid.UUID) ([]models.Mapping, error) {
	out := make([]models.Mapping, 0)
	for _, m := range s.byID {
		if m.ManagerUserID != nil && *m.ManagerUserID == managerID {
			out = append(out, *m)
		}
	}
	return out, nil
}
func (s *stubMappingRepo) FindByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) ([]models.Mapping, error) {
	return nil, nil
}
func (s *stubMappingRepo) CountActiveByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *stubMappingRepo) CountActiveByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *stubMappingRepo) Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	s.byID[m.ID] = m
	if m.IsPrimary {
		s.primary[m.UserID] = m
	}
	return nil
}

type stubPublisher struct{}

func (s *stubPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error {
	return nil
}

type stubAudit struct{}

func (s *stubAudit) Log(ctx context.Context, tx *gorm.DB, tenantID, userID, action, resource, resourceID string, metadata interface{}, ip, userAgent string) error {
	return nil
}

func newDeptSvc() (*stubDeptRepo, *stubTeamRepo, *stubMappingRepo, DepartmentService) {
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}, byName: map[string]*models.Department{}}
	teams := &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}
	mappings := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewDepartmentService(depts, teams, mappings, &stubPublisher{}, &stubAudit{})
	return depts, teams, mappings, svc
}

func TestDepartmentService_CreateDepartment(t *testing.T) {
	_, _, _, svc := newDeptSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	res, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Engineering"}, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "Engineering" || res.Status != "active" || res.Depth != 0 {
		t.Errorf("unexpected response: %+v", res)
	}

	// Duplicate name → conflict.
	if _, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Engineering"}, uuid.New()); err == nil {
		t.Fatal("expected conflict on duplicate name")
	}

	// Unknown parent → 404.
	parent := uuid.New().String()
	if _, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Backend", ParentDepartmentID: &parent}, uuid.New()); err == nil {
		t.Fatal("expected not-found on unknown parent")
	}
}

func TestDepartmentService_MoveCycle(t *testing.T) {
	depts, _, _, svc := newDeptSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	root, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Root"}, uuid.New())
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	rootID := root.ID
	child, err := svc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Child", ParentDepartmentID: &rootID}, uuid.New())
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	// Self-parent → 400.
	self := child.ID
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, mustParse(child.ID), dto.UpdateDepartmentRequest{ParentDepartmentID: &self}); err == nil {
		t.Fatal("expected error on self-parent")
	}

	// Move root under child → 409 cycle.
	childID := child.ID
	_ = depts
	if _, err := svc.UpdateDepartment(ctx, nil, tenantID, mustParse(root.ID), dto.UpdateDepartmentRequest{ParentDepartmentID: &childID}); err == nil {
		t.Fatal("expected cycle error moving root under child")
	}
}

func TestDepartmentService_DeactivateGuard(t *testing.T) {
	depts, teams, mappings, _ := newDeptSvc()
	ctx := context.Background()
	tenantID := uuid.New()

	res, err := NewDepartmentService(depts, teams, mappings, &stubPublisher{}, &stubAudit{}).CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "Sales"}, uuid.New())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	deptID := mustParse(res.ID)

	// Simulate an active team in the department.
	team := &models.Team{DepartmentID: deptID, Name: "Field"}
	team.TenantID = tenantID
	team.ID = uuid.New()
	teams.byID[team.ID] = team
	guarded := NewDepartmentService(depts, &countTeamRepo{stubTeamRepo: teams, count: 1}, mappings, &stubPublisher{}, &stubAudit{})

	if _, err := guarded.DeactivateDepartment(ctx, nil, tenantID, deptID, false, nil); err == nil {
		t.Fatal("expected conflict deactivating department with active teams")
	}

	// Force cascade deactivates children only and succeeds.
	if _, err := guarded.DeactivateDepartment(ctx, nil, tenantID, deptID, true, nil); err != nil {
		t.Fatalf("expected force deactivate to succeed, got: %v", err)
	}
}

type countTeamRepo struct {
	*stubTeamRepo
	count int64
}

func (c *countTeamRepo) CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	return c.count, nil
}

func mustParse(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}
