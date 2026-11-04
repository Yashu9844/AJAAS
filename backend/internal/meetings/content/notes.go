package content

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// GeneralSection names the notes that belong to no agenda item.
const GeneralSection = "general"

// NoteVersion is one saved version of a note section (append-only, DMT-04).
type NoteVersion struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid" json:"-"`
	OccurrenceID uuid.UUID  `gorm:"type:uuid" json:"occurrence_id"`
	Section      *uuid.UUID `gorm:"type:uuid" json:"section"`
	Version      int        `json:"version"`
	BodyMD       string     `gorm:"column:body_md" json:"body"`
	AuthorID     uuid.UUID  `gorm:"type:uuid" json:"author_id"`
	Source       string     `json:"source"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TableName maps to meeting_note_versions.
func (NoteVersion) TableName() string { return "meeting_note_versions" }

// Section is the latest version of one note section.
type Section struct {
	Section   string    `json:"section"` // "general" or the agenda item id
	Title     string    `json:"title"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	AuthorID  uuid.UUID `json:"author_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NotesInput saves a section; Version is the version the edit is based on (0 for the first save).
type NotesInput struct {
	Body    string `json:"body"`
	Version int    `json:"version"`
}

// parseSection maps "general" to nil and anything else to an agenda item id of the occurrence.
func parseSection(tx *gorm.DB, occurrenceID uuid.UUID, raw string) (*uuid.UUID, string, error) {
	if raw == GeneralSection {
		return nil, "General", nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, "", kernel.Invalid("section", `must be "general" or an agenda item id`)
	}
	var titles []string
	if err := tx.Model(&AgendaItem{}).Where("id = ? AND occurrence_id = ?", id, occurrenceID).Pluck("title", &titles).Error; err != nil {
		return nil, "", err
	}
	if len(titles) == 0 {
		return nil, "", kernel.NotFound("agenda item")
	}
	return &id, titles[0], nil
}

func latest(tx *gorm.DB, occurrenceID uuid.UUID, section *uuid.UUID) (*NoteVersion, error) {
	q := tx.Where("occurrence_id = ?", occurrenceID)
	if section == nil {
		q = q.Where("section IS NULL")
	} else {
		q = q.Where("section = ?", *section)
	}
	var rows []NoteVersion
	if err := q.Order("version DESC").Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// draftReader requires the right to read unpublished notes (note-takers and organizers, never auditors, DMT-11).
func draftReader(sc scope) error {
	if !sc.acc.ReadsDrafts() {
		return kernel.Forbidden(string(api.EditMinutes) + " (drafts are visible to note-takers; read the published minutes)")
	}
	return nil
}

// SaveNotes stores a new version of a section; a stale base version is a 409 that carries the current text (MN-1/2).
func (s *Service) SaveNotes(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, section string, in NotesInput) (*Section, error) {
	body, err := kernel.Text("body", in.Body, 0, 50000)
	if err != nil {
		return nil, err
	}
	var out *Section
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		sc, err := s.writable(ctx, tx, a, occurrenceID, api.EditMinutes)
		if err != nil {
			return err
		}
		sid, title, err := parseSection(tx, occurrenceID, section)
		if err != nil {
			return err
		}
		cur, err := latest(tx, occurrenceID, sid)
		if err != nil {
			return err
		}
		curVersion, curBody := 0, ""
		if cur != nil {
			curVersion, curBody = cur.Version, cur.BodyMD
		}
		if in.Version != curVersion {
			return &kernel.VersionConflict{Current: curVersion, Body: curBody}
		}
		v := &NoteVersion{ID: uuid.New(), TenantID: a.TenantID, OccurrenceID: occurrenceID, Section: sid, Version: curVersion + 1,
			BodyMD: body, AuthorID: a.UserID, Source: "human"}
		if err := tx.Create(v).Error; err != nil {
			return err
		}
		out = &Section{Section: section, Title: title, Version: v.Version, Body: v.BodyMD, AuthorID: v.AuthorID, UpdatedAt: v.CreatedAt}
		return emit(ctx, tx, sc, "minutes.version_saved", map[string]any{"section": section, "version": v.Version})
	})
	if kernel.IsUniqueViolation(err) { // a parallel save of the same section won the race
		return nil, &kernel.VersionConflict{Current: in.Version + 1}
	}
	return out, err
}

// Notes returns the latest version of every section, general first, then agenda order.
func (s *Service) Notes(ctx context.Context, a api.Actor, occurrenceID uuid.UUID) ([]Section, error) {
	sc, err := s.occurrence(ctx, s.db, a, occurrenceID)
	if err != nil {
		return nil, err
	}
	if err := draftReader(sc); err != nil {
		return nil, err
	}
	return s.sections(ctx, s.db, occurrenceID)
}

func (s *Service) sections(ctx context.Context, db *gorm.DB, occurrenceID uuid.UUID) ([]Section, error) {
	var rows []struct {
		NoteVersion
		Title *string
	}
	err := db.WithContext(ctx).Raw(`SELECT DISTINCT ON (n.section) n.*, a.title FROM meeting_note_versions n
		LEFT JOIN meeting_agenda_items a ON a.id = n.section WHERE n.occurrence_id = ? ORDER BY n.section NULLS FIRST, n.version DESC`,
		occurrenceID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	agenda := []AgendaItem{}
	if err := db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID).Order("rank, id").Find(&agenda).Error; err != nil {
		return nil, err
	}
	bySection := map[string]Section{}
	for _, r := range rows {
		key, title := GeneralSection, "General"
		if r.Section != nil {
			key, title = r.Section.String(), "(removed agenda item)"
			if r.Title != nil {
				title = *r.Title
			}
		}
		bySection[key] = Section{Section: key, Title: title, Version: r.Version, Body: r.BodyMD, AuthorID: r.AuthorID, UpdatedAt: r.CreatedAt}
	}
	return orderSections(bySection, agenda), nil
}

func orderSections(by map[string]Section, agenda []AgendaItem) []Section {
	out := []Section{}
	if g, ok := by[GeneralSection]; ok {
		out = append(out, g)
		delete(by, GeneralSection)
	}
	for _, it := range agenda {
		if sec, ok := by[it.ID.String()]; ok {
			out = append(out, sec)
			delete(by, it.ID.String())
		}
	}
	for _, sec := range by { // notes of removed agenda items come last
		out = append(out, sec)
	}
	return out
}

// NoteVersions lists every version of one section, newest first (MN-2).
func (s *Service) NoteVersions(ctx context.Context, a api.Actor, occurrenceID uuid.UUID, section string) ([]NoteVersion, error) {
	sc, err := s.occurrence(ctx, s.db, a, occurrenceID)
	if err != nil {
		return nil, err
	}
	if err := draftReader(sc); err != nil {
		return nil, err
	}
	sid, _, err := parseSection(s.db, occurrenceID, section)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Where("occurrence_id = ?", occurrenceID)
	if sid == nil {
		q = q.Where("section IS NULL")
	} else {
		q = q.Where("section = ?", *sid)
	}
	var rows []NoteVersion
	err = q.Order("version DESC").Limit(200).Find(&rows).Error
	return rows, err
}
