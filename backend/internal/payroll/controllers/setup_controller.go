package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/services"
)

// SetupController serves salary structures and CTC assignments (P1–P9).
type SetupController struct {
	structures  services.StructureService
	assignments services.AssignmentService
}

// NewSetupController builds a SetupController.
func NewSetupController(s services.StructureService, a services.AssignmentService) *SetupController {
	return &SetupController{structures: s, assignments: a}
}

// CreateStructure handles POST /payroll/structures (P1).
func (ctl *SetupController) CreateStructure(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.CreateStructureRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.structures.Create(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListStructures handles GET /payroll/structures (P2).
func (ctl *SetupController) ListStructures(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.structures.List(c.Request.Context(), a.TenantID, c.Query("status"), page(c))
	respondList(c, res, meta, err)
}

// GetStructure handles GET /payroll/structures/:id (P3).
func (ctl *SetupController) GetStructure(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.structures.Get(c.Request.Context(), a.TenantID, id)
	respond(c, http.StatusOK, res, err)
}

// UpdateStructure handles PATCH /payroll/structures/:id (P4).
func (ctl *SetupController) UpdateStructure(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	var req dto.UpdateStructureRequest
	if !okID || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.structures.Update(c.Request.Context(), a, id, req)
	respond(c, http.StatusOK, res, err)
}

// DeactivateStructure handles POST /payroll/structures/:id/deactivate (P5).
func (ctl *SetupController) DeactivateStructure(c *gin.Context) {
	a, okA := actorFrom(c)
	id, okID := pathIDIf(c, okA)
	if !okID {
		return
	}
	res, err := ctl.structures.Deactivate(c.Request.Context(), a, id)
	respond(c, http.StatusOK, res, err)
}

// Assign handles POST /payroll/assignments (P6).
func (ctl *SetupController) Assign(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.AssignRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.assignments.Assign(c.Request.Context(), a, req)
	respond(c, http.StatusCreated, res, err)
}

// ListAssignments handles GET /payroll/assignments?employee_id= (P7).
func (ctl *SetupController) ListAssignments(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, meta, err := ctl.assignments.List(c.Request.Context(), a.TenantID, c.Query("employee_id"), page(c))
	respondList(c, res, meta, err)
}

// MyAssignment handles GET /payroll/assignments/me (P8); the employee comes from the JWT only.
func (ctl *SetupController) MyAssignment(c *gin.Context) {
	a, okA := actorFrom(c)
	if !okA {
		return
	}
	res, err := ctl.assignments.Mine(c.Request.Context(), a)
	respond(c, http.StatusOK, res, err)
}

// Preview handles POST /payroll/assignments/preview (P9).
func (ctl *SetupController) Preview(c *gin.Context) {
	a, okA := actorFrom(c)
	var req dto.PreviewRequest
	if !okA || !bindJSON(c, &req) {
		return
	}
	res, err := ctl.assignments.Preview(c.Request.Context(), a.TenantID, req)
	respond(c, http.StatusOK, res, err)
}
