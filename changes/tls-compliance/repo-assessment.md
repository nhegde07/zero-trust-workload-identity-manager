# Repository Assessment Report

**Feature:** Cluster-Wide TLS Profile Compliance and Hybrid Post-Quantum Key Exchange (OCPSTRAT-2611)

## 0. Inputs & Tooling

- **repo:** `https://github.com/nhegde07/zero-trust-workload-identity-manager` (working-folder mode; upstream: `openshift/zero-trust-workload-identity-manager`)
- **branch:** `OAPE-859`
- **commit:** `2a5a19ef3c2047d20039cf2359da5eafdbe4d2d1`
- **tooling_status:** OK
- **Spec status:** `specs.md` approved; `validation.json` PASS (88%)
- **Feature readiness on branch:** **GREENFIELD** — no central TLS profile integration, no `requirePQKEM` CRD field, no APIServer RBAC, no `controller-runtime-common/pkg/tls` dependency. CSV declares `features.operators.openshift.io/tls-profiles: "false"`.

## 1. Architecture Overview

### 1.1 Project Type & Tech Stack

- **Type:** OpenShift OLM operator (controller-runtime, kubebuilder markers) managing upstream SPIFFE/SPIRE operands via bindata + imperative reconcile.
- **Language:** Go 1.25.7 (`go.mod`)
- **Framework:** `sigs.k8s.io/controller-runtime` v0.22.4; five reconcilers in one binary.
- **OpenShift deps:** `github.com/openshift/api` (Route, SCC — **not** `config/v1` APIServer today), OLM OperatorCondition.
- **Upstream:** `github.com/spiffe/spire-controller-manager` v0.6.4 (API types only; operand images from RELATED_IMAGE env).
- **Build:** GNU Make + openshift/build-machinery-go (bindata, codegen); FIPS via `hack/go-fips.sh`.

### 1.2 Component Map

| Package | Responsibility | Hand-written |
|---------|----------------|--------------|
| `cmd/zero-trust-workload-identity-manager/main.go` | Operator entrypoint, manager bootstrap, metrics/webhook TLS setup | Yes |
| `api/v1alpha1/` | ZTWIM + operand CRD types, conditions | Yes (+ `zz_generated.deepcopy.go`) |
| `pkg/controller/zero-trust-workload-identity-manager/` | Status aggregation, OLM Upgradeable sync | Yes |
| `pkg/controller/spire-server/` | SPIRE server StatefulSet, ConfigMaps, webhook, routes, federation | Yes |
| `pkg/controller/spire-agent/` | SPIRE agent DaemonSet, ConfigMap | Yes |
| `pkg/controller/spire-oidc-discovery-provider/` | OIDC Deployment, ConfigMap, Route | Yes |
| `pkg/controller/spiffe-csi-driver/` | CSI DaemonSet (no TLS server endpoints for this feature) | Yes |
| `pkg/controller/status/` | Shared condition manager | Yes |
| `pkg/controller/utils/` | Predicates, constants, config hash, resource comparison | Yes |
| `pkg/client/` | CustomCtrlClient + cache builder | Yes (+ counterfeiter fakes) |
| `bindata/` + `pkg/operator/assets/bindata.go` | Static operand YAML | YAML hand-written; bindata generated |
| `config/` | Kustomize CRDs, RBAC, manager, CSV base | Mixed (CRD bases generated) |
| `bundle/` | OLM bundle output | Generated |
| `test/e2e/` | Ginkgo e2e against live cluster | Yes |

**Out-of-repo (release repo submodules):** SPIRE server/agent/OIDC/controller-manager container images built from `openshift/zero-trust-workload-identity-manager-release` forks — required for operand-side TLS configurability (FR-015).

### 1.3 Framework & Pattern Architecture

- **Single framework:** controller-runtime only (not library-go).
- **Bootstrap** (`main.go`): parse flags → validate `OPERATOR_NAMESPACE` → configure metrics server (`:8443`, secure by default) and webhook server with optional HTTP/2 disable → register schemes (core, ZTWIM, SCC, Route, spire-controller-manager, OLM) → `NewCacheBuilder` → `ctrl.NewManager` → register 5 controllers → `mgr.Start`.
- **Operand pattern:** Each reconciler Get CR → `status.NewManager` + defer ApplyStatus → Get ZTWIM parent → create-only check → validate → ordered reconcile steps → config hash on pod template annotations triggers rolling update.
- **Config propagation:** SPIRE HCL/JSON configs generated in Go (`generateServerConfMap`, agent/OIDC generators) → ConfigMap → SHA256 hash → annotation on StatefulSet/DaemonSet/Deployment pod template → `needsUpdate` detects hash drift.

### 1.4 Runtime Data/Control Flow (TLS-relevant)

1. Cluster admin sets APIServer TLS profile / adherence (platform) — **not watched today**.
2. Admin sets `ZeroTrustWorkloadIdentityManager` spec (future: `requirePQKEM`) — ZTWIM spec change triggers operand reconciles via `ZTWIMSpecChangedPredicate`.
3. Operand reconciler reads ZTWIM + operand CR → generates ConfigMap with SPIRE config → updates hash annotation → workload rolls.
4. Operator metrics/webhook TLS configured once at startup in `main.go` — **no cluster profile applied today**; restart required for profile change (future: SecurityProfileWatcher cancels context → process exit).

**TLS-serving endpoints (evidence from code):**

| Endpoint | Port | Location |
|----------|------|----------|
| Operator metrics HTTPS | 8443 | `main.go` `--metrics-bind-address` default |
| Operator webhook | (manager default) | `webhook.NewServer` in `main.go` |
| SPIRE server gRPC | 8081 | `generateServerConfMap` `bind_port`; StatefulSet container port |
| SPIRE federation bundle HTTPS | 8443 | `generateBundleEndpointConfig` `port: 8443` |
| SPIRE server Prometheus | 8082 (sidecar ctrl-mgr) / 9402 (telemetry in server.conf) | `statefulset.go` + `configmap.go` telemetry block |
| OIDC discovery HTTPS | 8443 (via Route/service) | operand Deployment + Route |
| SPIRE controller-manager webhook | 9443 | `statefulset.go` container port `https` |

## 2. Target Files (Modification & Creation)

### Layer 1 — Operator TLS (Phase 1)

| File | Reason | Confidence |
|------|--------|------------|
| `go.mod` / `vendor/` | Add `github.com/openshift/controller-runtime-common` for TLS profile helpers | high |
| `cmd/zero-trust-workload-identity-manager/main.go` | Register `configv1` scheme; resolve cluster TLS profile; apply to metrics/webhook `TLSOpts`; add SecurityProfileWatcher for restart on profile/adherence change | high |
| `config/rbac/role.yaml` (+ generated bundle) | Grant `get/list/watch` on `apiservers.config.openshift.io` | high |
| `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml` | Set `features.operators.openshift.io/tls-profiles: "true"` | high |

### Layer 2 — Profile injection into operands (Phase 2)

| File | Reason | Confidence |
|------|--------|------------|
| `pkg/client/client.go` | Add `configv1.APIServer` to `cacheResourceWithoutReqSelectors` / `informerResources` for reconcile-time Get | high |
| `pkg/controller/tls/tls.go` | **(New)** Shared helpers: resolve APIServer profile → minTLS + cipher list + adherence semantics | high |
| `pkg/controller/spire-server/configmap.go` | Inject min TLS/ciphers into `generateServerConfMap`; federation bundle config; controller-manager config YAML | high |
| `pkg/controller/spire-agent/configmap.go` | Inject TLS settings into agent config JSON | high |
| `pkg/controller/spire-oidc-discovery-provider/configmaps.go` | Inject TLS settings into OIDC provider config | high |
| `pkg/controller/spire-server/controller.go` | Emit warning event when `requirePQKEM` + strict adherence coexist | medium |
| `pkg/controller/spire-agent/controller.go` | Same warning event pattern | medium |

### Layer 3 — PQC flag (Phase 3)

| File | Reason | Confidence |
|------|--------|------------|
| `api/v1alpha1/zero_trust_workload_identity_manager_types.go` | Add `RequirePQKEM *bool` to spec | high |
| `config/crd/bases/*.yaml` | Regenerated from kubebuilder markers | high |
| `pkg/controller/spire-server/configmap.go` | When `requirePQKEM`: emit `experimental.require_pq_kem`; skip profile injection | high |
| `pkg/controller/spire-agent/configmap.go` | Same PQC injection for agent-to-server mTLS | high |

### Tests

| File | Reason | Confidence |
|------|--------|------------|
| `pkg/controller/tls/tls_test.go` | **(New)** Profile resolution unit tests | high |
| `pkg/controller/spire-server/configmaps_test.go` | Extend for TLS/PQC config generation | high |
| `pkg/controller/spire-agent/configmap_test.go` / new tests | Agent config precedence tests | high |

### Upstream forks (outside this repo — plan must reference)

| Upstream component | Patches for configurable TLS on gRPC, federation HTTPS, Prometheus, OIDC, ctrl-mgr webhook | Confidence |
|--------------------|---------------------------------------------------------------------------------------------|------------|
| `openshift/spiffe-spire` | Read min TLS/ciphers from HCL; `experimental.require_pq_kem` handling | high (per ADR) |
| `openshift/spiffe-spire-controller-manager` | Webhook TLSMINVERSION / TLSCIPHERSUITES from config | medium |

## 3. Reference Context (Read-Only)

### 3.1 Entry Points & Wiring

- `cmd/zero-trust-workload-identity-manager/main.go` — manager, metrics, webhook TLS today
- `pkg/client/client.go` — cache/informer resource lists (extend for APIServer)
- Controller registration at bottom of `main.go` (lines 237–271)

### 3.2 API / Interface Patterns

- `api/v1alpha1/zero_trust_workload_identity_manager_types.go` — ZTWIM spec (trustDomain, clusterName, bundleConfigMap only today)
- `api/v1alpha1/spire_server_config_types.go` — SpireServer spec including Federation
- `api/v1alpha1/conditions.go` — Shared condition types
- `pkg/controller/status/manager.go` — Condition aggregation pattern

### 3.3 Build, CI & Tooling

- `Makefile` — `build`, `test`, `verify`, `manifests`, `generate`, `bundle`
- `.golangci.yml` — lint rules
- `hack/go-fips.sh` — production FIPS build
- CI likely external (`openshift/release`) — in-repo `make verify` + `make test` are local preflight

### 3.4 Manifest / Config Generation Pipelines

- `make manifests` → `config/crd/bases/`, RBAC from kubebuilder markers
- `make generate` → deepcopy
- `make bundle` → `bundle/manifests/`
- `make update-bindata` → `pkg/operator/assets/bindata.go`

### 3.5 Test Patterns & Fixtures

- Table-driven unit tests with `FakeCustomCtrlClient` — exemplar: `pkg/controller/spire-server/configmaps_test.go`, `controller_test.go`
- Config hash tests: `pkg/controller/utils/utils_test.go` (`GenerateConfigHash`)
- E2e: `test/e2e/` (Ginkgo v2, live OpenShift)

## 4. Configuration Surface & Runtime Behavior

### 4.1 Current Configuration Surface

**ZeroTrustWorkloadIdentityManager (cluster singleton)**

| Field | Type | Notes |
|-------|------|-------|
| `spec.trustDomain` | string | Immutable |
| `spec.clusterName` | string | Immutable |
| `spec.bundleConfigMap` | string | Default `spire-bundle`, immutable |
| `spec.requirePQKEM` | *bool | **NOT PRESENT** — to be added (FR-005) |

**Platform (external to operator CR)**

| Source | Fields | Read today? |
|--------|--------|-------------|
| `APIServer/cluster` (`config.openshift.io`) | `spec.tlsSecurityProfile`, `spec.tlsAdherencePolicy` | **No** |

**Operator runtime flags (`main.go`)**

| Flag | Default | TLS impact |
|------|---------|------------|
| `--metrics-bind-address` | `:8443` | Metrics listen port |
| `--metrics-secure` | `true` | HTTPS metrics |
| `--metrics-cert-dir` | `""` | Cert dir or self-signed |
| `--enable-http2` | `false` | Disables HTTP/2 on metrics/webhook TLS |

**Operand CRs (SpireServer, SpireAgent, etc.)** — no TLS-specific fields; TLS must be injected by operator into generated ConfigMaps.

### 4.2 Reconciliation / Processing Flow (Detailed)

**SpireServer reconciler** (`pkg/controller/spire-server/controller.go`):

| Step | Function | Error behavior |
|------|----------|----------------|
| 1 | Get SpireServer CR | NotFound → return nil |
| 2 | defer statusMgr.ApplyStatus | Log error |
| 3 | Get ZTWIM `cluster` | NotFound → Ready=False, no requeue |
| 4 | Set owner reference | Fail → return error |
| 5 | handleCreateOnlyMode | Sets condition |
| 6 | validateConfiguration | Invalid → condition, return nil |
| 7 | handleTTLValidation | Invalid → return nil |
| 8 | reconcileServiceAccount | Error → return err |
| 9 | reconcileService | Error → return err |
| 10 | reconcileRBAC | Error → return err |
| 11 | reconcileWebhook | Error → return err |
| 12 | reconcileSpireServerConfigMap → hash | Error → return err |
| 13 | reconcileSpireControllerManagerConfigMap → hash | Error → return err |
| 14 | reconcileBundleConfigMap | Error → return err |
| 15 | reconcileStatefulSet (applies hash annotations) | Error → return err |
| 16 | reconcileRoute (if federation) | Error → return err |

**TLS feature insertion points:** Steps 12–13 (config generation) and step 1 of `main.go` (operator TLS). APIServer watch/restart is new infrastructure in `main.go`, not in operand reconciler loop.

**Rolling restart mechanism (existing):** Config hash change → pod template annotation change → `needsUpdate` → StatefulSet/DaemonSet/Deployment update → Kubernetes rolling rollout.

### 4.3 Image / Dependency Resolution

- Operand images from `RELATED_IMAGE_*` env vars (OLM CSV `relatedImages`).
- SPIRE binaries run TLS inside containers — TLS config is **file/config driven**, not operator-image driven.
- `controller-runtime-common` not in `go.mod` — new dependency for Layer 1.

### 4.4 Status / Health Reporting

- Operand CRs: typed conditions via `status.Manager` (e.g., `ServerConfigMapAvailable`, `StatefulSetAvailable`).
- ZTWIM CR: aggregates operand Ready into `status.operands` + top-level Ready/OperandsAvailable.
- OLM: `Upgradeable` synced to `OperatorCondition` (best-effort).
- **No TLS-specific conditions today** — TLS failures surface as operand Not Ready or connectivity errors.

### 4.5 Feature Gate / Feature Flag Mechanism

- No operator-local feature gates for TLS.
- Platform: `TLSGroupPreferences` feature gate (OCPSTRAT-3123) — **deferred** per specs Assumption A-002.
- Application PQC: future `spec.requirePQKEM` on ZTWIM CR (opt-in).

## 5. Reusable Assets (Anti-Duplication)

- `pkg/controller/utils.GenerateConfigHash` / `GenerateConfigHashFromString` — use for any new config hashing; already drives rolling restarts. Evidence: `configmap.go` lines 544–551, agent `configmap.go` line 213.
- Config hash annotation keys — reuse existing pattern (`ztwim.openshift.io/spire-*-config-hash`); do not invent new restart triggers. Evidence: `statefulset.go` lines 26–27, 165–166.
- `pkg/controller/utils/resource_comparison.go` — extend if new pod template fields compared; includes hash annotation keys in ignore/diff lists.
- `pkg/controller/status.Manager` — use for warning events (FR-008) via `eventRecorder` on reconcilers.
- `github.com/openshift/controller-runtime-common/pkg/tls` — **use for Layer 1** (reference: cluster-control-plane-machine-set-operator per ADR); do not reimplement profile parsing.
- OpenShift `config/v1` APIServer types — already vendored under `vendor/github.com/openshift/api/config/v1/`; register via `configv1.Install(scheme)`.

## 6. Architectural Guardrails

**Structural**

- Five-controller single-binary layout — TLS helpers belong in shared `pkg/controller/tls/`, not duplicated per operand.
- All K8s access via `CustomCtrlClient`, not raw client. Evidence: all reconciler structs.
- New watched types must be added to `pkg/client/client.go` cache lists.

**API / Schema**

- ZTWIM is cluster-scoped singleton (`name=cluster` CEL). New `requirePQKEM` is optional `*bool` for tri-state semantics.
- After API change: `make manifests generate verify`.

**Build / Tooling**

- Go 1.25+; FIPS production builds via `hack/go-fips.sh`.
- Vendored deps — run `make vendor` after `go.mod` change.

**Deployment / Packaging**

- CSV annotation `tls-profiles: "true"` required for platform recognition (currently `"false"` line 15 of CSV base).
- RBAC in `config/rbac/` is source; bundle copies generated output.

**Code Generation**

- Never hand-edit `zz_generated.deepcopy.go`, `config/crd/bases/*.yaml`, `bindata.go`.

**Security**

- Metrics use authn/authz filter (`filters.WithAuthenticationAndAuthorization`).
- Do not change TLS **client** settings (FR-012).
- PQC opt-in only — default must preserve Intermediate/non-strict behavior (FR-004).

## 7. Change Cascade Checklist

| When you change... | You must also... | Verify with... |
|---|---|---|
| `api/v1alpha1/*_types.go` fields | `make manifests generate`; update CSV descriptions if user-facing | `make verify` |
| RBAC kubebuilder markers / `config/rbac/role.yaml` | `make manifests`; regenerate bundle | `make bundle` + `make verify` |
| `go.mod` (controller-runtime-common) | `make vendor` | `make verify` |
| SPIRE ConfigMap generation logic | Update `_test.go` in same package; expect hash annotation change → rollout | `go test ./pkg/controller/spire-server/...` |
| `pkg/client/client.go` cache types | Ensure APIServer not label-filtered (cluster-scoped) | `go test ./pkg/controller/...` |
| CSV feature annotations | Regenerate bundle | `make bundle` |
| Upstream SPIRE fork TLS patches | Bump RELATED_IMAGE digests in release repo | Integration test on cluster |

## 8. Test & CI Reference

### 8.1 Test Structure

- Unit: `pkg/controller/**/*_test.go` — table-driven, counterfeiter fakes
- Envtest: `make test` uses K8s 1.31.0 assets
- E2e: `test/e2e/` Ginkgo, 45min timeout

### 8.2 How to Run Tests Locally

```bash
make verify          # vet + fmt + golangci-lint
make test            # unit tests (manifests + generate + envtest)
go test ./pkg/controller/spire-server/... -count=1
go test ./pkg/controller/spire-agent/... -count=1
make test-e2e        # requires live OpenShift cluster
```

### 8.3 CI Pipeline

- In-repo: `make verify`, `make test` expected on PR
- Full OpenShift CI via `openshift/release` (not fully enumerated in this repo)

### 8.4 Test Coverage Gaps

- **No TLS/profile tests exist** — greenfield coverage needed in `pkg/controller/tls/` and configmap tests
- **No APIServer integration tests** in unit suite — may need envtest APIServer fixture or envtest mock
- E2e likely lacks tls-scanner automation today

## 9. Developer Workflow

### 9.1 Key Commands Reference

| Command | Purpose |
|---------|---------|
| `make build` | Full build (manifests + generate + vet + compile) |
| `make build-operator` | Compile binary only |
| `make manifests` | CRD/RBAC from kubebuilder markers |
| `make generate` | DeepCopy |
| `make vendor` | go mod tidy + vendor |
| `make bundle` | OLM bundle |
| `make verify` | Preflight lint/fmt/vet |
| `make test` | Unit tests |

### 9.2 Version Variables

- `VERSION` in Makefile (default 1.1.0) — bundle version
- `ENVTEST_K8S_VERSION = 1.31.0`
- Go version from `go.mod` (1.25.7)

### 9.3 Local Development Setup

- Go 1.25+, envtest assets downloaded by `make test`
- `OPERATOR_NAMESPACE`, `OPERATOR_CONDITION_NAME` required at runtime (OLM sets in cluster)

### 9.4 Common Development Scenarios

**How to add a new ZTWIM API field (`requirePQKEM`):**

1. Add field + kubebuilder markers in `api/v1alpha1/zero_trust_workload_identity_manager_types.go`
2. `make manifests generate verify`
3. Read field in config generators (`generateServerConfMap`, agent/OIDC generators) from `ztwim` parameter already passed
4. Add table tests in `configmaps_test.go`
5. ZTWIM spec change already triggers operand reconcile via `ZTWIMSpecChangedPredicate`

**How to add APIServer watch for TLS profile:**

1. `configv1.Install(scheme)` in `main.go` `init()` or before manager
2. Add `&configv1.APIServer{}` to `pkg/client/client.go` informer lists
3. Use controller-runtime-common SecurityProfileWatcher in `main.go` post-manager creation
4. RBAC: `config.openshift.io/apiservers` get/list/watch in `config/rbac/role.yaml`

## 10. Platform & Environment Integration

### 10.1 Security Context & Permissions

- SPIRE agent uses privileged SCC (existing); TLS change does not alter SCC needs.
- Metrics RBAC: `config/rbac/kustomization.yaml` auth delegator for metrics protection.

### 10.2 Proxy & Network Configuration

- CSV declares `proxy-aware: "true"`; TLS profile is orthogonal to HTTP proxy env.

### 10.3 Cloud Provider Integration

- Not applicable to TLS profile feature.

### 10.4 Build & Compliance Constraints

- CSV: `fips-compliant: "true"` — TLS changes must remain FIPS-compatible.
- Strict TLS adherence blocks upgrades if non-compliant — motivation for feature.

### 10.5 Console / UI Integration

- Not applicable — cluster admin configures APIServer and ZTWIM CR via CLI/API.

### 10.6 Packaging & Lifecycle

- Update CSV `tls-profiles` annotation before release.
- Operand image updates require release-repo submodule + digest pin workflow.

## 11. Risks & Downstream Impacts

- **Rolling restart disruption:** Profile change restarts operator then operands; brief SPIRE API unavailability. Mitigation: existing PDBs, agent reconnect. Impact: all clusters on strict adherence.
- **Upstream SPIRE patch lag:** Operand TLS read from config requires fork patches; operator-only changes insufficient for FR-011 full compliance. Mitigation: phase operator first, coordinate release-repo submodule bumps.
- **PQC handshake failures:** `requirePQKEM=true` rejects non-PQC clients. Mitigation: opt-in flag + warning events.
- **Cipher name mapping:** OpenShift profile cipher names may not map 1:1 to Go — log skipped ciphers per ADR.
- **CREATE_ONLY_MODE:** ConfigMap updates skipped — TLS profile changes won't propagate. Mitigation: document; existing CreateOnlyMode condition.

### 11.1 Assessment Limitations / UNVERIFIED Items

- **Upstream SPIRE fork state not inspected** — verify TLS config hooks exist on release branch submodules in `zero-trust-workload-identity-manager-release`.
- **`controller-runtime-common/pkg/tls` API surface not vendored yet** — verify exact function names (`NewTLSConfigFromProfile`, `SecurityProfileWatcher`) against current module version before plan.
- **Operator webhook listen port** — not explicitly set in `main.go`; confirm from deployment manifest `config/manager/manager.yaml` during plan.
- **SPIRE server Prometheus port 8082 vs 9402** — server.conf telemetry uses 9402; ctrl-mgr sidecar exposes 8082 in StatefulSet — map both during tls-scanner test plan.
- **E2e tls-scanner integration** — no in-repo automation verified; manual/OCP CI step likely required.

## 12. Quick Reference Card

### Preflight Checklist (run before every PR)

```
1. make verify
2. make test
3. make manifests generate   # if API/RBAC changed
4. make bundle               # if CSV/RBAC changed
5. make vendor               # if go.mod changed
```

### Key File Quick-Nav

| I want to... | Look at... |
|---|---|
| Apply cluster TLS to operator metrics/webhook | `cmd/zero-trust-workload-identity-manager/main.go` |
| Add requirePQKEM field | `api/v1alpha1/zero_trust_workload_identity_manager_types.go` |
| Inject TLS into SPIRE server config | `pkg/controller/spire-server/configmap.go` → `generateServerConfMap` |
| Inject TLS into SPIRE agent config | `pkg/controller/spire-agent/configmap.go` |
| Inject TLS into OIDC config | `pkg/controller/spire-oidc-discovery-provider/configmaps.go` |
| Add APIServer to cache | `pkg/client/client.go` |
| Shared profile resolution | `pkg/controller/tls/tls.go` (new) |
| RBAC for APIServer | `config/rbac/role.yaml` |
| CSV tls-profiles annotation | `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml` |
| Config hash / rolling restart | `pkg/controller/spire-server/statefulset.go`, `utils/resource_comparison.go` |
| Unit test patterns | `pkg/controller/spire-server/configmaps_test.go` |
