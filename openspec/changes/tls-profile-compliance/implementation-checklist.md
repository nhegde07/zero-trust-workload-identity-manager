# Traceability Checklist: tls-profile-compliance

**Purpose:** Map implementation files to specs.md requirement IDs  
**Created:** 2026-07-03  
**Feature:** [specs.md](specs.md)

## Operator foundation

| File | Requirement IDs | Reason |
|------|-----------------|--------|
| `go.mod`, `vendor/`, `tools/tools.go` | FR-013 | crt-common dependency for TLS profile APIs |
| `cmd/.../main.go` | FR-001, FR-002, FR-014 | Layer 1 metrics/webhook TLS; operator restart on profile change |
| `config/rbac/role.yaml` | FR-013 | APIServer read RBAC |
| `pkg/client/client.go` | FR-013 | APIServer cache for reconcile-time reads |
| `pkg/controller/tls/tls.go` | FR-003, FR-004, FR-007, FR-014 | Shared precedence and injection logic |
| `pkg/controller/tls/tls_test.go` | FR-004, FR-014, FR-005–FR-008 | Adherence matrix and PQ precedence tests |

## Layer 2 operand injection

| File | Requirement IDs | Reason |
|------|-----------------|--------|
| `pkg/controller/spire-server/configmap.go` | FR-003, FR-006, FR-007, FR-010 | Server + ctrl-mgr ConfigMap TLS/PQ fields |
| `pkg/controller/spire-agent/configmap.go` | FR-003, FR-006, FR-007, FR-010 | Agent mTLS config injection |
| `pkg/controller/spire-oidc-discovery-provider/configmaps.go` | FR-003, FR-006, FR-007, FR-010 | OIDC listener config injection |
| `pkg/controller/spire-*/controller.go` | FR-009, FR-013 | Resolve injection; warning events (FR-008) |
| `*_test.go` (configmap) | FR-003, FR-004, FR-009, FR-006, FR-007 | ConfigMap content and hash tests |

## API and OLM

| File | Requirement IDs | Reason |
|------|-----------------|--------|
| `api/v1alpha1/zero_trust_workload_identity_manager_types.go` | FR-005 | `RequirePQKEM` cluster-wide flag |
| `config/crd/bases/*`, `bundle/` | FR-005, FR-012 | CRD + CSV tls-profiles annotation |
| `pkg/controller/utils/utils.go` | FR-005 | ZTWIM spec change triggers operand reconcile |

## Documentation and verification

| File | Requirement IDs | Reason |
|------|-----------------|--------|
| `implementation/upstream-tls-patches-T4_1.md` | FR-010, FR-011 | Upstream patch backlog and config contract |
| `implementation/release-qualification-runbook.md` | SC-007, SC-008 | Release qualification matrix |
| `test/e2e/e2e_test.go` | SC-001–SC-004 (partial) | ConfigMap-level TLS/PQ e2e |

## Notes

- **FR-011 live handshakes:** ConfigMap contract complete; operand binaries need upstream patches.
- **T4_2 / SC-002 full operand scans:** Out of scope (task removed).
- **SC-007 CI tls-scanner:** Documented manual gate in runbook; automation optional follow-up.
