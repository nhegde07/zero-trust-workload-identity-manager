# Technical Implementation Plan

**Feature:** Cluster-Wide TLS Profile Compliance and Hybrid Post-Quantum Key Exchange (OCPSTRAT-2611)

## 0. Inputs acknowledged

| Input | Status |
|-------|--------|
| Spec source | `specs.md` — OCPSTRAT-2611 (+ OCPSTRAT-3145 PQC scope, OCPSTRAT-3123 deferred) |
| Repo assessment pin | `https://github.com/nhegde07/zero-trust-workload-identity-manager`, branch `OAPE-859`, commit `2a5a19ef3c2047d20039cf2359da5eafdbe4d2d1` (tooling_status: OK) |
| `agents.md` | PROVIDED — `openspec/inputs/agents.md` |
| `spec_validator_results.json` | PROVIDED — `validation.json` (PASS 88%) |
| `constitution.md` | PROVIDED — `openspec/inputs/constitution.md` (AgentRoutingMode: PROVIDED) |
| **User feedback (plan round 1)** | Reject — manual tls-scanner only; use library-go + controller-runtime-common TLS helpers; no warning events; upstream patches/images out of scope; vendor via `make vendor` only |

**Agent routing (from constitution):** API_Agent → OperatorController_Agent → RBACSecurity_Agent / OLMRelease_Agent → Testing_Agent. **Scope limited to this operator repo only.**

**Dependency policy (user feedback):** Add modules via `go.mod` require only; run `make vendor` to populate `vendor/`. **Do not hand-edit `vendor/`.** Avoid `replace` directives in `go.mod` unless strictly necessary.

## 1. Architectural strategy

This feature introduces a **two-layer TLS architecture** within the ZTWIM operator repository:

1. **Layer 1 (Direct):** The operator binary applies the cluster APIServer TLS security profile to its own TLS servers (metrics `:8443`, admission webhook) using **`github.com/openshift/controller-runtime-common/pkg/tls`** (`NewTLSConfigFromProfile`, `SecurityProfileWatcher`). On profile or adherence policy change, the watcher cancels manager context → graceful process restart.
2. **Layer 2 (Injected):** Operand reconcilers resolve effective TLS configuration and inject values into SPIRE operand ConfigMaps. **Reuse existing library helpers** — do not reimplement APIServer fetch/parse logic:
   - **`fetchTLSProfile`** and **`fetchTlsAdherence`** from **`github.com/openshift/library-go`** (and related TLS utilities from **`controller-runtime-common/pkg/tls`** where applicable)
   - Thin local wrapper in `pkg/controller/tls/` **only** for ZTWIM-specific precedence (`requirePQKEM` override vs central profile vs defaults)

Existing config-hash annotations on StatefulSet/DaemonSet/Deployment pod templates trigger operand rolling restarts — no new restart mechanism.

**Precedence model (binding):**

```
requirePQKEM=true  →  SPIRE experimental.require_pq_kem (TLS 1.3 + X25519MLKEM768 only); NO central profile injection
requirePQKEM=false/absent + StrictAllComponents  →  inject min TLS + ciphers from APIServer profile (via library helpers)
requirePQKEM=false/absent + non-strict adherence  →  Intermediate defaults; Go 1.24+ opportunistic PQC
```

**No Kubernetes events** are emitted for PQC vs strict-adherence coexistence (user decision — overrides specs FR-008 for this implementation; precedence is silent/config-driven only).

**Repo-grounded reality check:** On branch `OAPE-859` @ `2a5a19e`, TLS compliance is **100% greenfield** in this repo: no `controller-runtime-common` or library-go TLS helper usage today, no APIServer in cache/RBAC, no `requirePQKEM`, CSV `tls-profiles: "false"`. All operator-repo phases are net-new.

**Explicit out of scope (this change / this repo):**

- Upstream SPIRE fork patches, operand image builds, image digest bumps, release-repo submodule updates
- Automated tls-scanner CI jobs or e2e automation for tls-scanner
- TLS key exchange **groups** via central profile when `TLSGroupPreferences` graduates (OCPSTRAT-3123)
- Hand-editing `vendor/` or unnecessary `replace` directives in `go.mod`

Operand runtime TLS behavior depends on **currently shipped SPIRE images**; this plan covers operator-side profile resolution, injection, and operator TLS compliance only.

## 2. Persistence & state

**Kubernetes objects (source of truth):**

| Object | Role |
|--------|------|
| `APIServer/cluster` (`config.openshift.io`) | Cluster TLS profile + adherence — read-only via library helpers |
| `ZeroTrustWorkloadIdentityManager/cluster` | Parent CR; gains optional `spec.requirePQKEM` |
| Operand CRs | Unchanged; TLS via injected ConfigMaps only |
| Operator-generated ConfigMaps | SPIRE configs carrying TLS/PQC settings |
| StatefulSet/DaemonSet/Deployment | Config-hash annotations drive rollout |

**Operator process state:** No hot-reload (FR-013). Profile change → operator restart → reconcile → ConfigMap update → hash change → operand rolling update.

## 3. Interfaces & contracts (operator-native)

### 3.1 Kubernetes APIs (CRDs/CRs)

| CRD | Change |
|-----|--------|
| `ZeroTrustWorkloadIdentityManager` | Add `spec.requirePQKEM *bool` (optional, singleton `cluster`) |
| All other operator CRDs | No TLS-specific fields |

APIServer access: read-only `get/list/watch` on `apiservers.config.openshift.io`.

### 3.2 Controller/runtime interfaces (internal)

**Thin wrapper:** `pkg/controller/tls/`

| Responsibility | Implementation approach |
|--------------|----------------------|
| Fetch TLS profile | Delegate to **library-go `fetchTLSProfile`** (and controller-runtime-common as needed) |
| Fetch TLS adherence | Delegate to **library-go `fetchTlsAdherence`** |
| Apply precedence (`requirePQKEM` vs profile) | Local `ResolveEffectiveTLSConfig(ctx, client, ztwim)` — precedence only, no duplicate fetch logic |
| Layer 1 operator TLS | controller-runtime-common `NewTLSConfigFromProfile`, `SecurityProfileWatcher` |

**Modified call sites:**

- `cmd/zero-trust-workload-identity-manager/main.go`
- `pkg/controller/spire-server/configmap.go`
- `pkg/controller/spire-agent/configmap.go`
- `pkg/controller/spire-oidc-discovery-provider/configmaps.go`

**Not modified for events:** reconciler `controller.go` files — no Warning event emission.

### 3.3 Webhooks / admission (if applicable)

- Operator admission webhook receives Layer 1 TLS profile via `TLSOpts` in `main.go`.
- SPIRE controller-manager webhook TLS: **out of scope** — no upstream patch or image change in this change.

### 3.4 RBAC / security boundaries (if applicable)

| Permission | Justification |
|------------|---------------|
| `apiservers.config.openshift.io` `get/list/watch` | Read cluster TLS profile (FR-010) |

Read-only. No new cluster-scoped writes for TLS.

### 3.5 Packaging / OLM (if applicable)

| Change | Location |
|--------|----------|
| `features.operators.openshift.io/tls-profiles: "true"` | CSV base |
| Regenerated bundle | `make bundle` |

Dependencies: `go.mod` require + `make vendor` — no manual vendor edits.

## 4. Dependencies & sequencing graph

**Critical path (operator repo only):**

```
go.mod require (controller-runtime-common, library-go TLS deps) → make vendor
  → APIServer RBAC + cache registration
  → Layer 1 operator TLS (controller-runtime-common)
  → pkg/controller/tls precedence wrapper (library-go fetch helpers)
  → Layer 2 ConfigMap injection (server, agent, OIDC)
  → requirePQKEM API + PQC config injection
  → Unit tests (make test)
  → Manual tls-scanner validation on test cluster (no CI job)
  → Bundle + CSV annotation
```

**Parallelizable workstreams:** None outside this repo — single operator-repo stream.

**Explicit blockers:**

- None cross-repo. Full six-endpoint manual tls-scanner sign-off requires test cluster with strict adherence and **existing** operand images (no image bump in this change).

## 5. Implementation phases (logical sequence; NOT tasks)

### Phase 1: Foundation — dependencies, RBAC, cache, library-backed TLS resolution

- **Goal:** Add dependencies via `go.mod` + `make vendor`; enable APIServer read; expose precedence wrapper using library-go fetch helpers. FR-010; prerequisite for FR-001, FR-002, FR-014.
- **Dependencies:** None.
- **Target files:**
  - `go.mod`, `go.sum` → `make vendor` (no `replace` unless unavoidable; **never hand-edit vendor/**)
  - `pkg/client/client.go`
  - `config/rbac/role.yaml`
  - `pkg/controller/tls/tls.go` (new — thin wrapper over library-go + precedence)
  - `pkg/controller/tls/tls_test.go` (new)
  - `cmd/zero-trust-workload-identity-manager/main.go` (scheme registration)
- **Required capabilities:** OperatorController_Agent, RBACSecurity_Agent
- **Verification hooks:** `go test ./pkg/controller/tls/...`; `make verify`; mock/stub library outputs for precedence matrix tests

### Phase 2: Layer 1 — operator metrics and webhook TLS profile compliance

- **Goal:** Operator TLS servers honor cluster profile; restart on change. FR-001, FR-003 (operator), FR-004, FR-013.
- **Dependencies:** Phase 1.
- **Target files:**
  - `cmd/zero-trust-workload-identity-manager/main.go`
- **Required capabilities:** OperatorController_Agent, RBACSecurity_Agent
- **Verification hooks:** `make test && make verify`; **manual** tls-scanner/openssl on metrics `:8443` after deploy (no CI automation)

### Phase 3: Layer 2 — central TLS profile injection into SPIRE operand configs

- **Goal:** Inject min TLS + ciphers into server, agent, OIDC ConfigMaps when strict adherence and no PQC override. FR-002, FR-003 (operands), FR-011 (config injection), FR-015 (operator injection path only).
- **Dependencies:** Phase 1.
- **Target files:**
  - `pkg/controller/spire-server/configmap.go`
  - `pkg/controller/spire-agent/configmap.go`
  - `pkg/controller/spire-oidc-discovery-provider/configmaps.go`
  - Existing `*_test.go` in those packages
- **Required capabilities:** OperatorController_Agent, Testing_Agent
- **Verification hooks:** Table-driven config generation unit tests; **manual** tls-scanner on cluster endpoints with current operand images

### Phase 4: PQC opt-in — `requirePQKEM` API and SPIRE experimental config

- **Goal:** ZTWIM CR flag; inject `experimental.require_pq_kem`; suppress central profile injection when true. **No events.** FR-005, FR-006, FR-007 (FR-008 explicitly not implemented per user feedback).
- **Dependencies:** Phases 1, 3.
- **Target files:**
  - `api/v1alpha1/zero_trust_workload_identity_manager_types.go`
  - `config/crd/bases/` (via `make manifests`)
  - `pkg/controller/spire-server/configmap.go`
  - `pkg/controller/spire-agent/configmap.go`
- **Required capabilities:** API_Agent → `make generate && make manifests && make verify` before controller work; OperatorController_Agent; Testing_Agent
- **Verification hooks:** Unit tests for experimental block + omitted profile fields; **manual** openssl/tls-scanner PQC check (SC-004) — no CI job

### Phase 5: OLM packaging and manual validation sign-off

- **Goal:** CSV `tls-profiles: "true"`; operator-repo complete. FR-009. Manual cluster validation per specs SC-001–SC-007 using tls-scanner (operator + operands on **existing** images).
- **Dependencies:** Phases 1–4.
- **Target files:**
  - `config/manifests/bases/zero-trust-workload-identity-manager.clusterserviceversion.yaml`
  - `bundle/` (generated)
- **Required capabilities:** OLMRelease_Agent
- **Verification hooks:**
  - `make bundle`; CSV annotation check
  - **Manual only:** tls-scanner per profile (Old, Intermediate, Modern, Custom); profile migration observation (SC-003); upgrade readiness (SC-007)
  - **No** new CI job, **no** e2e test suite changes for tls-scanner

## 6. Verification matrix (maps to spec acceptance)

| Category | Coverage | Files / Suites |
|----------|----------|----------------|
| Unit | Precedence wrapper; ConfigMap TLS/PQC generation; profile fallback | `pkg/controller/tls/tls_test.go`, `pkg/controller/spire-server/configmaps_test.go`, agent/OIDC configmap tests |
| Integration | Reconciler + config hash annotation updates | `pkg/controller/spire-server/controller_test.go`, `pkg/controller/spire-agent/controller_test.go` |
| E2E | N/A — no tls-scanner e2e or CI automation added | User feedback: manual tls-scanner only |
| Manual / Cluster | **Primary GA acceptance:** tls-scanner/openssl on six endpoints; PQC negotiation; custom cipher denial; profile migration | Test cluster checklist — **not** automated in CI |
| N/A | FR-008 warning events | Not implemented per user feedback |
| N/A | Upstream SPIRE patches / image bumps | Out of scope |
| N/A | TLS groups (OCPSTRAT-3123) | Deferred |
| N/A | FR-012 client TLS | Out of scope |

**FR → verification mapping:**

| FR | Phase | Verification |
|----|-------|--------------|
| FR-001 | 2 | Manual tls-scanner operator metrics |
| FR-002 | 3 | Unit config tests + manual operand endpoints |
| FR-003 | 2, 3 | Manual profile change → restart |
| FR-004 | 2, 3 | Manual non-strict regression |
| FR-005–FR-007 | 4 | Unit + manual PQC |
| FR-008 | — | **Not implemented** (user feedback) |
| FR-009 | 5 | CSV inspection |
| FR-010 | 1 | RBAC + unit tests |
| FR-011 | 3 | Unit injection + manual scan (existing images) |
| FR-012 | — | N/A |
| FR-013 | 2, 3 | Manual |
| FR-014 | 1, 3 | Unit fallback |
| FR-015 | 3 | Unit injection only (operator repo) |
| FR-016 | 1, 3 | Unit opportunistic path |

## 7. Risks, migrations, and operational follow-ups

- **Upgrade/migration:** No behavior change for Intermediate/non-strict (FR-004). Optional `requirePQKEM` defaults unset.
- **Spec divergence:** FR-008 (warning events) and specs SC-006 not met — accepted per user feedback; document in release notes if product requires later.
- **Operand image gap:** Without upstream SPIRE TLS config patches, injected ConfigMap fields may be ignored by current operand binaries — manual tls-scanner may show partial compliance until separate upstream work lands (out of this change).
- **CREATE_ONLY_MODE:** Profile updates skipped in create-only — existing condition applies.
- **Cipher mapping:** Unsupported cipher names skipped with log warning.
- **Dependency hygiene:** All deps via `go.mod` + `make vendor`; avoid `replace` unless required for version resolution.

## 8. Open questions / SME decisions

| # | Question | Owner | Default if unresolved |
|---|----------|-------|----------------------|
| 1 | Exact module versions for `controller-runtime-common` and library-go TLS helpers (align with CPMSO reference in ADR)? | Operator SME | Match cluster-control-plane-machine-set-operator dependency set |
| 2 | Is `replace` in `go.mod` needed for any transitive conflict, or pure `require` sufficient? | Operator SME | Pure `require` + `make vendor`; escalate only if `go mod tidy` fails |
| 3 | Hypershift APIServer TLS profile visibility from operator pod? | Platform SME | Assume same as self-managed; one manual cluster check pre-GA |

**Resolved by user feedback (not open):**

- tls-scanner: **manual only**, no CI job
- Warning events on PQC + strict adherence: **do not implement**
- Upstream SPIRE patches / image bumps: **out of scope**
- Vendor updates: **`make vendor` only**, no direct vendor edits

None of the remaining open questions block Phase 1–4 operator-repo implementation.
