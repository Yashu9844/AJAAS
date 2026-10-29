package recurrence

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

// Every statement of the main flows is failed in turn: the service must return an error and never swallow it (DPJ-24).
func TestEveryStatementFailureIsReported(t *testing.T) {
	e := newEnv(t)
	inj := e.injector()
	f := e.fixture("FI")
	seq := 0
	scenarios := map[string]func() func() error{
		"crud": func() func() error {
			return func() error {
				r, err := e.svc.Create(bg, f.owner, f.p.ID, Input{TemplateTaskID: f.tpl, Frequency: Weekly})
				if err != nil {
					return err
				}
				off := false
				if _, err = e.svc.Patch(bg, f.owner, f.p.ID, r.ID, Update{Active: &off}); err != nil {
					return err
				}
				if _, err = e.svc.List(bg, f.owner, f.p.ID); err != nil {
					return err
				}
				return e.svc.Delete(bg, f.owner, f.p.ID, r.ID)
			}
		},
		"scheduler": func() func() error {
			g := e.fixture("FS" + strconv.Itoa(seq))
			seq++
			r := e.create(g, Input{Frequency: Daily})
			e.due(r, e.clock.Add(-time.Minute))
			return func() error {
				_, err := e.svc.Tick(bg)
				return err
			}
		},
		"run-now": func() func() error {
			g := e.fixture("FR" + strconv.Itoa(seq))
			seq++
			r := e.create(g, Input{Frequency: Daily})
			return func() error {
				_, err := e.svc.RunNow(bg, g.owner, g.p.ID, r.ID)
				return err
			}
		},
	}
	for name, sc := range scenarios {
		prepare := func() func() error {
			inj.armed = false
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
