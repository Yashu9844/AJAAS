package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/access"
	"github.com/jaas/jaas/internal/identity/models"
	"gorm.io/gorm"
)

// fakeResourceRepo is an in-memory repositories.ResourceRoleRepository.
type fakeResourceRepo struct {
	rows  []models.ResourceRoleAssignment
	roles map[uuid.UUID]*models.Role
	err   error
}

func (f *fakeResourceRepo) match(a models.ResourceRoleAssignment, ref access.ResourceRef) bool {
	return a.TenantID == ref.TenantID && a.ResourceType == ref.Type && a.ResourceID == ref.ID
}

func (f *fakeResourceRepo) withRole(a models.ResourceRoleAssignment) models.ResourceRoleAssignment {
	a.Role = f.roles[a.RoleID]
	return a
}

func (f *fakeResourceRepo) Assign(_ context.Context, _ *gorm.DB, a *models.ResourceRoleAssignment) error {
	if f.err != nil {
		return f.err
	}
	for i, r := range f.rows {
		if r.UserID == a.UserID && f.match(r, access.ResourceRef{TenantID: a.TenantID, Type: a.ResourceType, ID: a.ResourceID}) {
			f.rows[i] = *a
			return nil
		}
	}
	f.rows = append(f.rows, *a)
	return nil
}

func (f *fakeResourceRepo) Revoke(_ context.Context, _ *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for i, r := range f.rows {
		if r.UserID == userID && f.match(r, ref) {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeResourceRepo) Find(_ context.Context, _ *gorm.DB, ref access.ResourceRef, userID uuid.UUID) (*models.ResourceRoleAssignment, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, r := range f.rows {
		if r.UserID == userID && f.match(r, ref) {
			a := f.withRole(r)
			return &a, nil
		}
	}
	return nil, nil
}

func (f *fakeResourceRepo) ListByResource(_ context.Context, _ *gorm.DB, ref access.ResourceRef) ([]models.ResourceRoleAssignment, error) {
	var out []models.ResourceRoleAssignment
	for _, r := range f.rows {
		if f.match(r, ref) {
			out = append(out, f.withRole(r))
		}
	}
	return out, f.err
}

func (f *fakeResourceRepo) ListForUser(_ context.Context, _ *gorm.DB, ref access.ResourceRef, userID uuid.UUID, ids []uuid.UUID) ([]models.ResourceRoleAssignment, error) {
	var out []models.ResourceRoleAssignment
	for _, r := range f.rows {
		for _, id := range ids {
			if r.UserID == userID && r.TenantID == ref.TenantID && r.ResourceType == ref.Type && r.ResourceID == id {
				out = append(out, f.withRole(r))
			}
		}
	}
	return out, f.err
}

func (f *fakeResourceRepo) CountWithTemplate(_ context.Context, _ *gorm.DB, ref access.ResourceRef, key string) (int64, error) {
	var n int64
	for _, r := range f.rows {
		if role := f.roles[r.RoleID]; f.match(r, ref) && role != nil && role.TemplateKey != nil && *role.TemplateKey == key {
			n++
		}
	}
	return n, f.err
}

func (f *fakeResourceRepo) RoleInUse(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (int64, error) {
	return int64(len(f.rows)), f.err
}

type auditCall struct{ action, resource, resourceID, actor string }

type resourceEnv struct {
	svc    ResourceAccess
	f      *fakeAccess
	repo   *fakeResourceRepo
	audits *[]auditCall
	member *models.Role
	ref    access.ResourceRef
}

func newResourceEnv() resourceEnv {
	f := newFakeAccess()
	key := "project_member"
	member := f.role("Project member", nil, map[string]string{"project:read": "tenant", "task:read": "tenant"})
	member.AppliesTo, member.TemplateKey = access.AppliesProject, &key
	tenantRole := f.role("Staff", nil, map[string]string{"users:read": "tenant"})
	tenantRole.AppliesTo = access.AppliesTenant
	repo := &fakeResourceRepo{roles: f.roles}
	calls := &[]auditCall{}
	audit := &MockAuditService{LogFunc: func(_ context.Context, _ *gorm.DB, _, user, action, resource, id string, _ interface{}, _, _ string) error {
		*calls = append(*calls, auditCall{action, resource, id, user})
		return nil
	}}
	roles := &MockRoleRepository{FindByIDFunc: func(_ context.Context, _ *gorm.DB, _, id uuid.UUID) (*models.Role, error) { return f.roles[id], nil }}
	svc := NewResourceAccess(ResourceAccessDeps{Repo: repo, Roles: roles, Access: f, Resolver: f.resolver(&memCache{m: map[string]string{}}, now), Audit: audit})
	return resourceEnv{svc, f, repo, calls, member, access.ResourceRef{TenantID: f.tenant, Type: access.AppliesProject, ID: uuid.New()}}
}

func TestResourceAccessAssignResolveRevoke(t *testing.T) {
	e := newResourceEnv()
	ctx, actor, user := context.Background(), uuid.New(), uuid.New()
	if err := e.svc.AssignRole(ctx, nil, e.ref, access.RoleAssignment{ActorID: actor, UserID: user, RoleID: e.member.ID}); err != nil {
		t.Fatal(err)
	}
	role, perms, err := e.svc.PermissionsOn(ctx, nil, e.ref, user)
	if err != nil || role.RoleID != e.member.ID || role.TemplateKey != "project_member" || len(perms) != 2 {
		t.Fatalf("permissions on resource: %+v %v %v", role, perms, err)
	}
	if got, _ := e.svc.RolesFor(ctx, nil, e.ref, user, []uuid.UUID{e.ref.ID, uuid.New()}); len(got) != 1 {
		t.Fatalf("roles for: %v", got)
	}
	if on, _ := e.svc.RolesOn(ctx, nil, e.ref); len(on) != 1 || on[0].Source != "" {
		t.Fatalf("roles on: %+v", on)
	}
	if n, _ := e.svc.CountWithTemplate(ctx, nil, e.ref, "project_member"); n != 1 {
		t.Fatalf("count with template: %d", n)
	}
	if err := e.svc.RevokeRole(ctx, nil, e.ref, actor, user); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RevokeRole(ctx, nil, e.ref, actor, user); err != nil {
		t.Fatal("revoking a missing role is not an error")
	}
	if r, p, _ := e.svc.PermissionsOn(ctx, nil, e.ref, user); r.RoleID != uuid.Nil || p != nil {
		t.Fatal("no role after revoke")
	}
	want := []auditCall{
		{"resource_role.assigned", access.AppliesProject, e.ref.ID.String(), actor.String()},
		{"resource_role.revoked", access.AppliesProject, e.ref.ID.String(), actor.String()},
	}
	if len(*e.audits) != 2 || (*e.audits)[0] != want[0] || (*e.audits)[1] != want[1] {
		t.Fatalf("audit records the resource type and actor, once per real change: %+v", *e.audits)
	}
}

func TestResourceAccessAuditUsesResourceTypeAndSystemActor(t *testing.T) {
	e := newResourceEnv()
	user := uuid.New()
	if err := e.svc.AssignRole(context.Background(), nil, e.ref, access.RoleAssignment{UserID: user, RoleID: e.member.ID, Source: "group"}); err != nil {
		t.Fatal(err)
	}
	got := (*e.audits)[0]
	if got.resource != e.ref.Type || got.actor != "" {
		t.Fatalf("audit resource must be the ref type and a system change has no actor: %+v", got)
	}
	if e.repo.rows[0].AssignedBy != nil || e.repo.rows[0].Source != "group" {
		t.Fatalf("system assignment: %+v", e.repo.rows[0])
	}
}

func TestResourceAccessRefusals(t *testing.T) {
	e := newResourceEnv()
	ctx, user := context.Background(), uuid.New()
	bad := e.ref
	bad.Type = "spaceship"
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"assign type", e.svc.AssignRole(ctx, nil, bad, access.RoleAssignment{UserID: user, RoleID: e.member.ID}), access.ErrResourceType},
		{"revoke type", e.svc.RevokeRole(ctx, nil, bad, uuid.Nil, user), access.ErrResourceType},
		{"unknown role", e.svc.AssignRole(ctx, nil, e.ref, access.RoleAssignment{UserID: user, RoleID: uuid.New()}), access.ErrRoleNotFound},
	}
	for _, r := range e.f.roles {
		if r.AppliesTo == access.AppliesTenant {
			checks = append(checks, struct {
				name string
				err  error
				want error
			}{"tenant role on project", e.svc.AssignRole(ctx, nil, e.ref, access.RoleAssignment{UserID: user, RoleID: r.ID}), access.ErrRoleNotApplicable})
		}
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, c.err, c.want)
		}
	}
	if _, _, err := e.svc.PermissionsOn(ctx, nil, bad, user); !errors.Is(err, access.ErrResourceType) {
		t.Error("permissions on bad type")
	}
	if _, err := e.svc.RolesFor(ctx, nil, bad, user, nil); !errors.Is(err, access.ErrResourceType) {
		t.Error("roles for bad type")
	}
	if _, err := e.svc.RolesOn(ctx, nil, bad); !errors.Is(err, access.ErrResourceType) {
		t.Error("roles on bad type")
	}
	if _, err := e.svc.AvailableRoles(ctx, nil, bad); !errors.Is(err, access.ErrResourceType) {
		t.Error("available roles bad type")
	}
	if _, err := e.svc.CountWithTemplate(ctx, nil, bad, "x"); !errors.Is(err, access.ErrResourceType) {
		t.Error("count bad type")
	}
	if len(*e.audits) != 0 {
		t.Fatal("refusals are not audited")
	}
}

func TestResourceAccessCatalogueAndErrors(t *testing.T) {
	e := newResourceEnv()
	ctx := context.Background()
	avail, err := e.svc.AvailableRoles(ctx, nil, e.ref)
	if err != nil || len(avail) != 1 || avail[0].RoleID != e.member.ID || avail[0].TemplateKey != "project_member" {
		t.Fatalf("only project roles are assignable on a project: %+v %v", avail, err)
	}
	if id, err := e.svc.RoleByTemplate(ctx, nil, e.f.tenant, "project_member"); err != nil || id != e.member.ID {
		t.Fatalf("role by template: %v %v", id, err)
	}
	if _, err := e.svc.RoleByTemplate(ctx, nil, e.f.tenant, "nope"); err == nil {
		t.Fatal("missing template is an error")
	}
	e.repo.err = errBoom
	if err := e.svc.AssignRole(ctx, nil, e.ref, access.RoleAssignment{UserID: uuid.New(), RoleID: e.member.ID}); !errors.Is(err, errBoom) {
		t.Fatalf("repository error surfaces: %v", err)
	}
	if _, _, err := e.svc.PermissionsOn(ctx, nil, e.ref, uuid.New()); !errors.Is(err, errBoom) {
		t.Fatal("find error surfaces")
	}
	if _, err := e.svc.RolesFor(ctx, nil, e.ref, uuid.New(), []uuid.UUID{e.ref.ID}); !errors.Is(err, errBoom) {
		t.Fatal("list error surfaces")
	}
	if _, err := e.svc.RolesOn(ctx, nil, e.ref); !errors.Is(err, errBoom) {
		t.Fatal("list by resource error surfaces")
	}
	if err := e.svc.RevokeRole(ctx, nil, e.ref, uuid.Nil, uuid.New()); !errors.Is(err, errBoom) {
		t.Fatal("revoke error surfaces")
	}
	e.repo.err = nil
	e.f.err = errBoom
	if _, err := e.svc.AvailableRoles(ctx, nil, e.ref); !errors.Is(err, errBoom) {
		t.Fatal("tenant roles error surfaces")
	}
	if _, err := e.svc.RoleByTemplate(ctx, nil, e.f.tenant, "project_member"); !errors.Is(err, errBoom) {
		t.Fatal("tenant roles error surfaces in template lookup")
	}
}
