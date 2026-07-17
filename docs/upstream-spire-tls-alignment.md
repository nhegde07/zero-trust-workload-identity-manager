# Upstream SPIRE TLS Alignment Checklist

Release coordination checklist for operand TLS compliance (FR-011). The ZTWIM operator injects TLS and PQC settings into operand ConfigMaps, but **SPIRE binaries must read and enforce those settings** in the forked OpenShift images.

This document is in-repo guidance only. Fork patches and submodule bumps happen in external repositories.

## Scope

| Repository | Role |
|------------|------|
| `openshift/spiffe-spire` | SPIRE server and agent binaries; HCL config for gRPC, federation HTTPS, metrics, PQC |
| `openshift/spiffe-spire-controller-manager` | Admission webhook TLS configuration |
| `openshift/zero-trust-workload-identity-manager-release` | Submodule digests and release image builds |

## Operator-injected configuration keys

The operator (Phases 3–4) writes the following into operand ConfigMaps when strict adherence or PQC is active:

### SPIRE server (`pkg/controller/spire-server/configmap.go`)

- Minimum TLS version and cipher suites from cluster APIServer profile (strict adherence)
- Federation bundle endpoint TLS settings
- `experimental.require_pq_kem: true` when `spec.requirePQKEM` is enabled

### SPIRE agent (`pkg/controller/spire-agent/configmap.go`)

- Agent-to-server mTLS TLS settings from cluster profile (strict adherence)
- `experimental.require_pq_kem: true` when PQC enabled

### OIDC discovery provider (`pkg/controller/spire-oidc-discovery-provider/configmaps.go`)

- HTTPS listener TLS settings from cluster profile (strict adherence)

### SPIRE controller-manager (server StatefulSet sidecar config)

- Webhook TLS min version / cipher suites from cluster profile (strict adherence)
- **PQC fallback (Assumption A-004):** If upstream webhook PQC patch is unavailable at ship, webhook follows central profile injection only — no PQC override on webhook until fork support lands

## Pre-scan gate checklist

Complete before running [tls-compliance-verification-runbook.md](./tls-compliance-verification-runbook.md) against operand endpoints:

- [ ] **Fork patch merged** — `openshift/spiffe-spire` reads injected min TLS, cipher suites, and `experimental.require_pq_kem` from HCL/JSON config
- [ ] **Controller-manager patch merged** — `openshift/spiffe-spire-controller-manager` applies webhook TLS settings from config (TLSMINVERSION / TLSCIPHERSUITES or equivalent)
- [ ] **Release repo bump** — `openshift/zero-trust-workload-identity-manager-release` submodule digests updated to patched SPIRE builds
- [ ] **Operator RELATED_IMAGE_* env** — Release pipeline points operand images to patched digests (or cluster pulls updated images via CSV relatedImages)
- [ ] **Operand rollout** — SpireServer, SpireAgent, OIDC provider pods restarted with new images and ConfigMaps
- [ ] **Config verification** — `oc get configmap -n <spire-ns> -o yaml` shows injected TLS/PQC blocks matching cluster profile and ZTWIM spec

## Validation sequence

1. Deploy operator bundle from Phase 5 (in-repo).
2. Complete this checklist in the release repo.
3. Install or upgrade ZTWIM on a test cluster with desired APIServer TLS profile.
4. Execute the manual runbook (SC-001 through SC-007).
5. Record tls-scanner results per endpoint in release notes or CI artifact store.

## Known limitations

- **Operator-only scans can pass before operand scans:** Operator metrics (`:8443`) and operator webhook (`:9443`) use controller-runtime-common TLS profile resolution at startup. Operand endpoints depend on fork support.
- **Server metrics dual ports:** Scan both `:8082` (sidecar) and `:9402` (telemetry) until monitoring ownership is confirmed.
- **ML-DSA signatures (A-006):** Out of scope for this change.

## SME contacts

- SPIRE fork patch status: Release-repo / SPIRE upstream SME
- Submodule digest promotion: ZTWIM release engineering
- Manual tls-scanner execution: Cluster admin / OCP CI owner

## Related documents

- [tls-compliance-verification-runbook.md](./tls-compliance-verification-runbook.md) — SC-001 through SC-007 manual steps
- `openspec/changes/tls-compliance/plan.md` — Phase 6 goals and verification hooks
- `openspec/changes/tls-compliance/repo-assessment.md` — Endpoint port evidence
