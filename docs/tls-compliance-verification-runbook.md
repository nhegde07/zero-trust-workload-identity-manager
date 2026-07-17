# TLS Compliance Verification Runbook

Manual verification guide for OCPSTRAT-2611 (cluster-wide TLS profile compliance + hybrid PQC) on Zero Trust Workload Identity Manager (ZTWIM).

Use this runbook after Phases 1–5 are deployed on a live OpenShift cluster with the updated operator bundle installed.

## Prerequisites

- Cluster admin access (`oc` logged in)
- ZTWIM operator installed from the refreshed OLM bundle (Phase 5)
- `tls-scanner` or equivalent TLS probe tool available locally or in CI
- Upstream SPIRE fork patches applied per [upstream-spire-tls-alignment.md](./upstream-spire-tls-alignment.md) before expecting operand endpoints to honor injected TLS settings

## FR-011 endpoint inventory

The operator codifies the six covered TLS server endpoints in `pkg/controller/tls/endpoints.go` (`CoveredTLSEndpoints()`):

| ID | Endpoint | Port | Alternate ports | Workload |
|----|----------|------|-----------------|----------|
| `operator-metrics` | Operator metrics HTTPS | 8443 | — | `controller-manager` Deployment |
| `spire-server-grpc` | SPIRE server registration API | 8081 | — | `spire-server` StatefulSet |
| `spire-federation-https` | SPIRE federation bundle HTTPS | 8443 | — | `spire-server` StatefulSet |
| `spire-server-metrics` | SPIRE server metrics HTTPS | 8082 | 9402 | `spire-server` / ctrl-mgr sidecar |
| `oidc-discovery-https` | OIDC discovery provider HTTPS | 8443 | — | `spire-oidc-discovery-provider` Deployment |
| `spire-controller-manager-webhook` | SPIRE controller-manager admission webhook | 9443 | — | `spire-controller-manager` container |

### Operator admission webhook (FR-001, not one of the six FR-011 endpoints)

The ZTWIM operator manager uses controller-runtime's default webhook server on port **9443** (`webhook.DefaultPort`). The Deployment manifest exposes metrics on `:8443` but does not declare port 9443; scan via port-forward to the operator pod:

```bash
OPERATOR_NS="${OPERATOR_NS:-openshift-zero-trust-workload-identity-manager}"
POD=$(oc get pod -n "${OPERATOR_NS}" -l name=zero-trust-workload-identity-manager -o jsonpath='{.items[0].metadata.name}')
oc port-forward -n "${OPERATOR_NS}" "pod/${POD}" 9443:9443
# In another terminal:
tls-scanner --host 127.0.0.1 --port 9443
```

## Pre-checks

Record cluster TLS profile and ZTWIM configuration before scanning:

```bash
oc get apiserver cluster -o yaml | yq '.spec.tlsSecurityProfile'
oc get zerotrustworkloadidentitymanager cluster -o yaml
```

Note `tlsSecurityProfile.type` (Old, Intermediate, Modern, Custom), `custom` cipher/min-TLS settings if applicable, and whether `spec.tlsSecurityProfile.intermediate.profile` strict adherence is active.

For PQC testing, confirm `spec.requirePQKEM` on the ZTWIM CR:

```bash
oc get zerotrustworkloadidentitymanager cluster -o jsonpath='{.spec.requirePQKEM}{"\n"}'
```

## Port-forward helper

Most operand endpoints are cluster-internal. Use port-forward through the operand pod or Service:

```bash
# Example: SPIRE server gRPC TLS (8081)
SPIRE_NS="${SPIRE_NS:-spire}"
POD=$(oc get pod -n "${SPIRE_NS}" -l app=spire-server -o jsonpath='{.items[0].metadata.name}')
oc port-forward -n "${SPIRE_NS}" "pod/${POD}" 8081:8081
```

Repeat for each endpoint port from the inventory table, substituting namespace, pod label, and port as appropriate.

## tls-scanner command template

```bash
tls-scanner --host 127.0.0.1 --port <PORT> [--min-tls-version <VER>] [--cipher-suites <LIST>]
```

Adjust flags to match your `tls-scanner` build. Record negotiated TLS version and cipher suite for each endpoint.

## Verification matrix

### SC-001 — Strict adherence profile compliance

**When:** Strict TLS adherence active; profile type is Old, Intermediate, Modern, or Custom.

**Steps:**

1. Apply or confirm the desired APIServer TLS profile and strict adherence.
2. Wait for operator restart and operand rolling updates to complete (typically within 30 minutes).
3. Scan all six FR-011 endpoints (both `:8082` and `:9402` for server metrics).
4. Confirm negotiated TLS version and cipher suites match the active profile constraints on every endpoint.

### SC-002 — Non-strict Intermediate default continuity

**When:** Default Intermediate profile with non-strict adherence.

**Steps:**

1. Upgrade operator to the TLS-compliance build without changing APIServer profile.
2. Confirm existing SPIRE agent-to-server and workload mTLS connections succeed without manual intervention.
3. Optional: run SPIRE health checks and sample workload attestation.

### SC-003 — Profile change propagation

**When:** Administrator changes APIServer TLS profile or adherence policy.

**Steps:**

1. Change `cluster` APIServer `tlsSecurityProfile` (or adherence) via platform config.
2. Observe operator pod restart (SecurityProfileWatcher) and operand rolling updates.
3. Re-scan all six endpoints; confirm new effective TLS settings without manual pod deletion.

### SC-004 — PQC handshake behavior

**When:** `spec.requirePQKEM: true` on ZTWIM CR.

**Steps:**

1. Enable `requirePQKEM` and wait for operand rollout.
2. Probe SPIRE mTLS endpoints (server gRPC `:8081`, agent connections) for X25519MLKEM768 negotiation.
3. Confirm non-PQC clients fail handshake where expected.

### SC-005 — Custom profile cipher exclusion

**When:** Custom APIServer profile disables specific cipher suites.

**Steps:**

1. Configure custom profile with excluded ciphers.
2. Scan all six endpoints; confirm excluded suites are not offered.

### SC-006 — PQC + strict adherence warning event

**When:** `requirePQKEM: true` concurrently with strict TLS adherence.

**Steps:**

1. Enable both settings on ZTWIM.
2. Check operator events on the ZTWIM CR:

```bash
oc get events --field-selector involvedObject.name=cluster -n "${OPERATOR_NS}" | grep PQKESOverridesCentralTLSProfile
```

3. Confirm a Warning event documents that PQC policy overrides central profile injection.

### SC-007 — Upgrade readiness under strict adherence

**When:** Cluster has strict TLS adherence enabled.

**Steps:**

1. Run OpenShift upgrade readiness checks (e.g., `oc adm upgrade`).
2. Confirm ZTWIM/SPIRE TLS configuration does not block the upgrade path.
3. Document any platform warnings related to operand TLS.

## Profile test matrix (summary)

| Profile | Strict adherence | requirePQKEM | Endpoints to scan | Success criteria |
|---------|------------------|--------------|-------------------|------------------|
| Intermediate | No | false | All 6 + operator webhook | SC-002 continuity |
| Intermediate | Yes | false | All 6 + operator webhook | SC-001 compliance |
| Modern | Yes | false | All 6 + operator webhook | SC-001 compliance |
| Old | Yes | false | All 6 + operator webhook | SC-001 compliance |
| Custom | Yes | false | All 6 + operator webhook | SC-001, SC-005 |
| Any | Yes | true | All 6 + mTLS paths | SC-004, SC-006 |

## Troubleshooting

- **Operand scans fail after operator upgrade:** Verify upstream SPIRE fork alignment per [upstream-spire-tls-alignment.md](./upstream-spire-tls-alignment.md).
- **Operator metrics scan fails:** Confirm `METRICS_SECURE=true` and port `:8443` on the controller-manager Deployment.
- **Server metrics ambiguous port:** Scan both `:8082` (controller-manager sidecar) and `:9402` (server.conf telemetry); document which endpoint maps to your monitoring path.
- **CREATE_ONLY_MODE:** TLS profile changes may not propagate; check ZTWIM conditions for create-only limitation.

## References

- FR-011 endpoint constants: `pkg/controller/tls/endpoints.go`
- Operator metrics default: `cmd/zero-trust-workload-identity-manager/main.go` (`--metrics-bind-address :8443`)
- Webhook default port: `sigs.k8s.io/controller-runtime/pkg/webhook` (`DefaultPort = 9443`)
