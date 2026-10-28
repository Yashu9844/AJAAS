package fields

import (
	"errors"
	"strconv"
	"testing"

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
	owner := e.user(false)
	seq := 0
	type scenario struct {
		name string
		run  func() func() error
	}
	scenarios := []scenario{
		{"definitions", func() func() error {
			seq++
			p, _ := e.task(owner, "FD"+strconv.Itoa(seq))
			nm := "Renamed"
			return func() error {
				d, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "Env", Type: TypeSelect, Options: []string{"a", "b"}})
				if err != nil {
					return err
				}
				if _, err = e.svc.UpdateDefinition(bg, owner, p.ID, d.ID, DefinitionUpdate{Name: &nm, Options: &[]string{"a"}}); err != nil {
					return err
				}
				if _, err = e.svc.Definitions(bg, owner, p.ID); err != nil {
					return err
				}
				return e.svc.DeleteDefinition(bg, owner, p.ID, d.ID)
			}
		}},
		{"values", func() func() error {
			seq++
			p, tid := e.task(owner, "FV"+strconv.Itoa(seq))
			inj.armed = false
			if _, err := e.svc.CreateDefinition(bg, owner, p.ID, DefinitionInput{Name: "Tags", Type: TypeMultiselect, Options: []string{"a", "b"}}); err != nil {
				t.Fatal(err)
			}
			return func() error {
				if _, err := e.svc.SetTaskFields(bg, owner, tid, SetInput{Values: map[string]any{"tags": []any{"a"}}}); err != nil {
					return err
				}
				if _, err := e.svc.SetTaskFields(bg, owner, tid, SetInput{Values: map[string]any{"tags": nil}}); err != nil {
					return err
				}
				_, err := e.svc.TaskFields(bg, owner, tid)
				return err
			}
		}},
	}
	for _, sc := range scenarios {
		prepare := func() func() error {
			inj.armed = false
			run := sc.run()
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
