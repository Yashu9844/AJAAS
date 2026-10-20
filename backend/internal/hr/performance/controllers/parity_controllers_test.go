package controllers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/performance/dto"
	"github.com/jaas/jaas/internal/hr/performance/services"
)

type calStub struct{ stub }

func (s *calStub) Calibrate(context.Context, services.Actor, uuid.UUID, dto.CalibrateRequest) (*dto.CalibrationResponse, error) {
	return &dto.CalibrationResponse{}, s.err
}
func (s *calStub) List(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.Page) ([]dto.CalibrationResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}

type fbStub struct{ stub }

func (s *fbStub) Give(context.Context, services.Actor, dto.GiveFeedbackRequest) (*dto.FeedbackResponse, error) {
	return &dto.FeedbackResponse{}, s.err
}
func (s *fbStub) ListReceived(context.Context, services.Actor, dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) ListGiven(context.Context, services.Actor, dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) ListFor(context.Context, services.Actor, uuid.UUID, dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) ListPublic(context.Context, uuid.UUID, dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) Request(context.Context, services.Actor, dto.RequestFeedbackRequest) ([]dto.FeedbackRequestResponse, error) {
	return nil, s.err
}
func (s *fbStub) ListAssigned(context.Context, services.Actor, string, dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) ListMine(context.Context, services.Actor, string, dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error) {
	return nil, dto.PageMeta{}, s.err
}
func (s *fbStub) Decline(context.Context, services.Actor, uuid.UUID) (*dto.FeedbackRequestResponse, error) {
	return &dto.FeedbackRequestResponse{}, s.err
}

func init() {
	ops = append(ops, []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/hr/performance/cycles/" + id + "/calibrations", `{"employee_id":"` + id + `","overall_score":4.25,"reason":"calibration session"}`, 201},
		{"GET", "/hr/performance/cycles/" + id + "/calibrations?employee_id=" + id, "", 200},
		{"POST", "/hr/performance/feedback", `{"to_employee_id":"` + id + `","kind":"praise","visibility":"public","message":"great"}`, 201},
		{"GET", "/hr/performance/feedback?employee_id=" + id, "", 200},
		{"GET", "/hr/performance/feedback/received", "", 200},
		{"GET", "/hr/performance/feedback/given", "", 200},
		{"GET", "/hr/performance/feedback/public", "", 200},
		{"POST", "/hr/performance/feedback/requests", `{"responder_employee_ids":["` + id + `"],"question":"how?"}`, 201},
		{"GET", "/hr/performance/feedback/requests/assigned?status=pending", "", 200},
		{"GET", "/hr/performance/feedback/requests/mine", "", 200},
		{"POST", "/hr/performance/feedback/requests/" + id + "/decline", "", 200},
	}...)
}

func TestControllers_ParityInput(t *testing.T) {
	e := router(&stub{}, &goalStub{}, &reviewStub{}, false)
	for _, q := range []string{"/hr/performance/feedback", "/hr/performance/feedback?employee_id=x", "/hr/performance/cycles/" + id + "/calibrations?employee_id=x"} {
		if w := call(e, "GET", q, ""); w.Code != 400 {
			t.Errorf("%s → %d", q, w.Code)
		}
	}
	for _, b := range []string{`{"to_employee_id":"x","kind":"meh","visibility":"private","message":"m"}`, `{"to_employee_id":"x","kind":"praise","visibility":"team","message":"m"}`} {
		if w := call(e, "POST", "/hr/performance/feedback", b); w.Code != 400 {
			t.Errorf("feedback enum %s → %d", b, w.Code)
		}
	}
}
