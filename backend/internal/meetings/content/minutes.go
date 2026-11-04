package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// Minutes is one published, frozen version of the minutes (MN-3/4); rows are append-only.
type Minutes struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID `gorm:"type:uuid" json:"occurrence_id"`
	Version      int       `json:"version"`
	BodyMD       string    `gorm:"column:body_md" json:"body,omitempty"`
	SHA256       string    `gorm:"column:sha256" json:"sha256"`
	PublishedBy  uuid.UUID `gorm:"type:uuid" json:"published_by"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"published_at"`
}

// TableName maps to meeting_minutes.
func (Minutes) TableName() string { return "meeting_minutes" }

// PublishInput publishes minutes; a correction after the first publish needs a reason (MN-4).
type PublishInput struct {
	Reason string `json:"reason"`
}

// Publish freezes the current notes as minutes version 1, sets the occurrence status and tells M10 (J6 step 1).
func (s *Service) Publish(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) (*Minutes, error) {
	return s.publish(ctx, a, occurrenceID, "", false)
}

// Correct publishes a new version after the first, with a reason; earlier versions stay readable (J6 step 2).
func (s *Service) Correct(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, in PublishInput) (*Minutes, error) {
	reason, err := kernel.Text("reason", in.Reason, 1, 500)
	if err != nil {
		return nil, err
	}
	return s.publish(ctx, a, occurrenceID, reason, true)
}

func (s *Service) publish(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, reason string, correction bool) (*Minutes, error) {
	var out *Minutes
	err := s.inTx(ctx, func(tx *gorm.DB) error {
		sc, err := s.writable(ctx, tx, a, occurrenceID, api.PublishMinutes)
		if err != nil {
			return err
		}
		var last int
		if err := tx.Raw(`SELECT coalesce(max(version), 0) FROM meeting_minutes WHERE occurrence_id = ?`, occurrenceID).Scan(&last).Error; err != nil {
			return err
		}
		if !correction && last > 0 {
			return kernel.Conflict("MINUTES_PUBLISHED", "the minutes are published; publish a correction with a reason")
		}
		if correction && last == 0 {
			return kernel.Conflict("MINUTES_NOT_PUBLISHED", "publish the minutes before correcting them")
		}
		body, err := s.render(ctx, tx, sc)
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(body))
		out = &Minutes{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, Version: last + 1, BodyMD: body,
			SHA256: hex.EncodeToString(sum[:]), PublishedBy: a.UserID, Reason: reason}
		if err := tx.Create(out).Error; err != nil {
			return err
		}
		if err := s.p.Lifecycle.MarkPublished(ctx, tx, occurrenceID); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, sc, "minutes.published", map[string]int{"version": out.Version}); err != nil {
			return err
		}
		return emit(ctx, tx, sc, "minutes.published", map[string]any{"version": out.Version})
	})
	return out, err
}

// render assembles the published markdown: attendance, notes per section in agenda order, decisions.
func (s *Service) render(ctx context.Context, tx *gorm.DB, sc scope) (string, error) {
	secs, err := s.sections(ctx, tx, sc.occ.ID)
	if err != nil {
		return "", err
	}
	hasText := false
	for _, sec := range secs {
		hasText = hasText || strings.TrimSpace(sec.Body) != ""
	}
	if !hasText {
		return "", kernel.Unprocessable("NOTES_EMPTY", "write notes before publishing the minutes")
	}
	var b strings.Builder
	title := sc.acc.Series.Title
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", title, sc.occ.StartAt.UTC().Format("2006-01-02 15:04 UTC"))
	if att, err := s.attendanceLines(ctx, tx, sc); err != nil {
		return "", err
	} else if att != "" {
		b.WriteString("## Attendance\n\n" + att + "\n")
	}
	for _, sec := range secs {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", sec.Title, strings.TrimSpace(sec.Body))
	}
	var decisions []Decision
	if err := tx.Where("occurrence_id = ? AND status <> 'superseded'", sc.occ.ID).Order("decided_at, id").Find(&decisions).Error; err != nil {
		return "", err
	}
	if len(decisions) > 0 {
		b.WriteString("## Decisions\n\n")
		for _, d := range decisions {
			fmt.Fprintf(&b, "- %s (%s)\n", d.Text, d.Status)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func (s *Service) attendanceLines(ctx context.Context, tx *gorm.DB, sc scope) (string, error) {
	var rows []Attendance
	if err := tx.Where("occurrence_id = ?", sc.occ.ID).Order("state, user_id").Find(&rows).Error; err != nil {
		return "", err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	names := s.names(ctx, sc.a.TenantID, ids)
	var b strings.Builder
	for _, r := range rows {
		name := names[r.UserID]
		if name == "" {
			name = r.UserID.String()
		}
		fmt.Fprintf(&b, "- %s: %s\n", name, r.State)
	}
	return b.String(), nil
}

// MinutesVersions lists the published versions without bodies, newest first.
func (s *Service) MinutesVersions(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]Minutes, error) {
	if _, err := s.occurrence(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	var rows []Minutes
	err := s.db.WithContext(ctx).Select("id, occurrence_id, version, sha256, published_by, reason, created_at").
		Where("occurrence_id = ?", occurrenceID).Order("version DESC").Find(&rows).Error
	return rows, err
}

// MinutesVersion returns one published version (version 0 = the latest); readers of content, including auditors.
func (s *Service) MinutesVersion(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, version int) (*Minutes, error) {
	if _, err := s.occurrence(ctx, s.db, a, occurrenceID); err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID)
	if version > 0 {
		q = q.Where("version = ?", version)
	}
	var m Minutes
	err := q.Order("version DESC").Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("published minutes")
	}
	return &m, err
}
