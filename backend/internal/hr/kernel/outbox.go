package kernel

import "github.com/jaas/jaas/internal/shared/outbox"

// OutboxRelay is the shared table relay (internal/shared/outbox), kept under its HR name for the HR contexts.
type OutboxRelay = outbox.TableRelay

// NewOutboxRelay builds a relay for table publishing to exchange.
var NewOutboxRelay = outbox.NewTableRelay
