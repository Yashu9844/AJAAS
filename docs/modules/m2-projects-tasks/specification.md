# M2 Projects & Tasks — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** collaborative execution and delivery management.

**Data model:** Project, ProjectMember, Milestone, Board, Column/Status,
Task, TaskDependency, TaskAssignee, Comment, AttachmentReference,
Sprint, Label, ActivityEvent, TimeEntry (if approved in product scope).
Employee identity is referenced by stable user/employee IDs; do not
duplicate HR profile facts.

**Representative APIs:**
- `GET/POST /api/v1/projects`
- `GET/PATCH /api/v1/projects/{projectId}`
- `POST /api/v1/projects/{projectId}/members`
- `GET/POST /api/v1/projects/{projectId}/tasks`
- `GET/PATCH /api/v1/tasks/{taskId}`
- `POST /api/v1/tasks/{taskId}/comments`
- `POST /api/v1/tasks/{taskId}/dependencies`
- `POST /api/v1/projects/{projectId}/sprints`
- `POST /api/v1/sprints/{id}/start` and `/complete`

**Rules:** project membership and tenant boundary; legal status
transitions; cycle/dependency detection; optimistic concurrency on task
edits; authorization on comments/attachments; soft-delete/archive
policy; stable activity history.

**Events:** `project.created`, `project.member.changed`, `task.created`,
`task.updated`, `task.assigned`, `task.completed`, `sprint.started`,
`sprint.completed`. Events reference IDs and versions; Notifications
consumes assignment/due-date events.

**Latency/scale:** paginated task queries with indexes
`(tenant_id, project_id, status, updated_at)` and
`(tenant_id, assignee_id, due_at)` where workload confirms; avoid
loading full task history in board view; board summaries can be
materialized/cached with version invalidation.

**Acceptance tests:** duplicate task creation replay; concurrent status
updates; dependency cycles rejected; unauthorized project membership
denied; task event replay; large board pagination; attachment ACL;
tenant-isolated search.

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Project` | project_id, tenant_id, name, status, owner_user_id, version |
| `ProjectMember` | tenant_id, project_id, user_id, role |
| `Milestone` | milestone_id, project_id, due_at, status |
| `Board` | board_id, project_id, name |
| `Task` | task_id, tenant_id, project_id, parent_task_id, status, priority, due_at, version |
| `TaskAssignee` | tenant_id, task_id, user_id |
| `TaskDependency` | tenant_id, predecessor_task_id, successor_task_id, dependency_type |
| `TaskComment` | comment_id, task_id, author_user_id, body, created_at |
| `TaskActivity` | activity_id, task_id, actor_id, action, occurred_at |
| `Sprint` | sprint_id, project_id, start_at, end_at, status |
| `TaskLabel` | tenant_id, task_id, label_id |
| `AttachmentReference` | task_id, document_id, version_ref |

## Relationships and invariants

Project 1:N Milestone, Board, Task and Sprint. Project M:N User via ProjectMember. Task can have parent/child hierarchy and M:N assignees; TaskDependency has unique ordered pair and cycle detection. TaskComment and TaskActivity are append-oriented.

## Service dependencies

References users/employees by stable IDs only; never reads HR tables. Documents owns attachments and ACLs. Publishes task/project events consumed by Notifications and Reports.
