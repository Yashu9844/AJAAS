package models

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm/schema"
)

func parse(t *testing.T, model interface{}) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T: %v", model, err)
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

func TestLeaveModels_TableNames(t *testing.T) {
	cases := map[string]interface{}{
		"leave_types":         &LeaveType{},
		"leave_holidays":      &Holiday{},
		"leave_balances":      &Balance{},
		"leave_requests":      &Request{},
		"leave_ledger":        &LedgerEntry{},
		"leave_events_outbox": &OutboxEvent{},
	}
	for want, model := range cases {
		if got := parse(t, model).Table; got != want {
			t.Errorf("%T table = %q, want %q", model, got, want)
		}
	}
}

// TestLeaveModels_Indexes guards spec §8 index contracts, incl. partial/expression parity with SQL (D3-14 style).
func TestLeaveModels_Indexes(t *testing.T) {
	cases := []struct {
		model  interface{}
		index  string
		unique bool
		where  string
		fields []string
	}{
		{&LeaveType{}, "uq_leave_types_tenant_name", true, "deleted_at IS NULL", []string{"tenant_id", "name"}},
		{&LeaveType{}, "uq_leave_types_tenant_code", true, "deleted_at IS NULL", []string{"tenant_id", "code"}},
		{&Holiday{}, "uq_leave_holidays_tenant_date", true, "deleted_at IS NULL", []string{"tenant_id", "holiday_date"}},
		{&Balance{}, "uq_leave_balances_employee_type_year", true, "", []string{"tenant_id", "employee_profile_id", "leave_type_id", "year"}},
		{&Request{}, "idx_leave_requests_employee_start", false, "", []string{"tenant_id", "employee_profile_id", "start_date"}},
		{&Request{}, "idx_leave_requests_tenant_status", false, "", []string{"tenant_id", "status"}},
		{&LedgerEntry{}, "idx_leave_ledger_balance", false, "", []string{"tenant_id", "employee_profile_id", "leave_type_id", "year"}},
		{&OutboxEvent{}, "uq_leave_outbox_event_id", true, "", []string{"event_id"}},
		{&OutboxEvent{}, "idx_leave_outbox_published", false, "published = false", []string{"published"}},
	}
	for _, tc := range cases {
		idx := findIndex(parse(t, tc.model), tc.index)
		if idx == nil {
			t.Errorf("%T: missing index %s", tc.model, tc.index)
			continue
		}
		if (idx.Class == "UNIQUE") != tc.unique || idx.Where != tc.where {
			t.Errorf("%s: unique=%v where=%q, want %v %q", tc.index, idx.Class == "UNIQUE", idx.Where, tc.unique, tc.where)
		}
		if len(idx.Fields) != len(tc.fields) {
			t.Fatalf("%s has %d fields, want %d", tc.index, len(idx.Fields), len(tc.fields))
		}
		for i, f := range idx.Fields {
			if f.DBName != tc.fields[i] {
				t.Errorf("%s field[%d] = %s, want %s", tc.index, i, f.DBName, tc.fields[i])
			}
		}
	}
	name := findIndex(parse(t, &LeaveType{}), "uq_leave_types_tenant_name")
	if name.Fields[1].Expression != "lower(name)" {
		t.Errorf("type name uniqueness must be case-insensitive, got %q", name.Fields[1].Expression)
	}
}

// Column types must match migrations 000031–000036 (NUMERIC(7,2) days, INTEGER counters).
func TestLeaveModels_ColumnTypes(t *testing.T) {
	cols := map[interface{}]map[string]string{
		&LeaveType{}:   {"annual_allowance": "numeric(7,2)", "carry_forward_limit": "numeric(7,2)", "max_consecutive_days": "integer", "min_notice_days": "integer"},
		&Balance{}:     {"year": "integer", "opening": "numeric(7,2)", "accrued": "numeric(7,2)", "adjusted": "numeric(7,2)", "used": "numeric(7,2)", "reserved": "numeric(7,2)"},
		&Request{}:     {"total_days": "numeric(7,2)", "start_date": "date", "end_date": "date"},
		&LedgerEntry{}: {"year": "integer", "days": "numeric(7,2)"},
		&OutboxEvent{}: {"attempts": "integer", "payload": "jsonb"},
		&Holiday{}:     {"holiday_date": "date"},
	}
	for model, want := range cols {
		s := parse(t, model)
		for col, typ := range want {
			if f := s.LookUpField(col); f == nil || string(f.DataType) != typ {
				t.Errorf("%T.%s must be %s", model, col, typ)
			}
		}
	}
}

// LV-016: ledger/outbox/balances/requests are never soft-deleted; types and holidays are.
func TestLeaveModels_DeletionPolicy(t *testing.T) {
	for _, m := range []interface{}{&LedgerEntry{}, &OutboxEvent{}, &Balance{}, &Request{}} {
		if parse(t, m).LookUpField("deleted_at") != nil {
			t.Errorf("%T must not have deleted_at", m)
		}
	}
	for _, m := range []interface{}{&LeaveType{}, &Holiday{}} {
		if parse(t, m).LookUpField("deleted_at") == nil {
			t.Errorf("%T must support soft delete", m)
		}
	}
}

func TestLeaveModels_BeforeCreateGeneratesIDs(t *testing.T) {
	b, r, l, o, ty, h := &Balance{}, &Request{}, &LedgerEntry{}, &OutboxEvent{}, &LeaveType{}, &Holiday{}
	for _, err := range []error{b.BeforeCreate(nil), r.BeforeCreate(nil), l.BeforeCreate(nil), o.BeforeCreate(nil), ty.BeforeCreate(nil), h.BeforeCreate(nil)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{b.ID, r.ID, l.ID, o.ID, ty.ID, h.ID} {
		if id == uuid.Nil {
			t.Fatal("BeforeCreate must assign an id")
		}
	}
	keep := uuid.New()
	b2, r2, l2, o2 := &Balance{ID: keep}, &Request{ID: keep}, &LedgerEntry{ID: keep}, &OutboxEvent{ID: keep}
	_, _, _, _ = b2.BeforeCreate(nil), r2.BeforeCreate(nil), l2.BeforeCreate(nil), o2.BeforeCreate(nil)
	ty2, h2 := &LeaveType{}, &Holiday{}
	ty2.ID, h2.ID = keep, keep
	_, _ = ty2.BeforeCreate(nil), h2.BeforeCreate(nil)
	for _, id := range []uuid.UUID{b2.ID, r2.ID, l2.ID, o2.ID, ty2.ID, h2.ID} {
		if id != keep {
			t.Fatal("existing id must be kept")
		}
	}
}

// Available = opening + accrued + adjusted − used − reserved (architecture §4).
func TestBalance_Available(t *testing.T) {
	b := Balance{Opening: 500, Accrued: 1200, Adjusted: -100, Used: 300, Reserved: 250}
	if got := b.Available(); got != 1050 {
		t.Fatalf("available = %d, want 1050", got)
	}
}
