package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/hr/employee/controllers"
	"github.com/jaas/jaas/internal/hr/identityport"
)

// RegisterRoutes wires employee endpoint groups with Module 0 middleware and RBAC.
// Controllers groups the employee HTTP handlers.
type Controllers struct {
	Employees *controllers.EmployeeController
	Statutory *controllers.StatutoryController
	Documents *controllers.DocumentController
	Timeline  *controllers.TimelineController
}

func RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP, c Controllers) {
	empCtrl, statCtrl, docCtrl, tlCtrl := c.Employees, c.Statutory, c.Documents, c.Timeline
	tenantResolver, authMiddleware := idp.TenantResolver(), idp.Authenticate()
	auditMW := idp.AuditTrail

	read := idp.Require("employee", "read")
	create := idp.Require("employee", "create")
	update := idp.Require("employee", "update")
	updateSensitive := idp.Require("employee", "update_sensitive")
	admin := idp.Require("employee", "admin")
	// Resolves employee:read_sensitive without blocking; the statutory handler only unmasks PII when it is true.
	sensitiveFlag := idp.Flag("employee", "read_sensitive", "has_sensitive_perm")
	// Unmasked reads of PII are security-relevant and always audited.
	auditUnmasked := func(c *gin.Context) {
		if c.Query("unmasked") == "true" {
			auditMW("employee.statutory_unmasked", "employee")(c)
			return
		}
		c.Next()
	}

	employees := router.Group("/employees")
	employees.Use(tenantResolver, authMiddleware)
	{
		// Self service
		employees.GET("/me", empCtrl.GetMe)
		employees.PATCH("/me", auditMW("employee.self_updated", "employee"), empCtrl.UpdateMe)

		// Directory and management
		employees.POST("", create, auditMW("employee.created", "employee"), empCtrl.Create)
		employees.GET("", read, empCtrl.List)
		employees.GET("/:id", read, empCtrl.GetByID)
		employees.PATCH("/:id", update, auditMW("employee.updated", "employee"), empCtrl.Update)
		employees.POST("/:id/status", update, auditMW("employee.status_changed", "employee"), empCtrl.TransitionStatus)
		employees.POST("/:id/deactivate", update, auditMW("employee.deactivated", "employee"), empCtrl.Deactivate)

		// Statutory & Bank details
		employees.GET("/:id/statutory", read, sensitiveFlag, auditUnmasked, statCtrl.Get)
		employees.PUT("/:id/statutory", updateSensitive, auditMW("employee.statutory_updated", "employee"), statCtrl.Upsert)

		// Documents
		employees.POST("/:id/documents", update, auditMW("employee.document_uploaded", "employee"), docCtrl.Upload)
		employees.GET("/:id/documents", read, docCtrl.List)
		employees.POST("/:id/documents/:doc_id/verify", admin, auditMW("employee.document_verified", "employee"), docCtrl.Verify)

		// Career / status timeline
		employees.GET("/:id/timeline", read, tlCtrl.List)
	}
}
