# Module 7 — Golden Tests (FROZEN at P1 2026-10-09)

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
