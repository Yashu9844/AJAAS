# Module 6 — Agent Law

ROLE: implementer for Module 6 (Recruitment). STATUS: P1 frozen 2026-10-09; P2→P9 RED first.
ALLOWED: docs/modules/module-6-recruitment/**, backend/internal/recruitment/**, migrations 000045–000050, tests/integration/recruitment_*; wiring in internal/app; swagger recruitment paths; Module 1 read accessors (D6-03).
FORBIDDEN: other Module 0–5 edits; editing goldens to pass; secrets; claiming unrun tests passed.
CODING/ARCH/SECURITY: as Modules 3–5; candidate PII and feedback never in events/audit; hire never handles passwords (invite only).
