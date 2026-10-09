package recruitment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	orgDTO "github.com/jaas/jaas/internal/organization/dto"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

type orgStub struct{ err error }

func (s orgStub) GetDepartment(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*orgDTO.DepartmentResponse, error) {
	return &orgDTO.DepartmentResponse{}, s.err
}
func (s orgStub) GetDesignation(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*orgDTO.DesignationResponse, error) {
	return &orgDTO.DesignationResponse{}, s.err
}

// TestOrgDirectory — Module 1 not-found means "does not exist"; other failures propagate (D6-03).
func TestOrgDirectory(t *testing.T) {
	ctx, id := context.Background(), uuid.New()
	boom := errors.New("db down")
	for _, c := range []struct {
		err    error
		want   bool
		wantEr error
	}{{nil, true, nil}, {sharedErrors.ErrNotFound, false, nil}, {boom, false, boom}} {
		o := orgDirectory{depts: orgStub{c.err}, desigs: orgStub{c.err}}
		for _, f := range []func(context.Context, uuid.UUID, uuid.UUID) (bool, error){o.DepartmentExists, o.DesignationExists} {
			if got, err := f(ctx, id, id); got != c.want || !errors.Is(err, c.wantEr) {
				t.Fatalf("err %v: got %v %v", c.err, got, err)
			}
		}
	}
	m := NewModule(nil, Ports{})
	if len(m.RegisterModels()) != 6 || m.Relay() == nil {
		t.Fatal("module wiring")
	}
}
