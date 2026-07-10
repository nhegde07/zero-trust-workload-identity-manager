# Repository Assessment Report
**Feature:** TLS Profile Compliance and Hybrid Post-Quantum Key Exchange

## 0. Inputs & Tooling

| Field | Value |
| ----- | ----- |
| **repo** | `https://github.com/nhegde07/zero-trust-workload-identity-manager` (working-folder mode) |
| **working_folder_path** | `/home/nhegde/work/github.com/nhegde07/zero-trust-workload-identity-manager` |
| **branch** | `shiftweek-420` |
| **commit** | `feae63b399b415ff60223057601045aebb0d55a7` |
| **tooling_status** | OK |
| **spec status** | `specs.md` approved (validation PASS 89%) |
| **related upstream repos** | `github.com/spiffe/spire`, `github.com/spiffe/spire-controller-manager` (out of tree — see §0.1) |

**Feature verdict on pinned branch:** **GREENFIELD** — no TLS profile integration, no `requirePQKEM`, no `controller-runtime-common`, no APIServer cache/RBAC, CSV declares `tls-profiles: "false"`. Implementation follows ADR two-layer model from scratch on this branch.

### 0.1 Multi-Repo Scope

| Repo | Role for this feature | Assessed in this report |
| ---- | ----------------------- | ----------------------- |
| ZTWIM operator (this repo) | Layer 1 operator TLS + Layer 2 ConfigMap injection + CRD `requirePQKEM` | Yes — full assessment |
| `spiffe/spire` (OpenShift fork via release repo) | Read injected TLS/PQC config in server, agent, OIDC, Prometheus endpoints | Referenced — not cloned; planning must coordinate operand image bumps |
| `spire-controller-manager` | Webhook TLSMINVERSION / TLSCIPHERSUITES from config | Referenced — config generated in ZTWIM `pkg/controller/spire-server/configmap.go` |

## 1. Architecture Overview

### 1.1 Project Type & Tech Stack

- **Type:** OpenShift/Kubernetes operator (controller-runtime v0.22.4), OLM bundle, five reconcilers in one binary.
- **Language:** Go **1.25.7** (`go.mod`); FIPS production builds via `hack/go-fips.sh`.
- **Key deps:** `sigs.k8s.io/controller-runtime`, `github.com/openshift/api` (config/security/route), `github.com/spiffe/spire-controller-manager` (operand CR types + ControllerManagerConfig), `k8s.io/*` v0.35.3.
- **Missing for feature:** `github.com/openshift/controller-runtime-common` — not in `go.mod` (greenfield add).
- **Build:** GNU Make + `openshift/build-machinery-go` bindata; kubebuilder/controller-gen for CRDs.

### 1.2 Component Map

| Package | Responsibility |
| ------- | -------------- |
| `cmd/zero-trust-workload-identity-manager/main.go` | Entry: scheme registration, metrics/webhook TLS setup, manager bootstrap, controller wiring |
| `api/v1alpha1/` | CRD types (ZTWIM + four operand CRs); hand-written + `zz_generated.deepcopy.go` |
| `pkg/client/` | `CustomCtrlClient`, cache builder, informer resource lists |
| `pkg/controller/zero-trust-workload-identity-manager/` | Status aggregation; watches operand statuses + OLM OperatorCondition |
| `pkg/controller/spire-server/` | Server StatefulSet, ConfigMaps (server + ctrl-mgr), webhooks, federation, Route |
| `pkg/controller/spire-agent/` | Agent DaemonSet, ConfigMap, SCC, RBAC |
| `pkg/controller/spire-oidc-discovery-provider/` | OIDC Deployment, ConfigMap (`oidc-discovery-provider.conf` JSON), Route |
| `pkg/controller/spiffe-csi-driver/` | CSI DaemonSet (no TLS endpoints for this feature) |
| `pkg/controller/status/` | Shared condition manager with auto-Ready |
| `pkg/controller/utils/` | Predicates, constants, validation, `GenerateMapHash`, image env vars |
| `bindata/` + `pkg/operator/assets/bindata.go` | Static operand YAML (generated — do not hand-edit) |
| `config/` | Kustomize CRD/RBAC/manager; CSV base at `config/manifests/bases/` |
| `bundle/` | Generated OLM bundle |
| `test/e2e/` | Ginkgo e2e against live OpenShift |

### 1.3 Framework & Pattern Architecture

Single **controller-runtime** manager (not library-go). All operand reconcilers share:

1. `Get` operand CR (`cluster` singleton).
2. `status.NewManager` + **`defer statusMgr.ApplyStatus`**.
3. `Get` parent `ZeroTrustWorkloadIdentityManager` (`cluster`).
4. `handleCreateOnlyMode` check.
5. Validate spec → ordered `reconcile*` chain → return.

**ZTWIM aggregator** does not create operands; it aggregates operand status and syncs OLM `Upgradeable`. Operand controllers watch ZTWIM with `ZTWIMSpecChangedPredicate` — adding `requirePQKEM` to ZTWIM spec will trigger operand re-reconcile automatically.

**Dead-code / do-not-edit traps:**

- `pkg/operator/assets/bindata.go` — generated via `make update-bindata`.
- `config/crd/bases/*.yaml`, `zz_generated.deepcopy.go` — generated.
- `bundle/manifests/*` — generated from `make bundle`; edit `config/manifests/bases/` for CSV annotations.
- `PROJECT` file stale (ignore).

### 1.4 Runtime Data/Control Flow (TLS Feature)

**Today (pre-feature):**

1. Operator starts with hardcoded metrics TLS (`:8443`, self-signed or `--metrics-cert-dir`) and default webhook TLS — no cluster APIServer profile.
2. Operand ConfigMaps built from CR specs + ZTWIM trust domain; **no** `minTLSVersion`, `cipherSuites`, or `experimental.require_pq_kem`.
3. Config hash → pod template annotation → rolling update when hash changes.

**Target (post-feature):**

1. **Layer 1:** `main.go` resolves cluster TLS via controller-runtime-common, applies to metrics/webhook `TLSOpts`, registers `SecurityProfileWatcher` → process exit on profile/adherence change.
2. **Layer 2:** Shared `pkg/controller/tls/` helpers read `APIServer.spec.tlsAdherence` + `tlsSecurityProfile` during reconcile; inject into server/agent/OIDC/ctrl-mgr ConfigMaps when `requirePQKEM` absent and `tlsAdherence=StrictAllComponents`.
3. **PQC:** Read `ztwim.Spec.RequirePQKEM`; when true inject `experimental.require_pq_kem` and skip central profile fields.
4. Upstream operand images (patched SPIRE/ctrl-mgr) read injected config at process start.

## 2. Target Files (Modification & Creation)

### 2.1 Phase 1 — Operator Layer 1

| File | Action | Reason |
| ---- | ------ | ------ |
| `go.mod` / `go.sum` / `vendor/` | Modify | Add `github.com/openshift/controller-runtime-common`. Evidence: absent from `go.mod`. |
| `cmd/zero-trust-workload-identity-manager/main.go` | Modify | Register `configv1` scheme; resolve profile → metrics/webhook TLS; SecurityProfileWatcher + cancel context on change. Evidence: current metrics TLS is manual (`metricsTLSOpts`, lines 125–186); no configv1 import. |
| `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml` | Modify | Set `features.operators.openshift.io/tls-profiles: "true"`. Evidence: currently `"false"` (line 15). |
| `config/rbac/role.yaml` | Modify | Add `config.openshift.io/apiservers` get/list/watch. Evidence: no apiservers rules in `config/rbac/`. |

### 2.2 Phase 2 — Layer 2 Shared + Operand Injection

| File | Action | Reason |
| ---- | ------ | ------ |
| `pkg/controller/tls/tls.go` | **Create** | Resolve APIServer TLS profile + adherence; export helpers for Layer 2 injection. Evidence: directory does not exist. |
| `pkg/client/client.go` | Modify | Add `configv1.APIServer{}` to `cacheResourceWithoutReqSelectors` and `informerResources`. Evidence: lists omit APIServer (lines 54–84). |
| `pkg/controller/spire-server/configmap.go` | Modify | Inject profile TLS or `require_pq_kem` in `generateServerConfMap`; ctrl-mgr config in `generateSpireControllerManagerConfigYaml`. Evidence: `generateServerConfMap` takes `ztwim` but no TLS fields today. |
| `pkg/controller/spire-agent/configmap.go` | Modify | Inject in agent config generator (agent-to-server mTLS). Evidence: `generateSpireAgentConfigMap(agent, ztwim)`. |
| `pkg/controller/spire-oidc-discovery-provider/configmaps.go` | Modify | Inject into `generateOIDCConfigMapFromCR` JSON. Evidence: `serving_cert_file` block only (lines 122–126). |
| `pkg/controller/spire-server/statefulset.go` | Verify | Config hash annotations already drive rollout. Evidence: `ztwim.openshift.io/spire-server-config-hash` annotations. |
| `pkg/controller/spire-agent/daemonset.go` | Verify | Agent config hash annotation pattern. |
| `pkg/controller/spire-oidc-discovery-provider/deployments.go` | Verify | Deployment uses ConfigMap hash for rollout. |

### 2.3 Phase 3 — CRD + Events

| File | Action | Reason |
| ---- | ------ | ------ |
| `api/v1alpha1/zero_trust_workload_identity_manager_types.go` | Modify | Add `RequirePQKEM *bool` to `ZeroTrustWorkloadIdentityManagerSpec`. Evidence: spec only has TrustDomain, ClusterName, BundleConfigMap (lines 114–147). |
| `config/crd/bases/` | Regenerate | Via `make manifests` after API change. |
| `bundle/` | Regenerate | Via `make bundle`. |
| `pkg/controller/spire-server/controller.go` | Modify | Emit Warning event when `requirePQKEM=true` && strict adherence. |
| `pkg/controller/spire-agent/controller.go` | Modify | Same warning event pattern. |

### 2.4 Tests

| File | Action | Reason |
| ---- | ------ | ------ |
| `pkg/controller/tls/tls_test.go` | Create | Profile resolution + adherence matrix unit tests. |
| `pkg/controller/spire-server/configmaps_test.go` | Modify | Extend existing hash/config tests for TLS injection. |
| `pkg/controller/spire-agent/configmap_test.go` | Modify | Agent TLS/PQC config tests. |
| `pkg/controller/spire-oidc-discovery-provider/configmaps_test.go` | Modify | OIDC ConfigMap JSON injection tests. |
| `test/e2e/` | Modify (future) | tls-scanner or handshake probes — currently no TLS compliance e2e. |

### 2.5 Do NOT Edit for TLS Logic

- `pkg/controller/spiffe-csi-driver/` — no TLS server endpoints in scope.
- `bindata/` operand YAML — TLS comes from generated ConfigMaps, not static bindata.
- Generated `bundle/manifests/*.yaml` directly — edit bases + regenerate.

## 3. Reference Context (Read-Only)

### 3.1 Entry Points & Wiring

- `cmd/zero-trust-workload-identity-manager/main.go` — manager, metrics `:8443`, webhook server, five controllers.
- `pkg/client/client.go` — `NewCacheBuilder()`, resource watch lists.

### 3.2 API / Interface Patterns

- `api/v1alpha1/zero_trust_workload_identity_manager_types.go` — ZTWIM spec (target for `RequirePQKEM`).
- `api/v1alpha1/common_types.go` — `CommonConfig`, `ConditionalStatus`.
- `vendor/github.com/openshift/api/config/v1/types_apiserver.go` — `TLSAdherence`, `TLSSecurityProfile` field definitions.

### 3.3 Build, CI & Tooling

- `Makefile` — `make all`, `verify`, `test`, `manifests`, `generate`, `bundle`, `update-bindata`.
- `.github/workflows/` or `openshift/release` — CI external (verify in repo if present).

### 3.4 Manifest / Config Generation

- `make manifests` → `config/crd/bases/`, RBAC from kubebuilder markers.
- `make bundle` → `bundle/manifests/`.
- Operand runtime config: **ConfigMap JSON/HCL** in controller packages (not bindata).

### 3.5 Test Patterns

- Table-driven unit tests with `FakeCustomCtrlClient` (`pkg/client/fakes/`).
- `pkg/controller/spire-server/configmaps_test.go` — extensive config hash tests (pattern to follow).
- `test/e2e/` — Ginkgo, requires live cluster (`make test-e2e`, 45min timeout).

## 4. Configuration Surface & Runtime Behavior

### 4.1 Current Configuration Surface

**ZeroTrustWorkloadIdentityManager** (`cluster` singleton):

| Field | Type | Notes |
| ----- | ---- | ----- |
| `trustDomain` | string | Required, immutable |
| `clusterName` | string | Required, immutable |
| `bundleConfigMap` | string | Default `spire-bundle`, immutable |
| `requirePQKEM` | *bool | **NOT PRESENT — to be added** |

**Cluster APIServer** (read-only, not cached today):

| Field | Relevance |
| ----- | --------- |
| `spec.tlsSecurityProfile` | Old/Intermediate/Modern/Custom — min version, ciphers, future groups |
| `spec.tlsAdherence` | `StrictAllComponents` triggers Layer 2 injection |

**Operand CRs** (`SpireServer`, `SpireAgent`, etc.): `CommonConfig` (labels, resources, affinity, tolerations, nodeSelector). **No per-operand PQC flags** — by design per specs FR-005.

**Operator runtime flags** (`main.go`): `--metrics-bind-address :8443`, `--metrics-secure`, `--metrics-cert-dir`, `--enable-http2` (disabled by default).

### 4.2 Reconciliation / Processing Flow (SpireServer — TLS touchpoints)

| Step | Function | Error behavior |
| ---- | -------- | -------------- |
| 1 | `reconcileServiceAccount` | Condition False; return err |
| 2 | `reconcileService` | Condition False; return err |
| 3 | `reconcileRBAC` | Condition False; return err |
| 4 | `reconcileWebhook` | Condition False; return err |
| 5 | `reconcileSpireServerConfigMap` | Generates server conf + **hash**; TLS injection point |
| 6 | `reconcileSpireControllerManagerConfigMap` | Ctrl-mgr YAML + hash; webhook TLS env injection point |
| 7 | `reconcileSpireBundleConfigMap` | Bundle CM |
| 8 | `reconcileStatefulSet` | Applies hash annotations → triggers rollout if changed |
| 9 | `reconcileRoute` | Optional federation/OIDC routes |

Agent/OIDC follow same pattern: ConfigMap reconcile → hash → DaemonSet/Deployment update.

**ZTWIM spec change:** `ZTWIMSpecChangedPredicate` requeues all operands when `requirePQKEM` changes.

### 4.3 Image / Dependency Resolution

Operand images via OLM `RELATED_IMAGE_*` env vars (see agents.md Environment Variables table). TLS feature does not change images until upstream SPIRE patches are vendored into operand image builds (release repo submodule pipeline).

### 4.4 Status / Health Reporting

- Operand controllers: typed conditions per reconcile step (`ServerConfigMapAvailable`, etc.).
- ZTWIM: aggregates `status.operands[]`, sets `Ready`, `OperandsAvailable`.
- Warning events for PQ+strict coexistence — **not implemented** (new).

### 4.5 Feature Gate / Feature Flag Mechanism

- OpenShift `TLSAdherence` feature gate on APIServer API (`+openshift:enable:FeatureGate=TLSAdherence` in vendored types).
- `TLSGroupPreferences` — not graduated; Phase 4 defer per specs.
- Operator `requirePQKEM` — new opt-in application flag (not a cluster feature gate).

## 5. Reusable Assets (Anti-Duplication)

| Asset | Use for TLS feature | Evidence |
| ----- | ------------------- | -------- |
| `utils.GenerateMapHash()` / `generateConfigHash()` in spire-server | Config change detection for operand rollouts — reuse when TLS fields added to ConfigMap | `pkg/controller/spire-server/configmap.go:553`, agent uses similar |
| `ZTWIMSpecChangedPredicate` | Auto-reconcile operands when `requirePQKEM` toggles | `pkg/controller/utils/predicates.go` |
| `status.NewManager` + defer `ApplyStatus` | Warning events via `eventRecorder` on reconcilers | All operand controllers |
| `github.com/openshift/api/config/v1` | APIServer types already vendored | `vendor/.../types_apiserver.go` — `TLSAdherence` field present |
| `github.com/openshift/controller-runtime-common/pkg/tls` | Layer 1 profile fetch/watch — **add dependency; do not reimplement** | ADR reference; absent from go.mod today |
| `marshalToJSON` / JSON config builders in configmap.go | Extend for TLS/PQC JSON blocks | spire-server configmap patterns |
| Counterfeiter fakes | Unit test APIServer reads via `FakeCustomCtrlClient` | `pkg/client/fakes/` |

## 6. Architectural Guardrails

**Structural**

- Add shared TLS logic in `pkg/controller/tls/` — do not duplicate profile resolution in each controller.
- Pass `ztwim *ZeroTrustWorkloadIdentityManager` into config generators (already done) — read `RequirePQKEM` from there only.
- Register new watched types in `pkg/client/client.go` cache builder.

**API / Schema**

- `RequirePQKEM` on ZTWIM only — no SpireServer/SpireAgent fields (specs FR-005).
- Run `make manifests generate bundle` after API edits.
- Preserve CEL singleton name=`cluster` constraints.

**Build / Tooling**

- Go 1.25.7; run `make vendor` after new deps.
- FIPS: use `hack/go-fips.sh` for production parity.

**Deployment / Packaging**

- Update CSV **base** annotation `tls-profiles: "true"`, not generated bundle alone.
- ConfigMap content changes must update hash annotations to roll pods.

**Code Generation**

- Never hand-edit `bindata.go`, CRD bases, deepcopy, bundle manifests.

**Security**

- Layer 2 skip central injection when `requirePQKEM=true` (precedence model).
- RBAC least privilege: apiservers read-only.
- Metrics already use `filters.WithAuthenticationAndAuthorization` — preserve when adding TLS profile.

## 7. Change Cascade Checklist

| When you change... | You must also... | Verify with... |
| ------------------ | ---------------- | -------------- |
| `api/v1alpha1/*_types.go` (add `RequirePQKEM`) | `make generate manifests bundle`; update CSV descriptions if needed | `make verify` |
| `go.mod` (controller-runtime-common) | `make vendor`; commit vendor/ | `make verify && make test` |
| `config/rbac/role.yaml` | Regenerate bundle RBAC or run `make manifests` | `make bundle` diff |
| `pkg/client/client.go` cache lists | Ensure APIServer informer works in unit tests | `make test` |
| Operand ConfigMap generators | Hash annotations update → StatefulSet/DaemonSet/Deployment rollout | `pkg/controller/spire-server/configmaps_test.go` |
| CSV base annotations | `make bundle` | grep `tls-profiles` in bundle |
| Upstream SPIRE TLS patches | Bump operand images in release repo; update RELATED_IMAGE digests | Release repo pipeline (out of tree) |

## 8. Test & CI Reference

### 8.1 Test Structure

- Unit: `pkg/controller/**/*_test.go` — table-driven, testify/gomega in places.
- E2e: `test/e2e/` — Ginkgo v2, live OpenShift.
- Envtest K8s 1.31.0 for unit (`ENVTEST_K8S_VERSION` in Makefile).

### 8.2 How to Run Tests Locally

```bash
make verify          # vet + fmt + golangci-lint
make test            # unit tests with envtest
make test-e2e        # requires live cluster, 45min timeout
```

### 8.3 CI Pipeline

- In-repo: `make verify`, `make test` expected on PR (per agents.md).
- Full OpenShift CI likely in `openshift/release` — not fully enumerated in this branch.

### 8.4 Test Coverage Gaps (TLS-specific)

- No unit tests for APIServer profile resolution (greenfield).
- No tests for operator metrics/webhook TLS profile alignment.
- No tls-scanner e2e — add in Phase 1–3 per specs A-006.
- OIDC ConfigMap tests exist but do not cover TLS fields.

## 9. Developer Workflow

### 9.1 Key Commands Reference

| Target | Purpose |
| ------ | ------- |
| `make all` | build + verify (default) |
| `make build-operator` | Compile binary |
| `make manifests` | CRD/RBAC from kubebuilder markers |
| `make generate` | DeepCopy |
| `make bundle` | OLM bundle |
| `make update-bindata` | Regenerate bindata (not needed for TLS ConfigMap path) |
| `make verify` | Pre-push lint/vet/fmt |
| `make test` | Unit tests |

**Preflight:** `make verify && make test`

### 9.2 Version Variables

- `VERSION ?= 1.1.0` in Makefile; operand image tags from release repo / RELATED_IMAGE env.
- Go **1.25.7** in `go.mod` (supports Go 1.24+ PQC semantics per ADR).

### 9.3 Local Development Setup

- Go 1.25+, `OPERATOR_NAMESPACE` required at runtime.
- Deploy via `make install deploy` or OLM bundle on test cluster.
- Cluster APIServer object required for Layer 1/2 testing (OpenShift cluster).

### 9.4 How to Add a New Field to ZeroTrustWorkloadIdentityManager

1. Edit `api/v1alpha1/zero_trust_workload_identity_manager_types.go` — add field with kubebuilder markers.
2. Run `make generate manifests bundle`.
3. Read field in operand config generators from `ztwim` parameter (already threaded).
4. Add unit tests; operand controllers auto-watch ZTWIM spec changes.
5. Run `make verify && make test`.

**Example for `requirePQKEM`:** inject in `generateServerConfMap`, `generateAgentConfig`, `generateOIDCConfigMapFromCR`, `generateSpireControllerManagerConfigYaml` — all already receive `ztwim`.

## 10. Platform & Environment Integration

### 10.1 Security Context & Permissions

- SPIRE agent uses privileged SCC; OIDC/server use restricted patterns — unchanged by TLS feature.
- New RBAC: read `apiservers.config.openshift.io`.

### 10.2 Proxy & Network Configuration

- Not applicable to TLS profile injection directly — operands inherit cluster networking.

### 10.3 Cloud Provider Integration

- Not applicable — TLS feature is platform-profile driven.

### 10.4 Build & Compliance Constraints

- FIPS builds via `hack/go-fips.sh`; TLS cipher choices must remain FIPS-valid when profile requires.
- Multi-arch: standard operator Dockerfile patterns.

### 10.5 Console / UI Integration

- Not applicable — admin configures via ZTWIM CR and cluster APIServer.

### 10.6 Packaging & Lifecycle

- CSV `features.operators.openshift.io/tls-profiles` must flip to `"true"`.
- Operator pod restart on profile change (Layer 1) — ensure Deployment rolling strategy tolerates brief metrics scrape failures.
- Operand rolling updates via hash annotations — existing pattern.

## 11. Risks & Downstream Impacts

- **Rolling restart blast radius:** Profile or `requirePQKEM` changes restart operator then operands. Impact: brief SPIRE API unavailability. Mitigation: StatefulSet rolling + PDB; document in release notes.
- **Upstream patch dependency:** Layer 2 injection useless until SPIRE/ctrl-mgr images read config. Impact: Phase 2 blocked on fork image builds. Mitigation: coordinate release repo submodule bumps.
- **PQC handshake failures:** Opt-in `requirePQKEM` rejects non-hybrid clients. Impact: agent/workload disconnect. Mitigation: upgrade agents first; Warning event when strict adherence also set.
- **APIServer cache RBAC:** Missing permissions cause reconcile errors. Mitigation: add RBAC + test with fake APIServer object.
- **Cipher name mapping:** OpenShift profile cipher names may not map 1:1 to Go names. Mitigation: log skipped ciphers (ADR risk table).
- **Create-only mode:** TLS updates skipped when `CREATE_ONLY_MODE=true`. Impact: stale TLS on existing clusters. Mitigation: document; blocks Upgradeable already.

### 11.1 Assessment Limitations / UNVERIFIED Items

- **Upstream SPIRE fork state** on `openshift/spiffe-spire` — not cloned; verify configurable TLS patches exist or need authoring before Phase 2.
- **`controller-runtime-common/pkg/tls` API** — not vendored; verify exact `NewTLSConfigFromProfile` / `SecurityProfileWatcher` signatures against target OpenShift version before implementation.
- **OIDC Deployment hash rollout** — `deployments.go` not fully read; verify annotation pattern matches server/agent before planning OIDC Phase 2 tasks.
- **CI tls-scanner integration** — no in-repo job found; verify with release/CI owners for Phase 3 qualification gate.
- **Webhook server exposure** — `main.go` registers webhook server but validating webhook configs may be operand-side; operator webhook TLS is still in scope for Layer 1 (metrics confirmed `:8443`).

## 12. Quick Reference Card

### Preflight Checklist (run before every PR)

```
1. make verify
2. make test
3. make manifests generate bundle   # if API/RBAC/CSV changed
4. make vendor                      # if go.mod changed
```

### Key File Quick-Nav

| I want to... | Look at... |
| ------------ | ---------- |
| Add `requirePQKEM` API field | `api/v1alpha1/zero_trust_workload_identity_manager_types.go` |
| Operator metrics/webhook TLS (Layer 1) | `cmd/zero-trust-workload-identity-manager/main.go` |
| Shared profile resolution (Layer 2) | `pkg/controller/tls/tls.go` (new) |
| Cache APIServer for reconcile | `pkg/client/client.go` |
| Inject server TLS/PQC config | `pkg/controller/spire-server/configmap.go` |
| Inject agent TLS/PQC config | `pkg/controller/spire-agent/configmap.go` |
| Inject OIDC TLS/PQC config | `pkg/controller/spire-oidc-discovery-provider/configmaps.go` |
| Config hash → rollout | `pkg/controller/spire-server/statefulset.go` |
| CSV tls-profiles annotation | `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml` |
| RBAC for apiservers | `config/rbac/role.yaml` |
| Unit test patterns | `pkg/controller/spire-server/configmaps_test.go` |
| Warning event on PQ+strict | `pkg/controller/spire-server/controller.go`, `pkg/controller/spire-agent/controller.go` |
