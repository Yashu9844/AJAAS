// Package api is the contract between M3 contexts: the series context implements Authorizer, and the occurrence-content,
// action-item and feed contexts depend on this package only (docs/modules/m3-meetings/design.md section 1).
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/kernel"
	"gorm.io/gorm"
)

// Perm is a permission held on a meeting series through an M0 resource role (DMT-09). Keys are "meeting:<action>".
type Perm string

// Meeting-series permissions.
const (
	Read               Perm = "meeting:read"
	Update             Perm = "meeting:update"
	Cancel             Perm = "meeting:cancel"
	ManageParticipants Perm = "meeting:manage_participants"
	ProposeAgenda      Perm = "meeting:propose_agenda"
	ManageAgenda       Perm = "meeting:manage_agenda"
	EditMinutes        Perm = "meeting:edit_minutes"
	PublishMinutes     Perm = "meeting:publish_minutes"
	RecordDecision     Perm = "meeting:record_decision"
	CreateAction       Perm = "meeting:create_action"
	MarkAttendance     Perm = "meeting:mark_attendance"
)

// Role templates of M0 (identity/access TemplateMeeting*), least privileged first.
const (
	RoleViewer    = "meeting_viewer"
	RoleAttendee  = "meeting_attendee"
	RoleNoteTaker = "meeting_note_taker"
	RoleOrganizer = "meeting_organizer"
)

// AllPerms lists every meeting-series permission.
var AllPerms = []Perm{Read, Update, Cancel, ManageParticipants, ProposeAgenda, ManageAgenda, EditMinutes, PublishMinutes,
	RecordDecision, CreateAction, MarkAttendance}

// TemplatePerms returns the permissions a predefined role holds (its own and inherited).
func TemplatePerms(template string) map[Perm]bool {
	levels := map[string][]Perm{
		RoleViewer:    {Read},
		RoleAttendee:  {Read, ProposeAgenda},
		RoleNoteTaker: {Read, ProposeAgenda, EditMinutes, PublishMinutes, RecordDecision, CreateAction, MarkAttendance},
		RoleOrganizer: AllPerms,
	}
	out := map[Perm]bool{}
	for _, p := range levels[template] {
		out[p] = true
	}
	return out
}

// Visibility values (DMT-12).
const (
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal"
	VisibilityBusyOnly = "busy_only"
)

// Access levels: what a caller sees of a series.
const (
	LevelFull     = "full"     // a participant with a role, or manage_all
	LevelAudit    = "audit"    // meetings:read_all - metadata, participants, published minutes and decisions
	LevelInternal = "internal" // an internal meeting seen by a non-participant - title, time and place only
	LevelBusy     = "busy"     // a busy_only meeting seen by a non-participant - start and end only
)

// Actor is the authenticated caller plus the tenant-level permissions the request guards resolved (DMT-10).
type Actor struct {
	TenantID, UserID uuid.UUID
	IP, UserAgent    string
	Create           bool // meetings:create
	ReadAll          bool // meetings:read_all
	ReadConfidential bool // meetings:read_confidential
	ManageAll        bool // meetings:manage_all
}

// SeriesInfo is the part of a series every context needs.
type SeriesInfo struct {
	ID               uuid.UUID
	Title            string
	OrganizerID      uuid.UUID
	Visibility       string
	Confidential     bool
	Status           string
	DefaultProjectID *uuid.UUID
	TZ               string
}

// OccurrenceInfo is the part of an occurrence every context needs.
type OccurrenceInfo struct {
	ID       uuid.UUID
	SeriesID uuid.UUID
	StartAt  time.Time
	EndAt    time.Time
	Status   string
}

// Access is what the caller may do on one series.
type Access struct {
	Series      SeriesInfo
	Role        string // role name held through M0, "" for none
	Template    string // template key of that role, "" for a custom role or none
	Participant bool
	Level       string
	Perms       map[Perm]bool
}

// Can reports whether the caller holds p on the series.
func (a Access) Can(p Perm) bool { return a.Perms[p] }

// Require returns 403 when the caller can read the series but lacks p, and 409 when a cancelled series blocks a write.
func (a Access) Require(p Perm) error {
	if !a.Perms[p] {
		return kernel.Forbidden(string(p))
	}
	if p != Read && a.Series.Status == "cancelled" {
		return kernel.Conflict("MEETING_CANCELLED", "the meeting is cancelled")
	}
	return nil
}

// ReadsContent reports whether the caller may read agenda, decisions, attendance and published minutes.
func (a Access) ReadsContent() bool { return a.Level == LevelFull || a.Level == LevelAudit }

// ReadsDrafts reports whether the caller may read unpublished notes (note-takers and above; never auditors, DMT-11).
func (a Access) ReadsDrafts() bool { return a.Level == LevelFull && a.Perms[EditMinutes] }

// Authorizer resolves what a caller may do on a series (the series context implements it).
type Authorizer interface {
	// Access returns the caller access to the series, or 404 when the caller cannot see it at all.
	Access(ctx context.Context, db *gorm.DB, a Actor, seriesID uuid.UUID) (Access, error)
	// Occurrence resolves an occurrence and the caller access to its series (404 as Access).
	Occurrence(ctx context.Context, db *gorm.DB, a Actor, occurrenceID uuid.UUID) (Access, OccurrenceInfo, error)
}

// Flags are the context keys the route guards set for the tenant-level permissions.
const (
	FlagCreate           = "meetings_create"
	FlagReadAll          = "meetings_read_all"
	FlagReadConfidential = "meetings_read_confidential"
	FlagManageAll        = "meetings_manage_all"
)

// ActorFrom builds the Actor from the request context set by the identity guards.
func ActorFrom(c *gin.Context) (Actor, bool) {
	t, okT := c.Get("tenant_id")
	u, okU := c.Get("user_id")
	tid, ok1 := t.(uuid.UUID)
	uid, ok2 := u.(uuid.UUID)
	if !okT || !okU || !ok1 || !ok2 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized request"}})
		return Actor{}, false
	}
	return Actor{TenantID: tid, UserID: uid, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Create: c.GetBool(FlagCreate),
		ReadAll: c.GetBool(FlagReadAll), ReadConfidential: c.GetBool(FlagReadConfidential), ManageAll: c.GetBool(FlagManageAll)}, true
}
