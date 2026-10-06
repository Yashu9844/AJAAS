package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	identityMiddleware "github.com/jaas/jaas/internal/identity/middleware"
	identityRepos "github.com/jaas/jaas/internal/identity/repositories"
	identityServices "github.com/jaas/jaas/internal/identity/services"
	"github.com/jaas/jaas/internal/organization/controllers"
	"github.com/jaas/jaas/internal/organization/services"
	"gorm.io/gorm"
)

// RegisterRoutes wires organization endpoint groups with Module 0 middleware
// (tenantResolver + authMiddleware built from the identity module; rate limiting
// reuses Module 0 middleware on the same Gin engine).
func RegisterRoutes(
	router *gin.RouterGroup,
	db *gorm.DB,
	userRoleRepo identityRepos.UserRoleRepository,
	rolePermRepo identityRepos.RolePermissionRepository,
	tenantResolver gin.HandlerFunc,
	authMiddleware gin.HandlerFunc,
	deptCtrl *controllers.DepartmentController,
	teamCtrl *controllers.TeamController,
	desigCtrl *controllers.DesignationController,
	mappingCtrl *controllers.MappingController,
	chartCtrl *controllers.OrgChartController,
	dbForAudit *gorm.DB,
	auditSvc identityServices.AuditService,
	chartInv services.ChartInvalidator,
) {
	invalidate := chartInvalidation(chartInv)
	auditMW := func(action, resource string) gin.HandlerFunc {
		return identityMiddleware.AuditLog(dbForAudit, auditSvc, action, resource)
	}
	read := identityMiddleware.RequirePermission(db, "organization", "read", userRoleRepo, rolePermRepo)
	write := identityMiddleware.RequirePermission(db, "organization", "update", userRoleRepo, rolePermRepo)

	depts := router.Group("/departments")
	depts.Use(tenantResolver, authMiddleware, invalidate)
	{
		depts.POST("", write, auditMW("department.created", "department"), deptCtrl.Create)
		depts.GET("", read, deptCtrl.List)
		depts.GET("/:id", read, deptCtrl.GetByID)
		depts.PATCH("/:id", write, auditMW("department.updated", "department"), deptCtrl.Update)
		depts.POST("/:id/deactivate", write, auditMW("department.deactivated", "department"), deptCtrl.Deactivate)
	}

	teams := router.Group("/teams")
	teams.Use(tenantResolver, authMiddleware, invalidate)
	{
		teams.POST("", write, auditMW("team.created", "team"), teamCtrl.Create)
		teams.GET("", read, teamCtrl.List)
		teams.GET("/:id", read, teamCtrl.GetByID)
		teams.PATCH("/:id", write, auditMW("team.updated", "team"), teamCtrl.Update)
		teams.POST("/:id/deactivate", write, auditMW("team.deactivated", "team"), teamCtrl.Deactivate)
	}

	desigs := router.Group("/designations")
	desigs.Use(tenantResolver, authMiddleware)
	{
		desigs.POST("", write, auditMW("designation.created", "designation"), desigCtrl.Create)
		desigs.GET("", read, desigCtrl.List)
		desigs.GET("/:id", read, desigCtrl.GetByID)
		desigs.PATCH("/:id", write, auditMW("designation.updated", "designation"), desigCtrl.Update)
		desigs.POST("/:id/deactivate", write, auditMW("designation.deactivated", "designation"), desigCtrl.Deactivate)
	}

	mappings := router.Group("/mappings")
	mappings.Use(tenantResolver, authMiddleware, invalidate)
	{
		mappings.POST("", write, auditMW("mapping.created", "mapping"), mappingCtrl.Create)
		mappings.GET("", read, mappingCtrl.ListByUser)
		mappings.GET("/:id", read, mappingCtrl.GetByID)
		mappings.PATCH("/:id", write, auditMW("mapping.updated", "mapping"), mappingCtrl.Update)
		mappings.POST("/:id/deactivate", write, auditMW("mapping.deactivated", "mapping"), mappingCtrl.Deactivate)
	}

	chart := router.Group("/org-chart")
	chart.Use(tenantResolver, authMiddleware, read)
	{
		chart.GET("", chartCtrl.Chart)
		chart.GET("/chain", chartCtrl.UserChain)
	}
}

// chartInvalidation drops the tenant's cached org chart after every successful org write
// (departments, teams, mappings), so GET /org-chart is read-your-writes consistent.
func chartInvalidation(inv services.ChartInvalidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if inv == nil || c.Request.Method == "GET" || c.Writer.Status() >= 300 {
			return
		}
		if tid, ok := c.Get("tenant_id"); ok {
			if id, ok := tid.(uuid.UUID); ok {
				inv.InvalidateChart(c.Request.Context(), id)
			}
		}
	}
}
