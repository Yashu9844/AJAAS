# Module 3 — Handoff (2026-10-09, G3-5 DONE → G3-6 CURRENT)

Completed: P2 models+migrations; P3 calc (G4/G5), DTOs (G13), validators; P4 repositories; P5 services + events + outbox relay with unit goldens G2, G3, G6, G7, G8, G10, G11, G12.
Not completed: P6 HTTP/wiring/swagger, P7 live ring, P8 frontend, P9 close.
Known issues: Docker Desktop daemon not reachable on this machine yet (P7 dependency); Module 4 re-scope pending (D3-01).
Files changed (P5): backend/internal/attendance/{events,services}/**.
Tests executed: `go test -cover ./internal/attendance/...` — calc 100, dto 100, events 100, models 100, validators 100, services 91.8 (PASS). `-race` not run (no cgo on this box).
Remaining risks: repositories exercised only by fakes until P7.
Required follow-up (G3-6): (1) controllers + error/validation mapping, (2) routes + module.go + permission seed + relay, (3) main.go wiring + swagger, then router test.
## Phase: P5 → DONE, P6 → CURRENT, P7 → NEXT.
