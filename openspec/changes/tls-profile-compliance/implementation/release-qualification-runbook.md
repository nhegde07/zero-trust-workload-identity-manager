# TLS Profile Compliance — Release Qualification Runbook

**Change:** tls-profile-compliance  
**Related:** ADR-TLS-Compliance.md, OCPSTRAT-2611, OCPSTRAT-3145

## Scope

This runbook covers manual qualification for operator Layer 1 TLS, Layer 2 ConfigMap injection, and optional `requirePQKEM` (hybrid PQC). Live operand TLS handshakes require upstream SPIRE patches documented in `upstream-tls-patches-T4_1.md`.

## Prerequisites

- OpenShift cluster with `APIServer` TLS profile configured
- Operator installed from this branch (CSV `features.operators.openshift.io/tls-profiles: "true"`)
- `oc`, `openssl`, optional tls-scanner tooling
- All SPIRE operands on Go 1.24+ before enabling `requirePQKEM`

## Phase A — Operator Layer 1 (FR-001, SC-001)

1. Record cluster profile: `oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile.type}{"\n"}{.spec.tlsAdherence}{"\n"}'`
2. Port-forward operator metrics: `oc port-forward -n zero-trust-workload-identity-manager svc/zero-trust-workload-identity-manager-metrics-service 8443:8443`
3. Probe TLS: `openssl s_client -connect localhost:8443 -servername metrics 2>/dev/null | openssl x509 -noout -text | grep -E 'Protocol|Cipher'`
4. Change APIServer profile (test cluster only); confirm operator pod restarts (FR-002) and handshake reflects new profile.

## Phase B — Layer 2 ConfigMap injection (FR-003, FR-004, SC-002 partial)

**StrictAllComponents + requirePQKEM absent/false:**

1. Set `spec.tlsAdherence: StrictAllComponents` on APIServer (if not already).
2. Confirm operand ConfigMaps contain `minTLSVersion` (and `cipherSuites` for TLS 1.2 profiles):
   - `oc get cm spire-server -n zero-trust-workload-identity-manager -o jsonpath='{.data.server\.conf}' | jq '.minTLSVersion'`
   - Repeat for `spire-agent`, `spire-spiffe-oidc-discovery-provider`, `spire-controller-manager`.

**Non-strict adherence (FR-014):**

1. Set `spec.tlsAdherence: LegacyAdheringComponentsOnly` or omit.
2. Confirm operand ConfigMaps **lack** injected `minTLSVersion` / `cipherSuites`.

## Phase C — requirePQKEM (FR-005–FR-008, SC-004 partial)

1. Patch ZTWIM: `oc patch zerotrustworkloadidentitymanager cluster --type=merge -p '{"spec":{"requirePQKEM":true}}'`
2. Verify ConfigMaps contain `"experimental": {"require_pq_kem": true}` and **no** central profile fields.
3. If `tlsAdherence=StrictAllComponents`, check Warning events on SpireServer/SpireAgent:
   - `oc get events -n zero-trust-workload-identity-manager --field-selector reason=PQOverridesCentralTLSProfile`
4. **Live PQC handshake** (blocked until upstream operand patches): use tls-scanner or `openssl s_client` to confirm X25519MLKEM768 when patches are available.

## Phase D — tls-scanner matrix (SC-007, SC-008)

| Profile   | Operator metrics | Operand gRPC (post-upstream) | requirePQKEM |
|-----------|------------------|------------------------------|--------------|
| Old       | Scan             | Scan                         | N/A          |
| Intermediate | Scan          | Scan                         | N/A          |
| Modern    | Scan             | Scan                         | N/A          |
| Custom    | Scan             | Scan                         | N/A          |
| requirePQKEM=true | Go defaults | TLS 1.3 + PQ only      | Scan         |

**Upgrade ordering (A-004):** upgrade all SPIRE agents to Go 1.24+ before enabling `requirePQKEM`; then server; validate workloads last.

## Automated coverage

- Unit tests: `pkg/controller/tls`, operand ConfigMap generators
- E2E (label `tls`): `requirePQKEM` propagation to server/agent ConfigMaps — run `make test-e2e` on qualifying cluster

## Out of scope (removed tasks)

- T4_2 release-repo image bump — tracked separately by release engineering
- Full operand live handshake e2e — blocked on upstream fork merges
