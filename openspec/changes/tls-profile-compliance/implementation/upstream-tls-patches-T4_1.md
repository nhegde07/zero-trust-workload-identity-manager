# Upstream TLS Patch Coordination — T4_1

**Change:** tls-profile-compliance  
**Status:** NOT STARTED (operator Layer 2 injection complete; upstream read support pending)  
**Owner:** SPIRE / ZTWIM operand maintainers (out of tree)

## Purpose

Document the cross-repo work required so operand processes **read and apply** TLS settings the ZTWIM operator already injects into ConfigMaps (Phase 3). No SPIRE logic belongs in the operator repository (constitution Principle VI).

## Operator → operand config contract (implemented Phase 3)

When `APIServer.spec.tlsAdherence=StrictAllComponents` and `requirePQKEM` is absent, the operator injects these fields:

| Operand | Config source | Injected keys |
|---------|---------------|---------------|
| SPIRE Server | `spire-server` ConfigMap / `server.conf` JSON root | `minTLSVersion`, `cipherSuites` |
| SPIRE Agent | `spire-agent` ConfigMap / agent JSON root | `minTLSVersion`, `cipherSuites` |
| OIDC Discovery Provider | `oidc-discovery-provider.conf` JSON root | `minTLSVersion`, `cipherSuites` |
| SPIRE Controller Manager | `spire-controller-manager` ConfigMap YAML | `minTLSVersion`, `cipherSuites` |

When unset (non-strict adherence or no injection), operands must retain current behavior: TLS 1.2 minimum and Go default ciphers.

Phase 5 will add `experimental.require_pq_kem` at the same config roots; upstream must honor that via existing SPIRE `tlspolicy` (OpenShift fork already documents `require_pq_kem` on server).

## Target fork repositories

| Repo | URL | Role |
|------|-----|------|
| OpenShift SPIRE fork | https://github.com/openshift/spiffe-spire | Server, agent, OIDC, Prometheus TLS |
| OpenShift ctrl-mgr fork | https://github.com/openshift/spiffe-spire-controller-manager | Webhook TLS `:9443` |
| Upstream reference | https://github.com/spiffe/spire | Upstream PR target |
| Upstream ctrl-mgr | https://github.com/spiffe/spire-controller-manager | Upstream PR target |

**Baseline audit (2026-07-03):** `openshift/spiffe-spire` `main` still hardcodes `MinVersion = tls.VersionTLS12` in `pkg/server/endpoints/endpoints.go` (`getTLSConfig`). Configurable profile fields are **not yet consumed** at runtime.

## Patch backlog (ADR upstream table)

| # | Component | Endpoint | Upstream file | Change |
|---|-----------|----------|---------------|--------|
| 1 | SPIRE Server | gRPC TCP API | `pkg/server/endpoints/endpoints.go` | Parse root `minTLSVersion` / `cipherSuites` from server config; apply in `getTLSConfig` instead of hardcoded TLS 1.2 |
| 2 | SPIRE Server | Federation bundle HTTPS | `pkg/server/endpoints/bundle/server.go` | Read min TLS from config |
| 3 | SPIRE Server | Prometheus HTTPS | `pkg/common/telemetry/prometheus.go` | Add MinTLSVersion/CipherSuites to telemetry config; use in `GetTLSConfig()` |
| 4 | SPIRE Agent | Agent→server mTLS | agent config loader | Read root `minTLSVersion` / `cipherSuites` (operator injects into agent JSON) |
| 5 | OIDC Discovery Provider | `:8443` serving / ACME | `support/oidc-discovery-provider/main.go` | Read `minTLSVersion`, `cipherSuites`, `experimental.require_pq_kem` from injected JSON in `newListenerWithServingCert()` / `newACMEListener()` |
| 6 | Controller Manager | Admission webhook | `cmd/main.go` (or config loader) | Read `minTLSVersion` / `cipherSuites` from operator-injected YAML for webhook `TLSOpts` |

## Verification checklist (fork repos)

- [ ] Unit tests: config unset → TLS 1.2 + default ciphers (backward compatible)
- [ ] Unit tests: Intermediate profile fields applied on each endpoint class
- [ ] Unit tests: Modern profile → TLS 1.3 min, ciphers omitted (Go TLS 1.3 behavior)
- [ ] Integration: deploy operator-injected ConfigMaps; `openssl s_client` / tls-scanner on server `:8081`, federation `:8443`, metrics `:8082`, OIDC `:8443`, ctrl-mgr webhook `:9443`
- [ ] `require_pq_kem` path (Phase 5): SPIRE tlspolicy enforces TLS 1.3 + X25519MLKEM768

## Coordination steps

1. Open tracking issues on `openshift/spiffe-spire` and `openshift/spiffe-spire-controller-manager` linking to this change and ADR-TLS-Compliance.md.
2. Implement patches 1–5 in SPIRE fork; patch 6 in ctrl-mgr fork.
3. Add fork unit tests per checklist.
4. Open upstream PRs to `spiffe/spire` and `spiffe/spire-controller-manager` (optional follow-up).
5. Hand off image digests to **T4_2** (`zero-trust-workload-identity-manager-release` / `images_digest.conf`).

## Blocks

| Downstream | Blocked until |
|------------|---------------|
| T4_2 release image bump | Fork patches merged + CI green |
| T6_1 e2e live handshakes | Patched operand images on test cluster |
| SC-002 operand tls-scanner | Same |

## References

- ADR-TLS-Compliance.md — upstream changes table
- `openspec/changes/tls-profile-compliance/plan.md` § Phase 4
- Operator injection: `pkg/controller/spire-server/configmap.go`, `pkg/controller/spire-agent/configmap.go`, `pkg/controller/spire-oidc-discovery-provider/configmaps.go`
