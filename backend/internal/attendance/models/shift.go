package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/database"
	"gorm.io/gorm"
)

// Shift is a tenant work-time template; times are minutes from midnight in Timezone (D3-03). FR-SH001.
// Integer fields carry no GORM default so an explicit 0 (e.g. zero grace) is stored as 0.
type Shift struct {
	database.BaseModel
	TenantID          uuid.UUID `gorm:"type:uuid;not null;index:idx_shifts_tenant_id;uniqueIndex:uq_shifts_tenant_name,priority:1,where:deleted_at IS NULL;uniqueIndex:uq_shifts_tenant_code,priority:1,where:deleted_at IS NULL AND code IS NOT NULL"`
	Name              string    `gorm:"type:varchar(100);not null;uniqueIndex:uq_shifts_tenant_name,priority:2,expression:lower(name)"`
	Code              *string   `gorm:"type:varchar(32);uniqueIndex:uq_shifts_tenant_code,priority:2"`
	StartMinute       int       `gorm:"type:integer;not null"`
	EndMinute         int       `gorm:"type:integer;not null"`
	GracePeriodMins   int       `gorm:"type:integer;not null"`
	BreakDurationMins int       `gorm:"type:integer;not null"`
	FullDayMinutes    *int      `gorm:"type:integer"`
	HalfDayMinutes    *int      `gorm:"type:integer"`
	Timezone          string    `gorm:"type:varchar(64);not null"`
	IsNightShift      bool      `gorm:"not null"`
	Status            string    `gorm:"type:varchar(20);not null;index:idx_shifts_status"`
}

// ShiftAssignment binds an employee to a shift for an inclusive date range (AT-011). FR-SA001.
type ShiftAssignment struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_shift_assignments_employee,priority:1"`
	EmployeeProfileID uuid.UUID  `gorm:"type:uuid;not null;index:idx_shift_assignments_employee,priority:2"`
	ShiftID           uuid.UUID  `gorm:"type:uuid;not null;index:idx_shift_assignments_shift"`
	EffectiveFrom     time.Time  `gorm:"type:date;not null;index:idx_shift_assignments_employee,priority:3"`
	EffectiveTo       *time.Time `gorm:"type:date"`
	CreatedByUserID   *uuid.UUID `gorm:"type:uuid"`
	CreatedAt         time.Time  `gorm:"not null"`
	UpdatedAt         time.Time  `gorm:"not null"`
}

// BeforeCreate assigns a UUID v4 when unset.
func (a *ShiftAssignment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
