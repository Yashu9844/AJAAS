package testdb

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/shared/identityport"
	"gorm.io/gorm"
)

// FakeRoleTable holds the stand-in role assignments inside the test database so they roll back with the transaction that
// wrote them, as the real M0 assignments do.
const FakeRoleTable = `CREATE TABLE IF NOT EXISTS fake_resource_roles (
	tenant_id UUID NOT NULL, user_id UUID NOT NULL, resource_id UUID NOT NULL, role_id UUID NOT NULL, source TEXT NOT NULL,
	PRIMARY KEY (tenant_id, user_id, resource_id))`

// Resources is a stand-in for M0 role assignments on projects. It knows the four predefined project roles with the
// permissions of the built-in roles and lets a test define custom roles. The real M0 is covered by its own tests and by the
// end-to-end test in internal/app.
type Resources struct {
	mu    sync.Mutex
	roles map[uuid.UUID]fakeRole
	byKey map[string]uuid.UUID
}

type fakeRole struct {
	name, template string
	perms          []string
}

type fakeRow struct {
	UserID, ResourceID, RoleID uuid.UUID
	Source                     string
}

// NewResources returns the stand-in with the predefined project roles.
func NewResources() *Resources {
	r := &Resources{roles: map[uuid.UUID]fakeRole{}, byKey: map[string]uuid.UUID{}}
	for _, k := range api.BuiltinRoles() {
		var perms []string
		for p := range api.BuiltinPerms(k) {
			perms = append(perms, colon(string(p)))
		}
		id := uuid.New()
		r.roles[id] = fakeRole{name: "Project " + k, template: "project_" + k, perms: perms}
		r.byKey["project_"+k] = id
	}
	return r
}

func colon(p string) string { return strings.Replace(p, ".", ":", 1) }

// DefineRole adds a custom project role holding perms (written as "task.read") and returns its id.
func (r *Resources) DefineRole(name string, perms ...string) uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := uuid.New()
	ks := make([]string, len(perms))
	for i, p := range perms {
		ks[i] = colon(p)
	}
	r.roles[id] = fakeRole{name: name, perms: ks}
	return id
}

var _ identityport.ResourceAccess = (*Resources)(nil)

func (r *Resources) role(id, user uuid.UUID, source string) identityport.ResourceRole {
	r.mu.Lock()
	defer r.mu.Unlock()
	fr := r.roles[id]
	return identityport.ResourceRole{UserID: user, RoleID: id, RoleName: fr.name, TemplateKey: fr.template, Source: source}
}

func (r *Resources) rows(ctx context.Context, db *gorm.DB, where string, args ...any) ([]fakeRow, error) {
	var rows []fakeRow
	err := db.WithContext(ctx).Raw(`SELECT user_id, resource_id, role_id, source FROM fake_resource_roles WHERE `+where, args...).Scan(&rows).Error
	return rows, err
}

// PermissionsOn returns the role and permissions the user holds on the project.
func (r *Resources) PermissionsOn(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef, userID uuid.UUID) (identityport.ResourceRole, []string, error) {
	rows, err := r.rows(ctx, db, `tenant_id = ? AND user_id = ? AND resource_id = ?`, ref.TenantID, userID, ref.ID)
	if err != nil || len(rows) == 0 {
		return identityport.ResourceRole{}, nil, err
	}
	r.mu.Lock()
	perms := r.roles[rows[0].RoleID].perms
	r.mu.Unlock()
	return r.role(rows[0].RoleID, userID, rows[0].Source), perms, nil
}

// RolesFor maps the projects to the role the user holds on each.
func (r *Resources) RolesFor(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]identityport.ResourceRole, error) {
	out := map[uuid.UUID]identityport.ResourceRole{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.rows(ctx, db, `tenant_id = ? AND user_id = ? AND resource_id IN ?`, ref.TenantID, userID, ids)
	for _, row := range rows {
		out[row.ResourceID] = r.role(row.RoleID, userID, row.Source)
	}
	return out, err
}

// AssignRole records the assignment; an unknown role is refused like M0 does.
func (r *Resources) AssignRole(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef, a identityport.RoleAssignment) error {
	r.mu.Lock()
	_, ok := r.roles[a.RoleID]
	r.mu.Unlock()
	if !ok {
		return identityport.ErrRoleNotFound
	}
	return db.WithContext(ctx).Exec(`INSERT INTO fake_resource_roles (tenant_id, user_id, resource_id, role_id, source) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, resource_id) DO UPDATE SET role_id = EXCLUDED.role_id, source = EXCLUDED.source`,
		ref.TenantID, a.UserID, ref.ID, a.RoleID, a.Source).Error
}

// RevokeRole removes the assignment.
func (r *Resources) RevokeRole(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef, _, userID uuid.UUID) error {
	return db.WithContext(ctx).Exec(`DELETE FROM fake_resource_roles WHERE tenant_id = ? AND user_id = ? AND resource_id = ?`, ref.TenantID, userID, ref.ID).Error
}

// RolesOn lists the assignments on the project.
func (r *Resources) RolesOn(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef) ([]identityport.ResourceRole, error) {
	rows, err := r.rows(ctx, db, `tenant_id = ? AND resource_id = ?`, ref.TenantID, ref.ID)
	out := make([]identityport.ResourceRole, 0, len(rows))
	for _, row := range rows {
		out = append(out, r.role(row.RoleID, row.UserID, row.Source))
	}
	return out, err
}

// AvailableRoles lists the predefined and custom roles.
func (r *Resources) AvailableRoles(context.Context, *gorm.DB, identityport.ResourceRef) ([]identityport.ResourceRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []identityport.ResourceRole
	for id, fr := range r.roles {
		out = append(out, identityport.ResourceRole{RoleID: id, RoleName: fr.name, TemplateKey: fr.template})
	}
	return out, nil
}

// RoleByTemplate returns a predefined role id.
func (r *Resources) RoleByTemplate(_ context.Context, _ *gorm.DB, _ uuid.UUID, key string) (uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byKey[key]
	if !ok {
		return uuid.Nil, identityport.ErrRoleNotFound
	}
	return id, nil
}

// CountWithTemplate counts assignments on the project whose role is the template.
func (r *Resources) CountWithTemplate(ctx context.Context, db *gorm.DB, ref identityport.ResourceRef, key string) (int64, error) {
	rows, err := r.rows(ctx, db, `tenant_id = ? AND resource_id = ?`, ref.TenantID, ref.ID)
	var n int64
	for _, row := range rows {
		if r.role(row.RoleID, row.UserID, row.Source).TemplateKey == key {
			n++
		}
	}
	return n, err
}
