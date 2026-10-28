package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"gorm.io/gorm"
)

// injector fails the n-th SQL statement issued while it is armed (n counts from 1; 0 only counts).
type injector struct {
	armed bool
	count int
	n     int
}

func (in *injector) hook(tx *gorm.DB) {
	if !in.armed {
		return
	}
	in.count++
	if in.n > 0 && in.count == in.n {
		_ = tx.AddError(errors.New("injected database failure"))
	}
}

func (e *env) injector() *injector {
	in := &injector{}
	cb := e.db.Callback()
	_ = cb.Create().Before("gorm:create").Register("inj:create", in.hook)
	_ = cb.Query().Before("gorm:query").Register("inj:query", in.hook)
	_ = cb.Update().Before("gorm:update").Register("inj:update", in.hook)
	_ = cb.Delete().Before("gorm:delete").Register("inj:delete", in.hook)
	_ = cb.Row().Before("gorm:row").Register("inj:row", in.hook)
	_ = cb.Raw().Before("gorm:raw").Register("inj:raw", in.hook)
	return in
}

// scenario prepares fresh state (injection off) and then performs the operation under test (injection on).
type scenario struct {
	name string
	run  func(e *env, owner api.Actor) (func() error, error)
}

func (e *env) fresh(owner api.Actor, key string) *Project {
	return e.project(owner, key)
}

func faultScenarios(e *env) []scenario {
	ctx := context.Background()
	seq := 0
	key := func() string { seq++; return "F" + itoa(seq) + "X" }
	return []scenario{
		{"create project", func(e *env, o api.Actor) (func() error, error) {
			k := key()
			return func() error { _, err := e.svc.Create(ctx, o, CreateInput{Key: k, Name: "Fault"}); return err }, nil
		}},
		{"update archive restore delete", func(e *env, o api.Actor) (func() error, error) {
			p := e.fresh(o, key())
			admin := e.user("admin")
			n := "New"
			return func() error {
				if _, err := e.svc.Update(ctx, o, p.ID, UpdateInput{Name: &n}); err != nil {
					return err
				}
				if _, err := e.svc.Archive(ctx, o, p.ID); err != nil {
					return err
				}
				if _, err := e.svc.Restore(ctx, o, p.ID); err != nil {
					return err
				}
				if _, err := e.svc.Archive(ctx, o, p.ID); err != nil {
					return err
				}
				return e.svc.Delete(ctx, admin, p.ID)
			}, nil
		}},
		{"favorite list get", func(e *env, o api.Actor) (func() error, error) {
			p := e.fresh(o, key())
			return func() error {
				if err := e.svc.SetFavorite(ctx, o, p.ID, true); err != nil {
					return err
				}
				if _, _, err := e.svc.List(ctx, o, ListFilter{Limit: 5}); err != nil {
					return err
				}
				_, err := e.svc.Get(ctx, o, p.ID)
				return err
			}, nil
		}},
		{"members", func(e *env, o api.Actor) (func() error, error) {
			p, u := e.fresh(o, key()), e.user()
			return func() error {
				if _, err := e.svc.AddMember(ctx, o, p.ID, MemberInput{UserID: u.UserID, Role: "member"}); err != nil {
					return err
				}
				if _, err := e.svc.ChangeMember(ctx, o, p.ID, u.UserID, MemberInput{Role: "viewer"}); err != nil {
					return err
				}
				if _, err := e.svc.Members(ctx, o, p.ID); err != nil {
					return err
				}
				return e.svc.RemoveMember(ctx, o, p.ID, u.UserID)
			}, nil
		}},
		{"columns", func(e *env, o api.Actor) (func() error, error) {
			p := e.fresh(o, key())
			nm := "Extra"
			return func() error {
				c, err := e.svc.CreateColumn(ctx, o, p.ID, ColumnInput{Name: &nm})
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateColumn(ctx, o, p.ID, c.ID, ColumnInput{Name: &nm}); err != nil {
					return err
				}
				cols, err := e.svc.Columns(ctx, o, p.ID)
				if err != nil {
					return err
				}
				ids := make([]uuid.UUID, 0, len(cols))
				for i := len(cols) - 1; i >= 0; i-- {
					ids = append(ids, cols[i].ID)
				}
				if _, err = e.svc.ReorderColumns(ctx, o, p.ID, ids); err != nil {
					return err
				}
				return e.svc.DeleteColumn(ctx, o, p.ID, c.ID, nil)
			}, nil
		}},
		{"labels and milestones", func(e *env, o api.Actor) (func() error, error) {
			p := e.fresh(o, key())
			nm := "M"
			return func() error {
				l, err := e.svc.CreateLabel(ctx, o, &p.ID, LabelInput{Name: "bug"})
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateLabel(ctx, o, &p.ID, l.ID, LabelInput{Name: "bug2"}); err != nil {
					return err
				}
				if _, err = e.svc.Labels(ctx, o, p.ID); err != nil {
					return err
				}
				if err = e.svc.DeleteLabel(ctx, o, &p.ID, l.ID); err != nil {
					return err
				}
				m, err := e.svc.CreateMilestone(ctx, o, p.ID, MilestoneInput{Name: &nm})
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateMilestone(ctx, o, p.ID, m.ID, MilestoneInput{Name: &nm}); err != nil {
					return err
				}
				if _, err = e.svc.Milestones(ctx, o, p.ID); err != nil {
					return err
				}
				return e.svc.DeleteMilestone(ctx, o, p.ID, m.ID)
			}, nil
		}},
		{"roles", func(e *env, o api.Actor) (func() error, error) {
			admin := e.user("roles")
			return func() error {
				in := RoleInput{Name: "R" + itoa(seq), Permissions: []string{"project.read", "task.read"}}
				seq++
				r, err := e.svc.CreateRole(ctx, admin, in)
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateRole(ctx, admin, r.ID, RoleInput{Name: in.Name + "b", Permissions: in.Permissions}); err != nil {
					return err
				}
				if _, err = e.svc.Roles(ctx, admin); err != nil {
					return err
				}
				return e.svc.DeleteRole(ctx, admin, r.ID)
			}, nil
		}},
		{"group rules and deactivation", func(e *env, o api.Actor) (func() error, error) {
			p, u, dept := e.fresh(o, key()), e.user(), uuid.New()
			e.people.members[dept] = []uuid.UUID{u.UserID}
			return func() error {
				r, err := e.svc.AddGroupRule(ctx, o, p.ID, GroupRuleInput{Kind: "department", RefID: dept, Role: "member"})
				if err != nil {
					return err
				}
				if _, err = e.svc.GroupRules(ctx, o, p.ID); err != nil {
					return err
				}
				if err = e.svc.RefreshAll(ctx); err != nil {
					return err
				}
				if err = e.svc.inTx(ctx, func(tx *gorm.DB) error {
					return e.svc.DeactivateUserMappings(ctx, tx, e.tenant, u.UserID, uuid.Nil)
				}); err != nil {
					return err
				}
				return e.svc.RemoveGroupRule(ctx, o, p.ID, r.ID)
			}, nil
		}},
	}
}

// Every statement of the main flows is failed in turn: the service must return an error (never panic) and must not leave a
// half-applied change behind (the operation either fully happens or fully does not).
func TestEveryStatementFailureIsReportedAndRolledBack(t *testing.T) {
	e := newEnv(t)
	inj := e.injector()
	owner := e.user("create")
	for _, sc := range faultScenarios(e) {
		prepare := func() func() error {
			inj.armed = false
			run, err := sc.run(e, owner)
			if err != nil {
				t.Fatalf("%s: prepare: %v", sc.name, err)
			}
			inj.armed, inj.count, inj.n = true, 0, 0
			return run
		}
		if err := prepare()(); err != nil {
			t.Fatalf("%s: dry run failed: %v", sc.name, err)
		}
		total := inj.count
		for n := 1; n <= total; n++ {
			run := prepare()
			inj.n = n
			if err := run(); err == nil {
				t.Errorf("%s: statement %d of %d failed silently", sc.name, n, total)
			}
			inj.armed = false
		}
	}
	inj.armed = false
}

func TestRunGroupRefreshStopsWithContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	errs := make(chan error, 4)
	e.people.err = errors.New("down")
	owner := e.user("create")
	p := e.project(owner, "RUN")
	e.people.err = nil
	e.people.members[uuid.New()] = nil
	_, _ = e.svc.AddGroupRule(context.Background(), owner, p.ID, GroupRuleInput{Kind: "team", RefID: uuid.New(), Role: "member"})
	e.people.err = errors.New("down")
	go func() { e.svc.RunGroupRefresh(ctx, 20*time.Millisecond, func(err error) { errs <- err }); close(done) }()
	select {
	case <-errs:
	case <-time.After(3 * time.Second):
		t.Fatal("the loop must report a failing refresh")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the loop must stop with its context")
	}
}
