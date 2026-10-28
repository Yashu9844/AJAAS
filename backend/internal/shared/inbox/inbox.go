// Package inbox makes event handlers idempotent. The broker delivers at least once, so a handler calls Claim inside the
// transaction that applies the event's effect: the first delivery records the event id and proceeds, a redelivery finds
// the row and the handler skips it. Each service owns one inbox table (consumer, event_id, tenant_id, event_type).
package inbox

import (
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var tableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Claim records that consumer is handling event eventID and reports whether this is the first time. Call it in the
// handler's transaction; if the transaction rolls back, the claim rolls back with it and the event is handled on
// redelivery. table is a trusted constant.
func Claim(tx *gorm.DB, table, consumer string, tenantID, eventID uuid.UUID, eventType string) (bool, error) {
	if !tableName.MatchString(table) {
		return false, fmt.Errorf("inbox: invalid table name %q", table)
	}
	res := tx.Exec(`INSERT INTO `+table+` (event_id, consumer, tenant_id, event_type) VALUES (?, ?, ?, ?)
		ON CONFLICT (consumer, event_id) DO NOTHING`, eventID, consumer, tenantID, eventType)
	return res.RowsAffected == 1, res.Error
}
