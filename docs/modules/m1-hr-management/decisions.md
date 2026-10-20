# M1 HR Management — Decisions

Service-level decisions. Context-level decisions (D1-…, D3-…, D4-…, D5-…, D6-…, D7-…) stay in
`contexts/<context>/decisions.md`. Format: decision — reason — alternatives — trade-off.

## DHR-01 (2026-10-20) HR is one module with bounded contexts

- **Decision:** Organization, Employee, Attendance, Leave, Payroll, Recruitment and Performance are contexts of
  M1 HR Management, not modules 1–7.
- **Reason:** the product IA and the owner's architecture define one HR Management module; the old masterplan's
  numbering mislabelled HR contexts as product modules and pushed real modules (Assets, Approvals, Notifications) out
  of their numbers.
- **Alternatives:** keep old numbering with a mapping table (rejected: two numbering systems keep causing errors).
- **Trade-off:** decision IDs like `D3-17` keep their historical prefixes inside context docs.

## DHR-02 (2026-10-20) One HR deployable first

- **Decision:** one HR service; contexts are packages with their own tables, events and `api` ports.
- **Reason:** `docs/ARCHITECTURE.md` §4 HR boundary note and §11 recommendation; no measured need to split.
- **Alternatives:** seven network services (rejected: latency and operational cost without benefit).
- **Trade-off:** in-process calls are allowed between contexts, but only through `api` ports, so a split stays cheap.

## DHR-03 (2026-10-20) Breaking route change to `/api/v1/hr/...`

- **Decision:** move every HR route under `/api/v1/hr/` in Phase C with no compatibility aliases.
- **Reason:** nothing is in production; the only clients are the test console and integration tests, which move
  in the same change.
- **Alternatives:** keep old paths as aliases for a release (rejected: doubles the surface for no consumer).
- **Trade-off:** the partner's frontend guides for the old paths are retired.

## DHR-04 (2026-10-20) Workday-depth scope

- **Decision:** HR adds five contexts (`core`, `onboarding`, `compensation`, `learning`, `benefits`) and the P1
  parity items in `specification.md` §9 before HR is called complete.
- **Reason:** owner requirement — HR must match an enterprise HCM in quality and completeness; no half-built features.
- **Alternatives:** ship the seven built contexts as "HR v1" (rejected by the owner).
- **Trade-off:** later modules (M2–M12) start after HR completes.

## DHR-05 (2026-10-20) Learning content is links and documents, not hosted media

- **Decision:** courses point to external URLs or documents with a thumbnail reference; assignments, custom plans and
  enrolment are fully supported; no video hosting or SCORM.
- **Reason:** owner direction; keeps storage and streaming out of HR.
- **Trade-off:** no in-app progress tracking inside content; completion is attested.

## DHR-06 (2026-10-20) Single-step approvals stay in-context until M7

- **Decision:** HR business processes use one request shape (`specification.md` HR-013) with single-step approval
  inside the context; when M7 Approvals exists, contexts create M7 instances and react to its decision events
  without changing their HTTP API.
- **Reason:** owner wants HR complete before other modules; M7 is a later phase.
- **Trade-off:** multi-step routing (e.g. manager then HR) waits for M7.

## DHR-07 (2026-10-20) Every feature ships with a test-console page

- **Decision:** a context's feature is not done until its `/dev-test` page and scenarios exist and pass live.
- **Reason:** owner verifies each module by hand without waiting for product UI.
- **Trade-off:** small frontend work per backend phase.

## DHR-08 (2026-10-20) Scoped, hierarchical access

- **Decision:** HR authorization uses the M0 access model (`docs/ARCHITECTURE.md` §17): permission + scope, role
  inheritance, predefined and automatic roles, per-user grants and blocks (blocks win). HR evaluates scopes against
  its reporting lines; every HR endpoint declares its accepted scopes.
- **Reason:** owner requirement — CEO / HR / manager / employee access like enterprise HR systems, with predefined
  roles and per-person exceptions.
- **Trade-off:** HR routes keep requiring tenant scope until they adopt scoped checks in Phase C.

## DHR-09 (2026-10-20) Re-baseline migrations at the schema split

- **Decision:** at Phase C step 7 the `identity` and `hr` schemas start from fresh migration sets; dev and test
  databases are recreated. Owner approved.
- **Reason:** nothing runs in production; data-moving migrations would be effort without a consumer.
