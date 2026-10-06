package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
	"github.com/jaas/jaas/internal/employee/models"
	"github.com/jaas/jaas/internal/employee/services"
	identityDto "github.com/jaas/jaas/internal/identity/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// Same technique as identity/services: run an operation fault-free, count dependency calls, then fail each in turn.

var errBoom = errors.New("boom")

type faults struct{ calls, failAt int }

func (f *faults) hit() error {
	f.calls++
	if f.failAt > 0 && f.calls == f.failAt {
		return errBoom
	}
	return nil
}

func sweep(t *testing.T, name string, op func(f *faults) error) {
	t.Helper()
	happy := &faults{}
	if err := op(happy); err != nil {
		t.Fatalf("%s: happy path failed: %v", name, err)
	}
	surfaced := 0
	for i := 1; i <= happy.calls; i++ {
		f := &faults{failAt: i}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic when dependency call #%d fails: %v", name, i, r)
				}
			}()
			if err := op(f); errors.Is(err, errBoom) {
				surfaced++
			}
		}()
	}
	if happy.calls > 0 && surfaced == 0 {
		t.Errorf("%s: no injected failure surfaced as an error", name)
	}
}

type fProfiles struct {
	*mockProfileRepo
	f *faults
}

func (r fProfiles) GetByID(ctx context.Context, t, id uuid.UUID) (*models.EmployeeProfile, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockProfileRepo.GetByID(ctx, t, id)
}
func (r fProfiles) GetByUserID(ctx context.Context, t, id uuid.UUID) (*models.EmployeeProfile, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockProfileRepo.GetByUserID(ctx, t, id)
}
func (r fProfiles) GetByCode(ctx context.Context, t uuid.UUID, c string) (*models.EmployeeProfile, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockProfileRepo.GetByCode(ctx, t, c)
}
func (r fProfiles) List(ctx context.Context, t uuid.UUID, fl dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error) {
	if err := r.f.hit(); err != nil {
		return nil, 0, err
	}
	return r.mockProfileRepo.List(ctx, t, fl)
}
func (r fProfiles) Update(ctx context.Context, tx *gorm.DB, p *models.EmployeeProfile) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.mockProfileRepo.Update(ctx, tx, p)
}
func (r fProfiles) UpdateContact(ctx context.Context, tx *gorm.DB, c *models.EmployeeContact) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.mockProfileRepo.UpdateContact(ctx, tx, c)
}

type fStat struct {
	*mockStatutoryRepo
	f *faults
}

func (r fStat) GetByProfileID(ctx context.Context, t, id uuid.UUID) (*models.EmployeeStatutory, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockStatutoryRepo.GetByProfileID(ctx, t, id)
}
func (r fStat) Upsert(ctx context.Context, tx *gorm.DB, s *models.EmployeeStatutory) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.mockStatutoryRepo.Upsert(ctx, tx, s)
}

type fDocs struct {
	*mockDocumentRepo
	f *faults
}

func (r fDocs) Create(ctx context.Context, tx *gorm.DB, d *models.EmployeeDocument) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.mockDocumentRepo.Create(ctx, tx, d)
}
func (r fDocs) GetByID(ctx context.Context, t, id uuid.UUID) (*models.EmployeeDocument, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockDocumentRepo.GetByID(ctx, t, id)
}
func (r fDocs) ListByProfileID(ctx context.Context, t, id uuid.UUID) ([]models.EmployeeDocument, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockDocumentRepo.ListByProfileID(ctx, t, id)
}
func (r fDocs) Update(ctx context.Context, tx *gorm.DB, d *models.EmployeeDocument) error {
	if err := r.f.hit(); err != nil {
		return err
	}
	return r.mockDocumentRepo.Update(ctx, tx, d)
}

type fTimeline struct {
	*mockTimelineRepo
	f *faults
}

func (r fTimeline) ListByProfileID(ctx context.Context, t, id uuid.UUID) ([]models.EmployeeTimeline, error) {
	if err := r.f.hit(); err != nil {
		return nil, err
	}
	return r.mockTimelineRepo.ListByProfileID(ctx, t, id)
}

type fUsers struct {
	*mockUserService
	f *faults
}

func (u fUsers) GetByID(ctx context.Context, db *gorm.DB, t, id uuid.UUID) (*identityDto.UserResponse, error) {
	if err := u.f.hit(); err != nil {
		return nil, err
	}
	return u.mockUserService.GetByID(ctx, db, t, id)
}

type seeded struct {
	tenant, user, profile uuid.UUID
	pr                    *mockProfileRepo
	f                     *faults
}

func seed(f *faults, status string) *seeded {
	s := &seeded{tenant: uuid.New(), user: uuid.New(), profile: uuid.New(), pr: newMockProfileRepo(), f: f}
	p := &models.EmployeeProfile{UserID: s.user, EmployeeCode: "EMP-1", FirstName: "A", LastName: "B", Status: status}
	p.ID, p.TenantID = s.profile, s.tenant
	p.EmploymentDetail = &models.EmploymentDetail{EmployeeProfileID: s.profile, EmploymentType: "full_time", NoticePeriodDays: 30, JoiningDate: time.Now()}
	p.Contact = &models.EmployeeContact{EmployeeProfileID: s.profile, EmergencyContacts: "[]"}
	_ = s.pr.Create(context.Background(), nil, p)
	return s
}

func (s *seeded) employees() services.EmployeeService {
	users := newMockUserService()
	users.users[s.user] = &identityDto.UserResponse{ID: s.user.String()}
	return services.NewEmployeeService(nil, fProfiles{s.pr, s.f}, newMockTimelineRepo(), fUsers{users, s.f}, nil, &mockPublisher{})
}

func TestFaults_EmployeeService(t *testing.T) {
	ctx := context.Background()
	sweep(t, "CreateEmployee", func(f *faults) error {
		s := seed(f, "active")
		newUser := uuid.New()
		users := newMockUserService()
		users.users[newUser] = &identityDto.UserResponse{ID: newUser.String()}
		svc := services.NewEmployeeService(nil, fProfiles{s.pr, f}, newMockTimelineRepo(), fUsers{users, f}, nil, &mockPublisher{})
		pe := time.Now().Add(time.Hour)
		_, err := svc.CreateEmployee(ctx, s.tenant, dto.CreateEmployeeRequest{UserID: newUser, EmployeeCode: "NEW-1", FirstName: "N", LastName: "E", EmploymentType: "full_time", JoiningDate: time.Now(), ProbationEndDate: &pe})
		return err
	})
	sweep(t, "GetByID", func(f *faults) error {
		s := seed(f, "active")
		_, err := s.employees().GetByID(ctx, s.tenant, s.profile)
		return err
	})
	sweep(t, "GetByUserID", func(f *faults) error {
		s := seed(f, "active")
		_, err := s.employees().GetByUserID(ctx, s.tenant, s.user)
		return err
	})
	sweep(t, "List", func(f *faults) error {
		s := seed(f, "active")
		_, _, err := s.employees().List(ctx, s.tenant, dto.EmployeeFilter{})
		return err
	})
	sweep(t, "Update", func(f *faults) error {
		s := seed(f, "active")
		str := "x"
		now := time.Now()
		_, err := s.employees().Update(ctx, s.tenant, s.profile, dto.UpdateEmployeeRequest{FirstName: &str, LastName: &str, DisplayName: &str, Gender: &str, DateOfBirth: &now, MaritalStatus: &str, BloodGroup: &str, AvatarURL: &str})
		return err
	})
	sweep(t, "UpdateSelfContact", func(f *faults) error {
		s := seed(f, "active")
		str := "x"
		ec := `[{"name":"M","phone":"1"}]`
		_, err := s.employees().UpdateSelfContact(ctx, s.tenant, s.user, dto.UpdateSelfContactRequest{PersonalPhone: &str, CurrentAddress: &str, PermanentAddress: &str, EmergencyContacts: &ec})
		return err
	})
	sweep(t, "TransitionStatus", func(f *faults) error {
		s := seed(f, "active")
		_, err := s.employees().TransitionStatus(ctx, s.tenant, s.profile, dto.TransitionStatusRequest{Status: "notice"})
		return err
	})
	sweep(t, "Deactivate", func(f *faults) error {
		s := seed(f, "active")
		return s.employees().Deactivate(ctx, s.tenant, s.profile)
	})
}

func TestEmployeeService_BusinessRules(t *testing.T) {
	ctx := context.Background()
	s := seed(&faults{}, "active")
	svc := s.employees()

	is := func(err error, status int) bool {
		var ae *sharedErrors.AppError
		return errors.As(err, &ae) && ae.StatusCode == status
	}
	if _, err := svc.GetByID(ctx, s.tenant, uuid.New()); !is(err, 404) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := svc.GetByUserID(ctx, s.tenant, uuid.New()); !is(err, 404) {
		t.Fatalf("unknown user: %v", err)
	}
	if _, err := svc.GetByID(ctx, uuid.New(), s.profile); !is(err, 404) {
		t.Fatalf("other tenant: %v", err)
	}
	// state machine
	if _, err := svc.TransitionStatus(ctx, s.tenant, s.profile, dto.TransitionStatusRequest{Status: "resigned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TransitionStatus(ctx, s.tenant, s.profile, dto.TransitionStatusRequest{Status: "active"}); !is(err, 409) {
		t.Fatalf("resigned is terminal: %v", err)
	}
	if _, err := svc.TransitionStatus(ctx, s.tenant, s.profile, dto.TransitionStatusRequest{Status: "resigned"}); err != nil {
		t.Fatalf("same status is a no-op: %v", err)
	}
	if _, err := svc.TransitionStatus(ctx, s.tenant, s.profile, dto.TransitionStatusRequest{Status: "nope"}); !is(err, 400) {
		t.Fatalf("invalid status: %v", err)
	}
	if _, err := svc.TransitionStatus(ctx, s.tenant, uuid.New(), dto.TransitionStatusRequest{Status: "notice"}); !is(err, 404) {
		t.Fatalf("unknown employee: %v", err)
	}
	// emergency contacts validation
	bad := "free text"
	if _, err := seed(&faults{}, "active").employees().UpdateSelfContact(ctx, s.tenant, s.user, dto.UpdateSelfContactRequest{EmergencyContacts: &bad}); err == nil {
		t.Fatal("free-text emergency contacts must be rejected")
	}
	empty := "  "
	s2 := seed(&faults{}, "active")
	res, err := s2.employees().UpdateSelfContact(ctx, s2.tenant, s2.user, dto.UpdateSelfContactRequest{EmergencyContacts: &empty})
	if err != nil || res.Contact.EmergencyContacts != "[]" {
		t.Fatalf("blank contacts stored as []: %+v %v", res, err)
	}
	if _, err := s2.employees().UpdateSelfContact(ctx, s2.tenant, uuid.New(), dto.UpdateSelfContactRequest{}); !is(err, 404) {
		t.Fatalf("self contact for user without profile: %v", err)
	}
}

func TestFaults_StatutoryDocumentsTimeline(t *testing.T) {
	ctx := context.Background()
	sweep(t, "Statutory.Get", func(f *faults) error {
		s := seed(f, "active")
		sr := newMockStatutoryRepo()
		st := &models.EmployeeStatutory{EmployeeProfileID: s.profile, TaxID: "ABCDE1234F", BankAccountNumber: "123456789012"}
		st.TenantID = s.tenant
		sr.byProfileID[s.profile] = st
		svc := services.NewEmployeeStatutoryService(fStat{sr, f}, fProfiles{s.pr, f})
		_, err := svc.GetByProfileID(ctx, s.tenant, s.profile, true)
		return err
	})
	sweep(t, "Statutory.Upsert", func(f *faults) error {
		s := seed(f, "active")
		svc := services.NewEmployeeStatutoryService(fStat{newMockStatutoryRepo(), f}, fProfiles{s.pr, f})
		_, err := svc.Upsert(ctx, s.tenant, s.profile, dto.UpdateStatutoryRequest{TaxID: "x"})
		return err
	})
	sweep(t, "Documents.Upload+List+Verify", func(f *faults) error {
		s := seed(f, "active")
		svc := services.NewEmployeeDocumentService(fDocs{newMockDocumentRepo(), f}, fProfiles{s.pr, f}, &mockPublisher{})
		d, err := svc.Upload(ctx, s.tenant, s.profile, dto.UploadDocumentRequest{DocumentType: "id", FileName: "a", FileURL: "https://e.com/a", FileSize: 1, MimeType: "a/b"})
		if err != nil {
			return err
		}
		if _, err := svc.List(ctx, s.tenant, s.profile); err != nil {
			return err
		}
		_, err = svc.Verify(ctx, s.tenant, s.profile, d.ID, uuid.New())
		return err
	})
	sweep(t, "Timeline.List", func(f *faults) error {
		s := seed(f, "active")
		tr := newMockTimelineRepo()
		tl := models.EmployeeTimeline{EmployeeProfileID: s.profile, EventType: "hired", EffectiveDate: time.Now()}
		tr.timelines[s.profile] = []models.EmployeeTimeline{tl}
		_, err := services.NewEmployeeTimelineService(fTimeline{tr, f}, fProfiles{s.pr, f}).List(ctx, s.tenant, s.profile)
		return err
	})
}

func TestEventConsumer_ConvergenceRules(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{"resigned", "terminated", "inactive"} {
		s := seed(&faults{}, status)
		c := services.NewEventConsumer(s.employees(), nil)
		if err := c.HandleUserDeactivated(ctx, s.tenant, s.user); err != nil {
			t.Fatalf("%s profile must be left alone (idempotent), got %v", status, err)
		}
		got, _ := s.employees().GetByID(ctx, s.tenant, s.profile)
		if got.Status != status {
			t.Fatalf("%s profile was changed to %s", status, got.Status)
		}
	}
	s := seed(&faults{}, "active")
	c := services.NewEventConsumer(s.employees(), nil)
	if err := c.HandleUserDeactivated(ctx, s.tenant, s.user); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.employees().GetByID(ctx, s.tenant, s.profile); got.Status != "inactive" {
		t.Fatalf("active profile must become inactive, got %s", got.Status)
	}
	// unknown user: nothing to do
	if err := c.HandleUserDeactivated(ctx, s.tenant, uuid.New()); err != nil {
		t.Fatal(err)
	}
}
