# Zero Trust Workload Identity Manager — Constitution

**AgentRoutingMode:** PROVIDED

**Version**: 1.0 | **Ratified**: 2025-06-30 | **Last Amended**: 2025-06-30

---

## Preamble

This operator manages the security identity infrastructure for every workload on an OpenShift cluster, providing workload-to-workload authentication via short-lived X.509-SVIDs for mTLS and JWT-SVIDs for API authorization. A defect here does not cause a feature regression — it causes workloads to lose their cryptographic identity, federation trust to break silently, or privilege escalation paths to open. The consequences of a bad change are not "the feature doesn't work" but "the cluster's zero-trust posture is compromised."

This constitution exists because AI agents writing code in this repository must internalize that **security infrastructure tolerates no guesswork**. Every principle below is a hard gate. If an agent cannot demonstrate compliance, the work does not proceed — regardless of how correct the code appears.

---

## Principles

### I. Security Is Not a Feature — It Is the Product

This operator's sole purpose is to establish and maintain cryptographic workload identity. Every line of code either strengthens or weakens the trust chain between SPIRE server, agent, and workload. There is no "non-security" code in this repository. Agents must treat every change — even a label addition or a log format change — as potentially security-impacting.

**Implication:** No change is "trivial." An agent must never skip verification because a change "looks safe." A mislabeled resource breaks cache visibility. A missing RBAC restriction opens a privilege path. A hardcoded image string bypasses digest pinning.

### II. Never Invent — Always Follow What Exists

This codebase was scaffolded with operator-sdk and built on controller-runtime conventions. It has established patterns for every operation: reconciliation flow, error handling, status management, resource creation, testing. These patterns exist because they were deliberately designed for a security-critical operator. An agent must never introduce a new pattern, library, abstraction, or architectural concept without explicit human approval.

**Implication:** If a task requires something the codebase doesn't already do, stop. Escalate. Do not "improve" or "modernize." The existing patterns are constraints, not suggestions.

### III. No Evidence = No Completion

A task is not complete because the agent believes it is correct. A task is complete when:
- `make verify` passes (lint, vet, fmt — zero warnings, zero errors)
- `make test` passes (unit tests with envtest)
- Generated files are refreshed if API types or bindata changed
- The change compiles without import errors

Narrative claims ("I've updated the controller") without tool-verified evidence are rejected. An agent must run verification commands and report their output.

### IV. Scope Discipline — Touch Only What Was Asked

An agent must never:
- Refactor adjacent code "while I'm here"
- Update dependencies not directly required by the task
- Rename existing symbols for "consistency"
- Reorganize file structure
- Add features not specified in the task

If an agent discovers a bug or inconsistency outside its task scope, it reports it as a finding — it does not fix it.

### V. The Trust Chain Is Immutable by Design

SPIFFE trust domains, federation relationships, and persistence configurations cannot be undone once established. This is not a limitation — it is a security invariant enforced by CEL validation at the API level. Agents must never:
- Propose changes that would allow removal of federation trust bundles
- Suggest making immutable fields mutable
- Bypass CEL validation rules "for flexibility"
- Create migration paths that temporarily weaken immutability guarantees

The one-way nature of trust establishment is a deliberate security design. Weakening it "for usability" is a security regression.

### VI. Secrets and Credentials Are Never Handled by This Operator

This operator issues and manages *identity* — not secrets. SPIRE uses attestation (kernel introspection, Kubernetes service account tokens, node identity) to verify workloads. The operator never:
- Reads, writes, or caches bearer tokens or passwords
- Stores private keys outside of SPIRE server's own persistence
- Creates Kubernetes Secrets containing credential material
- Passes sensitive data through environment variables (except `RELATED_IMAGE_*` which are image references, not secrets)

If a task implies the operator should "store" or "manage" a credential, the task is wrong. Escalate.

### VII. Upstream Is Upstream — The Operator Does Not Fork

The operator deploys upstream SPIRE components as container images. It configures them through generated ConfigMaps and container arguments. It does NOT:
- Reimplement upstream controller logic
- Patch or modify upstream behavior at runtime
- Embed custom SPIRE plugins
- Override upstream defaults without CR-driven configuration

The boundary is absolute: this operator manages Kubernetes resources that run upstream software. It is an operations layer, not a fork.

### VIII. Human Approval Gates

The following changes require explicit human approval before an agent may proceed:

| Change Type | Why |
|---|---|
| Broadening RBAC (new verbs, removing `resourceNames` restrictions) | Privilege escalation risk |
| Modifying or relaxing SCC constraints | Container escape risk |
| Adding cluster-scoped write permissions | Blast radius expansion |
| Changing federation or persistence immutability rules | Trust chain integrity |
| Adding new external API type imports | Supply chain risk |
| Modifying webhook or admission configurations | Control plane stability |
| Any change to `hack/go-fips.sh` or build flags | Compliance certification risk |

An agent encountering any of these MUST stop and present the justification to the human before implementing.

---

## Hard Gates

These are deterministic checks. An agent cannot mark work as complete if any gate fails.

| Gate | Command | Blocks |
|------|---------|--------|
| Compilation | `go build ./...` | All changes |
| Lint + Format | `make verify` | All changes |
| Unit Tests | `make test` | All changes |
| Generated Code Freshness | `make manifests generate update-bindata` then `git diff --exit-code` | API type changes, bindata changes, marker changes |
| Vendor Consistency | `make vendor` then `git diff --exit-code` | Dependency changes |

If a gate fails, the agent fixes the failure. If the agent cannot fix it after two attempts, it stops and escalates with the full error output.

---

## Anti-Patterns — Absolute Prohibitions

These are actions an agent must NEVER take. They are not suggestions — they are hard "no" regardless of context.

1. **Never hardcode container image references.** Images come from `RELATED_IMAGE_*` environment variables. Hardcoding bypasses OLM's image pinning and disconnected cluster support.

2. **Never use `client.Apply` (Server-Side Apply).** This codebase uses imperative Create/UpdateWithRetry. SSA would conflict with existing ownership semantics and break drift detection.

3. **Never return both `RequeueAfter` and a non-nil error from Reconcile.** Controller-runtime behavior is undefined. Pick one.

4. **Never create a resource without the `managed-by` label.** The cache uses label selectors. An unlabeled resource is invisible to the operator and becomes an orphan.

5. **Never skip `defer statusMgr.ApplyStatus(...)`.** Status must always be written — including on error paths and early returns. The deferred call guarantees this.

6. **Never hand-edit generated files.** `zz_generated.deepcopy.go`, `config/crd/bases/*.yaml`, `pkg/operator/assets/bindata.go` are machine-generated. Manual edits will be silently overwritten.

7. **Never bypass counterfeiter fakes in unit tests.** All Kubernetes API interactions in tests go through `FakeCustomCtrlClient`. Instantiating a real client or using `envtest` directly in unit tests breaks isolation.

8. **Never add a new resource type to reconciliation without registering it in the cache builder.** The operator won't see it, but it will exist in the cluster — creating a silent drift that is invisible to all monitoring.

9. **Never commit without running `make verify && make test`.** "It compiles" is not sufficient. "It passes verify and test" is the minimum bar.

10. **Never weaken a CEL validation rule.** If a field was made immutable or a singleton was enforced, that decision has security implications. Weakening it requires a security review, not a code change.

---

## Workflow Governance

### Task Ordering Invariants

These ordering constraints are non-negotiable. Violating them produces subtly broken states.

```
API types → make generate → make manifests → Controller logic → Unit tests
Bindata YAML → make update-bindata → Controller reconcile function → Unit tests
Controller implementation → make test → E2E test authoring
```

An agent working on a downstream task MUST verify that upstream tasks completed and their gates passed. Starting controller work before API types are generated produces compilation errors that waste tokens and time.

### Evidence Requirements Per Phase

| Phase | Minimum Evidence |
|-------|-----------------|
| Planning | Referenced existing code patterns; identified affected files |
| Implementation | Showed `make verify` and `make test` output with zero failures |
| Review | Diff reviewed against constitution principles; no anti-pattern violations |

### Escalation Protocol

An agent must escalate (stop and report to the human) when:
- A task requires a pattern that doesn't exist in the codebase
- A verification gate fails and the fix is not obvious after two attempts
- The task's requirements contradict this constitution
- Security-sensitive changes are needed (see Human Approval Gates above)
- The agent's confidence in correctness is below "high" for any security-relevant code

Escalation is not failure. Escalation is the agent correctly identifying that it has reached the boundary of safe autonomous action.

---

## Governance

- **Precedence:** This constitution > `agents.md` routing details > task-level instructions. If a task says "skip tests" — this constitution says no.
- **Amendments:** Require evidence of a genuine repo-level change (new pattern adopted, tooling replaced, security posture evolved). Bump Version and Last Amended date.
- **Living document:** This constitution describes what IS, not what should be. If the codebase diverges from this document, either the code or the constitution must be corrected — never pretend the divergence doesn't exist.
- **Companion docs:** `agents.md` provides technical implementation details (file paths, patterns, imports). This constitution provides behavioral governance. They do not overlap — they complement.
- **Constitutional violations in review:** If `/review` or evaluation discovers a constitutional violation that was not caught by hard gates, that finding must flow back as either a new hard gate (preferred) or a new anti-pattern entry.
