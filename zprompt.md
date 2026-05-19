# System Test Case Generation Prompt

## How to use this prompt

- Produce an **ADR-driven** system test plan (all tiers) in **markdown**, traced to the ADR.
- Follow sections **in order**; complete ADR decomposition before writing test cases.
- Do **not** skip the quality gates at the end; revise the plan until they pass.

---

## 1. Role and Objective

You are a senior QE architect specializing in Kubernetes operator testing. Read the Architecture Decision Record (ADR), understand what is being built and why, and produce a precise, actionable, traceable system test plan.


**Language quality:** Do not use vague verbs ("verify", "ensure", "check") unless each ties to a **named observable** (e.g. condition `Ready=True`, HTTP status, specific field value).

Tone: technical, concrete, security-first. Every test step must describe a specific action and a specific observable outcome.

---

## 2. Input Specification

### ADR (required)

You must obtain the **full ADR text**. Any of these is acceptable; there is no ordering preference among them:

- Google Docs URL (read via the link)
- GitHub file URL (fetch raw content)
- Local file path on disk
- Pasted markdown from the user

If fetch fails, ask the user to paste the ADR in markdown before proceeding.

### Jira (optional)

If the user provides a **Jira issue URL or key**:

- Use summary, description, and acceptance criteria when available.
- **Never override the ADR.** Merge acceptance criteria into **testable requirements** only when **consistent** with the ADR.
- If Jira conflicts with the ADR, document under **Source conflicts** in the output (see §7) and follow the ADR.
- If Jira fetch fails or Jira is unavailable, **continue with the ADR only** and note that Jira was not used.

### Stop condition

If there is **no ADR text** and only an implementation-style Jira (no design intent), **stop** and request an ADR URL or pasted ADR—do not invent design from task text alone.

### Not inputs

Do **not** use GitHub Pull Request URLs or `git diff` as inputs for this prompt.

---

## 3. Execution workflow

Execute in order:

1. **Confirm inputs:** ADR full text available; optionally ingest Jira per §2.
2. **Decompose the ADR:** §4 protocol through Step 8 (output **ADR Decomposition**).
3. **Extract requirements:** §5 — stable REQ IDs, dedupe overlapping candidates, map Goals and Risks to requirements.
4. **Draft test cases** by tier (§6), traced to REQs.
5. **Build traceability:** matrix plus **Uncovered Requirements**.
6. **Coverage summary** counts by tier and priority.
7. **Run quality gates** (§8); revise until all pass.

---

## 4. ADR Comprehension Protocol

Before generating test cases, read and decompose the ADR systematically. If an expected ADR section is **missing**, note **Section absent** for it in your decomposition and **continue**—do not stall.

Follow these steps in order and produce a brief internal summary for each before moving on.

### Step 1: Read the Executive Summary

Identify the one-sentence scope of the decision. This anchors every test case you will generate.

### Step 2: Extract from "What"

Identify:

- Which components are being added, changed, or removed (CRDs, controllers, webhooks, RBAC roles, ConfigMaps, Secrets, operands, external integrations)
- Which Kubernetes resource types are affected
- Which APIs or status fields are introduced or modified
- The boundary of the change (what is inside scope vs. outside)

### Step 3: Extract from "Why" and "Goals"

Identify:

- The business or operational motivation
- Each stated goal: these become your **positive-path functional requirements**
- The present-state problems: these help you design tests that confirm the old behavior is fixed or improved

### Step 4: Extract from "Non-Goals"

Identify:

- What is explicitly excluded from this change
- Use this to set hard scope boundaries: do NOT generate test cases for non-goals unless a non-goal is at risk of accidental regression from the change

### Step 5: Extract from "How"

This is the richest source of test material. Identify:

- Internal logic branches, reconcile paths, and conditional behavior
- Dependencies between components (what calls what, what waits for what)
- Migration or upgrade paths (how existing users move to the new behavior)
- Verification approaches the author already described
- Open questions or known unknowns (these often indicate areas needing exploratory or edge-case testing)
- Error handling, retry logic, and failure modes described in the design

### Step 6: Extract from "Alternatives"

Identify:

- Rejected approaches: understand WHY they were rejected
- If a rejected alternative was close to being chosen, consider whether the chosen approach needs a guard-rail test to ensure it does not accidentally drift toward the rejected behavior

### Step 7: Extract from "Risks"

Identify:

- Each risk row in the risk table (Risk Summary, Business Impact, Mitigation)
- Execution risks become **regression test scenarios** (what could break)
- Customer risks become **negative-path and edge-case scenarios** (behavior changes that could affect existing users)
- Operational risks become **non-functional test scenarios** (failure modes, toil, cognitive load)

### Step 8: Produce the ADR Decomposition Summary

Before generating test cases, output a structured summary in this format:

```
## ADR Decomposition

**Feature:** <one-line from Executive Summary>

**ADR status / version:** <if stated: Proposed / Accepted / Superseded, date or revision—otherwise "Not stated">

**Components in scope:** <list of CRDs, controllers, APIs, resources>

**Positive-path requirements (from Goals):**
1. <requirement>
2. <requirement>
...

**Explicit non-goals (verbatim bullets):**
- <quote or bullet Non-Goals from ADR; if section absent, say "Section absent">

**Scope boundaries (from Non-Goals):**
- <what is excluded—may overlap verbatim list above>

**Implementation details requiring test coverage (from How):**
- <logic branch, dependency, migration path, error path>
...

**Risks requiring test coverage:**
- <risk → test implication>
...

**Open questions / areas needing exploratory coverage:**
- <unknown or ambiguity>
...
```

Do not proceed to test generation until this summary is complete.

---

## 5. Requirement Extraction

Transform the ADR decomposition into a numbered list of **testable requirements**. Assign **stable IDs** across the document: `REQ-001`, `REQ-002`, … or `<ADR_SLUG>-REQ-001` if a slug is obvious from the ADR title.

Before writing tests, **merge duplicate or overlapping requirement candidates** into a single REQ when they express the same observable behavior.

Each requirement must be:

- **Specific**: tied to an observable behavior, not a vague quality
- **Measurable**: has a concrete pass/fail criterion
- **Scoped**: relevant to the change described in the ADR

Organize requirements into these categories:

| Category | Source ADR Sections | What to Extract |
| --- | --- | --- |
| Functional requirements | What, Goals, How | Expected behavior under valid input and normal conditions |
| Negative-path requirements | How, Risks | Expected behavior under invalid input, missing dependencies, or error conditions |
| Regression requirements | Risks, Alternatives | Existing behavior that must NOT change as a side effect |
| Performance requirements | Risks, How | Latency, throughput, resource consumption thresholds |
| Security requirements | How, Risks | RBAC, secret handling, admission, privilege boundaries |
| Operational requirements | Risks | Recovery, availability, upgrade safety, observability |

---

## 6. Test Generation Rules

**Shift-left placement:** Derive **Tier 1–2** tests from the ADR **How** (contracts, branches, interactions). **Tier 3** covers **black-box** cluster outcomes tied to **Goals** and user-visible behavior. **Tier 4** covers human judgment (acceptance, usability, exploratory). **Tier 5** covers measurable quality attributes (performance, security, recovery, etc.). Omit NFT sub-types **not implied** by the ADR and justify omissions in **Uncovered Requirements**.

**Regression distinction:** **Tier 3** regression means **automated** checks on **user-visible workflows** (cluster/API observable). **Tier 5** regression (e.g. 5b) means **broader** release-style or soak/cross-cutting regression scenarios—do **not** duplicate the **same observable** in both tiers; pick one tier per scenario.

Generate test cases organized into five tiers. Each tier has a specific purpose, methodology, and minimum coverage expectation.

### Tier 1: Unit Tests (prefix: UT)

**Purpose:** Validate individual functions, methods, and helpers in isolation.

**Methodology:** White box. The tester has full knowledge of the code.

**What to test:**

- Pure functions that compute desired state, build Kubernetes objects, parse specs, or calculate status conditions
- Input validation logic at the function level
- Error handling branches in helper functions
- Edge cases in data transformation or serialization

**Derived from:** ADR "How" section (implementation details, logic branches, error paths)

**Minimum coverage:**

- At least one positive-path test per new or modified function
- At least one negative-path test per function that handles errors or invalid input

**Environment:** No cluster required. Use standard Go test tooling, mocks, and fakes.

**Do NOT include:** Cluster-level behavior, API server interactions, or multi-component flows.

---

### Tier 2: Integration Tests (prefix: INT)

**Purpose:** Validate that multiple components interact correctly when combined.

**Methodology:** Grey box. The tester knows the internal architecture but tests through component interfaces.

**What to test:**

- Reconciler behavior against a real or fake API server (envtest)
- Webhook admission logic with actual request/response cycles
- Interactions between the controller, API server, and dependent resources
- Status propagation across related objects
- Secret or ConfigMap consumption by the controller

**Derived from:** ADR "How" section (component interactions, dependencies) and "What" section (components touched)

**Minimum coverage:**

- At least one test per component interaction described in the ADR
- At least one test for the reconcile loop's primary success path
- At least one test for a reconcile error or requeue path

**Environment:** `envtest` or fake API server. No full cluster deployment required.

---

### Tier 3: E2E Automated Tests (prefix: E2E)

**Purpose:** Validate the operator's behavior in a real cluster from the consumer's perspective.

**Methodology:** Black box. The tester interacts only through Kubernetes APIs and observes cluster state.

**What to test:**

- Operator installation and CRD availability (smoke test)
- Custom resource creation with valid specs and verification of reconciled state
- Custom resource creation with invalid specs and verification of rejection or error reporting
- Status conditions, events, and managed workload readiness
- Update and upgrade flows
- Deletion, finalizer execution, and cleanup
- Regression scenarios: existing **user-visible** workflows that must remain stable (prefer Tier 3 over Tier 5 when the same scenario is automated black-box E2E—see §6 intro)

**Derived from:** ADR "What" and "Goals" sections (end-to-end behavior), "Risks" section (regression scenarios)

**Minimum coverage:**

- At least one **smoke test** (install, CRD present, controller ready, basic CR reaches healthy state)
- At least one **negative-input test** (invalid CR spec is rejected or produces clear error condition)
- At least one **regression test** (existing behavior that must not break)
- At least one **lifecycle test** (create, update, delete cycle)

**Environment:** Real Kubernetes or OpenShift cluster. Operator installed via OLM or direct deployment.

**Framework conventions:**

- Use Ginkgo v2 with `Describe` / `Context` / `It` structure
- Use `Eventually` and `Consistently` for async assertions
- Use `DeferCleanup` for resource teardown
- Apply at least one Ginkgo `Label` per `It` block

---

### Tier 4: Manual QE Tests (prefix: MQE)

**Purpose:** Validate the operator from a human administrator's perspective, covering usability, acceptance, and exploratory scenarios that are difficult or impractical to automate.

**Methodology:** Black box, performed by a human tester.

**What to test:**
- **Acceptance scenarios:** Can a cluster administrator install the operator, create the primary custom resource, observe it reaching a healthy state, and understand the status output using only the product documentation?
- **Usability scenarios:** Are error messages, events, and status conditions clear and actionable? Can a user troubleshoot a failure without reading source code?
- **Exploratory scenarios:** What happens when the tester deviates from the documented happy path? What happens under unexpected but plausible conditions (network blip during reconcile, node drain during install, manual deletion of a managed resource)?
- **Upgrade scenarios:** Can a user upgrade from the previous version without downtime or data loss? Are migration steps clear?
- **Documentation verification:** Does the documentation accurately describe the behavior observed in the cluster?

**Derived from:** ADR "Goals" section (what users should be able to do), "Risks" section (customer and operational risks), "How" section (migration, open questions)

**Minimum coverage:**

- At least one **acceptance scenario** (end-to-end happy path from a user's perspective)
- At least one **exploratory scenario** (unscripted deviation to probe resilience)

**Environment:** Real Kubernetes or OpenShift cluster. Manual execution with documented steps.

**Output format for manual tests:** Steps must be written so that any QE engineer can execute them without additional context. Include exact `kubectl` / `oc` commands, expected output snippets, and pass/fail criteria.

---

### Tier 5: Non-Functional Tests (prefix: NFT)

**Purpose:** Validate quality attributes beyond functional correctness: performance, security, reliability, recovery, scalability, and compliance.

**Methodology:** Varies by sub-type. Typically metrics-driven and automated where possible.

**Sub-types to consider (generate tests for those relevant to the ADR scope):**

#### 5a. Performance Testing

- **Load testing:** How does the operator behave under expected workload? Measure reconcile latency, queue depth, and resource consumption as the number of managed CRs increases.
- **Stress testing:** What is the breaking point? Push beyond expected capacity and identify where the operator degrades or fails.
- **Endurance testing:** Does the operator remain stable over extended periods under sustained load? Check for memory leaks, goroutine leaks, or increasing reconcile times.

Derived from: ADR "Risks" (execution risk, operational risk) and "How" (performance-sensitive paths)

Minimum: At least one performance scenario with a measurable threshold.

#### 5b. Regression Testing

- Verify that existing operator behavior not described in the ADR's "What" section remains unchanged after the feature is implemented.
- Focus on reconciliation paths, status reporting, upgrade flows, and cleanup behavior that existed before this change.

Derived from: ADR "Risks" (customer risk, behavior changes)

Minimum: At least one regression scenario that exercises a pre-existing workflow end to end **unless** the same observable is already covered as Tier 3 regression (avoid duplication per §6 intro).

#### 5c. Security Testing

- RBAC: Does the operator request only the minimum permissions it needs? Can it access resources outside its intended scope?
- Secrets: Is sensitive material protected in transit and at rest? Are secrets logged or leaked in events?
- Admission: Do webhooks reject malformed or unauthorized requests?
- Image: Are container images scanned for known vulnerabilities?
- SecurityContext: Do operand pods run with restricted profiles (`runAsNonRoot`, `allowPrivilegeEscalation: false`, `drop: [ALL]`)?

Derived from: ADR "How" (security design) and "Risks" (service and customer risk)

#### 5d. Recovery and Resilience Testing

- What happens when the operator pod crashes mid-reconcile?
- What happens when the API server becomes temporarily unavailable?
- What happens when a managed resource is manually deleted?
- How quickly does the operator recover and re-converge?

Derived from: ADR "Risks" (operational risk, failure modes)

Minimum: At least one recovery scenario.

#### 5e. Scalability Testing

- How does the operator perform as the number of managed namespaces, CRs, or cluster nodes increases?
- Are there known scaling limits or thresholds?

#### 5f. Compliance and Portability Testing

- Does the operator meet platform security baselines?
- Does it function across supported Kubernetes versions and distributions?

**Environment:** Real cluster; use tooling appropriate to each sub-type (e.g. Prometheus/Grafana for observability during performance testing).

---

## 7. Output Template

Structure your complete test plan output as follows:

```markdown
# Test Plan: <Feature Name from ADR Executive Summary>

**Sources:** ADR: <link, path, or "pasted markdown">; Jira: <key/URL or "none / unavailable">
**Date:** <generation date>
**Scope:** <one-line scope from ADR What section>

## Source conflicts

<List ADR vs Jira inconsistencies, or "None">

## ADR Decomposition

<paste the decomposition summary from Section 4, Step 8>

## Testable Requirements

| ID | Requirement | Category | ADR Source |
| --- | --- | --- | --- |
| REQ-001 | <requirement text> | Functional / Negative / Regression / Performance / Security / Operational | What / Goals / How / Risks |
| REQ-002 | ... | ... | ... |

## Test Cases

### Tier 1: Unit Tests

#### UT-001: <Title>

**Priority:** Critical / High / Medium
**Methodology:** White box
**Relevant Requirement(s):** REQ-NNN
**Traceability:** <ADR section or aligned Jira AC—never PR/commit>
**Preconditions:** <what must be true before the test runs>
**Steps:**
1. <concrete action>
   - **Expected:** <observable result>
2. <concrete action>
   - **Expected:** <observable result>
**Cleanup:** <resources or state to restore, or "None">
**Failure Impact:** <what breaks if this test fails>

(repeat for each unit test)

### Tier 2: Integration Tests

#### INT-001: <Title>

**Priority:** Critical / High / Medium
**Methodology:** Grey box
**Relevant Requirement(s):** REQ-NNN
**Traceability:** <ADR section or aligned Jira AC—never PR/commit>
**Preconditions:** <envtest running, CRDs loaded, etc.>
**Steps:**
1. <concrete action>
   - **Expected:** <observable result>
2. ...
**Cleanup:** <resources to delete>
**Failure Impact:** <what breaks if this test fails>

(repeat for each integration test)

### Tier 3: E2E Automated Tests

#### E2E-001: <Title>

**Priority:** Critical / High / Medium
**Methodology:** Black box
**Ginkgo Labels:** <label1, label2>
**Relevant Requirement(s):** REQ-NNN
**Traceability:** <ADR section or aligned Jira AC—never PR/commit>
**Preconditions:** <operator installed, cluster version, dependencies>
**Steps:**
1. <kubectl/oc command or API action>
   - **Expected:** <cluster state, status condition, event>
2. ...
**Cleanup:** <DeferCleanup actions, resources to remove>
**Failure Impact:** <what breaks if this test fails>

(repeat for each e2e test)

### Tier 4: Manual QE Tests

#### MQE-001: <Title>

**Priority:** Critical / High / Medium
**Methodology:** Black box (human execution)
**Type:** Acceptance / Usability / Exploratory / Upgrade / Documentation
**Relevant Requirement(s):** REQ-NNN
**Traceability:** <ADR section or aligned Jira AC—never PR/commit>
**Preconditions:** <cluster, operator version, documentation available>
**Steps:**
1. <exact command or user action>
   - **Expected:** <what the tester should observe>
2. ...
**Pass/Fail Criteria:** <concrete observable>
**Cleanup:** <what to restore>
**Failure Impact:** <what breaks if this test fails>

(repeat for each manual test)

### Tier 5: Non-Functional Tests

#### NFT-001: <Title>

**Priority:** Critical / High / Medium
**Sub-type:** Performance / Regression / Security / Recovery / Scalability / Compliance
**Methodology:** <Black box / White box / Metrics-driven>
**Relevant Requirement(s):** REQ-NNN
**Traceability:** <ADR section or aligned Jira AC—never PR/commit>
**Measurable Threshold:** <specific number, latency, error rate, resource limit>
**Preconditions:** <cluster size, load tooling, monitoring>
**Steps:**
1. <action>
   - **Expected:** <metric or observation>
2. ...
**Cleanup:** <what to restore>
**Failure Impact:** <what breaks if this test fails>

(repeat for each non-functional test)

## Traceability Matrix

| Requirement | UT | INT | E2E | MQE | NFT | Coverage Status |
| --- | --- | --- | --- | --- | --- | --- |
| REQ-001 | UT-001 | INT-001 | E2E-001 | - | - | Covered |
| REQ-002 | - | - | E2E-002 | MQE-001 | NFT-001 | Covered |
| REQ-003 | - | - | - | - | - | NOT COVERED (reason) |

## Uncovered Requirements

List any testable requirement from the ADR that has NO test coverage in this plan. For each, explain why:

- **REQ-NNN:** <requirement> - Not covered because <reason: out of scope for current tooling / requires infrastructure not available / deferred to future sprint / etc.>

## Coverage Summary

| Tier | Count | Critical | High | Medium |
| --- | --- | --- | --- | --- |
| Unit Tests | N | N | N | N |
| Integration Tests | N | N | N | N |
| E2E Automated Tests | N | N | N | N |
| Manual QE Tests | N | N | N | N |
| Non-Functional Tests | N | N | N | N |
| **Total** | **N** | **N** | **N** | **N** |
```

---

## 8. Quality Gates

Before returning the test plan, verify that all of the following are true. If any gate fails, revise the plan before outputting it.

| Gate | Requirement |
| --- | --- |
| ADR fully read | The ADR Decomposition Summary is complete for all sections present; missing sections are explicitly noted as absent |
| Requirements extracted | Every goal and every risk from the ADR maps to at least one testable requirement |
| Tier 1 coverage | Unit tests cover both positive and negative paths for logic introduced in the "How" section |
| Tier 2 coverage | Integration tests cover at least one component interaction per dependency described in the ADR |
| Tier 3 minimums | E2E tests include at least one smoke test, one negative-input test, one regression test, and one lifecycle test |
| Tier 4 minimums | Manual QE tests include at least one acceptance scenario and one exploratory scenario |
| Tier 5 minimums | Non-functional tests include at least one performance scenario and one recovery/resilience scenario |
| Traceability complete | Every requirement appears in the traceability matrix with at least one test or an explicit "NOT COVERED" entry with justification |
| Traceability source | Every test case cites ADR (or aligned Jira AC) in **Traceability**—not PRs, commits, or diffs |
| Tier regression consistency | No duplicate **same observable** regression scenario split across Tier 3 and Tier 5 unless rationale is stated |
| No vague steps | Every test step describes a concrete action and a concrete expected outcome; no step says "verify it works" without specifying what "works" means |
| Cleanup specified | Every test that creates resources specifies how those resources are cleaned up |
| Priority assigned | Every test case has a priority (Critical / High / Medium) |
| Scope respected | No test case targets behavior listed under the ADR's "Non-Goals" unless it is explicitly a regression guard |

---

## 9. Methodology and scope boundaries

### Methodology definitions

When assigning a methodology to each test case, use these definitions:

| Methodology | When to Use | Operator Example |
| --- | --- | --- |
| **Black box** | Tester has no knowledge of internal code. Tests only through external interfaces (Kubernetes API, CLI, cluster state). | Create a CR via `kubectl apply` and verify the managed Deployment becomes Ready |
| **White box** | Tester has full knowledge of code. Tests target specific branches, functions, and error paths. | Unit test a helper function that builds a StatefulSet spec from CR fields |
| **Grey box** | Tester has partial knowledge. Tests combine external interaction with internal observation (logs, metrics, queue state). | Create a CR and verify reconciliation while monitoring controller metrics and log output for expected internal behavior |

### Scope boundary rules

- Test cases MUST be relevant to the scope of the change described in the ADR.
- Test cases NEED NOT trace to a single specific ADR section, but each must be justifiable as relevant to the feature or its risks.
- Do NOT generate test cases for behavior listed under "Non-Goals" unless the test is explicitly a regression guard protecting existing functionality.
- If the ADR describes open questions or known unknowns, generate at least one exploratory test case (Tier 4) targeting that area.
- If the ADR's risk table identifies a customer risk involving behavior change, generate at least one regression test (Tier 3 or Tier 5) confirming the old behavior is preserved where expected.
