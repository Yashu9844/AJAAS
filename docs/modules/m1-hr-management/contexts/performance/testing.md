# HR / Performance & OKRs — Testing

> Part of **M1 HR Management** (bounded context `performance`). Formerly the stand-alone "module-7-performance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| G1 | Isolation | foreign ids 404; foreign JWT 403 | integration |
| G2 | Cycle + goal status graphs | every allowed/forbidden transition | lifecycle unit |
| G3 | Score math | KR progress (rising, falling, clamps), goal progress mean, goal score, overall score rounding, 9-box band boundaries 2.49/2.50/3.49/3.50 | score unit |
| G4 | Weight cap | 60+40 ok, +0.01 → WEIGHT_EXCEEDED; cancel frees | service + integration |
| G5 | KR-driven progress + history | KR check-in updates goal progress; direct progress 409; each check-in recorded | service + integration |
| G6 | Access | owner, manager, manage write; read reads; colleague 404 | service + integration |
| G7 | Cycle locks | goal create/edit in review → CYCLE_STATE; check-in in completed → CYCLE_STATE | service |
| G8 | Launch reviews | self + manager created; idempotent; without_manager counted | service + integration |
| G9 | Submit | only reviewer; exact criteria; potential rules; immutable | service + integration |
| G10 | Subject visibility | nothing before completion; peer anonymous after | service + integration |
| G11 | 9-box | bands × potential; unrated listed; goal score attached | service + integration |
| G12 | Privacy | no ratings/scores/text in events/audit | integration |
| G13 | RBAC | member 403 on manage/read routes; self routes work | integration |
| G14 | Pagination | clamped | controllers + integration |
| G15 | Cascading | parent same cycle; depth 4 → GOAL_DEPTH | service |

## Test strategy and commands

RED first. Pure score + lifecycle packages 100% (G2, G3); services ≥ 95% (G4–G11, G15 on fakes + failure injection); controllers/routes ~100%; integration G1, G4–G6, G8–G14; migration cycle + AutoMigrate↔SQL parity.

```bash
go test -count=1 -cover ./internal/performance/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Performance ./tests/integration/
```

## Delivery evidence (status and Definition of Done at backend close)

- P1: design frozen — 22 ops, PM-001..PM-017, 6 tables, 4 events, goldens G1–G15, decisions D7-01..D7-06.
- P2: models (6) 100%; score + lifecycle pure packages 100% (G2, G3 green); migrations 000051–000056 cycle-verified; AutoMigrate↔SQL identical.
