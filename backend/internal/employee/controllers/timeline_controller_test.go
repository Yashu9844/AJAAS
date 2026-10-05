package controllers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/employee/dto"
)

func TestTimelineController_List(t *testing.T) {
	empID := uuid.New()
	tlRes := &dto.TimelineResponse{
		ID:                uuid.New(),
		EmployeeProfileID: empID,
		EventType:         "hired",
		EffectiveDate:     time.Now().UTC(),
	}
	stubSvc := &stubTlSvc{list: []dto.TimelineResponse{*tlRes}}
	ctrl := NewTimelineController(stubSvc)

	c, w := newTestContext(t, http.MethodGet, "/employees/"+empID.String()+"/timeline", nil)
	c.Params = gin.Params{{Key: "id", Value: empID.String()}}
	ctrl.List(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
