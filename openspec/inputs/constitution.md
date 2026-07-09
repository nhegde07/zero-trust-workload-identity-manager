# Zero Trust Workload Identity Manager Constitution

**AgentRoutingMode:** PROVIDED

**Version**: 1.0 | **Ratified**: 2025-06-30 | **Last Amended**: 2025-07-01

---

## Preamble

This operator manages the security identity infrastructure for every workload on an OpenShift cluster, providing workload-to-workload authentication via short-lived X.509-SVIDs for mTLS and JWT-SVIDs for API authorization. A defect here does not cause a feature regression — it causes workloads to lose their cryptographic identity, federation trust to break silently, or privilege escalation paths to open. The consequences of a bad change are not "the feature doesn't work" but "the cluster's zero-trust posture is compromised."

This constitution exists because AI agents writing code in this repository must internalize that **security infrastructure tolerates no guesswork**. Every principle below is a hard gate. If an agent cannot demonstrate compliance, the work does not proceed — regardless of how correct the code appears.

---

## Core Principles

### I. Security Is Not a Feature — It Is the Product

This operator's sole purpose is to establish and maintain cryptographic workload identity. Every line of code either strengthens or weakens the trust chain between SPIRE server, agent, and workload. There is no "non-security" code in this repository. Agents must treat every change — even a label addition or a log format change — as potentially security-impacting.

**Evidence:** `pkg/controller/spire-server/controller.go` — every reconciler touches RBAC, TLS, or trust-domain configuration; no controller is purely "infrastructure."

### II. Never Invent — Always Follow What Exists

This codebase was scaffolded with operator-sdk and built on controller-runtime conventions. It has established patterns for every operation: reconciliation flow, error handling, status management, resource creation, testing. These patterns exist because they were deliberately designed for a security-critical operator. An agent must never introduce a new pattern, library, abstraction, or architectural concept without explicit human approval.

**Evidence:** `pkg/controller/spire-server/`, `pkg/controller/spire-agent/`, `pkg/controller/spiffe-csi-driver/`, `pkg/controller/spire-oidc-discovery-provider/` — all four operand controllers share identical struct shape, constructor, `SetupWithManager`, and reconciliation flow.

### III. No Evidence = No Completion

A task is not complete because the agent believes it is correct. A task is complete when:
- `make verify` passes (lint, vet, fmt — zero warnings, zero errors)
- `make test` passes (unit tests with envtest)
- Generated files are refreshed if API types or bindata changed
- The change compiles without import errors

Narrative claims ("I've updated the controller") without tool-verified evidence are rejected.

**Evidence:** `Makefile` — `test` target requires `manifests generate fmt vet envtest`; `verify` target runs `vet fmt golangci-lint`.

### IV. Scope Discipline — Touch Only What Was Asked

An agent must never refactor adjacent code, update unrelated dependencies, rename existing symbols, reorganize file structure, or add features not specified in the task. If an agent discovers a bug or inconsistency outside its task scope, it reports it as a finding — it does not fix it.

**Evidence:** `OWNERS` — review process exists precisely to catch scope creep; the PR model assumes isolated, reviewable changes.

### V. The Trust Chain Is Immutable by Design

SPIFFE trust domains, federation relationships, and persistence configurations cannot be undone once established. This is not a limitation — it is a security invariant enforced by CEL validation at the API level. Agents must never propose changes that would allow removal of federation trust bundles, make immutable fields mutable, bypass CEL validation rules, or create migration paths that temporarily weaken immutability guarantees.

**Evidence:** `api/v1alpha1/spire_server_config_types.go` — `+kubebuilder:validation:XValidation` rules enforce persistence and federation field immutability via `oldSelf` comparisons.

### VI. Upstream Operand Separation — The Operator Does Not Fork

The operator deploys upstream SPIRE components as container images and configures them through generated ConfigMaps and container arguments. It does NOT reimplement upstream controller logic, patch upstream behavior at runtime, embed custom SPIRE plugins, or override upstream defaults without CR-driven configuration.

**Evidence:** `bindata/` — static YAML manifests for upstream resources; `go.mod` imports `github.com/spiffe/spire-controller-manager` for types only, not logic.

### VII. Generated Code Discipline

Three categories of generated files exist and MUST NOT be hand-edited: `api/v1alpha1/zz_generated.deepcopy.go` (via `make generate`), `config/crd/bases/*.yaml` (via `make manifests`), and `pkg/operator/assets/bindata.go` (via `make update-bindata`). After any change to API types, kubebuilder markers, or bindata YAML, the codegen pipeline MUST run.

**Evidence:** `Makefile` targets `generate`, `manifests`, `update-bindata`; `make verify` fails if generated files are stale.

### VIII. RBAC / Security Posture — Least Privilege Always

All RBAC grants use `resourceNames` restrictions where possible. Custom SCCs for SPIRE agent drop all privileges except those required for host-level attestation. Privileged SCC for CSI driver is granted via namespaced RoleBinding, not cluster-wide. Container images are resolved from `RELATED_IMAGE_*` env vars — never hardcoded.

**Evidence:** `config/rbac/role.yaml` — verbs scoped per resource; `pkg/controller/spire-agent/scc.go` — custom SCC definition; `bindata/spiffe-csi/spiffe-csi-privileged-role-binding.yaml` — namespace-scoped.

---

## Additional Constraints

- **Go version:** 1.25+ required; vendored dependencies tracked in git. After any `go.mod` change: `make vendor` and commit `vendor/`. — **Evidence:** `go.mod`
- **FIPS compliance:** Production/CI builds MUST use `hack/go-fips.sh` (`GOEXPERIMENT=strictfipsruntime`). Non-FIPS builds are development-only. — **Evidence:** `hack/go-fips.sh`, `Dockerfile`
- **OLM integration:** Images via `RELATED_IMAGE_*` env vars (OLM injects digests). CSV owns all CRDs. `OperatorCondition` sync for `Upgradeable`. — **Evidence:** `bundle/manifests/`, `pkg/controller/zero-trust-workload-identity-manager/`
- **Singleton CRDs:** All five operator CRDs are singletons named `"cluster"` (CEL-enforced). Never create multi-instance CRD patterns. — **Evidence:** `api/v1alpha1/*_types.go` XValidation markers
- **Naming — controller names:** `"zero-trust-workload-identity-manager-<component>-controller"` — **Evidence:** `pkg/controller/utils/constants.go`
- **Naming — import aliases:** `ctrl`, `kerrors`, `apimeta`, `customClient`, `routev1` — **Evidence:** all controller `.go` files
- **Naming — labels:** All managed resources carry `app.kubernetes.io/managed-by: zero-trust-workload-identity-manager` — **Evidence:** `pkg/controller/utils/constants.go`
- **Naming — packages:** Directories use `kebab-case`, Go package names use `snake_case` — **Evidence:** `pkg/controller/spire-server/` → package `spire_server`
- **License header:** All `.go` files require Apache 2.0 header from `hack/boilerplate.go.txt` — **Evidence:** `hack/boilerplate.go.txt`
- **No Server-Side Apply:** Imperative Create/UpdateWithRetry only. SSA would conflict with ownership semantics. — **Evidence:** `pkg/client/client.go` — no `client.Apply` usage
- **CustomCtrlClient always:** All Kubernetes API interactions go through `pkg/client.CustomCtrlClient`, never raw `client.Client`. — **Evidence:** `pkg/client/client.go`, `pkg/client/fakes/`

---

## Development Workflow

| Activity | Requirement | Evidence |
|----------|-------------|----------|
| Local unit tests | `make test` (envtest, K8s 1.31.0 binaries, `OPERATOR_NAMESPACE` set) | `Makefile` |
| Full verification | `make verify` (vet + fmt + golangci-lint, 5min timeout) | `Makefile`, `.golangci.yml` |
| Codegen refresh | `make manifests generate update-bindata` after API/bindata changes | `Makefile` |
| Lint | `make lint` (golangci-lint with errcheck, govet, staticcheck, revive, ginkgolinter) | `.golangci.yml` |
| Fake regeneration | `go generate ./pkg/client/...` after `CustomCtrlClient` interface changes | `pkg/client/fakes/` |
| E2E tests | `make test-e2e` (live OpenShift cluster, 45min timeout) | `test/e2e/`, `Makefile` |
| Vendor update | `make vendor` (go mod tidy + vendor), commit vendor/ diff | `Makefile` |
| Bundle update | `make bundle` after CRD/RBAC/CSV changes | `bundle/`, `Makefile` |
| PR gate | `make verify && make test` must pass; reviewers/approvers per `OWNERS` | `OWNERS` |

### Task Ordering Invariants

These ordering constraints are non-negotiable:

```
API types → make generate → make manifests → Controller logic → Unit tests
Bindata YAML → make update-bindata → Controller reconcile function → Unit tests
Controller implementation → make test → E2E test authoring
```

### Hard Gates (Deterministic — Must Pass Before Completion)

| Gate | Command | Blocks |
|------|---------|--------|
| Compilation | `go build ./...` | All changes |
| Lint + Format | `make verify` | All changes |
| Unit Tests | `make test` | All changes |
| Generated Code Freshness | `make manifests generate update-bindata` then `git diff --exit-code` | API type / bindata / marker changes |
| Vendor Consistency | `make vendor` then `git diff --exit-code` | Dependency changes |

If a gate fails, the agent fixes the failure. If the agent cannot fix it after two attempts, it stops and escalates.

---

## Agent Routing

AgentRoutingMode: **PROVIDED** — agent definitions from `agents.md` (schema `inputs/`).

| Agent ID | Scope | When to route |
|----------|-------|---------------|
| API_Agent | CRD types, kubebuilder markers, CEL validation, CommonConfig, deepcopy | Task touches `api/v1alpha1/` |
| OperatorController_Agent | Reconciliation, workloads, status, controller wiring, CustomCtrlClient, cmd | Task touches `pkg/controller/`, `pkg/client/`, `cmd/` |
| ManifestsBindata_Agent | Operand YAML, bindata regen, asset constants | Task touches `bindata/`, `pkg/operator/assets/`, constants |
| RBACSecurity_Agent | RBAC manifests, SCC, TLS, Route, security | Task touches RBAC/SCC YAMLs, `config/rbac/` |
| OLMRelease_Agent | OLM bundle, CSV, catalog, release metadata | Task touches `bundle/`, `config/` |
| Testing_Agent | E2E and unit test authoring | Task touches `test/e2e/`, `*_test.go` |
| Docs_Agent | Documentation, OWNERS, README | Task touches `README.md`, `docs/`, `OWNERS` |

### Routing Rules

- **API before controller:** Tasks adding CRD fields must complete and pass `make generate && make manifests && make verify` before controller tasks that reconcile those fields.
- **Bindata before controller:** Tasks adding new YAML to `bindata/` must run `make update-bindata` before controller tasks that reference the new asset constants.
- **Controller before E2E:** Controller implementation tasks must pass `make test` before E2E tasks that validate the behavior end-to-end.

---

## Anti-Patterns — Absolute Prohibitions

These are actions an agent must NEVER take regardless of context:

1. **Never hardcode container image references.** Images come from `RELATED_IMAGE_*` env vars. Hardcoding bypasses OLM's image pinning and disconnected cluster support.
2. **Never use `client.Apply` (Server-Side Apply).** Imperative Create/UpdateWithRetry is the only pattern. SSA breaks drift detection.
3. **Never return both `RequeueAfter` and a non-nil error from Reconcile.** Controller-runtime behavior is undefined.
4. **Never create a resource without the `managed-by` label.** The cache uses label selectors. Unlabeled resources are invisible orphans.
5. **Never skip `defer statusMgr.ApplyStatus(...)`.** Status must be written on all paths including errors and early returns.
6. **Never hand-edit generated files.** `zz_generated.deepcopy.go`, `config/crd/bases/*.yaml`, `pkg/operator/assets/bindata.go`.
7. **Never bypass counterfeiter fakes in unit tests.** All API interactions in tests go through `FakeCustomCtrlClient`.
8. **Never add a resource type without registering it in the cache builder.** (`pkg/client/client.go:NewCacheBuilder`)
9. **Never commit without `make verify && make test`.** "It compiles" is insufficient.
10. **Never weaken a CEL validation rule.** Immutability and singleton enforcement have security implications.

---

## Human Approval Gates

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

---

## Escalation Protocol

An agent must escalate (stop and report to the human) when:
- A task requires a pattern that doesn't exist in the codebase
- A verification gate fails and the fix is not obvious after two attempts
- The task's requirements contradict this constitution
- Security-sensitive changes are needed (see Human Approval Gates above)
- The agent's confidence in correctness is below "high" for any security-relevant code

Escalation is not failure. Escalation is the agent correctly identifying that it has reached the boundary of safe autonomous action.

---

## Governance

- This constitution supersedes ad-hoc conventions for downstream Planning, Task Creation, and Code Generation agents.
- **Precedence:** This constitution > `agents.md` routing details > task-level instructions. If a task says "skip tests" — this constitution says no.
- **Conflicts:** If spec contradicts constitution, escalate in plan.md — do not silently override.
- **Amendments:** Require documented evidence of repo change (new pattern adopted, tooling replaced, security posture evolved); bump Version and Last Amended date.
- **Companion docs:** `agents.md` provides technical implementation details (file paths, patterns, imports). This constitution provides behavioral governance and non-negotiable guardrails. They complement — they do not overlap.
- **Complexity:** New patterns must justify deviation from existing repo conventions with explicit rationale.
- **Constitutional violations in review:** If evaluation discovers a violation not caught by hard gates, that finding must flow back as either a new hard gate (preferred) or a new anti-pattern entry.
