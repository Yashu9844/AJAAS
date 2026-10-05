# Module 12 — Golden Tests

STATUS: candidates defined at P1 design. Rules already in force (MASTER_PROMPT section 15):

Golden behavior / Input / Expected / Why / Location per test.
Protected contracts: never edit a golden test to make red pass.
Change procedure: reason -> spec update -> contract version -> dependent update -> full suite.

Likely candidates: tenant isolation on every endpoint, RBAC allow/deny, audit on state changes, event envelope correctness, no secret leakage in responses.
