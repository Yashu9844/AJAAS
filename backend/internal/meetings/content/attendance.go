package content

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Attendance is one participant attendance on an occurrence (AT-1).
type Attendance struct {
	TenantID     uuid.UUID `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	UserID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	State        string    `json:"state"`
	MarkedBy     uuid.UUID `gorm:"type:uuid" json:"marked_by"`
	UpdatedAt    time.Time `json:"updated_at"`
	Name         string    `gorm:"-" json:"name"`
}

// TableName maps to meeting_attendance.
func (Attendance) TableName() string { return "meeting_attendance" }

// AttendanceEntry marks one participant.
type AttendanceEntry struct {
	UserID uuid.UUID `json:"user_id"`
	State  string    `json:"state"`
}

// AttendanceInput marks several participants at once.
type AttendanceInput struct {
	Entries []AttendanceEntry `json:"entries"`
}

// AttendanceList returns the marks of an occurrence with names.
func (s *Service) AttendanceList(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]Attendance, error) {
	if _, err := s.occurrence(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	return s.attendance(ctx, s.db, a.TenantID, occurrenceID)
}

func (s *Service) attendance(ctx context.Context, db *gorm.DB, tenantID, occurrenceID uuid.UUID) ([]Attendance, error) {
	var rows []Attendance
	if err := db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID).Order("user_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	names := s.names(ctx, tenantID, ids)
	for i := range rows {
		rows[i].Name = names[rows[i].UserID]
	}
	return rows, nil
}

// MarkAttendance records attendance once the occurrence has started; every user must be a participant (J4 step 2).
func (s *Service) MarkAttendance(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in AttendanceInput) ([]Attendance, error) {
	if len(in.Entries) == 0 || len(in.Entries) > 500 {
		return nil, kernel.Invalid("entries", "1 to 500 entries")
	}
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sc, err := s.writable(ctx, tx, a, occurrenceID, api.MarkAttendance)
		if err != nil {
			return err
		}
		if s.now().Before(sc.occ.StartAt.Add(-15 * time.Minute)) {
			return kernel.Conflict("OCCURRENCE_NOT_STARTED", "attendance is marked once the occurrence starts")
		}
		for _, e := range in.Entries {
			if err := s.markOne(ctx, tx, sc, e); err != nil {
				return err
			}
		}
		return emit(ctx, tx, sc, "attendance.marked", map[string]any{"count": len(in.Entries)})
	})
	if err != nil {
		return nil, err
	}
	return s.attendance(ctx, s.db, a.TenantID, occurrenceID)
}

func (s *Service) markOne(ctx context.Context, tx *gorm.DB, sc scope, e AttendanceEntry) error {
	if err := kernel.OneOf("state", e.State, "present", "late", "absent", "excused"); err != nil {
		return err
	}
	var n int64
	if err := tx.WithContext(ctx).Table("meeting_participants").Where("series_id = ? AND user_id = ?", sc.occ.SeriesID, e.UserID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return kernel.Invalid("entries", "user "+e.UserID.String()+" is not a participant")
	}
	row := Attendance{TenantID: sc.a.TenantID, OccurrenceID: sc.occ.ID, UserID: e.UserID, State: e.State, MarkedBy: sc.a.UserID, UpdatedAt: s.now().UTC()}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "occurrence_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "marked_by", "updated_at"}),
	}).Create(&row).Error
}
