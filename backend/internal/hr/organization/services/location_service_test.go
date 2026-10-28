package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/identityport"
	"github.com/jaas/jaas/internal/hr/organization/dto"
	"github.com/jaas/jaas/internal/hr/organization/models"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// stubLocationRepo is an in-memory LocationRepository; fail injects an error into every call.
type stubLocationRepo struct {
	byID map[uuid.UUID]*models.Location
	fail error
}

func newStubLocations() *stubLocationRepo {
	return &stubLocationRepo{byID: map[uuid.UUID]*models.Location{}}
}

func (s *stubLocationRepo) Create(_ context.Context, _ *gorm.DB, l *models.Location) error {
	if s.fail != nil {
		return s.fail
	}
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	cp := *l
	s.byID[l.ID] = &cp
	return nil
}
func (s *stubLocationRepo) find(tenantID uuid.UUID, match func(*models.Location) bool) (*models.Location, error) {
	if s.fail != nil {
		return nil, s.fail
	}
	for _, l := range s.byID {
		if l.TenantID == tenantID && match(l) {
			cp := *l
			return &cp, nil
		}
	}
	return nil, nil
}
func (s *stubLocationRepo) FindByID(_ context.Context, _ *gorm.DB, tenantID, id uuid.UUID) (*models.Location, error) {
	return s.find(tenantID, func(l *models.Location) bool { return l.ID == id })
}
func (s *stubLocationRepo) FindByCode(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, code string) (*models.Location, error) {
	return s.find(tenantID, func(l *models.Location) bool { return l.Code == code })
}
func (s *stubLocationRepo) FindByName(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, name string) (*models.Location, error) {
	return s.find(tenantID, func(l *models.Location) bool { return strings.EqualFold(l.Name, name) })
}
func (s *stubLocationRepo) FindAll(_ context.Context, _ *gorm.DB, tenantID uuid.UUID, f dto.LocationFilter, page, perPage int) ([]models.Location, int64, error) {
	if s.fail != nil {
		return nil, 0, s.fail
	}
	var out []models.Location
	for _, l := range s.byID {
		if l.TenantID == tenantID && (f.Status == "" || l.Status == f.Status) && (f.StateCode == "" || l.StateCode == f.StateCode) &&
			(f.Search == "" || strings.Contains(strings.ToLower(l.Name+l.Code+l.City), strings.ToLower(f.Search))) {
			out = append(out, *l)
		}
	}
	return out, int64(len(out)), nil
}
func (s *stubLocationRepo) Update(_ context.Context, _ *gorm.DB, l *models.Location) error {
	if s.fail != nil {
		return s.fail
	}
	cp := *l
	s.byID[l.ID] = &cp
	return nil
}

func appCode(err error) string {
	var app *sharedErrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	if err == nil {
		return ""
	}
	return "?"
}

// FR-L001..FR-L004: create, validation, uniqueness, immutable code, list filters, status rules.
func TestLocationService_Lifecycle(t *testing.T) {
	ctx, tenant := context.Background(), uuid.New()
	locs, maps := newStubLocations(), &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewLocationService(locs, maps, &stubPublisher{})
	blr, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: " blr-hq ", Name: " Bengaluru HQ ", City: "Bengaluru", StateCode: "ka",
		AddressLine1: sp("MG Road")}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if blr.Code != "BLR-HQ" || blr.Name != "Bengaluru HQ" || blr.StateCode != "KA" || blr.CountryCode != "IN" || blr.TimeZone != "Asia/Kolkata" || blr.Status != "active" {
		t.Fatalf("defaults and normalization: %+v", blr)
	}
	for _, bad := range []dto.CreateLocationRequest{
		{Code: "x", Name: "N", City: "C", StateCode: "KA"},
		{Code: "OK1", Name: " ", City: "C", StateCode: "KA"},
		{Code: "OK2", Name: "N2", City: " ", StateCode: "KA"},
		{Code: "OK3", Name: "N3", City: "C", StateCode: "K-A"},
		{Code: "OK4", Name: "N4", City: "C", StateCode: "KA", CountryCode: sp("I1")},
		{Code: "OK5", Name: "N5", City: "C", StateCode: "KA", TimeZone: sp("Mars/Olympus")},
		{Code: "OK6", Name: "N6", City: "C", StateCode: "KA", TimeZone: sp("Local")},
		{Code: "OK7", Name: "N7", City: "C", StateCode: "KA", TimeZone: sp("IST")},
	} {
		if _, err := svc.CreateLocation(ctx, nil, tenant, bad, uuid.New()); appCode(err) != "VALIDATION_ERROR" {
			t.Errorf("%+v → %v", bad, err)
		}
	}
	if _, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: "BLR-HQ", Name: "Other", City: "C", StateCode: "KA"}, uuid.New()); appCode(err) != "CONFLICT" {
		t.Fatalf("duplicate code: %v", err)
	}
	if _, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: "OTHER", Name: "bengaluru hq", City: "C", StateCode: "KA"}, uuid.New()); appCode(err) != "CONFLICT" {
		t.Fatalf("duplicate name (case-insensitive): %v", err)
	}
	other, err := svc.CreateLocation(ctx, nil, uuid.New(), dto.CreateLocationRequest{Code: "BLR-HQ", Name: "Bengaluru HQ", City: "Bengaluru", StateCode: "KA"}, uuid.New())
	if err != nil {
		t.Fatal("codes are per tenant:", err)
	}
	pune, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: "PNQ", Name: "Pune", City: "Pune", StateCode: "MH", CountryCode: sp("in"),
		TimeZone: sp("UTC")}, uuid.New())
	if err != nil || pune.CountryCode != "IN" || pune.TimeZone != "UTC" {
		t.Fatalf("explicit country/zone: %+v %v", pune, err)
	}

	id := uuid.MustParse(blr.ID)
	if _, err := svc.GetLocation(ctx, nil, tenant, uuid.MustParse(other.ID)); err != sharedErrors.ErrNotFound {
		t.Fatalf("cross-tenant read: %v", err)
	}
	upd, err := svc.UpdateLocation(ctx, nil, tenant, id, dto.UpdateLocationRequest{Code: sp("blr-hq"), Name: sp("Bengaluru Campus"), PostalCode: sp("560001"),
		AddressLine1: sp(" "), TimeZone: sp("Asia/Calcutta")}, uuid.New())
	if err != nil || upd.Name != "Bengaluru Campus" || *upd.PostalCode != "560001" || upd.AddressLine1 != nil || upd.TimeZone != "Asia/Calcutta" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if _, err := svc.UpdateLocation(ctx, nil, tenant, id, dto.UpdateLocationRequest{Code: sp("NEW")}, uuid.New()); appCode(err) != "VALIDATION_ERROR" {
		t.Fatalf("code immutable: %v", err)
	}
	if _, err := svc.UpdateLocation(ctx, nil, tenant, id, dto.UpdateLocationRequest{Name: sp("pune")}, uuid.New()); appCode(err) != "CONFLICT" {
		t.Fatalf("rename onto another name: %v", err)
	}
	if _, err := svc.UpdateLocation(ctx, nil, tenant, id, dto.UpdateLocationRequest{StateCode: sp("")}, uuid.New()); appCode(err) != "VALIDATION_ERROR" {
		t.Fatalf("invalid update: %v", err)
	}

	list, err := svc.ListLocations(ctx, nil, tenant, dto.LocationFilter{StateCode: "mh"}, 1, 20)
	if err != nil || len(list.Data) != 1 || list.Data[0].Code != "PNQ" {
		t.Fatalf("state filter: %+v %v", list, err)
	}
	if list, _ = svc.ListLocations(ctx, nil, tenant, dto.LocationFilter{Search: "campus"}, 1, 20); len(list.Data) != 1 {
		t.Fatalf("search: %+v", list.Data)
	}
	if _, err := svc.ListLocations(ctx, nil, tenant, dto.LocationFilter{Status: "closed"}, 1, 20); appCode(err) != "VALIDATION_ERROR" {
		t.Fatalf("status filter: %v", err)
	}

	loc := id
	maps.byID[uuid.New()] = &models.Mapping{LocationID: &loc, Status: "active"}
	if _, err := svc.SetLocationStatus(ctx, nil, tenant, id, "inactive", uuid.New()); appCode(err) != "CONFLICT" {
		t.Fatalf("FR-L004 blocked by active mappings: %v", err)
	}
	if _, err := svc.SetLocationStatus(ctx, nil, tenant, id, "active", uuid.New()); appCode(err) != "CONFLICT" {
		t.Fatalf("already active: %v", err)
	}
	pid := uuid.MustParse(pune.ID)
	if res, err := svc.SetLocationStatus(ctx, nil, tenant, pid, "inactive", uuid.New()); err != nil || res.Status != "inactive" {
		t.Fatalf("deactivate: %+v %v", res, err)
	}
	if res, err := svc.SetLocationStatus(ctx, nil, tenant, pid, "active", uuid.New()); err != nil || res.Status != "active" {
		t.Fatalf("activate: %+v %v", res, err)
	}
	if _, err := svc.SetLocationStatus(ctx, nil, tenant, uuid.New(), "inactive", uuid.New()); err != sharedErrors.ErrNotFound {
		t.Fatalf("unknown: %v", err)
	}
}

// FR-L005: a mapping's location must be an active location of the tenant.
func TestMappingService_Location(t *testing.T) {
	ctx, tenant := context.Background(), uuid.New()
	locs := newStubLocations()
	maps := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	depts := &stubDeptRepo{byID: map[uuid.UUID]*models.Department{}}
	users := &stubUserChecker{users: map[uuid.UUID]*identityport.User{}}
	svc := NewMappingService(maps, depts, &stubTeamRepo{byID: map[uuid.UUID]*models.Team{}},
		&stubDesigRepo{byID: map[uuid.UUID]*models.Designation{}, byTitle: map[string]*models.Designation{}}, locs, users, &stubPublisher{}, &stubAudit{})
	user := uuid.New()
	users.users[user] = &identityport.User{ID: user, TenantID: tenant, Status: "active"}
	dept := &models.Department{Name: "Eng", Status: "active"}
	dept.ID, dept.TenantID = uuid.New(), tenant
	depts.byID[dept.ID] = dept
	active := &models.Location{Code: "BLR", Name: "BLR", City: "B", StateCode: "KA", CountryCode: "IN", TimeZone: "Asia/Kolkata", Status: "active"}
	active.ID, active.TenantID = uuid.New(), tenant
	inactive := &models.Location{Code: "OLD", Name: "OLD", City: "B", StateCode: "KA", CountryCode: "IN", TimeZone: "Asia/Kolkata", Status: "inactive"}
	inactive.ID, inactive.TenantID = uuid.New(), tenant
	locs.byID[active.ID], locs.byID[inactive.ID] = active, inactive

	create := func(loc string) (*dto.MappingResponse, error) {
		return svc.CreateMapping(ctx, nil, tenant, dto.CreateMappingRequest{UserID: user.String(), DepartmentID: sp(dept.ID.String()), LocationID: &loc}, uuid.New())
	}
	if _, err := create("bad"); appCode(err) != "VALIDATION_ERROR" {
		t.Fatalf("malformed: %v", err)
	}
	if _, err := create(uuid.NewString()); err != sharedErrors.ErrNotFound {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := create(inactive.ID.String()); appCode(err) != "CONFLICT" {
		t.Fatalf("inactive: %v", err)
	}
	m, err := create(active.ID.String())
	if err != nil || m.LocationID == nil || *m.LocationID != active.ID.String() {
		t.Fatalf("mapped with location: %+v %v", m, err)
	}
	mid := uuid.MustParse(m.ID)
	if _, err := svc.UpdateMapping(ctx, nil, tenant, mid, dto.UpdateMappingRequest{LocationID: sp(inactive.ID.String())}); appCode(err) != "CONFLICT" {
		t.Fatalf("update to inactive: %v", err)
	}
	locs.fail = errBoom
	if _, err := svc.UpdateMapping(ctx, nil, tenant, mid, dto.UpdateMappingRequest{LocationID: sp(active.ID.String())}); !errors.Is(err, errBoom) {
		t.Fatalf("repo failure propagates: %v", err)
	}
}

// Repository failures propagate from every location operation.
func TestLocationService_Failures(t *testing.T) {
	ctx, tenant := context.Background(), uuid.New()
	locs := newStubLocations()
	maps := &stubMappingRepo{byID: map[uuid.UUID]*models.Mapping{}, primary: map[uuid.UUID]*models.Mapping{}}
	svc := NewLocationService(locs, maps, &stubPublisher{})
	l, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: "AAA", Name: "A", City: "C", StateCode: "KA"}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.MustParse(l.ID)
	locs.fail = errBoom
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := svc.CreateLocation(ctx, nil, tenant, dto.CreateLocationRequest{Code: "BBB", Name: "B", City: "C", StateCode: "KA"}, uuid.New())
			return err
		},
		"get":  func() error { _, err := svc.GetLocation(ctx, nil, tenant, id); return err },
		"list": func() error { _, err := svc.ListLocations(ctx, nil, tenant, dto.LocationFilter{}, 1, 20); return err },
		"update": func() error {
			_, err := svc.UpdateLocation(ctx, nil, tenant, id, dto.UpdateLocationRequest{}, uuid.New())
			return err
		},
		"status": func() error {
			_, err := svc.SetLocationStatus(ctx, nil, tenant, id, "inactive", uuid.New())
			return err
		},
	} {
		if err := call(); !errors.Is(err, errBoom) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
