package automation

import (
	"errors"
	"strconv"
	"testing"
	"time"

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

// Every statement of the main flows is failed in turn: the service must return an error (or record a failed run) and
// never swallow it (DPJ-24).
func TestEveryStatementFailureIsReported(t *testing.T) {
	e := newEnv(t)
	inj := e.injector()
	seq := 0
	acts := Actions{{Type: ActionPriority, Priority: "high"}}
	scenarios := map[string]func() func() error{
		"rules": func() func() error {
			f := e.fixture("FR" + strconv.Itoa(seq))
			seq++
			name, tr := "Renamed", "task.created"
			return func() error {
				r, err := e.svc.CreateRule(bg, f.owner, f.p.ID, RuleInput{Name: "A", Trigger: "task.moved", Actions: acts})
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateRule(bg, f.owner, f.p.ID, r.ID, RuleUpdate{Name: &name, Trigger: &tr}); err != nil {
					return err
				}
				if _, err = e.svc.Rules(bg, f.owner, f.p.ID); err != nil {
					return err
				}
				if _, _, err = e.svc.Runs(bg, f.owner, f.p.ID, r.ID, RunFilter{Limit: 5}); err != nil {
					return err
				}
				return e.svc.DeleteRule(bg, f.owner, f.p.ID, r.ID)
			}
		},
		"dry run": func() func() error {
			f := e.fixture("FD" + strconv.Itoa(seq))
			seq++
			r := e.rule(f.owner, f.p.ID, RuleInput{Name: "A", Trigger: "task.created", Actions: acts})
			id := e.task(f.p, "todo", "none", f.owner.UserID)
			return func() error {
				_, err := e.svc.Test(bg, f.owner, f.p.ID, r.ID, id)
				return err
			}
		},
		"engine": func() func() error {
			f := e.fixture("FE" + strconv.Itoa(seq))
			seq++
			r := e.rule(f.owner, f.p.ID, RuleInput{Name: "A", Trigger: "task.created", Actions: acts})
			id := e.task(f.p, "todo", "none", f.owner.UserID)
			e.emit("task.created", id, f.p.ID, nil)
			return func() error {
				if _, err := e.eng.Tick(bg); err != nil {
					return err
				}
				inj.armed = false // the check below is the test's own read
				var status string
				e.db.Raw(`SELECT status FROM workflow_rule_runs WHERE rule_id = ?`, r.ID).Scan(&status)
				if status != StatusOK {
					return errors.New("run recorded as " + status)
				}
				return nil
			}
		},
	}
	for name, sc := range scenarios {
		prepare := func() func() error {
			inj.armed = false
			if err := e.db.Exec(`UPDATE automation_cursor SET last_created_at = now(), last_id = '00000000-0000-0000-0000-000000000000', lease_until = NULL`).Error; err != nil {
				t.Fatal(err)
			}
			e.eng.lastPrune = time.Time{} // the prune statement runs in every attempt
			run := sc()
			inj.armed, inj.count, inj.n = true, 0, 0
			return run
		}
		if err := prepare()(); err != nil {
			t.Fatalf("%s: dry run failed: %v", name, err)
		}
		total := inj.count
		for n := 1; n <= total; n++ {
			run := prepare()
			inj.n = n
			if err := run(); err == nil {
				t.Errorf("%s: statement %d of %d failed silently", name, n, total)
			}
			inj.armed = false
		}
	}
	inj.armed = false
}
