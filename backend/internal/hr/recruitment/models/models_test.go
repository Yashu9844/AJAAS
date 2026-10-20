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

func TestRecruitmentModels_TablesAndIndexes(t *testing.T) {
	for want, m := range map[string]interface{}{"recruitment_jobs": &Job{}, "recruitment_candidates": &Candidate{},
		"recruitment_stage_events": &StageEvent{}, "recruitment_interviews": &Interview{}, "recruitment_offers": &Offer{},
		"recruitment_events_outbox": &OutboxEvent{}} {
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
		{&Job{}, "idx_recruitment_jobs_tenant_status", false, "", []string{"tenant_id", "status"}},
		{&Candidate{}, "uq_recruitment_candidates_job_email", true, "", []string{"job_id", "email"}},
		{&Candidate{}, "idx_recruitment_candidates_job_stage", false, "", []string{"tenant_id", "job_id", "stage"}},
		{&StageEvent{}, "idx_recruitment_stage_events_candidate", false, "", []string{"candidate_id"}},
		{&Interview{}, "idx_recruitment_interviews_interviewer", false, "", []string{"tenant_id", "interviewer_employee_id"}},
		{&Interview{}, "idx_recruitment_interviews_candidate", false, "", []string{"candidate_id"}},
		{&Offer{}, "uq_recruitment_offers_open", true, "status = 'offered'", []string{"candidate_id"}},
		{&Offer{}, "idx_recruitment_offers_candidate", false, "", []string{"candidate_id"}},
		{&OutboxEvent{}, "uq_recruitment_outbox_event_id", true, "", []string{"event_id"}},
		{&OutboxEvent{}, "idx_recruitment_outbox_published", false, "published = false", []string{"published"}},
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
	if e := findIndex(parse(t, &Candidate{}), "uq_recruitment_candidates_job_email"); e.Fields[1].Expression != "lower(email)" {
		t.Error("candidate email uniqueness must be case-insensitive")
	}
}

func TestRecruitmentModels_ColumnTypesAndIDs(t *testing.T) {
	cols := map[interface{}]map[string]string{
		&Job{}:         {"headcount": "integer", "hired_count": "integer", "min_experience_years": "integer"},
		&Candidate{}:   {"expected_ctc": "numeric(14,2)", "notice_period_days": "integer"},
		&Interview{}:   {"duration_mins": "integer", "rating": "integer"},
		&Offer{}:       {"offered_ctc": "numeric(14,2)", "joining_date": "date", "expires_on": "date"},
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
	keep := uuid.New()
	j, c, e, i, o, ob := &Job{}, &Candidate{}, &StageEvent{}, &Interview{}, &Offer{}, &OutboxEvent{}
	for _, err := range []error{j.BeforeCreate(nil), c.BeforeCreate(nil), e.BeforeCreate(nil), i.BeforeCreate(nil), o.BeforeCreate(nil), ob.BeforeCreate(nil)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{j.ID, c.ID, e.ID, i.ID, o.ID, ob.ID} {
		if id == uuid.Nil {
			t.Fatal("id not assigned")
		}
	}
	j2 := &Job{ID: keep}
	_ = j2.BeforeCreate(nil)
	if j2.ID != keep {
		t.Fatal("existing id overwritten")
	}
}
