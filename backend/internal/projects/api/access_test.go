package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestBuiltinRolesAreNested(t *testing.T) {
	order := BuiltinRoles()
	if len(order) != 4 || order[0] != RoleOwner || order[3] != RoleViewer {
		t.Fatalf("roles: %v", order)
	}
	for i := 0; i+1 < len(order); i++ {
		hi, lo := BuiltinPerms(order[i]), BuiltinPerms(order[i+1])
		for p := range lo {
			if !hi[p] {
				t.Errorf("%s must include %s of %s", order[i], p, order[i+1])
			}
		}
	}
	if BuiltinPerms("nobody") != nil || BuiltinPerms(RoleViewer)[TaskCreate] || !BuiltinPerms(RoleOwner)[ProjectArchive] || BuiltinPerms(RoleManager)[ProjectArchive] {
		t.Fatal("role permission sets")
	}
	if len(BuiltinPerms(RoleOwner)) != len(AllPerms) {
		t.Fatal("the owner holds every permission")
	}
}

func TestValidPermAndSortedPerms(t *testing.T) {
	if !ValidPerm("task.create") || ValidPerm("task.fly") {
		t.Fatal("ValidPerm")
	}
	got := SortedPerms(map[Perm]bool{TaskRead: true, ProjectRead: true, TaskCreate: false})
	if len(got) != 2 || got[0] != "project.read" || got[1] != "task.read" {
		t.Fatalf("sorted: %v", got)
	}
}

func TestRequireDistinguishesForbiddenFromArchived(t *testing.T) {
	held := map[Perm]bool{ProjectRead: true, TaskRead: true, TaskCreate: true, ProjectArchive: true}
	acc := Access{Project: ProjectInfo{Status: StatusArchived}, Perms: Archived(held), Held: held}
	if acc.Can(TaskCreate) || !acc.Can(ProjectArchive) || !acc.Can(TaskRead) {
		t.Fatalf("archived keeps reads and restore only: %v", acc.Perms)
	}
	if err := acc.Require(TaskCreate); err == nil || err.Error() == "" {
		t.Fatal("a held permission on an archived project is a conflict")
	}
	if err := acc.Require(TaskDelete); err == nil {
		t.Fatal("a permission never held is forbidden")
	}
	if err := acc.Require(TaskRead); err != nil {
		t.Fatal(err)
	}
	live := Access{Project: ProjectInfo{Status: "active"}, Perms: held, Held: held}
	if err := live.Require(TaskDelete); err == nil {
		t.Fatal("forbidden on an active project")
	}
}

func TestActorFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	run := func(set func(*gin.Context)) (Actor, bool, int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		set(c)
		a, ok := ActorFrom(c)
		return a, ok, w.Code
	}
	if _, ok, code := run(func(*gin.Context) {}); ok || code != http.StatusUnauthorized {
		t.Fatal("no identity is a 401")
	}
	tid, uid := uuid.New(), uuid.New()
	a, ok, _ := run(func(c *gin.Context) {
		c.Set("tenant_id", tid)
		c.Set("user_id", uid)
		c.Set(FlagCreate, true)
		c.Set(FlagManageAll, true)
	})
	if !ok || a.TenantID != tid || a.UserID != uid || !a.CreateProject || !a.ManageAll || a.ReadAll || a.ManageRoles {
		t.Fatalf("actor: %+v", a)
	}
	if _, ok, _ = run(func(c *gin.Context) { c.Set("tenant_id", "x"); c.Set("user_id", uid) }); ok {
		t.Fatal("wrongly typed identity is rejected")
	}
}
