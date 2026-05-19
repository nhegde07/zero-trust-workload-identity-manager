# Test Plan: Native UpstreamAuthority Plugin Support for ZTWIM

**Sources:** ADR: `zADR_ Native UpstreamAuthority Plugin Support for ZTWIM.md` (workspace); Jira: [SPIRE-129](https://redhat.atlassian.net/browse/SPIRE-129) (fetched via REST)

**Date:** 2026-05-19

**Scope:** Add optional `upstreamAuthority` on `SpireServer` so SPIRE intermediate CA can chain to cert-manager or Vault (`k8s_auth`), with reconcile-driven ConfigMap/StatefulSet/RBAC and rollback to self-signed when removed.

---

## Source conflicts

**None.** Jira acceptance criteria (extend API, reconcile logic, unit tests) are a **subset** of the ADR. The test plan uses the ADR for full functional, integration, E2E, manual, and non-functional coverage; Jira items map to REQ-001, REQ-015–REQ-017 and existing reconcile/API REQ rows without contradicting the ADR.

**Jira snapshot (SPIRE-129):**

| Field | Value |
| --- | --- |
| Summary | Add support for Upstream Authority in ZTWIM |
| Type | Story |
| Status | Review |
| Labels | oap-plan |
| Acceptance Criteria | Extend the API to support upstream authority; Add Reconcile logic to support the updation; Add unit tests for same |

---

## ADR Decomposition

**Feature:** Add native UpstreamAuthority plugin support so SPIRE intermediate CA can be signed by external PKI (cert-manager or Vault `k8s_auth`).

**ADR status / version:** Accepted per Reviews table (Apr 2026); SPIRE 1.13.x; ZTWIM v1.0.0+.

**Components in scope:** `SpireServer` CR API (`upstreamAuthority`), reconciliation (validation, ConfigMap `server.conf`, StatefulSet volumes/mounts, cert-manager Role/RoleBinding in target namespace), status conditions (`ConfigurationValid`, `Available`), config-hash-driven rolling update. No new controllers/webhooks/CRDs.

**Positive-path requirements (from Goals):**

1. Declarative `upstreamAuthority` on `SpireServer` — no manual ConfigMap edits for plugin config.
2. cert-manager plugin with in-cluster client; Issuer and ClusterIssuer support.
3. Vault plugin with `k8s_auth` only; projected SA token path injection.
4. External Vault with TLS verification via referenced CA/material as designed.
5. cert-manager CertificateRequest RBAC created when enabled and deleted when plugin removed from CR.
6. Rolling update when upstream config changes via existing config hash annotation on StatefulSet.
7. `ConfigurationValid` and `Available` reflect misconfiguration vs runtime/upstream failure paths per ADR.
8. Removing `upstreamAuthority` removes plugin block, removes plugin volumes, deletes cert-manager RBAC, rolls server; SPIRE returns to self-signed CA; bundle accumulates old + new trust.

**Explicit non-goals (verbatim bullets):**

- Nested SPIRE (spire UpstreamAuthority plugin) — separate design.
- Auto-rotation of Vault tokens — manual pod restart; future.
- Other Vault auth methods (cert_auth, token_auth, app_role_auth) — out of scope.
- Cross-cluster cert-manager kubeconfig — descoped.

**Scope boundaries (from Non-Goals):** Same as above; tests must not require nested SPIRE, Vault token auto-rotation, unsupported Vault auth modes, or cross-cluster cert-manager control plane.

**Implementation details requiring test coverage (from How):**

- CEL/admission: exactly one of cert-manager vs Vault; Vault auth method constraints; referenced Secrets exist in operator namespace before proceeding.
- Validation failure → `ConfigurationValid=False`, **no** generation of broken ConfigMap.
- cert-manager path: RBAC for CRUD `CertificateRequest` in configured namespace; cleanup on removal.
- ConfigMap: UpstreamAuthority block; **no** credentials embedded in ConfigMap.
- StatefulSet: conditional projected token volumes and mounts for `k8s_auth`; hash annotation bump → rollout.
- Reconcile does **not** proactively validate Vault reachability or Issuer readiness; failures surface via pod health / `Available`.
- Removal and migration paths including CREATE_ONLY_MODE migration narrative.

**Risks requiring test coverage:**

- Vault/Kubernetes auth operational risks (token reviewer JWT, CA bundle drift, multi-cluster Vault mounts, deferred failure windows, mTLS limitation on TokenReview, operational complexity, ca_ttl vs max-lease-ttl).
- cert-manager not installed / Issuer missing → no signing.
- External Vault unreachable / TLS / DNS → signing blocked.

**Open questions / areas needing exploratory coverage:**

- Documented windows where health check stays green while Vault auth degrades (cached token); exploratory timing/TTL scenarios.

---

## Testable Requirements

| ID | Requirement | Category | ADR Source |
| --- | --- | --- | --- |
| REQ-001 | SpireServer exposes optional `upstreamAuthority` with schema enforcing mutual exclusion and supported plugin/auth shapes at admission | Functional | Goals §1, How, Mitigations |
| REQ-002 | With cert-manager plugin, server ConfigMap contains correct UpstreamAuthority issuer reference (name, kind, group, namespace per API) | Functional | Goals §2, How §3 |
| REQ-003 | With Vault plugin, server ConfigMap contains Vault PKI + `k8s_auth` block; credentials not in ConfigMap | Functional | Goals §3–§4, How §3–§4 |
| REQ-004 | Referenced TLS CA Secrets for external Vault exist and are validated before generating manifests | Negative / Functional | How §1 |
| REQ-005 | cert-manager Role/RoleBinding for CertificateRequest lifecycle: created when plugin enabled; removed when plugin removed from CR | Functional | Goals §5, How §2 |
| REQ-006 | StatefulSet receives projected SA token volumes/mounts for Vault `k8s_auth` when configured | Functional | How §4 |
| REQ-007 | Config hash annotation changes on upstream config change and triggers rolling update | Functional | Goals §6, How §4 |
| REQ-008 | `ConfigurationValid=False` with message on invalid spec; operator does not emit broken ConfigMap/StatefulSet patch | Negative | How §1, Mitigations |
| REQ-009 | `ConfigurationValid=True` when syntactically valid; `Available` tracks pod health including upstream activation failures | Functional | How conditions |
| REQ-010 | Remove `upstreamAuthority`: ConfigMap plugin removed, volumes removed, cert-manager RBAC deleted, rollout, self-signed CA behavior | Functional | Goals §8, Removal |
| REQ-011 | CR without `upstreamAuthority` unchanged behavior (self-signed); backward compatible | Regression | Migration §115 |
| REQ-012 | Invalid combinations rejected at API (e.g. both plugins); aligns with guard-rail vs rejected manual ConfigMap approach | Regression / Negative | Alternatives, How |
| REQ-013 | Upstream/external PKI failure modes (issuer missing, Vault unreachable) eventually reflected via pod restart / `Available` | Negative / Operational | Risks §8–§9, How |
| REQ-014 | Documented security/operational risks (token reviewer JWT exposure class, CA bundle rotation on Vault, ca_ttl mismatch) have documented QE/security verification where testable | Security / Operational | Risks §1–§7 |
| REQ-015 | **Jira SPIRE-129:** Extend API for upstream authority | Functional | Jira AC |
| REQ-016 | **Jira SPIRE-129:** Reconcile logic applies updates to owned resources for upstream authority | Functional | Jira AC |
| REQ-017 | **Jira SPIRE-129:** Unit tests cover new validation and reconcile helpers | Functional | Jira AC |

---

## Test Cases

### Tier 1: Unit Tests

#### UT-001: Validation — mutually exclusive plugins

**Priority:** Critical  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-001, REQ-008, REQ-012  
**Traceability:** ADR How §1 (validation); Mitigations admission validation; Jira AC extend API  

**Preconditions:** Go test harness for SpireServer spec validation / webhook or marker-generated validation helpers.

**Steps:**

1. Build spec structs with **both** `certManager` and `vault` populated (or illegal composite per API rules).
   - **Expected:** Validator returns error; no “success” path that would generate ConfigMap.

2. Build spec with Vault plugin but missing required `k8s_auth` fields (per API rules).
   - **Expected:** Validation error with stable message substring or reason code matching API contract.

**Cleanup:** None  

**Failure Impact:** Invalid configs reach cluster → broken installs or security gaps.

---

#### UT-002: Config builder — cert-manager UpstreamAuthority fragment

**Priority:** High  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-002  
**Traceability:** ADR How §3 ConfigMap generation  

**Preconditions:** Unit-testable function that renders SPIRE server config from SpireServer spec.

**Steps:**

1. Input valid `upstreamAuthority.certManager` with Issuer name/kind/namespace.
   - **Expected:** Rendered config contains UpstreamAuthority plugin stanza with issuer reference; no Secret material embedded.

2. Input ClusterIssuer variant per API enum/kind rules.
   - **Expected:** Correct group/kind/namespace fields in output string or structured map.

**Cleanup:** None  

**Failure Impact:** SPIRE misconfigured → CA chain breaks.

---

#### UT-003: Config builder — Vault `k8s_auth` fragment

**Priority:** High  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-003, REQ-006  
**Traceability:** ADR How §3–§4  

**Steps:**

1. Input valid Vault upstream spec with `k8s_auth` paths and Vault PKI mount URL fields per API.
   - **Expected:** Config contains Vault plugin + auth file paths; no raw tokens in ConfigMap.

**Cleanup:** None  

**Failure Impact:** Auth misconfiguration or secret leakage in ConfigMap.

---

#### UT-004: RBAC object naming and rules — CertificateRequest

**Priority:** High  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-005  
**Traceability:** ADR How §2  

**Steps:**

1. Call reconcile helper that synthesizes Role for cert-manager namespace `N`.
   - **Expected:** Rules include create/get/list/delete on `cert-manager.io/v1` `CertificateRequest` (API group/version per implementation); subject is SPIRE server ServiceAccount.

**Cleanup:** None  

**Failure Impact:** SPIRE cannot obtain intermediate CA via cert-manager.

---

#### UT-005: Hash annotation bump on upstream spec change

**Priority:** Medium  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-007  
**Traceability:** ADR Goals §6, How §4  

**Steps:**

1. Compute config hash for baseline SpireServer without upstream vs with upstreamAuthority block change.
   - **Expected:** Hash differs when upstreamAuthority content differs; identical specs produce identical hash.

**Cleanup:** None  

**Failure Impact:** No rollout on PKI config change → stale SPIRE config.

---

#### UT-006: Jira — unit coverage gate for new packages

**Priority:** High  
**Methodology:** White box  
**Relevant Requirement(s):** REQ-017, REQ-015  
**Traceability:** Jira SPIRE-129 AC “Add unit tests”  

**Steps:**

1. Run `go test ./...` scoped to packages touching upstreamAuthority (controller, API validation).
   - **Expected:** All tests pass; new exported helpers have direct unit tests (not only indirect coverage).

**Cleanup:** None  

**Failure Impact:** Regression risk on merge.

---

### Tier 2: Integration Tests

#### INT-001: envtest reconcile — validation failure sets condition, no ConfigMap update

**Priority:** Critical  
**Methodology:** Grey box  
**Relevant Requirement(s):** REQ-008, REQ-009  
**Traceability:** ADR How §1, What conditions  

**Preconditions:** envtest with CRDs installed; controller running against fake API.

**Steps:**

1. Apply SpireServer with upstreamAuthority referencing a Secret name that does **not** exist in operator namespace.
   - **Expected:** SpireServer status shows `ConfigurationValid=False` with explanatory message; existing server ConfigMap Generation or ResourceVersion unchanged from “invalid transition” expectation per implementation (no partial broken UpstreamAuthority block).

**Cleanup:** Delete SpireServer and test namespace objects via envtest teardown.

**Failure Impact:** Cluster ends up with unloadable SPIRE config.

---

#### INT-002: envtest — cert-manager RBAC reconcile and teardown

**Priority:** High  
**Methodology:** Grey box  
**Relevant Requirement(s):** REQ-005, REQ-010  
**Traceability:** ADR How §2, Removal  

**Preconditions:** envtest; namespace `cm-ns` exists; valid minimal cert-manager upstream spec.

**Steps:**

1. Reconcile with cert-manager upstream enabled targeting `cm-ns`.
   - **Expected:** Role + RoleBinding objects exist in `cm-ns` binding SPIRE SA to CertificateRequest verbs.

2. Patch SpireServer to remove upstreamAuthority entirely; reconcile.
   - **Expected:** Role/RoleBinding removed from `cm-ns`; SpireServer proceeds toward self-signed configuration.

**Cleanup:** Delete all created objects.

**Failure Impact:** Privilege leakage or orphaned RBAC in customer namespaces.

---

#### INT-003: envtest — StatefulSet volume projections for Vault path

**Priority:** High  
**Methodology:** Grey box  
**Relevant Requirement(s):** REQ-003, REQ-006  
**Traceability:** ADR How §4  

**Steps:**

1. Reconcile with valid Vault `k8s_auth` upstream spec.
   - **Expected:** StatefulSet template contains projected volume for SPIRE server pod matching audience/path fields from API; volumeMounts reference projected mount paths.

**Cleanup:** Delete StatefulSet / CR via envtest.

**Failure Impact:** SPIRE cannot authenticate to Vault.

---

### Tier 3: E2E Automated Tests

#### E2E-001: Smoke — cluster without upstreamAuthority unchanged

**Priority:** Critical  
**Methodology:** Black box  
**Ginkgo Labels:** `install-health`, `reconciliation`  
**Relevant Requirement(s):** REQ-011  
**Traceability:** ADR Migration no breaking changes  

**Preconditions:** OpenShift/K8s with ZTWIM installed; SpireServer default without `upstreamAuthority`.

**Steps:**

1. `oc get spireserver cluster -n <ns> -o jsonpath='{.spec.upstreamAuthority}'`
   - **Expected:** Empty / absent field.

2. Wait for SpireServer conditions `ConfigurationValid=True` and operand pods Ready per existing suite patterns.
   - **Expected:** SPIRE server Running; workload identity smoke path still succeeds per existing test harness.

**Cleanup:** None if using shared CI cluster baseline.

**Failure Impact:** Regression on default self-signed path.

---

#### E2E-002: Negative admission — invalid CR rejected at API

**Priority:** Critical  
**Methodology:** Black box  
**Ginkgo Labels:** `negative-input-validation`  
**Relevant Requirement(s):** REQ-001, REQ-012  
**Traceability:** ADR Mitigations admission; How mutual exclusion  

**Steps:**

1. `oc apply -f spireserver-invalid-both-plugins.yaml` (manifest sets both cert-manager and vault blocks).
   - **Expected:** API server rejects with `Denied`/`Invalid` including field detail from CEL/OpenAPI validation.

**Cleanup:** No resources created on failure; delete partial if server accepts wrongly.

**Failure Impact:** Invalid PKI topology reaches etcd.

---

#### E2E-003: Lifecycle — enable cert-manager upstream (happy path)

**Priority:** High  
**Methodology:** Black box  
**Ginkgo Labels:** `reconciliation`, `configmap`, `olm-lifecycle-install`  
**Relevant Requirement(s):** REQ-002, REQ-005, REQ-007, REQ-009, REQ-016  
**Traceability:** ADR Goals §2,§5,§6; Jira reconcile AC  

**Preconditions:** cert-manager installed; Issuer or ClusterIssuer Ready; SpireServer patch prepared per API.

**Steps:**

1. Patch SpireServer to add valid `upstreamAuthority.certManager` referencing Ready issuer.
   - **Expected:** SpireServer `ConfigurationValid=True`; ConfigMap `server.conf` contains UpstreamAuthority cert-manager stanza matching issuer reference.

2. Observe StatefulSet pod template annotation hash change and rolling update completes (`oc rollout status statefulset/spire-server -n <ns>`).
   - **Expected:** Rollout success; new pod Ready.

3. `oc get role,rolebinding -n <issuer-ns>` filtered by labels or names owned by operator for CertificateRequest.
   - **Expected:** RBAC present as designed.

**Cleanup:** Remove upstreamAuthority from SpireServer; confirm RBAC teardown (E2E-004 overlap acceptable as sequential phase with DeferCleanup restore).

**Failure Impact:** Production PKI integration broken.

---

#### E2E-004: Lifecycle — remove upstreamAuthority (revert self-signed)

**Priority:** High  
**Methodology:** Black box  
**Ginkgo Labels:** `reconciliation`, `configmap`  
**Relevant Requirement(s):** REQ-010, REQ-005  
**Traceability:** ADR Removal Behaviour  

**Steps:**

1. From state with cert-manager upstream enabled, patch SpireServer to delete `upstreamAuthority` field.
   - **Expected:** ConfigMap no longer lists UpstreamAuthority block; cert-manager RBAC removed from target namespace; rollout completes; SPIRE eventually serves self-signed per ADR bundle accumulation semantics.

**Cleanup:** Restore baseline CR if needed for shared env.

**Failure Impact:** Cannot safely roll back PKI strategy.

---

#### E2E-005: Regression — guard vs manual ConfigMap drift

**Priority:** Medium  
**Methodology:** Black box  
**Ginkgo Labels:** `reconciliation`, `create-only-mode` (if applicable)  
**Relevant Requirement(s):** REQ-012  
**Traceability:** ADR Alternatives (reject manual workaround as product path)  

**Steps:**

1. With operator reconciling, confirm applying SpireServer upstream spec updates ConfigMap deterministically (extract checksum or canonical stanza).
   - **Expected:** Declarative reconcile wins; documented migration off CREATE_ONLY_MODE + manual patch remains supported path without silent wipe of unrelated keys per product docs.

**Cleanup:** Revert test CR.

**Failure Impact:** Customers stuck on unsupported manual flows.

---

### Tier 4: Manual QE Tests

#### MQE-001: Acceptance — enable Vault `k8s_auth` upstream on OpenShift

**Priority:** High  
**Methodology:** Black box (human execution)  
**Type:** Acceptance  
**Relevant Requirement(s):** REQ-003, REQ-004, REQ-006, REQ-009  
**Traceability:** ADR Goals §3–§4, Supported Plugin Vault  

**Preconditions:** Vault reachable with PKI engine + Kubernetes auth configured per ADR prerequisites; TLS CA Secret created in operator namespace; docs available.

**Steps:**

1. Create/update SpireServer with Vault upstream and `k8s_auth` pointing at projected token path per API.
   - **Expected:** `oc describe spireserver cluster` shows `ConfigurationValid=True`; SPIRE server pod reaches Ready.

2. Inspect server logs for successful upstream activation (exact log line per engineering doc).
   - **Expected:** No fatal plugin load errors; SVID/CA chain reflects corporate PKI per openssl/`spire-server` inspection steps in runbook.

**Pass/Fail Criteria:** Human confirms intermediate CA chains to Vault PKI root per organization trust policy.

**Cleanup:** Remove upstreamAuthority; verify revert per E2E-004 checklist manually.

**Failure Impact:** Vault-backed deployments blocked for regulated customers.

---

#### MQE-002: Exploratory — upstream failure while pod appears Ready

**Priority:** Medium  
**Methodology:** Black box  
**Type:** Exploratory  
**Relevant Requirement(s):** REQ-013, REQ-014  
**Traceability:** ADR Risks §4 (cached token / health window)  

**Preconditions:** Vault upstream enabled and healthy baseline.

**Steps:**

1. Simulate Vault Kubernetes auth misconfiguration (e.g. temporarily wrong `kubernetes_ca_cert` on Vault side) while observing SpireServer conditions and pod restarts over ≥ `ca_ttl/2` window.
   - **Expected:** Behavior matches ADR expectation (eventual failure via liveness/restarts); document gaps if condition stays misleading.

**Pass/Fail Criteria:** Written observation of condition transitions vs pod logs with timestamps.

**Cleanup:** Restore Vault config; reconcile SpireServer to healthy state.

**Failure Impact:** Operational blind spots during incidents.

---

### Tier 5: Non-Functional Tests

#### NFT-001: Performance — reconcile latency under upstream CR churn

**Priority:** Medium  
**Sub-type:** Performance  
**Methodology:** Metrics-driven  
**Relevant Requirement(s):** REQ-007, REQ-016  
**Traceability:** ADR Goals §6; Jira reconcile AC  

**Measurable Threshold:** P95 reconcile duration for SpireServer controller increases &lt; X% vs baseline without upstreamAuthority on same cluster class (define X per team SLO, e.g. 25%); no unbounded resync loops.

**Preconditions:** Prometheus/metrics scraping controller; script to patch `upstreamAuthority` issuer name field N times.

**Steps:**

1. Baseline: record reconcile histogram / workqueue depth without upstreamAuthority.

2. Apply N sequential valid patches toggling issuer reference between two allowed values (still valid Ready issuer).
   - **Expected:** Rollouts complete; metrics stay below threshold; no OOM on operator.

**Cleanup:** Restore original SpireServer spec.

**Failure Impact:** Operator cannot scale with PKI automation CI churn.

---

#### NFT-002: Recovery — operator restart mid-reconcile with upstreamAuthority

**Priority:** High  
**Sub-type:** Recovery  
**Methodology:** Black box  
**Relevant Requirement(s):** REQ-016  
**Traceability:** ADR Risks operational failure modes  

**Steps:**

1. Patch SpireServer to add upstreamAuthority; immediately delete operator pod (`oc delete pod -l name=zero-trust-workload-identity-manager -n openshift-operators` or appropriate label).
   - **Expected:** After operator returns, SpireServer reaches `ConfigurationValid=True` and desired ConfigMap/StatefulSet/RBAC converge without manual intervention.

**Cleanup:** Restore CR if needed.

**Failure Impact:** Upgrade/restart windows corrupt PKI state.

---

#### NFT-003: Security — RBAC least privilege for CertificateRequest

**Priority:** High  
**Sub-type:** Security  
**Methodology:** Grey box  
**Relevant Requirement(s):** REQ-005, REQ-014  
**Traceability:** ADR Risks §1 (credential classes); How RBAC  

**Steps:**

1. Dump synthesized Role rules (`oc get role -o yaml`) in cert-manager namespace.
   - **Expected:** Only verbs/resources required for CertificateRequest workflow; no cluster-admin grants to SPIRE SA.

**Cleanup:** None.

**Failure Impact:** Compliance failure / blast radius.

---

## Traceability Matrix

| Requirement | UT | INT | E2E | MQE | NFT | Coverage Status |
| --- | --- | --- | --- | --- | --- | --- |
| REQ-001 | UT-001 | — | E2E-002 | — | — | Covered |
| REQ-002 | UT-002 | — | E2E-003 | — | — | Covered |
| REQ-003 | UT-003 | INT-003 | — | MQE-001 | — | Covered |
| REQ-004 | — | INT-001 | — | MQE-001 | — | Covered |
| REQ-005 | UT-004 | INT-002 | E2E-003, E2E-004 | — | NFT-003 | Covered |
| REQ-006 | UT-003 | INT-003 | — | MQE-001 | — | Covered |
| REQ-007 | UT-005 | — | E2E-003 | — | NFT-001 | Covered |
| REQ-008 | UT-001 | INT-001 | E2E-002 | — | — | Covered |
| REQ-009 | — | INT-001 | E2E-003 | MQE-001 | NFT-002 | Covered |
| REQ-010 | — | INT-002 | E2E-004 | MQE-001 | — | Covered |
| REQ-011 | — | — | E2E-001 | — | — | Covered |
| REQ-012 | UT-001 | — | E2E-002, E2E-005 | — | — | Covered |
| REQ-013 | — | — | — | MQE-002 | — | Covered |
| REQ-014 | — | — | — | MQE-002 | NFT-003 | Partial (manual/exploratory + RBAC) |
| REQ-015 | UT-006 | — | E2E-002 | — | — | Covered |
| REQ-016 | — | INT-002 | E2E-003 | MQE-001 | NFT-001, NFT-002 | Covered |
| REQ-017 | UT-006 | — | — | — | — | Covered |

## Uncovered Requirements

- **REQ-014 (full risk catalog):** Vault token-reviewer JWT leakage scenarios, multi-signer OpenShift CA rotation on Vault, and `ca_ttl` vs Vault `max-lease-ttl` mismatch are only partially covered (exploratory + RBAC). Dedicated automated security/compliance tests require Vault test harness and cluster upgrade fixtures—not covered end-to-end in this document.

## Coverage Summary

| Tier | Count | Critical | High | Medium |
| --- | --- | --- | --- | --- |
| Unit Tests | 6 | 1 | 4 | 1 |
| Integration Tests | 3 | 1 | 2 | 0 |
| E2E Automated Tests | 5 | 2 | 3 | 0 |
| Manual QE Tests | 2 | 0 | 1 | 1 |
| Non-Functional Tests | 3 | 0 | 2 | 1 |
| **Total** | **19** | **4** | **12** | **3** |
