# HR / Employee — Decisions

> Part of **M1 HR Management** (bounded context `employee`). Formerly the stand-alone "module-2-employee" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

| Date | ID | Context | Decision | Impact |
|---|---|---|---|---|
| 2026-10-05 | DEC-E001 | 1:1 relationship between User and EmployeeProfile | Enforce unique `user_id` per tenant on `EmployeeProfile`. An employee must link to an authenticated User. | Clear identity separation while maintaining referential integrity. |
| 2026-10-05 | DEC-E002 | Sensitive PII separation | Store bank accounts and government tax IDs in a separate `employee_statutory` table with strict RBAC (`employee:read_sensitive`). | Zero leak of sensitive financial PII on general employee directory queries. |
| 2026-10-05 | DEC-E003 | Atomic Onboarding | Create profile, employment details, contact records, and initial timeline event inside a single DB transaction. | Prevents dangling or incomplete employee records. |
| 2026-10-05 | DEC-E004 | Reactive user deactivation convergence | When M0 Identity user is deactivated, sync employee status to `inactive`/`terminated` and log timeline exit. | Guarantees consistency between authentication state and HR status. |

### D-HARDEN-5 (2026-10-06): sensitive data and lifecycle
* `GET /employees/:id/statutory` is masked unless `?unmasked=true` AND `employee:read_sensitive`; unmasked reads are audited.
* Status transitions follow a matrix (`resigned`/`terminated` are terminal); exit (`resigned`/`terminated`) deactivates the M0 Identity user in the same transaction.
* `emergency_contacts` is a JSON array of `{name, relation, phone}`.

## Changelog

- **2026-10-05:** Frozen P1 design bundle (specification FRs, Clean architecture, connections edge table, candidates G1..G10). Kicked off P2 backend implementation.

### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: PII unmask requires employee:read_sensitive (audited), status state machine + exit/confirmation effects, synchronous user-deactivation convergence (FR-EV001), emergency-contact validation, document verify ownership, statutory upsert fix.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.

### 2026-10-21 — records and change requests (HR parity P1)
- DE-R1: Name, address and bank changes from self-service go through approval (FR-CR). The existing
  `PATCH /employees/me` keeps phone and emergency contacts only, so a stolen session cannot redirect pay silently.
- DE-R2: Approval needs a new `employee:approve_changes` permission (default `hr_admin`). Approvers can never
  decide their own request.
- DE-R3: Structured emergency contacts live in their own table (FR-ER001). The legacy JSONB field on the contact
  record remains for compatibility.
- DE-R4: Prior-employment periods are kept non-overlapping by a `btree_gist` exclusion constraint as well as the
  service.
