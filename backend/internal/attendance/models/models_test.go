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

func TestAttendanceModels_TableNames(t *testing.T) {
	cases := map[string]interface{}{
		"shifts":                     &Shift{},
		"shift_assignments":          &ShiftAssignment{},
		"attendance_records":         &AttendanceRecord{},
		"attendance_punches":         &AttendancePunch{},
		"attendance_regularizations": &Regularization{},
		"attendance_events_outbox":   &OutboxEvent{},
	}
	for want, model := range cases {
		if got := parse(t, model).Table; got != want {
			t.Errorf("%T table = %q, want %q", model, got, want)
		}
	}
}

// TestAttendanceModels_Indexes guards the unique/lookup indexes the spec §8 relies on.
func TestAttendanceModels_Indexes(t *testing.T) {
	cases := []struct {
		model  interface{}
		index  string
		unique bool
		fields []string
	}{
		{&Shift{}, "uq_shifts_tenant_name", true, []string{"tenant_id", "name"}},
		{&Shift{}, "uq_shifts_tenant_code", true, []string{"tenant_id", "code"}},
		{&ShiftAssignment{}, "idx_shift_assignments_employee", false, []string{"tenant_id", "employee_profile_id", "effective_from"}},
		{&AttendanceRecord{}, "uq_attendance_records_employee_date", true, []string{"tenant_id", "employee_profile_id", "attendance_date"}},
		{&AttendanceRecord{}, "idx_attendance_records_tenant_date_status", false, []string{"tenant_id", "attendance_date", "status"}},
		{&AttendancePunch{}, "idx_attendance_punches_employee_time", false, []string{"tenant_id", "employee_profile_id", "punch_time"}},
		{&Regularization{}, "idx_attendance_regularizations_employee_date", false, []string{"tenant_id", "employee_profile_id", "attendance_date"}},
		{&OutboxEvent{}, "uq_attendance_outbox_event_id", true, []string{"event_id"}},
		{&Regularization{}, "uq_attendance_regularizations_pending", true, []string{"tenant_id", "employee_profile_id", "attendance_date"}},
	}
	for _, tc := range cases {
		s := parse(t, tc.model)
		idx := findIndex(s, tc.index)
		if idx == nil {
			t.Errorf("%T: missing index %s", tc.model, tc.index)
			continue
		}
		if (idx.Class == "UNIQUE") != tc.unique {
			t.Errorf("%s unique=%v, want %v", tc.index, idx.Class == "UNIQUE", tc.unique)
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
}

// AT-015 backstop must be partial on the dev (AutoMigrate) path too, matching migration 000029.
func TestAttendanceModels_PendingIndexIsPartial(t *testing.T) {
	idx := findIndex(parse(t, &Regularization{}), "uq_attendance_regularizations_pending")
	if idx == nil || idx.Where != "status = 'pending'" {
		t.Fatalf("pending index must be WHERE status = 'pending', got %+v", idx)
	}
}

// Partial/expression indexes must match migrations 000025 and 000030 on the AutoMigrate path (P7 parity).
func TestAttendanceModels_IndexParityWithSQL(t *testing.T) {
	name := findIndex(parse(t, &Shift{}), "uq_shifts_tenant_name")
	if name == nil || name.Where != "deleted_at IS NULL" || name.Fields[1].Expression != "lower(name)" {
		t.Errorf("uq_shifts_tenant_name must be (tenant_id, lower(name)) WHERE deleted_at IS NULL: %+v", name)
	}
	code := findIndex(parse(t, &Shift{}), "uq_shifts_tenant_code")
	if code == nil || code.Where != "deleted_at IS NULL AND code IS NOT NULL" {
		t.Errorf("uq_shifts_tenant_code partial filter missing: %+v", code)
	}
	pub := findIndex(parse(t, &OutboxEvent{}), "idx_attendance_outbox_published")
	if pub == nil || pub.Where != "published = false" {
		t.Errorf("idx_attendance_outbox_published must be partial: %+v", pub)
	}
}

// Minute counters are INTEGER in SQL migrations; AutoMigrate must agree (P7 parity check).
func TestAttendanceModels_IntegerColumns(t *testing.T) {
	cols := map[interface{}][]string{
		&Shift{}:            {"start_minute", "end_minute", "grace_period_mins", "break_duration_mins", "full_day_minutes", "half_day_minutes"},
		&AttendanceRecord{}: {"total_work_minutes", "total_break_minutes", "late_minutes", "overtime_minutes"},
		&OutboxEvent{}:      {"attempts"},
	}
	for model, names := range cols {
		s := parse(t, model)
		for _, n := range names {
			if f := s.LookUpField(n); f == nil || string(f.DataType) != "integer" {
				t.Errorf("%T.%s must be type:integer", model, n)
			}
		}
	}
}

func findIndex(s *schema.Schema, name string) *schema.Index {
	for _, idx := range s.ParseIndexes() {
		if idx.Name == name {
			return idx
		}
	}
	return nil
}

// TestAttendanceModels_AppendOnly guards AT-019: punches and outbox rows have no soft delete.
func TestAttendanceModels_AppendOnly(t *testing.T) {
	for _, model := range []interface{}{&AttendancePunch{}, &OutboxEvent{}, &AttendanceRecord{}} {
		if f := parse(t, model).LookUpField("deleted_at"); f != nil {
			t.Errorf("%T must not have deleted_at (append-only)", model)
		}
	}
	if parse(t, &Shift{}).LookUpField("deleted_at") == nil {
		t.Error("Shift must support soft delete")
	}
}

func TestAttendanceModels_BeforeCreateGeneratesIDs(t *testing.T) {
	a, r, p, g, o := &ShiftAssignment{}, &AttendanceRecord{}, &AttendancePunch{}, &Regularization{}, &OutboxEvent{}
	for _, err := range []error{a.BeforeCreate(nil), r.BeforeCreate(nil), p.BeforeCreate(nil), g.BeforeCreate(nil), o.BeforeCreate(nil)} {
		if err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
	}
	for name, id := range map[string]uuid.UUID{"assignment": a.ID, "record": r.ID, "punch": p.ID, "regularization": g.ID, "outbox": o.ID} {
		if id == uuid.Nil {
			t.Errorf("%s BeforeCreate did not set an ID", name)
		}
	}
	existing := uuid.New()
	keep := &AttendancePunch{ID: existing}
	_ = keep.BeforeCreate(nil)
	if keep.ID != existing {
		t.Error("BeforeCreate must not overwrite an existing ID")
	}
}
