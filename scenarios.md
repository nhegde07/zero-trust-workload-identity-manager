# Test Plan: SPIRE-439 — SCC hardening for SPIRE Agent (PR #105)

<!-- Source: https://github.com/openshift/zero-trust-workload-identity-manager/pull/105 -->
<!-- Repo: openshift/zero-trust-workload-identity-manager -->
<!-- Framework: Ginkgo v2 / controller-runtime -->

## Summary

PR #105 tightens the SPIRE Agent operand for OpenShift: the DaemonSet disables host networking, uses cluster-first DNS, keeps `HostPID` for Kubernetes workload attestation, and applies a non-privileged container `SecurityContext` (no privilege escalation, capabilities dropped to `ALL`, read-only root filesystem). The cluster `SecurityContextConstraints` object `spire-agent` is aligned (`allowHostNetwork` / `allowHostPorts` / privileged paths off; `runAsUser` strategy `RunAsAny`; required drop `ALL`). Production behavior is covered by unit tests in `pkg/controller/spire-agent/*_test.go`; **e2e today does not assert these fields on live objects**, so the cases below close that gap on a real OCP cluster.

## Test Cases

### SPIRE-439-TC-001: Cluster `SecurityContextConstraints` `spire-agent` matches hardened policy

**Priority:** Critical  
**Domain:** `openshift-scc`, `security-context`  
**Category:** 9 (Security)  
**OpenShift-specific:** yes  
**Coverage Gap:** E2E waits for `SecurityContextConstraintsAvailable=True` on `SpireAgent` but does not inspect the SCC object fields introduced/changed in PR #105.  
**Prerequisites:** Operator installed; `SpireAgent` named `cluster` reconciled; `oc` authenticated as a user who can read SCCs (e.g. cluster-admin).  
**Steps:**

1. `oc get securitycontextconstraints.security.openshift.io spire-agent -o yaml`  
   **Expected:** Resource exists; `allowHostNetwork: false`, `allowHostIPC: false`, `allowHostPorts: false`, `allowHostPID: true`, `allowPrivilegedContainer: false`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `requiredDropCapabilities` includes `ALL`, `allowedCapabilities` empty or absent, `runAsUser.type: RunAsAny`.  
2. Confirm `users` includes `system:serviceaccount:<operator-namespace>:spire-agent` (namespace matches install, e.g. `zero-trust-workload-identity-manager` from e2e constants).  
   **Expected:** Service account binding matches operator namespace.  

**Stop condition:** If the SCC is too permissive, workloads could run with excessive host/privileged access; if too strict, SPIRE Agent pods fail to schedule or crash.

---

### SPIRE-439-TC-002: SPIRE Agent DaemonSet pod template — network and PID boundary

**Priority:** Critical  
**Domain:** `controller-manager`, `security-context`  
**Category:** 1 (Core) / 9 (Security)  
**OpenShift-specific:** no (Kubernetes API; OpenShift enforces via SCC)  
**Coverage Gap:** `e2e_test.go` waits for DaemonSet available only; it does not assert `hostNetwork` / `dnsPolicy` / `hostPID`.  
**Prerequisites:** `SpireAgent` installed; DaemonSet `spire-agent` in operator namespace.  
**Steps:**

1. `oc get daemonset spire-agent -n <operator-namespace> -o jsonpath='{.spec.template.spec.hostNetwork}{" "}{.spec.template.spec.hostPID}{" "}{.spec.template.spec.dnsPolicy}{"\n"}'`  
   **Expected:** `hostNetwork` is `false`; `hostPID` is `true`; `dnsPolicy` is `ClusterFirst` (not `ClusterFirstWithHostNet`).  
2. Optionally confirm live spec remains stable across reconcile (no drift back to host network).  
   **Expected:** Values match hardened policy.  

**Stop condition:** Wrong DNS policy or host networking can break agent-to-server connectivity or weaken isolation.

---

### SPIRE-439-TC-003: SPIRE Agent container `securityContext` on running pods

**Priority:** Critical  
**Domain:** `security-context`, `openshift-scc`  
**Category:** 9 (Security)  
**OpenShift-specific:** yes (SCC admission must allow the pod)  
**Coverage Gap:** No e2e assertion on `privileged`, `allowPrivilegeEscalation`, `readOnlyRootFilesystem`, or `capabilities.drop` for operand container `spire-agent`.  
**Prerequisites:** At least one SPIRE Agent pod ready.  
**Steps:**

1. `SPIRE_POD=$(oc get pods -n <operator-namespace> -l app.kubernetes.io/name=spire-agent -o jsonpath='{.items[0].metadata.name}')`  
   **Expected:** Non-empty pod name.  
2. `oc get pod "$SPIRE_POD" -n <operator-namespace> -o jsonpath='{.spec.containers[?(@.name=="spire-agent")].securityContext}' | jq .`  
   **Expected:** `privileged: false`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop` includes `ALL`.  
3. `oc get pod "$SPIRE_POD" -n <operator-namespace> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'`  
   **Expected:** `True`.  

**Stop condition:** Mismatch between DaemonSet and SCC causes repeated `CreateContainerError` / failed pods or attestation outages.

---

### SPIRE-439-TC-004: Regression — Kubernetes workload attestation and SVID issuance

**Priority:** Critical  
**Domain:** `reconciliation`, `controller-manager`  
**Category:** 1 (Core)  
**OpenShift-specific:** yes (CSI + SPIRE on OCP)  
**Coverage Gap:** Behavior is **partially** covered by existing `It("Workload attestation should succeed and workload receives SVID")` in `Context("SpireAgent attestation")`; after PR #105, treat this as the **regression guard** for non-privileged / non-hostNetwork agent.  
**Prerequisites:** Full stack from installation context (SpireServer, SpireAgent, SpiffeCSIDriver, OIDC provider, ClusterSPIFFEID, etc.).  
**Steps:**

1. Run the existing e2e spec (or manual equivalent): create test namespace, ClusterSPIFFEID, attestation pod with CSI + helper, wait for `/certs/svid.pem` etc.  
   **Expected:** Same assertions as today — SVID material present; pod reaches Ready.  
2. On a node where SPIRE Agent runs, confirm agent pod still uses `hostPID: true` (TC-002) — required for k8s workload attestation path.  
   **Expected:** Attestation succeeds despite hardening.  

**Stop condition:** Hardening broke kubelet/API-based attestation or CSI delivery.

---

### SPIRE-439-TC-005: SPIRE Agent health endpoints reachable without host network

**Priority:** High  
**Domain:** `controller-manager`, `install-health`  
**Category:** 1 (Core)  
**OpenShift-specific:** no  
**Coverage Gap:** Probes use container port `healthz` (9982); with `hostNetwork: false`, readiness/liveness must still work via pod network.  
**Prerequisites:** SPIRE Agent pod running.  
**Steps:**

1. `oc exec -n <operator-namespace> "$SPIRE_POD" -c spire-agent -- wget -qO- http://127.0.0.1:9982/live` (or `curl` if available in image).  
   **Expected:** HTTP 200 from liveness path.  
2. `oc get pod "$SPIRE_POD" -n <operator-namespace> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'`  
   **Expected:** `True`.  

**Stop condition:** Probes flap or fail when not using host network.

---

### SPIRE-439-TC-006: SpireAgent status conditions remain healthy after hardening

**Priority:** High  
**Domain:** `reconciliation`, `install-health`  
**Category:** 4 (Integration)  
**OpenShift-specific:** no  
**Coverage Gap:** Existing install `It` already waits for `SecurityContextConstraintsAvailable`, `DaemonSetAvailable`, `Ready`; **extend** with assertions that live SCC/DS match TC-001–003 rather than adding a second `It` that only re-waits the same conditions.  
**Prerequisites:** `SpireAgent` `cluster` applied.  
**Steps:**

1. `oc get spireagent cluster -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}'`  
   **Expected:** `Ready=True`, `DaemonSetAvailable=True`, `SecurityContextConstraintsAvailable=True`, and other expected conditions True.  
2. If e2e is extended, reuse `utils.WaitForSpireAgentConditions` with the same map as in `e2e_test.go` `Installation` context.  
   **Expected:** No regression in happy-path conditions.  

**Stop condition:** False `Ready` or flapping SCC condition blocks upgrades and ZTWIM aggregate status.

---

### SPIRE-439-TC-007: ZeroTrustWorkloadIdentityManager aggregate operand readiness

**Priority:** Medium  
**Domain:** `reconciliation`  
**Category:** 4 (Integration)  
**OpenShift-specific:** no  
**Coverage Gap:** Covered by `It("ZeroTrustWorkloadIdentityManager should aggregate status from all operands")` — **skip** duplicate scenario; use as smoke in manual runs.  
**Prerequisites:** All four operands installed.  
**Steps:**

1. `oc get zerotrustworkloadidentitymanager cluster -o jsonpath='{.status.operands[*].kind}{" "}{.status.operands[*].ready}{"\n"}'`  
   **Expected:** `SpireAgent` shows `ready=true`.  

**Stop condition:** Aggregate CR reports SpireAgent not ready after agent changes.

---

### SPIRE-439-TC-008: SCC `spire-agent` users binding sanity

**Priority:** Medium  
**Domain:** `openshift-scc`, `multi-tenant / NS`  
**Category:** 5  
**OpenShift-specific:** yes  
**Coverage Gap:** SCC is cluster-scoped; wrong `users` on shared clusters is a security hygiene issue.  
**Prerequisites:** Standard single-operator install.  
**Steps:**

1. `oc get scc spire-agent -o jsonpath='{.users}{"\n"}'`  
   **Expected:** Exactly the expected `system:serviceaccount:<ns>:spire-agent` for this install; no unexpected extra users.  

**Stop condition:** Unintended workloads could use the `spire-agent` SCC if `users` is wrong.

---

### SPIRE-439-TC-009: Optional — `seccompProfile` on SPIRE Agent container

**Priority:** Medium  
**Domain:** `security-context`, `openshift-version-compat`  
**Category:** 9 (Security)  
**OpenShift-specific:** yes  
**Coverage Gap:** E2E runbook often expects `seccompProfile.type: RuntimeDefault` for hardened workloads; PR #105 does **not** add this to the SPIRE Agent container in `daemonset.go`. Document actual value; tie pass/fail only to org policy.  
**Prerequisites:** None for mandatory pass today.  
**Steps:**

1. Inspect `securityContext.seccompProfile` on `spire-agent` container in pod spec.  
   **Expected:** Document actual value; certify only if policy requires RuntimeDefault.  

**Stop condition:** Policy audit flags missing seccomp.

---

### SPIRE-439-TC-010: OLM upgrade path from pre-hardening to post-PR behavior

**Priority:** Medium  
**Domain:** `olm-lifecycle-install`, `upgrade`, `openshift-scc`  
**Category:** 7 (Upgrade / compat)  
**OpenShift-specific:** yes  
**Coverage Gap:** No dedicated e2e for CSV upgrade while `SpireAgent` already exists.  
**Prerequisites:** Test cluster where Subscription can move from a CSV before PR #105 to after merge.  
**Steps:**

1. With existing `SpireAgent`, upgrade operator to build containing PR #105.  
   **Expected:** DaemonSet rolls out; pods Ready; TC-001–003 pass post-upgrade.  
2. Re-run SPIRE-439-TC-004 or full `SpireAgent attestation` e2e.  
   **Expected:** No SVID regression.  

**Stop condition:** Stuck rollout or SCC update conflicts during upgrade.

---

## Coverage Map

| Scenario | Existing spec | Domain | Decision (skip / extend / new) |
| --- | --- | --- | --- |
| Operator + operands install | `Installation` context | install-health, reconciliation | **skip** (already present) |
| SpireAgent SCC condition True | `SPIRE Agent should be installed successfully...` | openshift-scc | **extend** — add SCC/DS/pod security assertions |
| DaemonSet security fields | none | security-context, controller-manager | **new** (e2e gap) |
| Workload attestation + SVID | `Workload attestation should succeed...` | reconciliation | **skip** duplicate — document as **regression** guard |
| ZTWIM aggregate ready | `ZeroTrustWorkloadIdentityManager should aggregate...` | reconciliation | **skip** |
| PR `controller.go` error string | none | reconciliation | **skip** for e2e (error path; unit-level) |
| Unit SCC + DaemonSet hardening | `daemonset_test.go`, `scc_test.go` | — | **covered** at unit level; e2e complements cluster |

---

## OLM / OpenShift / Red Hat

- **OLM:** Install covered by existing e2e; **upgrade** validation → SPIRE-439-TC-010.  
- **OpenShift:** SCC + admission → SPIRE-439-TC-001, TC-003 until automated in `test/`.  
- **Certification checklist:** OLM install [x]; SCC alignment [gap → TC-001/003]; attestation [x]; seccomp on operand [optional → TC-009]; upgrade [gap → TC-010].