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
		t.Fatal(err)
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

func TestPerformanceModels_TablesAndIndexes(t *testing.T) {
	for want, m := range map[string]interface{}{"performance_cycles": &Cycle{}, "performance_goals": &Goal{},
		"performance_key_results": &KeyResult{}, "performance_checkins": &CheckIn{}, "performance_reviews": &Review{},
		"performance_events_outbox": &OutboxEvent{}, "performance_calibrations": &Calibration{}, "performance_feedback": &Feedback{},
		"performance_feedback_requests": &FeedbackRequest{}} {
		if got := parse(t, m).Table; got != want {
			t.Errorf("%T table %q", m, got)
		}
	}
	cases := []struct {
		model  interface{}
		name   string
		unique bool
		where  string
		fields []string
	}{
		{&Cycle{}, "idx_performance_cycles_tenant_status", false, "", []string{"tenant_id", "status"}},
		{&Goal{}, "idx_performance_goals_cycle_employee", false, "", []string{"tenant_id", "cycle_id", "employee_id"}},
		{&Goal{}, "idx_performance_goals_parent", false, "", []string{"parent_goal_id"}},
		{&KeyResult{}, "idx_performance_key_results_goal", false, "", []string{"goal_id"}},
		{&CheckIn{}, "idx_performance_checkins_goal", false, "", []string{"goal_id"}},
		{&Review{}, "uq_performance_reviews_assignment", true, "", []string{"cycle_id", "employee_id", "reviewer_employee_id", "type"}},
		{&Review{}, "idx_performance_reviews_reviewer", false, "", []string{"tenant_id", "reviewer_employee_id"}},
		{&OutboxEvent{}, "uq_performance_outbox_event_id", true, "", []string{"event_id"}},
		{&OutboxEvent{}, "idx_performance_outbox_published", false, "published = false", []string{"published"}},
		{&Calibration{}, "idx_performance_calibrations_cycle_employee", false, "", []string{"tenant_id", "cycle_id", "employee_id", "created_at"}},
		{&Feedback{}, "idx_performance_feedback_to", false, "", []string{"tenant_id", "to_employee_id", "created_at"}},
		{&Feedback{}, "idx_performance_feedback_from", false, "", []string{"tenant_id", "from_employee_id", "created_at"}},
		{&FeedbackRequest{}, "idx_performance_feedback_requests_responder", false, "", []string{"tenant_id", "responder_employee_id", "status"}},
	}
	for _, c := range cases {
		idx := findIndex(parse(t, c.model), c.name)
		if idx == nil || (idx.Class == "UNIQUE") != c.unique || idx.Where != c.where || len(idx.Fields) != len(c.fields) {
			t.Errorf("%s: %+v", c.name, idx)
			continue
		}
		for i, f := range idx.Fields {
			if f.DBName != c.fields[i] {
				t.Errorf("%s field %d = %s", c.name, i, f.DBName)
			}
		}
	}
}

func TestPerformanceModels_ColumnTypes(t *testing.T) {
	cols := []struct {
		model interface{}
		want  map[string]string
	}{
		{&Cycle{}, map[string]string{"start_date": "date", "end_date": "date", "criteria": "jsonb"}},
		{&Goal{}, map[string]string{"weight": "numeric(5,2)", "progress": "numeric(5,2)", "description": "text"}},
		{&KeyResult{}, map[string]string{"start_value": "numeric(14,2)", "target_value": "numeric(14,2)", "current_value": "numeric(14,2)"}},
		{&CheckIn{}, map[string]string{"from_progress": "numeric(5,2)", "to_progress": "numeric(5,2)", "value": "numeric(14,2)"}},
		{&Review{}, map[string]string{"ratings": "jsonb", "overall_score": "numeric(3,2)", "potential": "varchar(10)"}},
		{&OutboxEvent{}, map[string]string{"payload": "jsonb", "attempts": "integer"}},
		{&Calibration{}, map[string]string{"before_score": "numeric(3,2)", "after_score": "numeric(3,2)", "reason": "varchar(1000)"}},
		{&Feedback{}, map[string]string{"message": "varchar(2000)", "kind": "varchar(20)", "visibility": "varchar(10)"}},
		{&FeedbackRequest{}, map[string]string{"question": "varchar(500)", "status": "varchar(20)"}},
	}
	for _, c := range cols {
		s := parse(t, c.model)
		for col, typ := range c.want {
			if f := s.LookUpField(col); f == nil || string(f.DataType) != typ {
				t.Errorf("%T.%s must be %s", c.model, col, typ)
			}
		}
	}
}

func TestPerformanceModels_JSONColumnsAndIDs(t *testing.T) {
	v, err := Criteria{"Quality", "Delivery"}.Value()
	if err != nil || v != `["Quality","Delivery"]` {
		t.Fatalf("Criteria.Value %v %v", v, err)
	}
	if v, _ := Criteria(nil).Value(); v != "[]" {
		t.Fatalf("nil criteria must store [] (NOT NULL column), got %v", v)
	}
	var c Criteria
	if err := c.Scan([]byte(`["A","B"]`)); err != nil || len(c) != 2 || c[1] != "B" {
		t.Fatal("Criteria.Scan bytes")
	}
	if err := c.Scan(`["X"]`); err != nil || c[0] != "X" {
		t.Fatal("Criteria.Scan string")
	}
	if c.Scan(42) == nil || c.Scan("not json") == nil {
		t.Fatal("Criteria.Scan errors")
	}
	if err := c.Scan(nil); err != nil {
		t.Fatal("Criteria.Scan nil")
	}
	r := Ratings{{Criterion: "Quality", Score: 4}}
	rv, _ := r.Value()
	var back Ratings
	if err := back.Scan(rv); err != nil || back[0] != r[0] {
		t.Fatalf("Ratings round trip %v %v", back, err)
	}
	if _, err := marshalJSON(func() {}); err == nil {
		t.Fatal("unmarshalable value must fail")
	}

	keep := uuid.New()
	cy, g, k, ci, rv2, ob := &Cycle{}, &Goal{}, &KeyResult{}, &CheckIn{}, &Review{}, &OutboxEvent{}
	ca, fb, fr := &Calibration{}, &Feedback{}, &FeedbackRequest{}
	for _, err := range []error{cy.BeforeCreate(nil), g.BeforeCreate(nil), k.BeforeCreate(nil), ci.BeforeCreate(nil), rv2.BeforeCreate(nil), ob.BeforeCreate(nil),
		ca.BeforeCreate(nil), fb.BeforeCreate(nil), fr.BeforeCreate(nil)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{cy.ID, g.ID, k.ID, ci.ID, rv2.ID, ob.ID, ca.ID, fb.ID, fr.ID} {
		if id == uuid.Nil {
			t.Fatal("id not assigned")
		}
	}
	g2 := &Goal{ID: keep}
	_ = g2.BeforeCreate(nil)
	if g2.ID != keep {
		t.Fatal("existing id overwritten")
	}
}
