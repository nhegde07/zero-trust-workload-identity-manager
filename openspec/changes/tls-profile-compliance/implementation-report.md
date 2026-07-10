# Implementation Report — tls-profile-compliance

**Completed:** 2026-07-03  
**Branch:** shiftweek-420 (working-folder mode)  
**Tasks:** 26 executed / 27 in linear order (T4_2 removed)

## Executive summary

Implemented OpenShift central TLS profile compliance and hybrid PQC opt-in for the Zero Trust Workload Identity Manager operator:

- **Layer 1:** Operator metrics/webhook honor cluster TLS profile via controller-runtime-common; SecurityProfileWatcher restarts operator on profile/adherence change.
- **Layer 2:** Injects `minTLSVersion` / `cipherSuites` into SPIRE operand ConfigMaps when `tlsAdherence=StrictAllComponents` and `requirePQKEM` is not enabled.
- **PQC:** Optional `spec.requirePQKEM` on ZTWIM CR injects `experimental.require_pq_kem` into all operand configs and suppresses central profile injection; Warning events when combined with strict adherence.
- **Upstream:** Coordination doc for SPIRE fork patches (T4_1); release image bump (T4_2) removed from scope.
- **Verification:** Unit tests, ConfigMap-level e2e, release qualification runbook.

## Task reports

| Phase | Tasks | Reports |
|-------|-------|---------|
| 1 | T1_1–T1_6 | [implementation/task-reports/](implementation/task-reports/) |
| 2 | T2_1–T2_4 | same |
| 3 | T3_1–T3_8 | same |
| 4 | T4_1 | [T4_1.md](implementation/task-reports/T4_1.md), [upstream-tls-patches-T4_1.md](implementation/upstream-tls-patches-T4_1.md) |
| 5 | T5_1–T5_5, T5_7 | T5_*.md |
| 6 | T6_1–T6_2 | T6_*.md, [release-qualification-runbook.md](implementation/release-qualification-runbook.md) |

## Key code paths

- `cmd/zero-trust-workload-identity-manager/main.go` — Layer 1 TLS + watcher
- `pkg/controller/tls/tls.go` — precedence, injection helpers
- `pkg/controller/spire-{server,agent}/`, `spire-oidc-discovery-provider/` — ConfigMap injection + reconcile
- `api/v1alpha1/zero_trust_workload_identity_manager_types.go` — `RequirePQKEM`
- `test/e2e/e2e_test.go` — TLS profile compliance context

## Known gaps

1. Upstream SPIRE must read injected TLS fields for live handshakes (FR-011 partial).
2. `make verify` corrupts XValidation quotes in spire_*_types.go — avoid or revert after run.
3. Pre-existing unit test failures in `pkg/controller/spire-server` controller tests unrelated to this change.

## Next steps (out of repo)

- Implement upstream patches per `upstream-tls-patches-T4_1.md`
- Release engineering image bump when forks merge
- Run full tls-scanner matrix per `release-qualification-runbook.md`
