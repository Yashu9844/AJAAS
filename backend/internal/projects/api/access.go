// Package api is the contract between M2 contexts: the project context implements Authorizer, and the task and planning
// contexts depend on this package only (never on project/). Specification section 8.
package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// Perm is a project-level permission (specification section 8).
type Perm string

// Project-level permissions.
const (
	ProjectRead           Perm = "project.read"
	ProjectUpdate         Perm = "project.update"
	ProjectArchive        Perm = "project.archive"
	ManageMembers         Perm = "project.manage_members"
	ColumnManage          Perm = "column.manage"
	LabelManage           Perm = "label.manage"
	MilestoneManage       Perm = "milestone.manage"
	TaskCreate            Perm = "task.create"
	TaskRead              Perm = "task.read"
	TaskUpdate            Perm = "task.update"
	TaskDelete            Perm = "task.delete"
	TaskAssign            Perm = "task.assign"
	TaskComment           Perm = "task.comment"
	CommentModerate       Perm = "comment.moderate"
	TimeLog               Perm = "time.log"
	TimeManage            Perm = "time.manage"
	AttachmentUpload      Perm = "attachment.upload"
	AttachmentDelete      Perm = "attachment.delete"
	SprintManage          Perm = "sprint.manage"
	ViewShare             Perm = "view.share"
	RoleOwner                  = "owner"
	RoleManager                = "manager"
	RoleMember                 = "member"
	RoleViewer                 = "viewer"
	StatusArchived             = "archived"
	VisibilityTenant           = "tenant"
	ModeScrum                  = "scrum"
	ModeKanban                 = "kanban"
	ColumnBacklog              = "backlog"
	ColumnTodo                 = "todo"
	ColumnInProgress           = "in_progress"
	ColumnDone                 = "done"
	ColumnCanceled             = "canceled"
	maxPermissionsPerRole      = 40
)

// AllPerms lists every project permission, sorted.
var AllPerms = []Perm{ProjectRead, ProjectUpdate, ProjectArchive, ManageMembers, ColumnManage, LabelManage, MilestoneManage,
	TaskCreate, TaskRead, TaskUpdate, TaskDelete, TaskAssign, TaskComment, CommentModerate, TimeLog, TimeManage,
	AttachmentUpload, AttachmentDelete, SprintManage, ViewShare}

var builtin = buildBuiltins()

func buildBuiltins() map[string]map[Perm]bool {
	set := func(base map[Perm]bool, add ...Perm) map[Perm]bool {
		out := map[Perm]bool{}
		for p := range base {
			out[p] = true
		}
		for _, p := range add {
			out[p] = true
		}
		return out
	}
	viewer := set(nil, ProjectRead, TaskRead)
	member := set(viewer, TaskCreate, TaskUpdate, TaskAssign, TaskComment, TimeLog, AttachmentUpload)
	manager := set(member, ProjectUpdate, ManageMembers, ColumnManage, LabelManage, MilestoneManage, TaskDelete, CommentModerate,
		TimeManage, AttachmentDelete, SprintManage, ViewShare)
	owner := set(manager, ProjectArchive)
	return map[string]map[Perm]bool{RoleViewer: viewer, RoleMember: member, RoleManager: manager, RoleOwner: owner}
}

// BuiltinRoles lists the fixed role keys, most privileged first.
func BuiltinRoles() []string { return []string{RoleOwner, RoleManager, RoleMember, RoleViewer} }

// BuiltinPerms returns the permission set of a built-in role (nil when the key is unknown).
func BuiltinPerms(role string) map[Perm]bool { return builtin[role] }

// ValidPerm reports whether p is a known project permission.
func ValidPerm(p string) bool {
	for _, k := range AllPerms {
		if string(k) == p {
			return true
		}
	}
	return false
}

// SortedPerms returns the permissions in a set as sorted strings.
func SortedPerms(set map[Perm]bool) []string {
	out := make([]string, 0, len(set))
	for p, ok := range set {
		if ok {
			out = append(out, string(p))
		}
	}
	sort.Strings(out)
	return out
}

// Actor is the authenticated caller plus the tenant-level permissions the request guards resolved.
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
	CreateProject    bool // projects:create
	ReadAll          bool // projects:read_all
	ManageAll        bool // projects:manage_all
	ManageRoles      bool // project_roles:manage
}

// ProjectInfo is the part of a project every context needs.
type ProjectInfo struct {
	ID           uuid.UUID
	Key          string
	Name         string
	Status       string
	Visibility   string
	Mode         string
	TimeTracking bool
	OwnerUserID  uuid.UUID
}

// Access is what the caller may do in one project.
type Access struct {
	Project ProjectInfo
	Role    string // built-in key, custom role name, or "" for a non-member
	Member  bool
	Perms   map[Perm]bool // effective: an archived project keeps only reads and archive/restore
	Held    map[Perm]bool // what the role grants before the archive filter
}

// Can reports whether the caller holds p in the project.
func (a Access) Can(p Perm) bool { return a.Perms[p] }

// Require returns a 403 when the caller lacks p, or a 409 when an archived project blocks a write.
func (a Access) Require(p Perm) error {
	if a.Perms[p] {
		return nil
	}
	if a.Project.Status == StatusArchived && a.Held[p] {
		return kernel.Conflict("PROJECT_ARCHIVED", "the project is archived; restore it to make changes")
	}
	return kernel.Forbidden(string(p))
}

// Archived reduces a permission set to what an archived project allows (PR-03).
func Archived(held map[Perm]bool) map[Perm]bool {
	out := map[Perm]bool{}
	for _, p := range []Perm{ProjectRead, TaskRead, ProjectArchive} {
		out[p] = held[p]
	}
	return out
}

// Authorizer resolves what a caller may do in a project (the project context implements it).
type Authorizer interface {
	// Access returns the caller access to the project, or 404 when the caller cannot read it (PG4).
	Access(ctx context.Context, db *gorm.DB, a Actor, projectID uuid.UUID) (Access, error)
	// ReadableProjectIDs lists the projects the caller can read, for cross-project queries (search, my work).
	ReadableProjectIDs(ctx context.Context, db *gorm.DB, a Actor) ([]uuid.UUID, error)
}

// Flags are the context keys the route guards set for the tenant-level permissions.
const (
	FlagCreate    = "projects_create"
	FlagReadAll   = "projects_read_all"
	FlagManageAll = "projects_manage_all"
	FlagRoles     = "project_roles_manage"
)

// ActorFrom builds the Actor from the request context set by the identity guards.
func ActorFrom(c *gin.Context) (Actor, bool) {
	t, okT := c.Get("tenant_id")
	u, okU := c.Get("user_id")
	tid, ok1 := t.(uuid.UUID)
	uid, ok2 := u.(uuid.UUID)
	if !okT || !okU || !ok1 || !ok2 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized request"}})
		return Actor{}, false
	}
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), CreateProject: c.GetBool(FlagCreate),
		ReadAll: c.GetBool(FlagReadAll), ManageAll: c.GetBool(FlagManageAll), ManageRoles: c.GetBool(FlagRoles)}, true
}
