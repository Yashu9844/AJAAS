// Package projects is Module 2, Projects & Tasks: its own service in the same repository, with its own schema
// (`projects`), migrations, routes and events (docs/modules/m2-projects-tasks/design.md). It reaches other services only
// through the interfaces in ports/ (identity, people, storage), never through their packages.
package projects

const (
	// Schema is the Postgres schema the module owns.
	Schema = "projects"
	// OutboxTable and InboxTable hold the module's published events and consumed-event ledger.
	OutboxTable = "projects_events_outbox"
	InboxTable  = "projects_events_inbox"
	// Exchange is the topic exchange the module publishes to.
	Exchange = "jaas.projects.events"
)
