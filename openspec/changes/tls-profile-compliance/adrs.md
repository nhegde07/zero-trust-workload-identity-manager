# Architectural Decision Records (Deviations)

**Change**: tls-profile-compliance  
**Related**: ADR-TLS-Compliance.md, OCPSTRAT-2611, OCPSTRAT-3145

---

## Recorded deviations

| Task | Decision | Rationale |
|------|----------|-----------|
| T2_1 | Intermediate fallback when APIServer fetch fails at operator startup | FR-014 — default cluster behavior unchanged |
| T2_4 | Scoped verification; full `make test` skipped | Pre-existing unrelated test failures; `go fmt` corrupts API XValidation quotes |
| T3_* | `ResolveOperandTLSInjection` returns empty injection when k8s client is nil | Unit test safety without fake APIServer in every controller test |
| T4_1 | Coordination documentation only; no fork patches in operator repo | Constitution Principle VI — SPIRE logic out of tree |

---

## Phase 4 note

Upstream patch implementation tracked in `implementation/upstream-tls-patches-T4_1.md`. Baseline audit (2026-07-03): `openshift/spiffe-spire` still hardcodes TLS 1.2 in `pkg/server/endpoints/endpoints.go`.
