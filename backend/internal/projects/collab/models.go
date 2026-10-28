package collab

import (
	"time"

	"github.com/google/uuid"
)

// Comment is a task comment; a deleted comment keeps its row as a tombstone with an empty body (CM-01).
type Comment struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID   `gorm:"type:uuid" json:"-"`
	TaskID       uuid.UUID   `gorm:"type:uuid" json:"task_id"`
	AuthorUserID uuid.UUID   `gorm:"type:uuid" json:"author_user_id"`
	Body         string      `json:"body"`
	EditedAt     *time.Time  `json:"edited_at"`
	DeletedAt    *time.Time  `json:"deleted_at"`
	DeletedBy    *uuid.UUID  `gorm:"type:uuid" json:"-"`
	CreatedAt    time.Time   `json:"created_at"`
	Deleted      bool        `gorm:"-" json:"deleted"`
	Mentions     []uuid.UUID `gorm:"-" json:"mentions"`
}

// TableName maps to comments.
func (Comment) TableName() string { return "comments" }

// Watcher is a user who follows a task (CM-03). Reason is explicit, reporter, assignee, commenter or mentioned.
type Watcher struct {
	TaskID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"task_id"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	TenantID  uuid.UUID `gorm:"type:uuid" json:"-"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName maps to task_watchers.
func (Watcher) TableName() string { return "task_watchers" }

// TimeEntry is time logged on a task; EndedAt is nil while a timer runs (TM-01).
type TimeEntry struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid" json:"-"`
	TaskID      uuid.UUID  `gorm:"type:uuid" json:"task_id"`
	UserID      uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	Minutes     int        `json:"minutes"`
	Note        string     `json:"note"`
	Source      string     `json:"source"`
	AutoStopped bool       `json:"auto_stopped"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName maps to time_entries.
func (TimeEntry) TableName() string { return "time_entries" }

// Attachment is the metadata of one stored file; the bytes live in object storage (AT-01).
type Attachment struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid" json:"-"`
	ProjectID   uuid.UUID  `gorm:"type:uuid" json:"project_id"`
	TaskID      uuid.UUID  `gorm:"type:uuid" json:"task_id"`
	StorageKey  string     `json:"-"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	SHA256      string     `gorm:"column:sha256" json:"sha256"`
	Status      string     `json:"status"`
	UploadedBy  uuid.UUID  `gorm:"type:uuid" json:"uploaded_by"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadyAt     *time.Time `json:"ready_at"`
	DeletedAt   *time.Time `json:"-"`
}

// TableName maps to attachments.
func (Attachment) TableName() string { return "attachments" }
