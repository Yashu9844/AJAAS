package series

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// loadSeries reads a live series of the tenant; deleted and foreign series are a 404.
func loadSeries(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*Series, error) {
	var row Series
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("meeting")
	}
	return &row, err
}

func info(r *Series) api.SeriesInfo {
	return api.SeriesInfo{ID: r.ID, Title: r.Title, OrganizerID: r.OrganizerID, Visibility: r.Visibility, Confidential: r.Confidential,
		Status: r.Status, DefaultProjectID: r.DefaultProjectID, TZ: r.TZ}
}

// Access implements api.Authorizer (design.md section 3): manage_all, then the M0 role on the series, then read_all
// (with read_confidential for confidential meetings), then the visibility seen by non-participants; otherwise 404.
func (s *Service) Access(ctx context.Context, db *gorm.DB, a api.Actor, seriesID uuid.UUID) (api.Access, error) {
	row, err := loadSeries(ctx, db, a.TenantID, seriesID)
	if err != nil {
		return api.Access{}, err
	}
	return s.accessFor(ctx, db, a, row)
}

func (s *Service) accessFor(ctx context.Context, db *gorm.DB, a api.Actor, row *Series) (api.Access, error) {
	acc := api.Access{Series: info(row), Perms: map[api.Perm]bool{}}
	role, keys, err := s.p.Resources.PermissionsOn(ctx, db, ref(a.TenantID, row.ID), a.UserID)
	if err != nil {
		return api.Access{}, err
	}
	if role.RoleID != uuid.Nil {
		acc.Role, acc.Template, acc.Participant, acc.Level = role.RoleName, role.TemplateKey, true, api.LevelFull
		for _, k := range keys {
			acc.Perms[api.Perm(k)] = true
		}
	}
	switch {
	case a.ManageAll:
		acc.Level = api.LevelFull
		for _, p := range api.AllPerms {
			acc.Perms[p] = true
		}
	case acc.Participant:
	case a.ReadAll && (!row.Confidential || a.ReadConfidential):
		acc.Level, acc.Perms[api.Read] = api.LevelAudit, true
	case row.Visibility == api.VisibilityInternal:
		acc.Level = api.LevelInternal
	case row.Visibility == api.VisibilityBusyOnly:
		acc.Level = api.LevelBusy
	default:
		return api.Access{}, kernel.NotFound("meeting")
	}
	return acc, nil
}

// Occurrence implements api.Authorizer for an occurrence id.
func (s *Service) Occurrence(ctx context.Context, db *gorm.DB, a api.Actor, occurrenceID uuid.UUID) (api.Access, api.OccurrenceInfo, error) {
	var o Occurrence
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", occurrenceID, a.TenantID).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.Access{}, api.OccurrenceInfo{}, kernel.NotFound("occurrence")
	}
	if err != nil {
		return api.Access{}, api.OccurrenceInfo{}, err
	}
	acc, err := s.Access(ctx, db, a, o.SeriesID)
	if err != nil {
		return api.Access{}, api.OccurrenceInfo{}, kernel.NotFound("occurrence")
	}
	oi := api.OccurrenceInfo{ID: o.ID, SeriesID: o.SeriesID, StartAt: o.StartAt, EndAt: o.EndAt, Status: o.EffectiveStatus(s.now())}
	return acc, oi, nil
}

// requireContent answers 404 to callers who see only a busy block or the title, so content routes leak nothing.
func requireContent(acc api.Access, what string) error {
	if !acc.ReadsContent() {
		return kernel.NotFound(what)
	}
	return nil
}

// auditRead records that a caller read a meeting they are not invited to through manage_all or read_all (DMT-11).
func (s *Service) auditRead(ctx context.Context, a api.Actor, acc api.Access) {
	if acc.Participant || (acc.Level != api.LevelAudit && acc.Level != api.LevelFull) {
		return
	}
	_ = s.audit(ctx, s.db, a, auditEntry{"meeting.read_privileged", acc.Series.ID, map[string]string{"level": acc.Level}})
}
