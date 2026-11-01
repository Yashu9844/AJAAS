# M0 - resource-scoped roles (project roles in M0)

**Status:** approved by the owner on 2026-10-30 ("project-scoped roles in M0"); built and tested (decision DPJ-49 in `docs/modules/m2-projects-tasks/decisions.md`).
**Why:** all permission decisions belong to M0. M2 Projects & Tasks used to keep per-project roles in its own tables; HR keeps no roles of its own. M0 now
owns project roles too, so there is one catalogue, one role editor, one resolver, one cache and one audit trail for every permission in the platform.

## Model

| Concept | Where | Notes |
|---|---|---|
| Permission with `applies_to` | `permissions.applies_to` (`tenant` default, `project`) | Catalogue entries registered by a module say what they apply to. Project-level permissions are `task:create`, `task:update`, `comment:moderate`, `column:manage`, ... (resource:action as for every permission). |
| Role with `applies_to` | `roles.applies_to` (`tenant` default, `project`) | A project role may hold only `project` permissions; a tenant role only `tenant` permissions (checked when grants are attached). Roles keep inheritance (a parent's grants flow down), templates and custom roles. |
| Predefined project roles | templates `project_viewer`, `project_member`, `project_manager`, `project_owner` (chain: viewer <- member <- manager <- owner) | Seeded into every tenant by the existing template mechanism; editable defaults come from the catalogue `Defaults`. |
| Assignment on a resource | `resource_role_assignments(tenant_id, user_id, role_id, resource_type, resource_id, source, assigned_by)` | One role per user per resource. `resource_id` has no foreign key (it belongs to another service's schema). `source` is `manual`, `group` or `automatic`. |
| Decision | `ResourceAccess.PermissionsOn(tenant, user, resource_type, resource_id)` | Resolves the user's role on that resource through the normal resolver (inheritance included) and returns the permission keys. Cached under the tenant access version; every assignment or role change bumps the version, so changes apply immediately. |

Tenant-wide permissions (`projects:create`, `projects:read_all`, `projects:manage_all`) stay ordinary M0 tenant permissions granted by ordinary roles.
`projects:manage_all` is the administrator override that M2 maps to "every project permission".

## Ports (so the same code works when M0 is a separate service)

`identityport.Port.Resources()` returns `ResourceAccess`: `PermissionsOn` (role and permission keys), `RolesFor` (one user across many resources), `RolesOn` (everyone on one resource), `AssignRole`, `RevokeRole`, `RoleByTemplate`, `AvailableRoles`, `CountWithTemplate`. Refusals are `ErrRoleNotFound` and `ErrRoleNotApplicable`.
Write methods take the caller's transaction (in-process today) so a membership change and its role assignment commit together. The in-process adapter calls the M0
service directly; a future network adapter would use an HTTP API with the same shapes. M2 never reads M0 tables.

## What moves out of M2

- M2 table `project_roles` and the columns `project_members.role_key` / `custom_role_id`: dropped by migration 000073. There is no backfill because module 2 was never released; development data loses its member roles.
- M2 routes `/project-roles*` and `/project-permissions`: removed. Roles are edited in the M0 role editor (`POST /roles` with `applies_to: project`; permissions carry `applies_to`). `GET /projects/{id}/roles` lists the roles a project manager can assign without any M0 role-management right.
- M2 tenant permission `project_roles:manage`: removed; defining roles needs M0 role management.
- M2 keeps what is not a permission: membership records (who is in a project, from which source, active or not), the archived-project freeze, WIP rules, owner-count guard (asked from M0 by template key).

## Security notes

- A role assigned on project P grants nothing outside P. The resolver never mixes resource roles into tenant-level decisions.
- Custom project roles cannot contain tenant permissions, and cannot be assigned tenant-wide (checked on attach and on assign).
- Escalation: assigning a project role requires `project.manage_members` on that project (M2 checks; only owners and tenant administrators grant `owner`). The M0 non-escalation rule still applies to tenant roles; it is skipped for project permissions because they act only inside a project where the caller assigns the role.
- Every project role must hold `project:read` and `task:read` (M0 refuses others), so a member can always read the project and its tasks.
- Deleting a role that is assigned on any resource is refused (409), like tenant roles.
- Audit: every assignment is audited by M0 (`resource_role.assigned|revoked`) with ids only; a system change (group rule refresh) is recorded without an actor.
- Cost: one decision is `Find` plus `RoleGrants` (about five queries); the access version already invalidates the tenant cache. A per-request cache of the decision is a later optimisation.
- Tests: `TestProjectRolesAreDecidedByIdentity` (real M0 with M2), `TestProjectsContext`, `TestProjectGroupRulesFollowHR`; M2 unit tests use `testdb.Resources`, a stand-in backed by a rollback-safe table.
