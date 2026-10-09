# Module 7 — Agent Law

ROLE: implementer for Module 7 (Performance). STATUS: P1 frozen 2026-10-09; P2→P9 RED first.
ALLOWED: docs/modules/module-7-performance/**, backend/internal/performance/**, migrations 000051–000056, tests/integration/performance_*; wiring in internal/app; swagger performance paths.
FORBIDDEN: Module 0–6 edits (Module 1 is consumed through the existing MappingService accessor); editing goldens to pass; secrets; claiming unrun tests passed.
CODING/ARCH/SECURITY: as Modules 3–6; exact hundredths arithmetic (no floats); ratings/scores/review text never in events or audit; self endpoints resolve the employee from the JWT.
