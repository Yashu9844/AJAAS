package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	identityDTO "github.com/jaas/jaas/internal/identity/dto"
	"github.com/jaas/jaas/internal/organization/dto"
	"github.com/jaas/jaas/internal/organization/models"
	"gorm.io/gorm"
)

// failDeptRepo fails every department lookup.
type failDeptRepo struct{ stubDeptRepo }

func (f *failDeptRepo) Create(ctx context.Context, tx *gorm.DB, d *models.Department) error {
	return errBoom
}
func (f *failDeptRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error) {
	return nil, errBoom
}
func (f *failDeptRepo) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error) {
	return nil, errBoom
}
func (f *failDeptRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error) {
	return nil, 0, errBoom
}
func (f *failDeptRepo) FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error) {
	return nil, errBoom
}
func (f *failDeptRepo) FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error) {
	return nil, errBoom
}
func (f *failDeptRepo) Update(ctx context.Context, tx *gorm.DB, d *models.Department) error {
	return errBoom
}
func (f *failDeptRepo) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error {
	return errBoom
}

// failTeamRepo fails every team lookup.
type failTeamRepo struct{ stubTeamRepo }

func (f *failTeamRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error) {
	return nil, errBoom
}
func (f *failTeamRepo) FindByName(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, name string) (*models.Team, error) {
	return nil, errBoom
}
func (f *failTeamRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error) {
	return nil, 0, errBoom
}
func (f *failTeamRepo) CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error) {
	return 0, errBoom
}

// failDesigRepo fails every designation lookup.
type failDesigRepo struct{ stubDesigRepo }

func (f *failDesigRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error) {
	return nil, errBoom
}
func (f *failDesigRepo) FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error) {
	return nil, errBoom
}
func (f *failDesigRepo) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error) {
	return nil, 0, errBoom
}

// failMappingRepo fails every mapping lookup.
type failMappingRepo struct{ stubMappingRepo }

func (f *failMappingRepo) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error) {
	return nil, errBoom
}
func (f *failMappingRepo) FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error) {
	return nil, errBoom
}

// failUsers fails user resolution.
type failUsers struct{ stubUserChecker }

func (f *failUsers) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityDTO.UserResponse, error) {
	return nil, errBoom
}

func TestServiceErrorPropagation(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	id := uuid.New()

	depSvc := NewDepartmentService(&failDeptRepo{}, &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}, &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}}, &stubPublisher{}, &stubAudit{})
	if _, err := depSvc.CreateDepartment(ctx, nil, tenantID, dto.CreateDepartmentRequest{Name: "X"}, uuid.New()); !isErr(err, errBoom) {
		t.Error("dept create must propagate repo error")
	}
	if _, err := depSvc.GetDepartment(ctx, nil, tenantID, id); !isErr(err, errBoom) {
		t.Error("dept get must propagate repo error")
	}
	if _, err := depSvc.ListDepartments(ctx, nil, tenantID, 1, 20); !isErr(err, errBoom) {
		t.Error("dept list must propagate repo error")
	}
	if _, err := depSvc.UpdateDepartment(ctx, nil, tenantID, id, dto.UpdateDepartmentRequest{}); !isErr(err, errBoom) {
		t.Error("dept update must propagate repo error")
	}
	if _, err := depSvc.DeactivateDepartment(ctx, nil, tenantID, id, false, nil); !isErr(err, errBoom) {
		t.Error("dept deactivate must propagate repo error")
	}

	teamSvc := NewTeamService(&failTeamRepo{}, &failDeptRepo{}, &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}}, &stubUserChecker{users: map[uuid.UUID]*identityDTO.UserResponse{}}, &stubPublisher{}, &stubAudit{})
	if _, err := teamSvc.CreateTeam(ctx, nil, tenantID, dto.CreateTeamRequest{Name: "X", DepartmentID: id.String()}, uuid.New()); !isErr(err, errBoom) {
		t.Error("team create must propagate dept repo error")
	}
	if _, err := teamSvc.GetTeam(ctx, nil, tenantID, id); !isErr(err, errBoom) {
		t.Error("team get must propagate repo error")
	}
	if _, err := teamSvc.ListTeams(ctx, nil, tenantID, nil, 1, 20); !isErr(err, errBoom) {
		t.Error("team list must propagate repo error")
	}
	if _, err := teamSvc.UpdateTeam(ctx, nil, tenantID, id, dto.UpdateTeamRequest{}); !isErr(err, errBoom) {
		t.Error("team update must propagate repo error")
	}
	if _, err := teamSvc.DeactivateTeam(ctx, nil, tenantID, id, nil); !isErr(err, errBoom) {
		t.Error("team deactivate must propagate repo error")
	}

	desigSvc := NewDesignationService(&failDesigRepo{}, &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}}, &stubPublisher{}, &stubAudit{})
	if _, err := desigSvc.CreateDesignation(ctx, nil, tenantID, dto.CreateDesignationRequest{Title: "X"}, uuid.New()); !isErr(err, errBoom) {
		t.Error("desig create must propagate repo error")
	}
	if _, err := desigSvc.GetDesignation(ctx, nil, tenantID, id); !isErr(err, errBoom) {
		t.Error("desig get must propagate repo error")
	}
	if _, err := desigSvc.ListDesignations(ctx, nil, tenantID, 1, 20); !isErr(err, errBoom) {
		t.Error("desig list must propagate repo error")
	}
	if _, err := desigSvc.UpdateDesignation(ctx, nil, tenantID, id, dto.UpdateDesignationRequest{}); !isErr(err, errBoom) {
		t.Error("desig update must propagate repo error")
	}
	if _, err := desigSvc.DeactivateDesignation(ctx, nil, tenantID, id, nil); !isErr(err, errBoom) {
		t.Error("desig deactivate must propagate repo error")
	}

	mapSvc := NewMappingService(&failMappingRepo{}, &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}}, &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}, &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}, &failUsers{}, &stubPublisher{}, &stubAudit{})
	if _, err := mapSvc.CreateMapping(ctx, nil, tenantID, dto.CreateMappingRequest{UserID: id.String(), DepartmentID: strPtr(id.String())}, uuid.New()); !isErr(err, errBoom) {
		t.Error("mapping create must propagate user service error")
	}
	if _, err := mapSvc.GetMapping(ctx, nil, tenantID, id); !isErr(err, errBoom) {
		t.Error("mapping get must propagate repo error")
	}
	if _, err := mapSvc.UpdateMapping(ctx, nil, tenantID, id, dto.UpdateMappingRequest{}); !isErr(err, errBoom) {
		t.Error("mapping update must propagate repo error")
	}
	if _, err := mapSvc.DeactivateMapping(ctx, nil, tenantID, id, dto.DeactivateMappingRequest{}); !isErr(err, errBoom) {
		t.Error("mapping deactivate must propagate repo error")
	}

	chartSvc := NewOrgChartService(&failDeptRepo{}, &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}}, &failMappingRepo{}, &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}, nil)
	if _, err := chartSvc.GetChart(ctx, nil, tenantID, 10, false); !isErr(err, errBoom) {
		t.Error("chart must propagate roots repo error")
	}
	if _, err := chartSvc.GetUserChain(ctx, nil, tenantID, id); !isErr(err, errBoom) {
		t.Error("user chain must propagate repo error")
	}
}

func isErr(got, want error) bool {
	return got != nil && errors.Is(got, want)
}
