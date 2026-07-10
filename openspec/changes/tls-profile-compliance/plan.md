# Technical Implementation Plan
**Feature:** TLS Profile Compliance and Hybrid Post-Quantum Key Exchange

## 0. Inputs acknowledged

| Input | Status |
| ----- | ------ |
| Spec source | ADR-TLS-Compliance / `specs.md` (FR-001–FR-014, SC-001–SC-008, Phases 1–4) |
| Repo assessment pin | `https://github.com/nhegde07/zero-trust-workload-identity-manager`, branch `shiftweek-420`, commit `feae63b3` (tooling_status: OK) |
| `agents.md` | PROVIDED — schema `openspec/schemas/openspec-agile-workflow/inputs/agents.md` |
| `spec_validator_results.json` | PROVIDED — `validation.json` (PASS 89%) |
| `constitution.md` | PROVIDED — schema `openspec/schemas/openspec-agile-workflow/inputs/constitution.md` |
| AgentRoutingMode | **PROVIDED** (constitution § Agent Routing) |

## 1. Architectural strategy

This feature implements a **two-layer TLS model** aligned with the ADR and approved specs:

1. **Layer 1 (direct):** The operator binary applies the cluster TLS security profile to its own TLS servers (metrics `:8443`, admission webhook) using `controller-runtime-common/pkg/tls`, and exits on profile or `tlsAdherence` change via `SecurityProfileWatcher`.
2. **Layer 2 (injected):** During operand reconciliation, shared helpers resolve `apiserver.config.openshift.io/cluster` and inject `minTLSVersion` / `cipherSuites` (and future `groups`) into operand ConfigMaps **only when** `requirePQKEM` is absent and `APIServer.spec.tlsAdherence` is `StrictAllComponents`.
3. **Application override:** A single `requirePQKEM` boolean on `ZeroTrustWorkloadIdentityManager.spec` propagates `experimental.require_pq_kem` to all managed operands, suppressing central profile injection and enforcing TLS 1.3 + hybrid X25519MLKEM768.

Operand processes apply injected settings only after **upstream SPIRE and spire-controller-manager patches** read config (constitution Principle VI — operator configures via ConfigMaps; does not fork upstream logic in this repo).

### Repo-grounded reality check

Per `repo-assessment.md` §0 and §11.1, on branch `shiftweek-420` this work is **entirely greenfield**:

| Expected capability | Branch state |
| ------------------- | ------------ |
| `controller-runtime-common` dependency | Absent from `go.mod` |
| `pkg/controller/tls/` | Does not exist |
| `configv1.APIServer` in client cache | Not registered in `pkg/client/client.go` |
| APIServer RBAC | No `config.openshift.io/apiservers` rules |
| `RequirePQKEM` on ZTWIM CR | Not in `zero_trust_workload_identity_manager_types.go` |
| CSV `tls-profiles` annotation | `"false"` in bases CSV |
| Operand TLS/PQC in ConfigMaps | Not injected today |
| Config hash → rolling restart | **Present** — reuse existing StatefulSet/DaemonSet/Deployment annotation pattern |

Phases MUST implement from scratch; they must **not** assume partial TLS integration exists on this branch. Existing config-hash rollout and `ZTWIMSpecChangedPredicate` are the primary reuse anchors.

### Design constraints from constitution

- **Least-privilege RBAC:** APIServer access is read-only (`get/list/watch` only); human approval required if scope broadens (constitution Human Approval Gates).
- **CustomCtrlClient + cache registration:** APIServer type must be added to `NewCacheBuilder` lists before reconcile reads (Anti-Pattern #8).
- **Generated code discipline:** API and CSV changes cascade through `make generate manifests bundle vendor verify` (Principle VII).
- **No upstream logic in operator repo:** OIDC/server/agent TLS enforcement in running pods requires fork image updates coordinated via release repo (Principle VI).

## 2. Persistence & state

**Kubernetes objects (source of truth):**

| Object | Role |
| ------ | ---- |
| `apiserver.config.openshift.io/cluster` | Authoritative cluster TLS profile + `tlsAdherence` |
| `ZeroTrustWorkloadIdentityManager/cluster` | Operator config; gains `spec.requirePQKEM` |
| Operand CRs (`SpireServer`, `SpireAgent`, `SpireOIDCDiscoveryProvider`) | Unchanged for TLS — no per-operand PQC flags |
| Operand ConfigMaps | Derived — JSON/HCL content injected by reconcilers; hash drives pod rollout |

**Annotations driving behavior:**

- `ztwim.openshift.io/spire-server-config-hash`, `ztwim.openshift.io/spire-controller-manager-config-hash` on StatefulSet pod template (existing).
- Agent/OIDC deployment annotations — verify pattern in `daemonset.go` / `deployments.go` during Phase 3 (repo-assessment §11.1 UNVERIFIED for OIDC).

**External/platform state:**

- OLM injects `RELATED_IMAGE_*` for operand images; upstream TLS patches ship in updated operand images via release repo submodules.
- Operator metrics certs: `--metrics-cert-dir` or self-signed (Layer 1 replaces ad-hoc TLS opts with profile-driven config).

**Not persisted:** In-process TLS hot reload — profile changes require operator pod restart and operand rolling updates (specs A-010).

## 3. Interfaces & contracts (operator-native)

### 3.1 Kubernetes APIs (CRDs/CRs)

**Change:** Add to `ZeroTrustWorkloadIdentityManagerSpec`:

- `requirePQKEM *bool` — optional; omitempty; default behavior unchanged when nil/false.

**Unchanged:** Operand CRDs remain TLS-config-free. Singleton `cluster` name CEL rules preserved.

**APIServer API (read-only):** `spec.tlsSecurityProfile`, `spec.tlsAdherence` (`StrictAllComponents`, `LegacyAdheringComponentsOnly`, omitted → platform default).

### 3.2 Controller/runtime interfaces (internal)

**New package:** `pkg/controller/tls/`

| Function / type (conceptual) | Responsibility |
| ------------------------------ | -------------- |
| Profile resolution helpers | Fetch APIServer; map profile → min TLS version + cipher list (+ groups when available) |
| Adherence gate | Returns whether Layer 2 injection applies (`StrictAllComponents` + not `requirePQKEM`) |
| Injection builders | Produce JSON fragments for server/agent/OIDC/ctrl-mgr configs |

**Layer 1 (main.go):** Integrate `controller-runtime-common/pkg/tls` — `NewTLSConfigFromProfile`, `SecurityProfileWatcher` with context cancel on `OnProfileChange` / `OnAdherencePolicyChange`.

**Precedence contract:**

```
if ztwim.Spec.RequirePQKEM == true → inject experimental.require_pq_kem only
else if tlsAdherence == StrictAllComponents → inject central profile fields
else → omit central profile fields (Go defaults at operand runtime)
```

### 3.3 Webhooks / admission (if applicable)

**Operator admission webhook TLS:** Layer 1 applies cluster profile to webhook server `TLSOpts` in `main.go` (same as metrics).

**Operand validating webhooks:** SPIRE server webhook configs unchanged structurally; TLS min/ciphers for ctrl-mgr webhook come from ctrl-mgr ConfigMap / env when upstream patch lands.

### 3.4 RBAC / security boundaries (if applicable)

**Add to ClusterRole** (`config/rbac/role.yaml`):

```yaml
- apiGroups: ["config.openshift.io"]
  resources: ["apiservers"]
  verbs: ["get", "list", "watch"]
```

No write access to cluster APIServer. Warning events use existing `EventRecorder` on operand reconcilers — no new cluster-scoped writes.

### 3.5 Packaging / OLM (if applicable)

**CSV annotation:** `features.operators.openshift.io/tls-profiles: "true"` in `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml`.

Regenerate bundle via `make bundle`. Document operator pod restart behavior on TLS profile change in release notes (operational follow-up).

## 4. Dependencies & sequencing graph

### Critical path

```
Phase 1 (deps + RBAC + cache + tls helpers skeleton)
  → Phase 2 (Layer 1 main.go + CSV)
  → Phase 3 (Layer 2 ConfigMap injection + hash rollout)
  → [Phase 4 upstream patches + image bump] (blocks live operand TLS compliance)
  → Phase 5 (requirePQKEM API + propagation + events)
  → Phase 6 (verification / tls-scanner gates)
```

### Parallelizable workstreams

| Stream | Can start after | Runs in parallel with |
| ------ | --------------- | --------------------- |
| Upstream SPIRE TLS patches (out of tree) | Phase 3 operator injection design frozen | Phase 3–5 operator work |
| Unit tests per phase | Same phase code complete | — |
| tls-scanner CI job design | Phase 2 complete (operator endpoints) | Phase 3–5 |

### Explicit blockers / external dependencies

| Blocker | Owner | Impact |
| ------- | ----- | ------ |
| `controller-runtime-common` version compatible with Go 1.25.7 / controller-runtime 0.22.4 | Operator maintainer | Phase 1–2 |
| Upstream SPIRE fork patches (server gRPC, federation, Prometheus, OIDC, agent) | Release repo / SPIRE fork | Phase 4 — FR-011, SC-002 |
| spire-controller-manager webhook TLS env/config patch | Release repo | Phase 4 — ctrl-mgr webhook :9443 |
| Operand image rebuild + digest pin | `zero-trust-workload-identity-manager-release` | End-to-end compliance |
| `TLSGroupPreferences` / APIServer `groups` field | Platform (Phase 4 future) | Deferred per specs A-007 |

## 5. Implementation phases (logical sequence; NOT tasks)

### Phase 1: Foundation — dependency, RBAC, APIServer cache, shared TLS package

- **Goal:** Enable secure read of cluster APIServer TLS configuration and establish shared Layer 2 resolution helpers without changing runtime TLS behavior yet.
- **Dependencies:** None (first phase).
- **Target files:**
  - `go.mod`, `go.sum`, `vendor/` — add `github.com/openshift/controller-runtime-common`
  - `config/rbac/role.yaml` — APIServer read RBAC
  - `pkg/client/client.go` — register `configv1.APIServer{}` in cache/informer lists
  - `pkg/controller/tls/tls.go` — **create** profile/adherence resolution + injection gate logic
  - `cmd/zero-trust-workload-identity-manager/main.go` — register `configv1.Install(scheme)` only (minimal; full Layer 1 in Phase 2)
- **Required capabilities:** OperatorController_Agent, RBACSecurity_Agent, API_Agent (scheme registration only)
- **Verification hooks:**
  - Unit: `pkg/controller/tls/tls_test.go` — adherence matrix, profile mapping, unknown adherence → strict
  - `make verify && make test`
  - Maps: FR-013, FR-004 (gate logic), specs edge cases for adherence

### Phase 2: Operator Layer 1 — metrics/webhook TLS profile + process restart

- **Goal:** Operator own TLS endpoints honor cluster profile; operator restarts on profile/adherence change (FR-001, FR-002, FR-012).
- **Dependencies:** Phase 1 complete (dependency, RBAC, scheme, tls helpers).
- **Target files:**
  - `cmd/zero-trust-workload-identity-manager/main.go` — `NewTLSConfigFromProfile` for metrics/webhook `TLSOpts`; `SecurityProfileWatcher` cancels root context
  - `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml` — `tls-profiles: "true"`
  - Regenerate: `make bundle`
- **Required capabilities:** OperatorController_Agent, OLMRelease_Agent, RBACSecurity_Agent
- **Verification hooks:**
  - Unit: main/bootstrap tests if feasible; otherwise manual cluster probe documented
  - Manual: handshake operator metrics `:8443` under Intermediate + Modern profiles (Story 1, SC-001)
  - tls-scanner CI scope: operator endpoints only (specs A-006 Phase 1)
  - `make verify && make test`

### Phase 3: Layer 2 — central profile injection into operand ConfigMaps

- **Goal:** When `tlsAdherence=StrictAllComponents` and `requirePQKEM` absent, inject resolved profile into all managed operand configs; hash change triggers rolling restart (FR-003, FR-009, FR-010 partial).
- **Dependencies:** Phase 1 tls helpers + APIServer cache; Phase 2 not strictly blocking but recommended for end-to-end strict-adherence story.
- **Target files:**
  - `pkg/controller/spire-server/configmap.go` — `generateServerConfMap`, `generateSpireControllerManagerConfigYaml`
  - `pkg/controller/spire-agent/configmap.go` — agent config generator
  - `pkg/controller/spire-oidc-discovery-provider/configmaps.go` — `generateOIDCConfigMapFromCR`
  - Verify rollout: `pkg/controller/spire-server/statefulset.go`, `pkg/controller/spire-agent/daemonset.go`, `pkg/controller/spire-oidc-discovery-provider/deployments.go`
- **Required capabilities:** OperatorController_Agent, Testing_Agent
- **Verification hooks:**
  - Unit: extend `configmaps_test.go` in server/agent/OIDC — assert injected fields + hash change when profile inputs change
  - Integration: reconcile with fake APIServer objects in envtest
  - Manual: ConfigMap content inspection before upstream patches (injection correctness without live handshake)
  - Maps: Story 2, FR-004 (non-strict → no injection), SC-002 (pending Phase 4 for live handshakes)

### Phase 4: Upstream operand configurable TLS (cross-repo)

- **Goal:** Operand binaries read injected TLS/PQC settings at runtime (FR-011); enable live handshake compliance on server API :8081, federation :8443, server metrics :8082, OIDC :8443, ctrl-mgr webhook :9443.
- **Dependencies:** Phase 3 operator injection stable; upstream fork access.
- **Target files (UNVERIFIED — out of tree; discovery in fork repos):**
  - `spire/pkg/server/endpoints/endpoints.go`, `bundle/server.go`, `pkg/common/telemetry/prometheus.go`
  - `spire/support/oidc-discovery-provider/main.go`
  - `spire-controller-manager/cmd/main.go`
  - Release repo: image digests, `images_digest.conf`, submodule bumps
- **Required capabilities:** Operator maintainer + release repo owners (no single agents.md agent — coordinate manually)
- **Verification hooks:**
  - Upstream unit tests in fork repos
  - ZTWIM e2e: handshake per endpoint class under strict + Intermediate/Modern/Custom (Story 2, SC-002, SC-003)
  - Operand image bump in test cluster before e2e

### Phase 5: `requirePQKEM` API, propagation, warning events

- **Goal:** Single ZTWIM flag enables strict hybrid PQC across all operands; overrides central injection; emits warning when combined with strict adherence (FR-005–FR-008).
- **Dependencies:** Phase 1 tls precedence helpers; Phase 3 config generators; Phase 4 for live PQC handshake validation.
- **Target files:**
  - `api/v1alpha1/zero_trust_workload_identity_manager_types.go` — `RequirePQKEM *bool`
  - `config/crd/bases/` — via `make manifests`
  - `bundle/` — via `make bundle`
  - Config generators (same as Phase 3) — PQC branch
  - `pkg/controller/spire-server/controller.go`, `pkg/controller/spire-agent/controller.go` — Warning event
- **Required capabilities:** API_Agent (must complete before controller), OperatorController_Agent, OLMRelease_Agent, Testing_Agent
- **Verification hooks:**
  - Unit: precedence matrix tests (PQ vs profile vs default); event emitted when PQ + strict
  - Manual/`openssl s_client`: X25519MLKEM768 when enabled; failure for non-PQC client (Story 4, SC-004, SC-006)
  - `ZTWIMSpecChangedPredicate` triggers operand reconcile on toggle
  - `make generate manifests bundle verify test`

### Phase 6: Conformance verification and release qualification

- **Goal:** Institutionalize tls-scanner and profile-matrix validation (Story 5, SC-007, SC-008).
- **Dependencies:** Phases 2–5 functionally complete on test cluster.
- **Target files:**
  - `test/e2e/` — add TLS compliance scenarios (UNVERIFIED exact layout — follow existing Ginkgo patterns)
  - CI/release pipeline configs (UNVERIFIED — may live in `openshift/release` or release repo)
- **Required capabilities:** Testing_Agent, Docs_Agent (runbook for release gate)
- **Verification hooks:**
  - CI: tls-scanner on operator endpoints per phase (specs A-006)
  - Release gate: Old/Intermediate/Modern/Custom matrix manual qualification before GA
  - PQC scan when `requirePQKEM=true` (Story 5 scenario 3)

### Phase 7 (future — out of current scope): Central profile TLS groups

- **Goal:** Inject APIServer profile `groups` when `TLSGroupPreferences` feature gate graduates (specs Phase 4 / A-007).
- **Dependencies:** `openshift/api` PR #2583 vendored; platform feature gate GA.
- **Target files:** `pkg/controller/tls/tls.go` + config generators — extend when platform API available.
- **Required capabilities:** OperatorController_Agent, API_Agent
- **Verification hooks:** Deferred — document in release notes as follow-up milestone.

## 6. Verification matrix (maps to spec acceptance)

| Category | Coverage | Files / Suites | Spec mapping |
| -------- | -------- | -------------- | ------------ |
| Unit | Profile resolution, adherence gate, ConfigMap injection, precedence matrix, hash change | `pkg/controller/tls/tls_test.go`, `pkg/controller/spire-server/configmaps_test.go`, `pkg/controller/spire-agent/configmap_test.go`, `pkg/controller/spire-oidc-discovery-provider/configmaps_test.go` | FR-003–008, FR-004, Story 2–4 |
| Integration | Reconcile with APIServer + ZTWIM in envtest | Operand controller tests with fake APIServer | FR-009, Story 3 |
| E2E | Full stack handshake / tls-scanner on OpenShift | `test/e2e/` (to be extended) | SC-001–SC-004, SC-008, Story 1–2 |
| Manual / Cluster | Operator restart on profile change; tls-scanner release matrix; PQC negative test | Cluster admin runbook; `openssl s_client` probes | FR-002, SC-003, SC-006, SC-007, Story 3–5 |
| N/A | CSI driver Unix socket path | `pkg/controller/spiffe-csi-driver/` — no TLS servers | FR-0010 out of scope |
| N/A | TLS client configuration | Not modified | specs Out of Scope |
| N/A | Phase 7 groups | Deferred until platform API | A-007 |

## 7. Risks, migrations, and operational follow-ups

- **Upgrade/migration:** No CR migration required — new optional `requirePQKEM` field is backward compatible. Default clusters (Intermediate + non-strict) must show zero behavior change (FR-014, SC-005). Clusters enabling strict adherence will trigger operand rollouts on first reconcile after upgrade.
- **Compatibility (OpenShift/MicroShift/Hypershift):** Feature depends on `apiserver.config.openshift.io/cluster` and `TLSAdherence` feature gate. Verify on target OCP 4.23+; Hypershift control-plane TLS profile semantics UNVERIFIED — escalate if HCPS differs.
- **Upstream API drift risks:** SPIRE config schema for `experimental.require_pq_kem` tied to upstream version in RELATED_IMAGE; pin and test on image bump.
- **Rolling restart blast radius:** Profile or PQC toggle causes operator then operand restarts — brief SPIRE API unavailability (ADR risk table). Mitigation: PDB, document maintenance window for `requirePQKEM` enablement.
- **Create-only mode:** `CREATE_ONLY_MODE=true` skips ConfigMap updates — TLS changes won't apply; existing `Upgradeable=false` behavior applies.
- **Cipher mapping gaps:** Unsupported OpenShift cipher names silently skipped — log warnings (repo-assessment §11).
- **PQC opt-in risk:** Enabling `requirePQKEM` before all agents on Go 1.24+ causes handshake failures — document upgrade ordering (specs A-004).
- **Human approval gates:** Broadening RBAC beyond read-only APIServer requires explicit approval per constitution.

## 8. Open questions / SME decisions

| # | Question | Owner | Plan assumption if unresolved |
| - | -------- | ----- | ----------------------------- |
| 1 | Which `controller-runtime-common` release tag matches OCP 4.23 / controller-runtime 0.22.4? | Operator maintainer | Use same version as reference operator (cluster-control-plane-machine-set-operator) — verify in Phase 1 before merge |
| 2 | Are upstream SPIRE fork patches already started on `openshift/spiffe-spire` for configurable TLS? | Release repo / SPIRE maintainers | Phase 4 proceeds in fork in parallel; operator Phase 3 delivers injection-only value for config inspection tests |
| 3 | Exact ctrl-mgr webhook TLS config surface (env vs YAML) after upstream patch? | SPIRE ctrl-mgr maintainers | Propagate via existing `generateSpireControllerManagerConfigYaml` + env vars as ADR specifies |
| 4 | Where does tls-scanner run in CI — this repo, release repo, or openshift/release? | Platform security engineer | Phase 6 adds e2e hooks in `test/e2e/`; release qualification gate is manual until CI owner confirms |
| 5 | OIDC Deployment config-hash annotation — same pattern as agent? | Operator maintainer | Verify in Phase 3 `deployments.go`; align with server/agent if gap found |
| 6 | Hypershift / hosted control plane TLS profile visibility to operands | Platform SME | Assume same APIServer object semantics as standard OCP unless HCPS testing proves otherwise |

None of the above block Phase 1–3 operator-only work; items 2 and 4 block GA qualification (SC-007, SC-008).
