# Module 4 — Architecture

STATUS: DESIGN PENDING (plan.md P1).

Constraints for the future design:
- Mirror Module 0 Clean layers: controllers (thin) -> services (logic, DTOs) -> repositories (data).
- Backend: backend/internal/projects/. Frontend: frontend/src/modules/projects/.
- Consume Module 0 Tenant/User/RBAC contracts only. Never duplicate identity logic.
- Document data flow, control flow, failure paths, caching, persistence, concurrency + WHY (MASTER_PROMPT section 10).
