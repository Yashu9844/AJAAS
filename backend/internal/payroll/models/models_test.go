package models

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm/schema"
)

func parse(t *testing.T, m interface{}) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(m, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T: %v", m, err)
	}
	return s
}

func findIndex(s *schema.Schema, name string) *schema.Index {
	for _, idx := range s.ParseIndexes() {
		if idx.Name == name {
			return idx
		}
	}
	return nil
}

func TestPayrollModels_TableNames(t *testing.T) {
	for want, m := range map[string]interface{}{
		"payroll_structures": &Structure{}, "payroll_components": &Component{}, "payroll_assignments": &Assignment{},
		"payroll_runs": &Run{}, "payroll_payslips": &Payslip{}, "payroll_payslip_lines": &PayslipLine{},
		"payroll_events_outbox": &OutboxEvent{},
	} {
		if got := parse(t, m).Table; got != want {
			t.Errorf("%T table %q, want %q", m, got, want)
		}
	}
}

// TestPayrollModels_Indexes guards spec §8 index contracts (parity with migrations 000038–000044).
func TestPayrollModels_Indexes(t *testing.T) {
	cases := []struct {
		model        interface{}
		index        string
		unique       bool
		where, expr1 string
		fields       []string
	}{
		{&Structure{}, "uq_payroll_structures_tenant_name", true, "deleted_at IS NULL", "lower(name)", []string{"tenant_id", "name"}},
		{&Component{}, "uq_payroll_components_structure_code", true, "", "", []string{"structure_id", "code"}},
		{&Assignment{}, "idx_payroll_assignments_employee", false, "", "", []string{"tenant_id", "employee_profile_id", "effective_from"}},
		{&Run{}, "uq_payroll_runs_tenant_period", true, "", "", []string{"tenant_id", "year", "month"}},
		{&Payslip{}, "uq_payroll_payslips_run_employee", true, "", "", []string{"run_id", "employee_profile_id"}},
		{&Payslip{}, "idx_payroll_payslips_employee", false, "", "", []string{"tenant_id", "employee_profile_id"}},
		{&PayslipLine{}, "idx_payroll_payslip_lines_payslip", false, "", "", []string{"payslip_id"}},
		{&OutboxEvent{}, "uq_payroll_outbox_event_id", true, "", "", []string{"event_id"}},
		{&OutboxEvent{}, "idx_payroll_outbox_published", false, "published = false", "", []string{"published"}},
	}
	for _, tc := range cases {
		idx := findIndex(parse(t, tc.model), tc.index)
		if idx == nil {
			t.Errorf("%T: missing %s", tc.model, tc.index)
			continue
		}
		if (idx.Class == "UNIQUE") != tc.unique || idx.Where != tc.where || len(idx.Fields) != len(tc.fields) {
			t.Errorf("%s: unique=%v where=%q fields=%d", tc.index, idx.Class == "UNIQUE", idx.Where, len(idx.Fields))
			continue
		}
		for i, f := range idx.Fields {
			if f.DBName != tc.fields[i] {
				t.Errorf("%s field[%d] = %s, want %s", tc.index, i, f.DBName, tc.fields[i])
			}
		}
		if tc.expr1 != "" && idx.Fields[1].Expression != tc.expr1 {
			t.Errorf("%s expression %q, want %q", tc.index, idx.Fields[1].Expression, tc.expr1)
		}
	}
}

func TestPayrollModels_ColumnTypes(t *testing.T) {
	cols := map[interface{}]map[string]string{
		&Component{}:   {"value": "numeric(14,2)", "position": "integer"},
		&Assignment{}:  {"annual_ctc": "numeric(14,2)", "effective_from": "date", "effective_to": "date"},
		&Run{}:         {"month": "integer", "year": "integer", "employee_count": "integer", "gross_total": "numeric(14,2)", "deduction_total": "numeric(14,2)", "net_total": "numeric(14,2)", "warnings": "jsonb"},
		&Payslip{}:     {"annual_ctc": "numeric(14,2)", "days_in_month": "integer", "payable_days": "numeric(7,2)", "lop_days": "numeric(7,2)", "gross": "numeric(14,2)", "deductions": "numeric(14,2)", "net": "numeric(14,2)"},
		&PayslipLine{}: {"amount": "numeric(14,2)", "position": "integer"},
		&OutboxEvent{}: {"attempts": "integer", "payload": "jsonb"},
	}
	for m, want := range cols {
		s := parse(t, m)
		for col, typ := range want {
			if f := s.LookUpField(col); f == nil || string(f.DataType) != typ {
				t.Errorf("%T.%s must be %s", m, col, typ)
			}
		}
	}
}

func TestPayrollModels_DeletionAndIDs(t *testing.T) {
	if parse(t, &Structure{}).LookUpField("deleted_at") == nil {
		t.Error("structures soft-delete")
	}
	for _, m := range []interface{}{&Component{}, &Assignment{}, &Run{}, &Payslip{}, &PayslipLine{}, &OutboxEvent{}} {
		if parse(t, m).LookUpField("deleted_at") != nil {
			t.Errorf("%T must not soft-delete", m)
		}
	}
	keep := uuid.New()
	type hook struct {
		run func() error
		id  *uuid.UUID
	}
	st, c, a, r, p, l, o := &Structure{}, &Component{}, &Assignment{}, &Run{}, &Payslip{}, &PayslipLine{}, &OutboxEvent{}
	hooks := []hook{
		{func() error { return st.BeforeCreate(nil) }, &st.ID}, {func() error { return c.BeforeCreate(nil) }, &c.ID},
		{func() error { return a.BeforeCreate(nil) }, &a.ID}, {func() error { return r.BeforeCreate(nil) }, &r.ID},
		{func() error { return p.BeforeCreate(nil) }, &p.ID}, {func() error { return l.BeforeCreate(nil) }, &l.ID},
		{func() error { return o.BeforeCreate(nil) }, &o.ID},
	}
	for i, h := range hooks {
		if err := h.run(); err != nil || *h.id == uuid.Nil {
			t.Fatalf("hook %d did not assign id", i)
		}
		*h.id = keep
		_ = h.run()
		if *h.id != keep {
			t.Fatalf("hook %d overwrote an existing id", i)
		}
	}
}
