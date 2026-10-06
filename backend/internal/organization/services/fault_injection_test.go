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

// Fault injection: run an operation fault-free, count dependency calls, then fail each call in turn. A failing
// dependency must never panic and must surface as an error for at least one call.

type faults struct{ calls, failAt int }

func (f *faults) hit() error {
	f.calls++
	if f.failAt > 0 && f.calls == f.failAt {
		return errBoom
	}
	return nil
}

func sweep(t *testing.T, name string, op func(f *faults) error) {
	t.Helper()
	happy := &faults{}
	if err := op(happy); err != nil {
		t.Fatalf("%s: happy path failed: %v", name, err)
	}
	surfaced := 0
	for i := 1; i <= happy.calls; i++ {
		f := &faults{failAt: i}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic when dependency call #%d fails: %v", name, i, r)
				}
			}()
			if err := op(f); errors.Is(err, errBoom) {
				surfaced++
			}
		}()
	}
	if happy.calls > 0 && surfaced == 0 {
		t.Errorf("%s: no injected failure surfaced as an error", name)
	}
}

type fDept struct {
	*stubDeptRepo
	f *faults
}

func (r fDept) Create(ctx context.Context, tx *gorm.DB, d *models.Department) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubDeptRepo.Create(ctx, tx, d)
}
func (r fDept) FindByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Department, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDeptRepo.FindByID(ctx, db, t, id)
}
func (r fDept) FindByName(ctx context.Context, db *gorm.DB, t uuid.UUID, n string) (*models.Department, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDeptRepo.FindByName(ctx, db, t, n)
}
func (r fDept) FindAll(ctx context.Context, db *gorm.DB, t uuid.UUID, p, pp int) ([]models.Department, int64, error) {
	if err := r.f.hit(); err != nil {
		return nil, 0, err
	}
	return r.stubDeptRepo.FindAll(ctx, db, t, p, pp)
}
func (r fDept) FindChildren(ctx context.Context, db *gorm.DB, t, p uuid.UUID) ([]models.Department, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDeptRepo.FindChildren(ctx, db, t, p)
}
func (r fDept) FindRoots(ctx context.Context, db *gorm.DB, t uuid.UUID) ([]models.Department, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDeptRepo.FindRoots(ctx, db, t)
}
func (r fDept) Update(ctx context.Context, tx *gorm.DB, d *models.Department) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubDeptRepo.Update(ctx, tx, d)
}

type fTeam struct {
	*stubTeamRepo
	f *faults
}

func (r fTeam) Create(ctx context.Context, tx *gorm.DB, x *models.Team) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubTeamRepo.Create(ctx, tx, x)
}
func (r fTeam) FindByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Team, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubTeamRepo.FindByID(ctx, db, t, id)
}
func (r fTeam) FindByName(ctx context.Context, db *gorm.DB, t, d uuid.UUID, n string) (*models.Team, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubTeamRepo.FindByName(ctx, db, t, d, n)
}
func (r fTeam) FindAll(ctx context.Context, db *gorm.DB, t uuid.UUID, d *uuid.UUID, p, pp int) ([]models.Team, int64, error) {
	if err := r.f.hit(); err != nil {
		return nil, 0, err
	}
	return r.stubTeamRepo.FindAll(ctx, db, t, d, p, pp)
}
func (r fTeam) CountByDepartment(ctx context.Context, db *gorm.DB, t, d uuid.UUID) (int64, error) {
	if err := r.f.hit(); err != nil {
		return 0, err
	}
	return r.stubTeamRepo.CountByDepartment(ctx, db, t, d)
}
func (r fTeam) Update(ctx context.Context, tx *gorm.DB, x *models.Team) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubTeamRepo.Update(ctx, tx, x)
}

type fMap struct {
	*stubMappingRepo
	f *faults
}

func (r fMap) Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubMappingRepo.Create(ctx, tx, m)
}
func (r fMap) FindByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Mapping, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubMappingRepo.FindByID(ctx, db, t, id)
}
func (r fMap) FindPrimaryByUser(ctx context.Context, db *gorm.DB, t, u uuid.UUID) (*models.Mapping, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubMappingRepo.FindPrimaryByUser(ctx, db, t, u)
}
func (r fMap) FindByUser(ctx context.Context, db *gorm.DB, t, u uuid.UUID, p, pp int) ([]models.Mapping, int64, error) {
	if err := r.f.hit(); err != nil {
		return nil, 0, err
	}
	return r.stubMappingRepo.FindByUser(ctx, db, t, u, p, pp)
}
func (r fMap) FindReports(ctx context.Context, db *gorm.DB, t, m uuid.UUID) ([]models.Mapping, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubMappingRepo.FindReports(ctx, db, t, m)
}
func (r fMap) CountActiveByDepartment(ctx context.Context, db *gorm.DB, t, d uuid.UUID) (int64, error) {
	if err := r.f.hit(); err != nil {
		return 0, err
	}
	return r.stubMappingRepo.CountActiveByDepartment(ctx, db, t, d)
}
func (r fMap) CountActiveByTeam(ctx context.Context, db *gorm.DB, t, d uuid.UUID) (int64, error) {
	if err := r.f.hit(); err != nil {
		return 0, err
	}
	return r.stubMappingRepo.CountActiveByTeam(ctx, db, t, d)
}
func (r fMap) CountActiveByDesignation(ctx context.Context, db *gorm.DB, t, d uuid.UUID) (int64, error) {
	if err := r.f.hit(); err != nil {
		return 0, err
	}
	return r.stubMappingRepo.CountActiveByDesignation(ctx, db, t, d)
}
func (r fMap) Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubMappingRepo.Update(ctx, tx, m)
}

type fDesig struct {
	*stubDesigRepo
	f *faults
}

func (r fDesig) Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubDesigRepo.Create(ctx, tx, d)
}
func (r fDesig) FindByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*models.Designation, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDesigRepo.FindByID(ctx, db, t, id)
}
func (r fDesig) FindByTitle(ctx context.Context, db *gorm.DB, t uuid.UUID, title string) (*models.Designation, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.stubDesigRepo.FindByTitle(ctx, db, t, title)
}
func (r fDesig) FindAll(ctx context.Context, db *gorm.DB, t uuid.UUID, p, pp int) ([]models.Designation, int64, error) {
	if err := r.f.hit(); err != nil {
		return nil, 0, err
	}
	return r.stubDesigRepo.FindAll(ctx, db, t, p, pp)
}
func (r fDesig) Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.stubDesigRepo.Update(ctx, tx, d)
}

type fUsers struct {
	*stubUserChecker
	f *faults
}

func (u fUsers) GetByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*identityDTO.UserResponse, error) {
	if err := u.f.hit(); err != nil {
		return nil, err
	}
	return u.stubUserChecker.GetByID(ctx, db, t, id)
}

// world is a small seeded org: root > child departments, a team, a designation, two active users and one mapping.
type world struct {
	f                                *faults
	tenant, root, child, team, desig uuid.UUID
	boss, emp, mapping               uuid.UUID
	depts                            *stubDeptRepo
	teams                            *stubTeamRepo
	maps                             *stubMappingRepo
	desigs                           *stubDesigRepo
	users                            *stubUserChecker
	deptSvc                          DepartmentService
	teamSvc                          TeamService
	desigSvc                         DesignationService
	mapSvc                           MappingService
	chartSvc                         OrgChartService
}

func newWorld(f *faults) *world {
	w := &world{f: f, tenant: uuid.New(), root: uuid.New(), child: uuid.New(), team: uuid.New(), desig: uuid.New(), boss: uuid.New(), emp: uuid.New(), mapping: uuid.New()}
	w.depts = &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}, byName: map[string]*models.Department{}, children: map[uuid.UUID][]models.Department{}}
	w.teams = &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}, byName: map[string]*models.Team{}}
	w.maps = &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	w.desigs = &stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}
	w.users = &stubUserChecker{users: map[uuid.UUID]*identityDTO.UserResponse{w.boss: activeUser(w.boss), w.emp: activeUser(w.emp)}}

	r := &models.Department{TenantID: w.tenant, Name: "Root", Status: "active"}
	r.ID = w.root
	c := &models.Department{TenantID: w.tenant, Name: "Child", Status: "active", ParentDepartmentID: &w.root}
	c.ID = w.child
	w.depts.byID[w.root], w.depts.byID[w.child] = r, c
	w.depts.byName["root"], w.depts.byName["child"] = r, c
	w.depts.children[w.root] = []models.Department{*c}
	w.depts.roots = []models.Department{*r}
	tm := &models.Team{TenantID: w.tenant, DepartmentID: w.root, Name: "Team", Status: "active"}
	tm.ID = w.team
	w.teams.byID[w.team] = tm
	w.teams.byName[teamNameKey(w.root, "Team")] = tm
	d := &models.Designation{TenantID: w.tenant, Title: "Eng", Status: "active"}
	d.ID = w.desig
	w.desigs.byID[w.desig], w.desigs.byTitle["eng"] = d, d
	m := &models.Mapping{TenantID: w.tenant, UserID: w.emp, DepartmentID: &w.root, TeamID: &w.team, DesignationID: &w.desig, ManagerUserID: &w.boss, IsPrimary: true, Status: "active"}
	m.ID = w.mapping
	w.maps.byID[w.mapping] = m
	w.maps.primary[w.emp] = m

	dr, tr, mr, gr, ur := fDept{w.depts, f}, fTeam{w.teams, f}, fMap{w.maps, f}, fDesig{w.desigs, f}, fUsers{w.users, f}
	w.deptSvc = NewDepartmentService(dr, tr, mr, &stubPublisher{}, &stubAudit{})
	w.teamSvc = NewTeamService(tr, dr, mr, ur, &stubPublisher{}, &stubAudit{})
	w.desigSvc = NewDesignationService(gr, mr, &stubPublisher{}, &stubAudit{})
	w.mapSvc = NewMappingService(mr, dr, tr, gr, ur, &stubPublisher{}, &stubAudit{})
	w.chartSvc = NewOrgChartService(dr, tr, mr, gr, nil)
	return w
}

func sp(s string) *string { return &s }

func TestFaults_Departments(t *testing.T) {
	ctx := context.Background()
	sweep(t, "CreateDepartment(with parent)", func(f *faults) error {
		w := newWorld(f)
		p := w.root.String()
		_, err := w.deptSvc.CreateDepartment(ctx, nil, w.tenant, dto.CreateDepartmentRequest{Name: "New", Code: sp("NEW1"), Description: sp("d"), ParentDepartmentID: &p}, uuid.New())
		return err
	})
	sweep(t, "GetDepartment", func(f *faults) error {
		w := newWorld(f)
		_, err := w.deptSvc.GetDepartment(ctx, nil, w.tenant, w.child)
		return err
	})
	sweep(t, "ListDepartments", func(f *faults) error {
		w := newWorld(f)
		_, err := w.deptSvc.ListDepartments(ctx, nil, w.tenant, 1, 20)
		return err
	})
	sweep(t, "UpdateDepartment(reparent)", func(f *faults) error {
		w := newWorld(f)
		other := &models.Department{TenantID: w.tenant, Name: "Other", Status: "active"}
		other.ID = uuid.New()
		w.depts.byID[other.ID] = other
		np := other.ID.String()
		_, err := w.deptSvc.UpdateDepartment(ctx, nil, w.tenant, w.child, dto.UpdateDepartmentRequest{Name: sp("Renamed"), Description: sp("d"), ParentDepartmentID: &np})
		return err
	})
	sweep(t, "DeactivateDepartment(force)", func(f *faults) error {
		w := newWorld(f)
		_, err := w.deptSvc.DeactivateDepartment(ctx, nil, w.tenant, w.root, true, sp("r"))
		return err
	})
}

func TestFaults_Teams(t *testing.T) {
	ctx := context.Background()
	sweep(t, "CreateTeam", func(f *faults) error {
		w := newWorld(f)
		lead := w.boss.String()
		_, err := w.teamSvc.CreateTeam(ctx, nil, w.tenant, dto.CreateTeamRequest{Name: "New", Code: sp("NT1"), Description: sp("d"), DepartmentID: w.root.String(), LeadUserID: &lead}, uuid.New())
		return err
	})
	sweep(t, "GetTeam", func(f *faults) error {
		w := newWorld(f)
		_, err := w.teamSvc.GetTeam(ctx, nil, w.tenant, w.team)
		return err
	})
	sweep(t, "ListTeams", func(f *faults) error {
		w := newWorld(f)
		_, err := w.teamSvc.ListTeams(ctx, nil, w.tenant, &w.root, 1, 20)
		return err
	})
	sweep(t, "UpdateTeam(move+lead)", func(f *faults) error {
		w := newWorld(f)
		d := w.child.String()
		lead := w.boss.String()
		_, err := w.teamSvc.UpdateTeam(ctx, nil, w.tenant, w.team, dto.UpdateTeamRequest{Name: sp("Renamed"), Description: sp("d"), DepartmentID: &d, LeadUserID: &lead})
		return err
	})
	sweep(t, "DeactivateTeam", func(f *faults) error {
		w := newWorld(f)
		delete(w.maps.byID, w.mapping) // no active mappings blocking
		_, err := w.teamSvc.DeactivateTeam(ctx, nil, w.tenant, w.team, sp("r"))
		return err
	})
}

func TestFaults_Designations(t *testing.T) {
	ctx := context.Background()
	lvl := 3
	sweep(t, "CreateDesignation", func(f *faults) error {
		w := newWorld(f)
		_, err := w.desigSvc.CreateDesignation(ctx, nil, w.tenant, dto.CreateDesignationRequest{Title: "New", Code: sp("NEW"), Level: &lvl, Description: sp("d")}, uuid.New())
		return err
	})
	sweep(t, "GetDesignation", func(f *faults) error {
		w := newWorld(f)
		_, err := w.desigSvc.GetDesignation(ctx, nil, w.tenant, w.desig)
		return err
	})
	sweep(t, "ListDesignations", func(f *faults) error {
		w := newWorld(f)
		_, err := w.desigSvc.ListDesignations(ctx, nil, w.tenant, 1, 20)
		return err
	})
	sweep(t, "UpdateDesignation", func(f *faults) error {
		w := newWorld(f)
		_, err := w.desigSvc.UpdateDesignation(ctx, nil, w.tenant, w.desig, dto.UpdateDesignationRequest{Title: sp("Renamed"), Level: &lvl, Description: sp("d")})
		return err
	})
	sweep(t, "DeactivateDesignation", func(f *faults) error {
		w := newWorld(f)
		delete(w.maps.byID, w.mapping)
		_, err := w.desigSvc.DeactivateDesignation(ctx, nil, w.tenant, w.desig, sp("r"))
		return err
	})
}

func TestFaults_MappingsAndChart(t *testing.T) {
	ctx := context.Background()
	sweep(t, "CreateMapping(all targets)", func(f *faults) error {
		w := newWorld(f)
		third := uuid.New()
		w.users.users[third] = activeUser(third)
		d, tm, g, mgr := w.root.String(), w.team.String(), w.desig.String(), w.boss.String()
		_, err := w.mapSvc.CreateMapping(ctx, nil, w.tenant, dto.CreateMappingRequest{UserID: third.String(), DepartmentID: &d, TeamID: &tm, DesignationID: &g, ManagerUserID: &mgr, IsPrimary: true}, uuid.New())
		return err
	})
	sweep(t, "GetMapping", func(f *faults) error {
		w := newWorld(f)
		_, err := w.mapSvc.GetMapping(ctx, nil, w.tenant, w.mapping)
		return err
	})
	sweep(t, "ListUserMappings", func(f *faults) error {
		w := newWorld(f)
		_, err := w.mapSvc.ListUserMappings(ctx, nil, w.tenant, w.emp, 1, 20)
		return err
	})
	sweep(t, "UpdateMapping", func(f *faults) error {
		w := newWorld(f)
		d, tm, g := w.root.String(), w.team.String(), w.desig.String()
		prim := true
		mgr := w.boss.String()
		_, err := w.mapSvc.UpdateMapping(ctx, nil, w.tenant, w.mapping, dto.UpdateMappingRequest{DepartmentID: &d, TeamID: &tm, DesignationID: &g, IsPrimary: &prim, ManagerUserID: &mgr})
		return err
	})
	sweep(t, "DeactivateMapping", func(f *faults) error {
		w := newWorld(f)
		_, err := w.mapSvc.DeactivateMapping(ctx, nil, w.tenant, w.mapping, dto.DeactivateMappingRequest{Reason: sp("r")})
		return err
	})
	sweep(t, "DeactivateUserMappings", func(f *faults) error {
		w := newWorld(f)
		return w.mapSvc.DeactivateUserMappings(ctx, nil, w.tenant, w.emp, uuid.New())
	})
	sweep(t, "GetChart", func(f *faults) error {
		w := newWorld(f)
		_, err := w.chartSvc.GetChart(ctx, nil, w.tenant, 10, true)
		return err
	})
	sweep(t, "GetUserChain", func(f *faults) error {
		w := newWorld(f)
		_, err := w.chartSvc.GetUserChain(ctx, nil, w.tenant, w.emp)
		return err
	})
	sweep(t, "GetUserChain(manager)", func(f *faults) error {
		w := newWorld(f)
		_, err := w.chartSvc.GetUserChain(ctx, nil, w.tenant, w.boss)
		return err
	})
}
