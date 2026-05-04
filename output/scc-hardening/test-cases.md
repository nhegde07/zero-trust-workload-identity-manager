# Test Plan: SCC Hardening for SPIRE Agent and SPIFFE CSI Driver

**Source:** `SCC Hardening for SPIRE Agent and SPIFFE CSI Driver.md`
**Date:** 2026-04-29
**Scope:** Remove privileged mode from SPIRE Agent, retain it for SPIFFE CSI Driver, standardize custom SCCs for both operands to enforce least privilege.

## ADR Decomposition

**Feature:** SCC hardening to enforce least privilege for SPIRE Agent and SPIFFE CSI Driver operands within the ZTWIM operator.

**Components in scope:**

- SPIRE Agent DaemonSet (`spire-agent`) -- podSpec SecurityContext and SCC
- SPIRE Agent custom SCC (`spire-agent`) -- permissions, capabilities, volume types, host access
- SPIFFE CSI Driver DaemonSet (`spire-spiffe-csi-driver`) -- no podSpec changes, SCC retained
- SPIFFE CSI Driver custom SCC (`spire-spiffe-csi-driver`) -- no changes, existing custom SCC remains
- Controller reconcile paths for SCC generation: `generateSpireAgentSCC()`, `reconcileSCC()`
- Controller reconcile paths for DaemonSet generation: `generateSpireAgentDaemonSet()`

**Positive-path requirements (from Goals):**

1. SPIRE Agent runs without privileged mode (`privileged: false`)
2. SPIRE Agent drops all Linux capabilities (`capabilities.drop: [ALL]`)
3. SPIRE Agent uses a custom SCC with least-privilege permissions
4. SPIRE Agent SCC denies `allowHostNetwork`, `allowHostPorts`, `allowHostIPC`
5. SPIRE Agent SCC allows `allowHostPID: true` and `allowHostDirVolumePlugin: true`
6. SPIRE Agent SCC sets `readOnlyRootFilesystem: true`
7. SPIRE Agent SCC uses `runAsUser: RunAsAny` to permit UID 0
8. SPIRE Agent SCC binds only the `spire-agent` service account
9. SPIFFE CSI Driver retains privileged mode for bidirectional mount propagation
10. Full functional correctness is maintained after hardening
11. No breaking changes to existing workloads

**Scope boundaries (from Non-Goals):**

- Do NOT test redesign of SPIFFE CSI Driver to avoid privilege
- Do NOT test removal of hostPath or PID-based attestation
- Do NOT test adding capabilities to non-root user

**Implementation details requiring test coverage (from How):**

- `generateSpireAgentSCC()` produces the exact SCC struct with all hardened fields
- `generateSpireAgentDaemonSet()` produces a DaemonSet with `privileged: false`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem: true`
- DaemonSet sets `hostPID: true`, `hostNetwork: false`, `dnsPolicy: ClusterFirst`
- `reconcileSCC()` creates the SCC when missing, updates it when drifted, preserves OpenShift-managed fields (`Priority`, `UserNamespaceLevel`, `ResourceVersion`)
- SCC `Volumes` list is exactly: `configMap`, `hostPath`, `projected`, `secret`, `emptyDir`
- SCC `Users` list is scoped to `system:serviceaccount:<namespace>:spire-agent`
- SPIFFE CSI Driver SCC and DaemonSet are unchanged

**Risks requiring test coverage:**

- Privileged mode for CSI Driver remains a known security surface -- verify it is scoped via custom SCC, not system:privileged
- Root user for SPIRE Agent -- verify capabilities are dropped and readOnlyRFS is enforced
- HostPath mounting -- verify volume types are tightly scoped in SCC
- Namespace PSA escalation -- verify SCC does not grant broader permissions than needed

**Open questions / areas needing exploratory coverage:**

- Behavior after OCP upgrade: does the SCC survive platform reconciliation?
- Interaction with third-party admission controllers or policy engines (OPA, Kyverno) that may reject root UID pods

---

## Testable Requirements

| ID | Requirement | Category | ADR Source |
| --- | --- | --- | --- |
| REQ-001 | `generateSpireAgentSCC()` returns an SCC with `allowPrivilegedContainer: false` | Functional | How |
| REQ-002 | `generateSpireAgentSCC()` returns an SCC with `requiredDropCapabilities: [ALL]` | Functional | How |
| REQ-003 | `generateSpireAgentSCC()` returns an SCC with `allowPrivilegeEscalation: false` | Functional | How |
| REQ-004 | `generateSpireAgentSCC()` returns an SCC with `readOnlyRootFilesystem: true` | Functional | How |
| REQ-005 | `generateSpireAgentSCC()` returns an SCC with `allowHostPID: true` | Functional | How |
| REQ-006 | `generateSpireAgentSCC()` returns an SCC with `allowHostNetwork: false`, `allowHostPorts: false`, `allowHostIPC: false` | Functional | How |
| REQ-007 | `generateSpireAgentSCC()` returns an SCC with `runAsUser.type: RunAsAny` | Functional | How |
| REQ-008 | `generateSpireAgentSCC()` returns an SCC with `Users` containing only the `spire-agent` SA in the operator namespace | Security | How |
| REQ-009 | `generateSpireAgentSCC()` returns an SCC with `Volumes` limited to `configMap`, `hostPath`, `projected`, `secret`, `emptyDir` | Security | How |
| REQ-010 | `generateSpireAgentDaemonSet()` produces a pod spec with `privileged: false`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem: true` | Functional | How |
| REQ-011 | `generateSpireAgentDaemonSet()` produces a pod spec with `hostPID: true`, `hostNetwork: false`, `dnsPolicy: ClusterFirst` | Functional | How |
| REQ-012 | `reconcileSCC()` creates the SCC when it does not exist and reports `SecurityContextConstraintsAvailable: True` | Functional | How |
| REQ-013 | `reconcileSCC()` updates the SCC when it drifts from desired state | Functional | How |
| REQ-014 | `reconcileSCC()` preserves OpenShift-managed fields (`Priority`, `UserNamespaceLevel`) during update | Functional | How |
| REQ-015 | `reconcileSCC()` reports status condition `False` when SCC creation or update fails | Negative | How |
| REQ-016 | SPIFFE CSI Driver SCC and DaemonSet remain unchanged (privileged, custom SCC) | Regression | What, Risks |
| REQ-017 | SPIRE Agent starts successfully and creates UDS socket with the hardened security context | Functional | Goals |
| REQ-018 | Workload attestation via hostPID continues to function after hardening | Functional | Goals |
| REQ-019 | SPIFFE CSI Driver bidirectional mount propagation continues to function | Regression | Risks |
| REQ-020 | Operator reconciliation latency does not degrade after SCC changes | Performance | Risks |
| REQ-021 | Operator recovers and re-creates the SCC after manual deletion | Operational | Risks |
| REQ-022 | SPIRE Agent SCC does not grant permissions beyond what is specified (no `allowedCapabilities`, no `defaultAddCapabilities`) | Security | Risks |

---

## Test Cases

### Tier 1: Unit Tests

#### UT-001: generateSpireAgentSCC returns non-privileged SCC with all capabilities dropped

**Priority:** Critical
**Methodology:** White box
**Relevant Requirement(s):** REQ-001, REQ-002, REQ-003
**Preconditions:** None; pure function test
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid `SpireAgent` config
   - **Expected:** Returned SCC has `AllowPrivilegedContainer == false`
2. Inspect `RequiredDropCapabilities`
   - **Expected:** Contains exactly `["ALL"]`
3. Inspect `AllowPrivilegeEscalation`
   - **Expected:** Points to `false`
**Cleanup:** None
**Failure Impact:** SPIRE Agent pods could run with elevated privileges in production

---

#### UT-002: generateSpireAgentSCC sets correct host access flags

**Priority:** Critical
**Methodology:** White box
**Relevant Requirement(s):** REQ-005, REQ-006
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid config
   - **Expected:** `AllowHostPID == true`
2. Assert host network and port flags
   - **Expected:** `AllowHostNetwork == false`, `AllowHostPorts == false`, `AllowHostIPC == false`
**Cleanup:** None
**Failure Impact:** Workload attestation breaks (if hostPID missing) or unnecessary network surface exposed (if hostNetwork true)

---

#### UT-003: generateSpireAgentSCC sets readOnlyRootFilesystem and RunAsAny

**Priority:** High
**Methodology:** White box
**Relevant Requirement(s):** REQ-004, REQ-007
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid config
   - **Expected:** `ReadOnlyRootFilesystem == true`
2. Inspect `RunAsUser.Type`
   - **Expected:** `RunAsUserStrategyRunAsAny`
**Cleanup:** None
**Failure Impact:** Root filesystem writes could be exploited; or socket creation fails if UID 0 is denied

---

#### UT-004: generateSpireAgentSCC scopes Users to spire-agent SA only

**Priority:** Critical
**Methodology:** White box
**Relevant Requirement(s):** REQ-008
**Preconditions:** `OPERATOR_NAMESPACE` env set
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid config
   - **Expected:** `Users` contains exactly one entry: `system:serviceaccount:<namespace>:spire-agent`
2. Assert `Groups` is empty
   - **Expected:** `Groups == []`
**Cleanup:** None
**Failure Impact:** Unintended service accounts gain SCC permissions

---

#### UT-005: generateSpireAgentSCC restricts volume types

**Priority:** High
**Methodology:** White box
**Relevant Requirement(s):** REQ-009
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid config
   - **Expected:** `Volumes` contains exactly `configMap`, `hostPath`, `projected`, `secret`, `emptyDir` (5 entries, no others)
**Cleanup:** None
**Failure Impact:** Overly permissive volume types could allow mounting arbitrary storage

---

#### UT-006: generateSpireAgentSCC does not grant any additional capabilities

**Priority:** High
**Methodology:** White box
**Relevant Requirement(s):** REQ-022
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentSCC()` with a valid config
   - **Expected:** `AllowedCapabilities` is empty
2. Inspect `DefaultAddCapabilities`
   - **Expected:** Empty slice
**Cleanup:** None
**Failure Impact:** Unintended capabilities could be added to containers using this SCC

---

#### UT-007: generateSpireAgentDaemonSet produces hardened container security context

**Priority:** Critical
**Methodology:** White box
**Relevant Requirement(s):** REQ-010, REQ-011
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentDaemonSet()` with a valid spec and ZTWIM config
   - **Expected:** Pod spec has `HostPID == true`, `HostNetwork == false`, `DNSPolicy == ClusterFirst`
2. Inspect the `spire-agent` container's `SecurityContext`
   - **Expected:** `Privileged == false` (or nil, defaulting to false), `AllowPrivilegeEscalation == false`, `Capabilities.Drop == [ALL]`, `ReadOnlyRootFilesystem == true`
**Cleanup:** None
**Failure Impact:** SPIRE Agent container runs with privilege escalation or capabilities in cluster

---

#### UT-008: generateSpireAgentSCC negative -- nil config does not panic

**Priority:** Medium
**Methodology:** White box
**Relevant Requirement(s):** REQ-001
**Preconditions:** None
**Steps:**
1. Call `generateSpireAgentSCC(nil)`
   - **Expected:** Returns a valid (non-nil) SCC with all hardened defaults, or handles nil safely without panic
**Cleanup:** None
**Failure Impact:** Nil pointer dereference during reconciliation

---

### Tier 2: Integration Tests

#### INT-001: reconcileSCC creates SPIRE Agent SCC when absent

**Priority:** Critical
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-012
**Preconditions:** Fake or envtest API server running; no existing `spire-agent` SCC
**Steps:**
1. Initialize a `SpireAgentReconciler` with a fake client that returns `NotFound` on `Get`
   - **Expected:** Setup completes without error
2. Call `reconcileSCC()` with a valid `SpireAgent` CR and status manager
   - **Expected:** `Create` is called on the fake client with the desired SCC
3. Inspect status manager conditions
   - **Expected:** `SecurityContextConstraintsAvailable` condition is `True` with reason `SpireAgentSCCResourceCreated`
**Cleanup:** None (fake client)
**Failure Impact:** SPIRE Agent pods cannot schedule because the SCC is missing

---

#### INT-002: reconcileSCC updates SCC when existing does not match desired

**Priority:** Critical
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-013, REQ-014
**Preconditions:** Fake client returns an existing SCC with `AllowPrivilegedContainer: true` (drifted state)
**Steps:**
1. Call `reconcileSCC()` with a valid `SpireAgent` CR
   - **Expected:** `Update` is called on the fake client
2. Inspect the SCC passed to `Update`
   - **Expected:** `AllowPrivilegedContainer == false`, `ResourceVersion` matches existing, `Priority` preserved from existing
3. Inspect status conditions
   - **Expected:** `SecurityContextConstraintsAvailable` is `True` with reason `SpireAgentSCCResourceUpdated`
**Cleanup:** None
**Failure Impact:** Drifted SCC leaves SPIRE Agent running with excess privileges

---

#### INT-003: reconcileSCC skips update when SCC is already up to date

**Priority:** High
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-013
**Preconditions:** Fake client returns an SCC matching the desired state
**Steps:**
1. Call `reconcileSCC()` with a valid config
   - **Expected:** `Update` is NOT called on the fake client
2. Inspect status conditions
   - **Expected:** `SecurityContextConstraintsAvailable` is `True` with reason `SpireAgentSCCResourceUpToDate`
**Cleanup:** None
**Failure Impact:** Unnecessary API writes on every reconcile loop, increasing API server load

---

#### INT-004: reconcileSCC reports failure status when Get returns unexpected error

**Priority:** High
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-015
**Preconditions:** Fake client returns a non-NotFound error on `Get`
**Steps:**
1. Call `reconcileSCC()`
   - **Expected:** Returns an error
2. Inspect status conditions
   - **Expected:** `SecurityContextConstraintsAvailable` is `False` with reason `SpireAgentSCCGetFailed`
**Cleanup:** None
**Failure Impact:** Silent SCC failures with no status visibility

---

#### INT-005: reconcileSCC reports failure status when Create fails

**Priority:** High
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-015
**Preconditions:** Fake client returns `NotFound` on `Get` and an error on `Create`
**Steps:**
1. Call `reconcileSCC()`
   - **Expected:** Returns an error
2. Inspect status conditions
   - **Expected:** `SecurityContextConstraintsAvailable` is `False` with reason `SpireAgentSCCCreationFailed`
**Cleanup:** None
**Failure Impact:** SCC creation failures go unreported to users

---

#### INT-006: DaemonSet reconciliation produces hardened pod spec through envtest

**Priority:** Critical
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-010, REQ-011
**Preconditions:** envtest running with CRDs loaded
**Steps:**
1. Create a `SpireAgent` CR via the envtest API server
   - **Expected:** CR is accepted
2. Wait for the reconciler to create the SPIRE Agent DaemonSet
   - **Expected:** DaemonSet exists with `hostPID: true`, `hostNetwork: false`
3. Inspect the `spire-agent` container's SecurityContext on the DaemonSet
   - **Expected:** `privileged: false`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`
**Cleanup:** Delete the `SpireAgent` CR
**Failure Impact:** Hardened security context is not applied to real DaemonSet objects

---

### Tier 3: E2E Automated Tests

#### E2E-001: Smoke -- Operator installs and SPIRE Agent SCC exists with hardened settings

**Priority:** Critical
**Methodology:** Black box
**Ginkgo Labels:** `install-health`, `openshift-scc`, `security-context`
**Relevant Requirement(s):** REQ-001, REQ-002, REQ-003, REQ-004, REQ-012
**Preconditions:** ZTWIM operator installed via OLM; `ZeroTrustWorkloadIdentityManager` CR `cluster` created
**Steps:**
1. `oc get scc spire-agent -o yaml`
   - **Expected:** SCC exists; `allowPrivilegedContainer: false`; `requiredDropCapabilities: [ALL]`; `allowPrivilegeEscalation: false`; `readOnlyRootFilesystem: true`
2. Wait for `SpireAgent` conditions to include `SecurityContextConstraintsAvailable: True`
   - **Expected:** Condition is present and True
3. `oc get daemonset spire-agent -n <operator-ns> -o yaml`
   - **Expected:** DaemonSet is running with `hostPID: true`, `hostNetwork: false`, container `privileged: false`
**Cleanup:** `DeferCleanup` -- none beyond default operator lifecycle
**Failure Impact:** Fundamental SCC hardening is missing from the deployment

---

#### E2E-002: SPIRE Agent SCC denies host network and host ports

**Priority:** Critical
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `security-context`
**Relevant Requirement(s):** REQ-006
**Preconditions:** Operator installed; `spire-agent` SCC exists
**Steps:**
1. `oc get scc spire-agent -o jsonpath='{.allowHostNetwork}'`
   - **Expected:** `false`
2. `oc get scc spire-agent -o jsonpath='{.allowHostPorts}'`
   - **Expected:** `false`
3. `oc get scc spire-agent -o jsonpath='{.allowHostIPC}'`
   - **Expected:** `false`
4. `oc get scc spire-agent -o jsonpath='{.allowHostPID}'`
   - **Expected:** `true`
**Cleanup:** None
**Failure Impact:** SPIRE Agent pods gain unnecessary network-level host access

---

#### E2E-003: SPIRE Agent SCC scopes user binding and volume types

**Priority:** High
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `rbac`
**Relevant Requirement(s):** REQ-008, REQ-009
**Preconditions:** Operator installed
**Steps:**
1. `oc get scc spire-agent -o jsonpath='{.users}'`
   - **Expected:** Contains exactly `["system:serviceaccount:<operator-ns>:spire-agent"]`
2. `oc get scc spire-agent -o jsonpath='{.volumes}'`
   - **Expected:** Contains exactly `["configMap","hostPath","projected","secret","emptyDir"]` (5 entries)
3. `oc get scc spire-agent -o jsonpath='{.allowedCapabilities}'`
   - **Expected:** Empty (`[]` or null)
**Cleanup:** None
**Failure Impact:** Unintended accounts or volume types are permitted by the SCC

---

#### E2E-004: Negative -- SPIRE Agent SCC rejects pods requesting additional capabilities

**Priority:** High
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `negative-input-validation`
**Relevant Requirement(s):** REQ-022
**Preconditions:** Operator installed; `spire-agent` SCC is the only SCC available to the `spire-agent` SA
**Steps:**
1. Create a Pod in the operator namespace using the `spire-agent` service account with `securityContext.capabilities.add: ["NET_ADMIN"]`
   - **Expected:** Pod is rejected by the SCC admission or fails to schedule with a clear SCC violation event
2. Verify that no running pod with `NET_ADMIN` capability exists for the `spire-agent` SA
   - **Expected:** No such pod exists
**Cleanup:** Delete the test pod manifest
**Failure Impact:** SCC allows capabilities that should be denied

---

#### E2E-005: Regression -- SPIFFE CSI Driver retains privileged custom SCC

**Priority:** Critical
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `security-context`, `reconciliation`
**Relevant Requirement(s):** REQ-016, REQ-019
**Preconditions:** Operator installed
**Steps:**
1. `oc get scc spire-spiffe-csi-driver -o jsonpath='{.allowPrivilegedContainer}'`
   - **Expected:** `true`
2. `oc get daemonset spire-spiffe-csi-driver -n <operator-ns> -o jsonpath='{.spec.template.spec.containers[0].securityContext.privileged}'`
   - **Expected:** `true`
3. Verify CSI Driver DaemonSet pods are Ready on all scheduled nodes
   - **Expected:** All pods in `Running` state with `Ready` condition
**Cleanup:** None
**Failure Impact:** CSI Driver loses bidirectional mount propagation; all workload volume mounts break

---

#### E2E-006: Regression -- Workload attestation functions after agent hardening

**Priority:** Critical
**Methodology:** Black box
**Ginkgo Labels:** `security-context`, `reconciliation`
**Relevant Requirement(s):** REQ-017, REQ-018
**Preconditions:** Operator installed; SPIRE Server, Agent, and CSI Driver running
**Steps:**
1. Create a `ClusterSPIFFEID` and a workload pod using the SPIFFE CSI driver volume
   - **Expected:** Pod starts and mounts the CSI volume
2. Wait for the workload to receive an SVID (check for certificate files in the expected mount path)
   - **Expected:** SVID certificate and key are present and valid
3. Verify SPIRE Agent logs show successful attestation of the workload
   - **Expected:** Log entry confirming PID-based attestation succeeded
**Cleanup:** Delete `ClusterSPIFFEID` and test workload pod
**Failure Impact:** Core identity issuance breaks after the security hardening

---

#### E2E-007: Lifecycle -- SCC is re-created after manual deletion

**Priority:** High
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `reconciliation`
**Relevant Requirement(s):** REQ-021
**Preconditions:** Operator installed; `spire-agent` SCC exists
**Steps:**
1. `oc delete scc spire-agent`
   - **Expected:** SCC is deleted
2. Wait for reconciliation (use `Eventually` with timeout)
   - **Expected:** `oc get scc spire-agent` returns the SCC with all hardened fields intact
3. Verify `SpireAgent` status condition
   - **Expected:** `SecurityContextConstraintsAvailable: True`
**Cleanup:** None (SCC is restored by operator)
**Failure Impact:** Manual or accidental SCC deletion permanently breaks SPIRE Agent scheduling

---

#### E2E-008: Lifecycle -- SCC drift is corrected by reconciler

**Priority:** High
**Methodology:** Black box
**Ginkgo Labels:** `openshift-scc`, `reconciliation`
**Relevant Requirement(s):** REQ-013
**Preconditions:** Operator installed; `spire-agent` SCC exists
**Steps:**
1. Patch the SCC to set `allowPrivilegedContainer: true`: `oc patch scc spire-agent --type=merge -p '{"allowPrivilegedContainer": true}'`
   - **Expected:** Patch succeeds
2. Wait for reconciliation
   - **Expected:** `oc get scc spire-agent -o jsonpath='{.allowPrivilegedContainer}'` returns `false`
3. Verify status condition
   - **Expected:** `SecurityContextConstraintsAvailable: True` with reason indicating update
**Cleanup:** None (operator restores desired state)
**Failure Impact:** Manual privilege escalation via SCC editing goes uncorrected

---

### Tier 4: Manual QE Tests

#### MQE-001: Acceptance -- Fresh install produces hardened SPIRE Agent

**Priority:** Critical
**Methodology:** Black box (human execution)
**Type:** Acceptance
**Relevant Requirement(s):** REQ-001, REQ-010, REQ-012, REQ-017
**Preconditions:** Clean OpenShift cluster; operator not yet installed
**Steps:**
1. Install the ZTWIM operator via OLM (OperatorHub or `oc apply` Subscription)
   - **Expected:** CSV reaches `Succeeded` phase; controller pod is Running
2. Create the `ZeroTrustWorkloadIdentityManager` CR named `cluster`
   - **Expected:** All operand conditions reach `True`
3. `oc get scc spire-agent -o yaml`
   - **Expected:** `allowPrivilegedContainer: false`, `requiredDropCapabilities: [ALL]`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `allowHostPID: true`, `allowHostNetwork: false`
4. `oc get pods -n <operator-ns> -l app.kubernetes.io/name=spire-agent -o yaml`
   - **Expected:** All pods Running; container security context shows `privileged: false`, `capabilities.drop: [ALL]`
5. Create a test workload with `ClusterSPIFFEID` and verify SVID issuance
   - **Expected:** Workload receives valid SVID
**Pass/Fail Criteria:** All five steps pass; no privileged containers; attestation works
**Cleanup:** Delete test workload and `ClusterSPIFFEID`; optionally uninstall operator
**Failure Impact:** End-to-end SCC hardening is not functional on a fresh install

---

#### MQE-002: Exploratory -- Tamper with SPIRE Agent SCC and observe recovery

**Priority:** High
**Methodology:** Black box (human execution)
**Type:** Exploratory
**Relevant Requirement(s):** REQ-013, REQ-021
**Preconditions:** Operator installed; operands healthy
**Steps:**
1. `oc edit scc spire-agent` -- change `allowPrivilegedContainer` to `true`, save
   - **Expected:** Edit succeeds
2. Observe the SCC over the next 30-60 seconds: `watch oc get scc spire-agent -o jsonpath='{.allowPrivilegedContainer}'`
   - **Expected:** Value reverts to `false` after reconciliation
3. `oc delete scc spire-agent`
   - **Expected:** SCC is deleted
4. Wait 30-60 seconds and re-check: `oc get scc spire-agent`
   - **Expected:** SCC exists again with all hardened fields
5. Delete the SPIRE Agent DaemonSet: `oc delete ds spire-agent -n <operator-ns>`
   - **Expected:** DaemonSet is re-created; pods come up with hardened security context
6. Monitor operator logs during all of the above: `oc logs -f deployment/<operator-deployment> -n <operator-ns>`
   - **Expected:** Logs show reconciliation activity without errors or crash loops
**Pass/Fail Criteria:** Operator recovers from every tampering action without human intervention
**Cleanup:** None needed if operator recovers; otherwise re-create the ZTWIM CR
**Failure Impact:** Operator cannot self-heal from SCC or DaemonSet drift

---

#### MQE-003: Upgrade -- SCC hardening applied correctly after operator upgrade

**Priority:** High
**Methodology:** Black box (human execution)
**Type:** Upgrade
**Relevant Requirement(s):** REQ-012, REQ-013, REQ-016
**Preconditions:** Previous operator version installed (pre-hardening); operands running with old SCC
**Steps:**
1. Record current SCC state: `oc get scc spire-agent -o yaml > /tmp/scc-before.yaml`
   - **Expected:** File saved
2. Upgrade operator to the new version via OLM channel update
   - **Expected:** CSV transitions to new version; controller pod restarts
3. Wait for all operand conditions to reach `True`
   - **Expected:** `SecurityContextConstraintsAvailable: True`
4. `oc get scc spire-agent -o yaml` and compare with pre-upgrade
   - **Expected:** New SCC has `allowPrivilegedContainer: false`, `requiredDropCapabilities: [ALL]`, other hardened fields
5. Verify SPIRE Agent pods are Running with new security context
   - **Expected:** All pods show `privileged: false` in container security context
6. Verify SPIFFE CSI Driver SCC and pods are unchanged
   - **Expected:** CSI Driver SCC still has `allowPrivilegedContainer: true`; pods Running
7. Run a workload attestation test
   - **Expected:** SVID issuance succeeds
**Pass/Fail Criteria:** Upgrade applies hardened SCC without breaking existing workloads
**Cleanup:** None
**Failure Impact:** Upgrades silently break security posture or workload attestation

---

### Tier 5: Non-Functional Tests

#### NFT-001: Performance -- Reconcile latency for SCC after hardening

**Priority:** High
**Sub-type:** Performance
**Methodology:** Metrics-driven
**Relevant Requirement(s):** REQ-020
**Measurable Threshold:** SCC reconcile cycle completes within 5 seconds under normal conditions
**Preconditions:** Operator installed; Prometheus scraping operator metrics
**Steps:**
1. Delete the `spire-agent` SCC to trigger re-creation
   - **Expected:** SCC is re-created
2. Measure the time between SCC deletion and `SecurityContextConstraintsAvailable: True` condition update via `controller_runtime_reconcile_time_seconds` metric or by timestamp comparison
   - **Expected:** Re-creation completes within 5 seconds
3. Repeat 10 times and compute p95
   - **Expected:** p95 reconcile time is under 5 seconds
**Cleanup:** None
**Failure Impact:** SCC reconciliation becomes a bottleneck for operator responsiveness

---

#### NFT-002: Recovery -- Operator recovers after pod crash during SCC reconciliation

**Priority:** High
**Sub-type:** Recovery
**Methodology:** Black box
**Relevant Requirement(s):** REQ-021
**Measurable Threshold:** Full recovery within 2 minutes of pod restart
**Preconditions:** Operator installed; operands healthy
**Steps:**
1. Delete the `spire-agent` SCC: `oc delete scc spire-agent`
   - **Expected:** SCC deleted
2. Immediately kill the operator controller pod: `oc delete pod -l control-plane=controller-manager -n <operator-ns>`
   - **Expected:** Pod is terminated; new pod starts via Deployment
3. Wait for the new controller pod to become Ready
   - **Expected:** Pod is Running and Ready within 60 seconds
4. Verify the `spire-agent` SCC is re-created with all hardened fields
   - **Expected:** SCC exists with `allowPrivilegedContainer: false`, `requiredDropCapabilities: [ALL]`
5. Verify all SPIRE Agent pods are Running
   - **Expected:** All pods healthy; attestation functional
**Cleanup:** None
**Failure Impact:** Operator cannot recover from crash during SCC reconciliation

---

#### NFT-003: Security -- SPIRE Agent SCC does not grant escalation paths

**Priority:** Critical
**Sub-type:** Security
**Methodology:** Black box
**Relevant Requirement(s):** REQ-001, REQ-003, REQ-008, REQ-022
**Measurable Threshold:** Zero unexpected capabilities or privilege escalation paths
**Preconditions:** Operator installed
**Steps:**
1. `oc get scc spire-agent -o json | jq '.allowedCapabilities'`
   - **Expected:** Empty array `[]`
2. `oc get scc spire-agent -o json | jq '.defaultAddCapabilities'`
   - **Expected:** Empty array `[]`
3. `oc get scc spire-agent -o json | jq '.allowPrivilegeEscalation'`
   - **Expected:** `false`
4. `oc get scc spire-agent -o json | jq '.allowPrivilegedContainer'`
   - **Expected:** `false`
5. For each running SPIRE Agent pod, verify effective capabilities: `oc exec <pod> -n <ns> -- cat /proc/1/status | grep -i cap`
   - **Expected:** `CapEff` is `0000000000000000` (no effective capabilities)
**Cleanup:** None
**Failure Impact:** Container escape or privilege escalation vulnerability in the SPIRE Agent

---

#### NFT-004: Regression -- Full operator lifecycle unaffected by SCC changes

**Priority:** Critical
**Sub-type:** Regression
**Methodology:** Black box
**Relevant Requirement(s):** REQ-016, REQ-018, REQ-019
**Measurable Threshold:** All operand conditions reach `True`; attestation succeeds; CSI volumes mount
**Preconditions:** Operator installed with hardened SCC
**Steps:**
1. Verify all four operand conditions: `SpireServer`, `SpireAgent`, `SpiffeCSIDriver`, `SpireOIDCDiscoveryProvider` conditions are `True`
   - **Expected:** All True
2. Create a `ClusterSPIFFEID` and deploy a test workload with CSI volume
   - **Expected:** Workload pod starts; CSI volume is mounted
3. Verify SVID issuance
   - **Expected:** Certificate files present in mount path
4. Scale SPIRE Server StatefulSet to 0 and back to 1
   - **Expected:** SPIRE Server pod restarts; Agent re-establishes connection; SVIDs continue to be issued
5. Delete and re-create the `ZeroTrustWorkloadIdentityManager` CR
   - **Expected:** All operands are re-created with correct security contexts; SCC is present
**Cleanup:** Delete test workload, `ClusterSPIFFEID`
**Failure Impact:** SCC hardening introduces regression in core operator functionality

---

#### NFT-005: Compliance -- SPIRE Agent pod passes OpenShift restricted profile checks

**Priority:** High
**Sub-type:** Compliance
**Methodology:** Metrics-driven
**Relevant Requirement(s):** REQ-001, REQ-002, REQ-003, REQ-004
**Measurable Threshold:** All security context fields match or exceed `restricted-v2` requirements, except for `runAsNonRoot` (permitted as root due to socket creation requirement) and `hostPID` (required for attestation)
**Preconditions:** Operator installed
**Steps:**
1. For each SPIRE Agent pod, verify container security context fields:
   - `oc get pod <pod> -n <ns> -o jsonpath='{.spec.containers[0].securityContext}'`
   - **Expected:** `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `readOnlyRootFilesystem: true`
2. Verify pod-level fields:
   - `oc get pod <pod> -n <ns> -o jsonpath='{.spec.hostNetwork}'`
   - **Expected:** `false` (or absent, defaulting to false)
3. Document the two intentional deviations from `restricted-v2`:
   - `runAsNonRoot` is not set (root UID required for socket creation)
   - `hostPID: true` (required for workload attestation)
   - **Expected:** These are the ONLY deviations
**Cleanup:** None
**Failure Impact:** Operator fails OpenShift security compliance audit

---

## Traceability Matrix

| Requirement | UT | INT | E2E | MQE | NFT | Coverage Status |
| --- | --- | --- | --- | --- | --- | --- |
| REQ-001 | UT-001 | - | E2E-001 | MQE-001 | NFT-003, NFT-005 | Covered |
| REQ-002 | UT-001 | - | E2E-001 | MQE-001 | NFT-005 | Covered |
| REQ-003 | UT-001 | - | E2E-001 | - | NFT-003, NFT-005 | Covered |
| REQ-004 | UT-003 | - | E2E-001 | - | NFT-005 | Covered |
| REQ-005 | UT-002 | - | E2E-002 | - | - | Covered |
| REQ-006 | UT-002 | - | E2E-002 | - | - | Covered |
| REQ-007 | UT-003 | - | - | - | - | Covered |
| REQ-008 | UT-004 | - | E2E-003 | - | NFT-003 | Covered |
| REQ-009 | UT-005 | - | E2E-003 | - | - | Covered |
| REQ-010 | UT-007 | INT-006 | E2E-001 | MQE-001 | - | Covered |
| REQ-011 | UT-007 | INT-006 | E2E-001 | - | - | Covered |
| REQ-012 | - | INT-001 | E2E-001 | MQE-001 | - | Covered |
| REQ-013 | - | INT-002, INT-003 | E2E-008 | MQE-002 | - | Covered |
| REQ-014 | - | INT-002 | - | - | - | Covered |
| REQ-015 | - | INT-004, INT-005 | - | - | - | Covered |
| REQ-016 | - | - | E2E-005 | MQE-003 | NFT-004 | Covered |
| REQ-017 | - | - | E2E-006 | MQE-001 | - | Covered |
| REQ-018 | - | - | E2E-006 | - | NFT-004 | Covered |
| REQ-019 | - | - | E2E-005 | - | NFT-004 | Covered |
| REQ-020 | - | - | - | - | NFT-001 | Covered |
| REQ-021 | - | - | E2E-007 | MQE-002 | NFT-002 | Covered |
| REQ-022 | UT-006 | - | E2E-004 | - | NFT-003 | Covered |

## Uncovered Requirements

All 22 requirements have test coverage. No uncovered requirements.

## Coverage Summary

| Tier | Count | Critical | High | Medium |
| --- | --- | --- | --- | --- |
| Unit Tests | 8 | 4 | 3 | 1 |
| Integration Tests | 6 | 3 | 3 | 0 |
| E2E Automated Tests | 8 | 4 | 4 | 0 |
| Manual QE Tests | 3 | 1 | 2 | 0 |
| Non-Functional Tests | 5 | 2 | 3 | 0 |
| **Total** | **30** | **14** | **15** | **1** |
