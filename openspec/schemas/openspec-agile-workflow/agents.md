This file provides guidance to AI agents working with the **zero-trust-workload-identity-manager** for OpenShift — a Go operator (controller-runtime) managing upstream SPIFFE/SPIRE components as static YAML manifests (bindata) applied imperatively. It does NOT embed upstream code; it deploys upstream container images.

## Docs Index

Detailed domain-specific guidelines are in these files — read them before working in the corresponding area:

- [docs/error-handling-guidelines.md](docs/error-handling-guidelines.md) — Error wrapping, status conditions, retry logic, ReconcileError classification
- [docs/testing-guidelines.md](docs/testing-guidelines.md) — Unit test patterns, FakeCustomCtrlClient, E2E with Ginkgo, test helpers
- [docs/api-contracts-guidelines.md](docs/api-contracts-guidelines.md) — CRD types, kubebuilder markers, CEL validation, CommonConfig, code generation
- [docs/security-guidelines.md](docs/security-guidelines.md) — FIPS builds, RBAC, SCCs, TLS, federation security, metrics protection
- [docs/performance-guidelines.md](docs/performance-guidelines.md) — Cache architecture, watch predicates, drift detection, status update optimization
- [docs/integration-guidelines.md](docs/integration-guidelines.md) — Bindata pattern, ConfigMap generation, federation, SPIRE controller-manager, OpenShift platform

## Repository Layout

```
api/v1alpha1/                    CRD types (ZTWIM, SpireServer, SpireAgent, SpiffeCSIDriver, SpireOIDCDiscoveryProvider), conditions, meta, deepcopy
bindata/                         Static operand YAML manifests (compiled via go-bindata)
  spire-server/                  Server RBAC, ServiceAccount, Service, external-cert roles
  spire-agent/                   Agent RBAC, ServiceAccount, Service
  spire-bundle/                  Bundle role + binding
  spire-controller-manager/      Controller-manager RBAC, webhook service, webhook config
  spire-oidc-discovery-provider/ OIDC provider RBAC, ServiceAccount, Service, external-cert roles
  spiffe-csi/                    CSI driver ServiceAccount, RoleBinding (privileged SCC), CSIDriver object
bundle/manifests/                OLM bundle (CSV, CRDs, metadata)
cmd/zero-trust-workload-identity-manager/  Operator entrypoint (main.go)
config/                          Kustomize manifests (CRDs, RBAC, manager, samples)
hack/                            go-fips.sh, boilerplate license header
pkg/controller/
  zero-trust-workload-identity-manager/  Status aggregation controller (watches all operand CRs)
  spire-server/                  SPIRE server operand controller (StatefulSet, ConfigMaps, Routes, federation, webhook)
  spire-agent/                   SPIRE agent operand controller (DaemonSet, SCC, ConfigMap)
  spiffe-csi-driver/             SPIFFE CSI driver operand controller (DaemonSet, CSIDriver)
  spire-oidc-discovery-provider/ OIDC discovery provider operand controller (Deployment, Routes, ClusterSPIFFEIDs)
  status/                        Shared status management (condition collection, auto-Ready derivation)
  utils/                         Constants, predicates, validation, resource comparison, errors, labels
pkg/client/                      CustomCtrlClient interface + counterfeiter fakes + cache builder
pkg/client/fakes/                Generated fake_custom_ctrl_client.go (counterfeiter)
pkg/operator/assets/             Generated bindata.go (NEVER hand-edit)
pkg/version/                     Build-time version info (ldflags)
test/e2e/                        End-to-end tests (Ginkgo v2 + live OpenShift cluster)
tools/                           Go module for build-time tool dependencies
vendor/                          Tracked vendored dependencies
```

## Single Controller Pattern — controller-runtime Only

This operator uses **only controller-runtime** (no library-go, no informer factories). All five controllers share a single `ctrl.Manager` created in `main.go`. Never create separate managers.

| Aspect | All controllers |
|--------|----------------|
| Framework | controller-runtime, `ctrl.Manager` |
| API spec | Domain-specific specs per CRD — never embed `operatorv1.OperatorSpec` |
| Client | `pkg/client.CustomCtrlClient` (wraps controller-runtime `client.Client`) |
| Resource apply | Imperative Create/UpdateWithRetry (NOT SSA) |
| Status | `pkg/controller/status.Manager` with deferred `ApplyStatus()` |

## Five Controllers — Architecture

| Controller | Package | Watches | Purpose |
|---|---|---|---|
| `zero-trust-workload-identity-manager-controller` | `pkg/controller/zero-trust-workload-identity-manager/` | ZTWIM CR + all operand CR statuses + `OperatorCondition` | Aggregates operand statuses, sets Ready/OperandsAvailable/CreateOnlyMode, syncs Upgradeable to OLM OperatorCondition |
| `zero-trust-workload-identity-manager-spire-server-controller` | `pkg/controller/spire-server/` | `SpireServer` CR + ZTWIM CR + managed resources | Reconciles StatefulSet, RBAC, ConfigMaps, webhooks, Routes, federation |
| `zero-trust-workload-identity-manager-spire-agent-controller` | `pkg/controller/spire-agent/` | `SpireAgent` CR + ZTWIM CR + managed resources | Reconciles DaemonSet, RBAC, SCC, ConfigMap |
| `zero-trust-workload-identity-manager-spiffe-csi-driver-controller` | `pkg/controller/spiffe-csi-driver/` | `SpiffeCSIDriver` CR + ZTWIM CR + managed resources | Reconciles DaemonSet, RoleBinding (privileged SCC), CSIDriver |
| `zero-trust-workload-identity-manager-spire-oidc-discovery-provider-controller` | `pkg/controller/spire-oidc-discovery-provider/` | `SpireOIDCDiscoveryProvider` CR + ZTWIM CR + managed resources | Reconciles SA, Service, ClusterSPIFFEIDs, ConfigMap, Deployment, RBAC, Route |

## Shared Packages — Never Duplicate

| Package / Symbol | Use for |
|---|---|
| `pkg/client.CustomCtrlClient` | All K8s operations (Get/List/Create/UpdateWithRetry/StatusUpdateWithRetry/CreateOrUpdateObject/Exists) |
| `pkg/client.NewCacheBuilder()` | Unified cache with label selectors — registered in `main.go` |
| `pkg/client/fakes.FakeCustomCtrlClient` | Unit test mocking (counterfeiter-generated) |
| `pkg/controller/status.NewManager()` | Status condition management + auto-Ready derivation |
| `pkg/controller/status.Manager.AddCondition()` | Record typed conditions during reconciliation |
| `pkg/controller/status.Manager.ApplyStatus()` | Deferred write — skips no-op updates via semantic equality |
| `pkg/controller/utils.ResourceNeedsUpdate()` | Field-level comparison per resource type (never full DeepEqual) |
| `pkg/controller/utils.ValidateAndUpdateStatus()` | CommonConfig validation (affinity, tolerations, nodeSelector, resources, labels) |
| `pkg/controller/utils.GetOperatorNamespace()` | Read `OPERATOR_NAMESPACE` env |
| `pkg/controller/utils.GetRelatedImage()` | Read `RELATED_IMAGE_*` env vars |
| `pkg/controller/utils` predicates | `GenerationOrOwnerReferenceChangedPredicate`, `ZTWIMSpecChangedPredicate`, `ControllerManagedResourcesForComponent` |
| `pkg/operator/assets.MustAsset()` | Decode bindata YAML at reconcile time |

## Operand Reconciliation Flow (All Operand Controllers)

Every operand reconciler follows this exact flow. Do NOT deviate:

1. `Get` the operand CR (`cluster`); `IsNotFound` → return nil (no requeue).
2. Create `status.NewManager(...)` with **`defer statusMgr.ApplyStatus(...)`** (auto-calls `SetReadyCondition()` if Ready not explicitly set).
3. `Get` the parent `ZeroTrustWorkloadIdentityManager` CR (`cluster`); missing → `Ready=False/Failed`, return nil.
4. Set controller reference from ZTWIM → operand if needed.
5. Check `CREATE_ONLY_MODE` env via `handleCreateOnlyMode`.
6. **Validate configuration** (`validateConfiguration` / `ValidateAndUpdateStatus`); if invalid → set condition, return nil (no requeue).
7. Run ordered `reconcile*` steps (SA → Service → RBAC → ConfigMaps → workload → Route...).
8. Each step adds a typed condition to the status manager.

**Key rules:**
- Never return both `RequeueAfter` and a non-nil error from `Reconcile`
- Status writes use `StatusUpdateWithRetry` with retry-on-conflict
- `ApplyStatus` uses `k8s.io/apimachinery/pkg/api/equality` to skip no-op writes

## ZTWIM Aggregator Pattern

The top-level `ZeroTrustWorkloadIdentityManager` controller does NOT create operand CRs. It:
1. Reads each operand CR's status and aggregates into `status.operands`
2. Sets `Ready`, `OperandsAvailable`, and `CreateOnlyMode` conditions on the ZTWIM CR
3. Syncs `Upgradeable` to the OLM `OperatorCondition` resource (best-effort)

Watches: Own CR with `predicate.GenerationChangedPredicate` (standard, not custom); all four operand CRs with `operandStatusChangedPredicate`; `OperatorCondition` with same.

## Static Manifest (Bindata) Pattern

All operand Kubernetes resources live as YAML in `bindata/` organized by component. Compiled into `pkg/operator/assets/bindata.go` via go-bindata.

At reconcile time:
1. Decode bytes with `assets.MustAsset(path)` and runtime deserialization
2. Mutate the decoded object: set namespace, merge labels from `CommonConfig`, set owner references
3. Check existence via `Exists()`, compare with `ResourceNeedsUpdate()`, then `Create` or `UpdateWithRetry`

When adding a new resource: add YAML to `bindata/<component>/`, add constant in `utils/constants.go`, run `make update-bindata`, follow existing `reconcile*` patterns.

## Watch and Predicate Conventions

- **Operand controllers** watch their own CR with `GenerationOrOwnerReferenceChangedPredicate` and managed resources with component-specific label predicates using `ComponentControlPlane`, `ComponentNodeAgent`, `ComponentCSI`, `ComponentDiscovery`.
- **All operand controllers** also watch `ZeroTrustWorkloadIdentityManager` CR with `ZTWIMSpecChangedPredicate` (re-reconcile when parent spec changes).
- **ZTWIM controller** uses standard `predicate.GenerationChangedPredicate` on itself.
- All managed resources carry `app.kubernetes.io/managed-by: zero-trust-workload-identity-manager` label.

## CRD Conventions

All five CRDs are **cluster-scoped singletons** named `"cluster"` (CEL-enforced: `self.metadata.name == 'cluster'`).

| CRD | Spec highlights |
|---|---|
| `ZeroTrustWorkloadIdentityManager` | Global operand config, aggregated status with `Operands[]` |
| `SpireServer` | Trust domain, TTLs, persistence (immutable fields), federation (cannot be removed), CA key type |
| `SpireAgent` | Workload attestor config, kubelet verification type (skip/auto/hostCert) |
| `SpiffeCSIDriver` | Minimal spec (CommonConfig only) |
| `SpireOIDCDiscoveryProvider` | OIDC-specific settings |

**CEL patterns**: singleton (`self.metadata.name == 'cluster'`), immutable fields (`oldSelf.spec.persistence.size == self.spec.persistence.size`), one-way fields (federation cannot be removed).

**Shared types**: `ConditionalStatus` (embedded in all CRD statuses), `CommonConfig` (labels, affinity, tolerations, nodeSelector, resources — available to all operands).

## Build System (Key Makefile Targets)

| Target | What it does |
|---|---|
| `make all` | `build verify` (default) |
| `make build` | Full build: manifests + generate + fmt + vet + compile binary |
| `make build-operator` | Compile binary only (FIPS-aware, vendor mode) |
| `make test` | Unit tests with envtest (K8s 1.31.0 assets), requires `OPERATOR_NAMESPACE` |
| `make test-e2e` | E2E tests against live OpenShift cluster (45min timeout) |
| `make lint` | Run golangci-lint |
| `make verify` | vet + fmt check + golangci-lint |
| `make manifests` | Regenerate CRD/RBAC/webhook YAML from kubebuilder markers |
| `make generate` | Regenerate DeepCopy methods |
| `make update-bindata` | Regenerate `pkg/operator/assets/bindata.go` from `bindata/` YAML |
| `make vendor` | `go mod tidy && go mod vendor` |
| `make bundle` | Generate OLM bundle |

After code changes to API types or bindata, run `make manifests generate update-bindata` then `make verify`.

## Test Exemplar

### Unit Test Pattern

All tests use Go's standard `testing` package with counterfeiter fakes — NOT Ginkgo for unit tests.

```go
func newTestReconciler(fakeClient *fakes.FakeCustomCtrlClient) *SpireServerReconciler {
    return &SpireServerReconciler{
        ctrlClient:    fakeClient,
        ctx:           context.Background(),
        log:           logr.Discard(),
        scheme:        runtime.NewScheme(),
        eventRecorder: record.NewFakeRecorder(100),
    }
}

func TestReconcile_SpireServerNotFound(t *testing.T) {
    fakeClient := &fakes.FakeCustomCtrlClient{}
    reconciler := newTestReconciler(fakeClient)
    notFoundErr := kerrors.NewNotFound(schema.GroupResource{...}, "cluster")
    fakeClient.GetReturns(notFoundErr)
    // ... assertions with t.Errorf/t.Error
}
```

**Conventions:**
- Each reconciler sub-function gets its own `*_test.go` file (e.g., `service_account_test.go`, `rbac_test.go`)
- Use `fakes.FakeCustomCtrlClient{}` — configure return values via `fakeClient.GetReturns(...)`, `fakeClient.GetStub = func(...)`
- Table-driven tests for multi-case validation scenarios
- Assertions use `t.Errorf` / `t.Error` (no testify, no gomega in unit tests)
- Test function names: `TestReconcile_<Scenario>` or `Test<Function>_<Scenario>`
- `record.NewFakeRecorder(100)` for event assertions
- `logr.Discard()` for logger

### E2E Test Pattern

E2E uses Ginkgo v2 + Gomega against a live OpenShift cluster (build tag: e2e, 45min timeout).

### File Naming

| Source file | Test file |
|---|---|
| `controller.go` | `controller_test.go` |
| `service_account.go` | `service_account_test.go` |
| `rbac.go` | `rbac_test.go` |
| `configmap.go` | `configmap_test.go` |
| `daemonset.go` | `daemonset_test.go` |
| `statefulset.go` | `statefulset_test.go` |

To regenerate fakes after changing the `CustomCtrlClient` interface: `go generate ./pkg/client/...`

## Code Style

### Import Order (goimports enforced)

1. Standard library
2. `k8s.io/*`, `sigs.k8s.io/*`
3. Third-party (`github.com/go-logr/logr`, `github.com/operator-framework/api`, `github.com/spiffe/*`)
4. OpenShift (`github.com/openshift/api`)
5. This project (`github.com/openshift/zero-trust-workload-identity-manager/...`)

### Standard Import Aliases

| Alias | Package |
|---|---|
| `ctrl` | `sigs.k8s.io/controller-runtime` |
| `kerrors` | `k8s.io/apimachinery/pkg/api/errors` |
| `apimeta` | `k8s.io/apimachinery/pkg/api/meta` |
| `customClient` | `github.com/openshift/zero-trust-workload-identity-manager/pkg/client` |
| `routev1` | `github.com/openshift/api/route/v1` |
| `securityv1` | `github.com/openshift/api/security/v1` |
| `ctrlmgr` | `github.com/spiffe/spire-controller-manager/api/v1alpha1` |

### Linting

golangci-lint with selective allowlist (`.golangci.yml`):
- Enabled: `errcheck`, `govet`, `staticcheck`, `revive`, `ginkgolinter`, `gofmt`, `goimports`, `dupl`, `lll`, `misspell`, `goconst`, `gocyclo`, `gosimple`, `ineffassign`, `nakedret`, `prealloc`, `typecheck`, `unconvert`, `unparam`, `unused`
- `dupl` and `lll` relaxed under `pkg/*` and `api/*`
- `revive` enforces `comment-spacings`
- Timeout: 5 minutes

### File Headers

All `.go` files must include the Apache 2.0 license header from `hack/boilerplate.go.txt`.

### Vendoring

Dependencies are vendored and tracked in git. After any `go.mod` change: `make vendor` (runs `go mod tidy && go mod vendor`). Always commit the `vendor/` diff alongside `go.mod`/`go.sum`.

### FIPS Build

Production builds use `hack/go-fips.sh` which enables `GOEXPERIMENT=strictfipsruntime` and build tags `strictfipsruntime,openssl`. CGO_ENABLED=1 required. Local dev builds without FIPS work but are not suitable for CI/production.

## Naming Conventions

- Controller names: `"zero-trust-workload-identity-manager-<component>-controller"` (in `pkg/controller/utils/constants.go`)
- Asset path constants: `Spire*AssetName` or `Spiffe*AssetName` (e.g., `SpireServerServiceAssetName`)
- Image env var constants: `RELATED_IMAGE_*` suffix with `*ImageEnv` Go constant name
- Resource kind constants: `ResourceKind*` (e.g., `ResourceKindSpireServer`)
- Package directories: `kebab-case` (`spire-server`); Go package names: `snake_case` (`spire_server`)
- CRD singleton names: always `"cluster"`
- Operator namespace: `"zero-trust-workload-identity-manager"` (from `OPERATOR_NAMESPACE` env)
- Managed-by label: `app.kubernetes.io/managed-by: zero-trust-workload-identity-manager`

## Environment Variables

| Variable | Purpose |
|---|---|
| `OPERATOR_NAMESPACE` | Namespace the operator runs in (required) |
| `OPERATOR_CONDITION_NAME` | OLM OperatorCondition name for Upgradeable sync (required) |
| `CREATE_ONLY_MODE` | When `true`, skip updates to existing resources |
| `RELATED_IMAGE_SPIRE_SERVER` | SPIRE server container image |
| `RELATED_IMAGE_SPIRE_AGENT` | SPIRE agent container image |
| `RELATED_IMAGE_SPIFFE_CSI_DRIVER` | SPIFFE CSI driver container image |
| `RELATED_IMAGE_SPIRE_OIDC_DISCOVERY_PROVIDER` | OIDC discovery provider container image |
| `RELATED_IMAGE_SPIRE_CONTROLLER_MANAGER` | SPIRE controller manager container image |
| `RELATED_IMAGE_NODE_DRIVER_REGISTRAR` | CSI node driver registrar container image |
| `RELATED_IMAGE_SPIFFE_CSI_INIT_CONTAINER` | SPIFFE CSI init container image |

## Common Mistakes

1. Do NOT edit generated files by hand: `zz_generated.deepcopy.go`, `config/crd/bases/*.yaml`, `pkg/operator/assets/bindata.go`. Use `make generate`, `make manifests`, `make update-bindata`.
2. Do NOT return both `RequeueAfter` and a non-nil error from `Reconcile`. Return one or the other.
3. Do NOT create separate `ctrl.Manager` instances — all controllers share one manager in `main.go`.
4. Do NOT add new watched resources without updating `NewCacheBuilder` in `pkg/client/client.go` with the correct label selector.
5. Do NOT use full `DeepEqual` for resource comparison — add type-specific comparison in `ResourceNeedsUpdate()`.
6. Do NOT embed `operatorv1.OperatorSpec` in CRD types — use domain-specific specs.
7. Do NOT use SSA (Server-Side Apply) — this operator uses imperative Create/UpdateWithRetry.
8. Do NOT skip `make verify` after code changes — CI rejects PRs with stale generated files or lint failures.
9. Do NOT forget `OPERATOR_NAMESPACE` env when running tests locally (`make test` sets it automatically).
10. Do NOT hand-edit `PROJECT` file — it references stale types (`ExternalSecrets`). Ignore it; actual CRDs are in `api/v1alpha1/`.
11. Do NOT use Ginkgo/Gomega in unit tests — use Go standard `testing` + counterfeiter fakes.
12. Do NOT create resources without `app.kubernetes.io/managed-by` label — the cache label selector won't see them.
13. Federation is one-way — `SpireServer` federation cannot be removed once set; persistence fields are immutable (CEL-enforced).

## Upstream Operand Projects

| Upstream repo | Operator integration |
|---|---|
| `github.com/spiffe/spire` | Container images for server (StatefulSet), agent (DaemonSet), OIDC provider (Deployment) |
| `github.com/spiffe/spire-controller-manager` | Go module dependency for API types; image deployed as sidecar with SPIRE server |
| `github.com/spiffe/spiffe-csi` | Container image for CSI driver (DaemonSet) |

The operator imports `github.com/spiffe/spire-controller-manager` as a Go module for `ControllerManagerConfig` and CRD types (`ClusterSPIFFEID`, `ClusterFederatedTrustDomain`, `ClusterStaticEntry`).

---

## Per-task testing during `/opsx-apply` (code generation eval gate)

During implementation, each code generation task is verified with **real command execution** (not agent assertions). See **[`stage-gate/CODE_GENERATION_EVAL_PROMPT.md`](stage-gate/CODE_GENERATION_EVAL_PROMPT.md)** for the full protocol.

| Task type | Verification | Test strategy |
|-----------|-------------|--------------|
| API types (`api/v1alpha1/`) | `go build ./...`, `go vet ./...` | Build-only |
| Codegen (`make generate/manifests`) | `make generate && make manifests && make verify` | Consistency check |
| Controller logic (`pkg/controller/`) | `go build ./...`, `go vet ./...` | Co-generated `_test.go` + `go test ./pkg/controller/<component>/...` |
| Bindata / manifests (`bindata/`) | `make update-bindata && make verify` | `make verify` |
| OLM bundle (`bundle/`) | `make bundle` | Bundle scripts |
| Status / utils (`pkg/controller/status/`, `pkg/controller/utils/`) | `go test ./pkg/controller/status/... ./pkg/controller/utils/...` | Existing test suites |
| Client changes (`pkg/client/`) | `go generate ./pkg/client/... && go test ./pkg/client/...` | Regenerate fakes + test |

---

## Execution agent routing

Use these **Assigned Agent** IDs in `tasks.md` §3 when **`AgentRoutingMode: PROVIDED`**. Each task gets exactly one primary agent. Map work to paths below; split mixed tasks.

| Agent ID | Scope | Route when task touches | OAPE / execution |
|----------|-------|-------------------------|------------------|
| **API_Agent** | CRD/API types, markers, CEL validation, CommonConfig | `api/v1alpha1/` | `api-generate` (implementation) or `api-generate-tests` (verification-only) |
| **OperatorController_Agent** | Reconciliation, operand workloads, status, controller wiring | `pkg/controller/spire-server/`, `pkg/controller/spire-agent/`, `pkg/controller/spiffe-csi-driver/`, `pkg/controller/spire-oidc-discovery-provider/`, `pkg/controller/zero-trust-workload-identity-manager/`, `pkg/controller/status/`, `pkg/controller/utils/`, `pkg/client/`, `cmd/` | `api-implement` |
| **ManifestsBindata_Agent** | Operand YAML, bindata regeneration, asset constants | `bindata/`, `pkg/operator/assets/`, `pkg/controller/utils/constants.go` (asset paths), `Makefile` (bindata targets) | Manual — `make update-bindata && make verify` |
| **RBACSecurity_Agent** | RBAC manifests, SCC, TLS, service-ca annotations, security | `bindata/*/spire-*-cluster-role*.yaml`, `bindata/*/spire-*-role*.yaml`, `config/rbac/`, SCC resources | Manual |
| **OLMRelease_Agent** | OLM bundle, CSV, relatedImages, catalog | `bundle/`, `config/`, `Makefile` (bundle targets) | Manual — `make bundle` |
| **Testing_Agent** | E2E and unit test authoring | `test/e2e/`, `pkg/controller/*_test.go` | `e2e-generate` when task is e2e |
| **Docs_Agent** | User-facing docs, OWNERS, README | `README.md`, `docs/`, `OWNERS` | Manual |

### Controller routing rules

- **All controllers** use controller-runtime reconcilers — there is NO library-go pattern in this repo.
- **Operand controllers** (spire-server, spire-agent, spiffe-csi-driver, spire-oidc-discovery-provider): use `CustomCtrlClient` + imperative Create/UpdateWithRetry; register on the single shared manager in `main.go`.
- **ZTWIM aggregator controller**: read-only of operand CRs; sets aggregate status + OLM OperatorCondition sync.
- **API before controller**: tasks that add CRD fields must complete (and pass `make generate && make manifests && make verify`) before controller tasks that reconcile those fields.

### Verification pairing

- API changes → pair with `make generate && make manifests && make verify`
- Controller / status changes → pair with unit tests (`make test`) and e2e when user-visible
- Bindata / operand manifest changes → pair with `make update-bindata && make verify`
- Client interface changes → pair with `go generate ./pkg/client/... && make test`

---

## Stage-Specific Agent Guidance

The sections below provide zero-trust-workload-identity-manager-specific hints that each pipeline stage agent MUST incorporate when processing this repository. Templates remain generic; this file is the single source of project-specific depth.

---

### Repo-Assessment Stage Hints

#### Deep-Dive Requirements

When the repo is `zero-trust-workload-identity-manager` (detected via `operator.openshift.io` CRDs for ZTWIM/SpireServer/SpireAgent/SpiffeCSIDriver/SpireOIDCDiscoveryProvider, bindata directories for spire-server/spire-agent/spiffe-csi, or `openshift/zero-trust-workload-identity-manager` module path in go.mod), apply ALL generic Kubernetes/OpenShift operator hints from the template AND these additional requirements.

**Architecture (§1):**
- Document the single-pattern architecture: ALL controllers use controller-runtime with `CustomCtrlClient` wrapper. There is NO library-go / informer factory pattern.
- Document the ZTWIM aggregator controller as a special case: it does NOT create operand CRs, only reads their status.
- Call out `CREATE_ONLY_MODE` as a runtime flag that blocks all updates (surfaced as condition + blocks `Upgradeable`).
- Document the `status.Manager` pattern with deferred `ApplyStatus()` and auto-Ready derivation.
- Note: `pkg/operator/assets/bindata.go` is a generated file (800KB+) — NEVER hand-edit.

**Controllers & Reconciliation (§4.2):**
- Document the standard reconciliation flow (8 steps — see "Operand Reconciliation Flow" above).
- For SpireServer: document federation one-way constraint, persistence immutability, TTL validation, webhook management, controller-manager sidecar ConfigMap generation, Route creation.
- For SpireAgent: document SCC (SecurityContextConstraints) management, workload attestor verification types (skip/auto/hostCert), ConfigMap generation with server address templating.
- For SpiffeCSIDriver: document privileged SCC RoleBinding, CSIDriver object management, node-driver-registrar sidecar.
- For SpireOIDCDiscoveryProvider: document ClusterSPIFFEID creation, Route management, external certificate support.

**Configuration Surface (§4.1):**
- List `CommonConfig` fields shared by all operands: labels, affinity, tolerations, nodeSelector, resources.
- Document SpireServer-specific fields: logLevel, logFormat, jwtIssuer, caValidity, defaultX509Validity, defaultJWTValidity, caKeyType, persistence (size, accessMode, storageClass — all immutable), federation (cannot be removed), externalCertificate.
- Document validation pipeline: `ValidateAndUpdateStatus()` runs first, sets per-field conditions, blocks reconciliation on failure.

**Cache & Watch Architecture (§4.3):**
- Document `NewCacheBuilder()` in `pkg/client/client.go`: managed resources cached with `managed-by` label selector; CRD objects cached without selectors.
- Explain that creating a resource without the managed-by label means the cache won't track it.
- Document predicate chain: `GenerationOrOwnerReferenceChangedPredicate` for operand CRs, `ZTWIMSpecChangedPredicate` for parent CR watches, component-specific label predicates for managed resources.

**Status & Conditions (§4.4):**
- Document the status condition system: all operands use `ConditionalStatus` with standard metav1.Condition.
- Ready condition is auto-derived by `SetReadyCondition()` from all other conditions (distinguishes "Progressing" vs "Failed").
- ZTWIM aggregator maintains `Operands[]` list keyed by kind.
- `ApplyStatus` uses semantic equality to skip no-op writes.

**Scheme Registration (§10):**
- `main.go` registers: clientgoscheme, operatoropenshiftiov1alpha1 (ZTWIM CRDs), securityv1 (SCC), routev1 (Routes), operatorv1 (OLM OperatorCondition), ctrlmgr (SPIRE controller-manager CRDs).
- New external types MUST be registered in `main.go`.

**Anti-patterns (forbidden):**
- Claiming library-go patterns exist in this repo.
- Framing resource application as SSA — this repo uses imperative Create/UpdateWithRetry.
- Using `make test-unit` — the target is `make test`.
- Suggesting admission/conversion webhooks for CRD validation — CEL handles it.

---

### Planning Stage Hints

#### Agent Role Scoping

The Technical Planning Agent operates as a planner for the **SPIFFE/SPIRE zero-trust workload identity** ecosystem (ZTWIM operator, managed upstream operands, and related packaging, tests, and docs).

#### ZTWIM Planning Content Expectations

Prefer operator-native thinking:
- CRD/API evolution with CEL validation, immutability constraints, CommonConfig extensions
- Operand reconciliation boundaries and status conditions (per-step condition model)
- Bindata manifest management and go-bindata regeneration
- Cache label selectors and watch predicate wiring for new resources
- RBAC blast radius (ClusterRoles for SPIRE components, SCC grants)
- OLM/CSV/bundle constraints and OperatorCondition sync
- Federation security implications (one-way constraint, trust domain boundaries)
- OpenShift platform integration: Routes, service-ca annotations, SCC
- Upstream SPIRE version tracking and image pinning via RELATED_IMAGE env vars

#### Default Repo Pin (User Message Template)

When no explicit repo is provided, default to:

```
primary_repo: "https://github.com/openshift/zero-trust-workload-identity-manager"
branch: "main"
commit: "<sha|unknown>"
```

---

### Validation Stage Hints

#### Ecosystem Evaluation Trigger

SPIFFE/SPIRE ecosystem items are mandatory to evaluate when the spec touches operators/operands/CRDs/workload-identity/TLS/RBAC/SCC/federation/networking/monitoring/upgrades/OpenShift.

#### ZTWIM Ecosystem Pillars

When evaluating a spec for this project, assess the following pillars (if absent → `missing_elements` and/or `ztwim_ecosystem.gaps`):

- API & CRD lifecycle (scope, defaults, immutability, CEL validation, CommonConfig patterns)
- Install / uninstall / reconcile semantics (CREATE_ONLY_MODE, resource ownership, condition model)
- RBAC & blast radius (ClusterRoles, SCC grants for privileged CSI, service-ca certs)
- Security (FIPS, trust domain isolation, federation boundaries, mTLS between server/agent)
- Platform matrix (OpenShift 4.19+; SCC requirements; Route integration for OIDC/SPIRE server)
- Observability (status conditions per component, Ready auto-derivation, OLM Upgradeable sync)
- Upgrade / downgrade / version skew (immutable persistence fields, one-way federation, CREATE_ONLY_MODE)

#### JSON Schema Extension

The validation output JSON MUST include a `ztwim_ecosystem` object:

```json
"ztwim_ecosystem": {
  "api_lifecycle_complete": true,
  "rbac_blast_radius_documented": true,
  "security_federation_addressed": true,
  "install_uninstall_semantics_clear": true,
  "platform_matrix_addressed": true,
  "observability_status_documented": true,
  "upgrade_skew_addressed": true,
  "gaps": ["string"]
}
```

Rules for booleans: set true ONLY if the spec text substantively covers that area; otherwise false. Put questions and missing details in `gaps`.

#### Few-Shot Calibration Examples

##### Example 1: Well-Written Spec (PASS)

**Input spec text:**
> **Title**: Add SPIFFE Helper sidecar injection support
>
> **Motivation**: Workload teams currently must manually configure SPIFFE helper sidecars
> to fetch and renew X.509 SVIDs from the SPIRE agent's Workload API socket. This
> manual process is error-prone and leads to certificate expiry incidents.
>
> **User Persona**: Platform engineer managing workloads on OpenShift 4.19+ with
> zero-trust-workload-identity-manager installed.
>
> **Acceptance Criteria**:
> 1. Given a Pod with annotation `spiffe.io/inject-helper: "true"`, When the mutating
>    webhook fires, Then a SPIFFE helper init container and sidecar are injected with
>    the correct Workload API socket mount.
> 2. Given the SPIFFE helper sidecar is running, When the X.509 SVID approaches 50%
>    lifetime, Then it is renewed without pod restart.
> 3. Given the SPIRE agent is unavailable, When the sidecar attempts renewal, Then it
>    logs a warning and retries with exponential backoff, not crashing the pod.
>
> **Scope**: X.509 SVID fetch/renewal only. JWT-SVID support out of scope.
> No changes to SpireServer or SpireAgent CRDs.
>
> **Dependencies**: Requires SPIFFE Helper image (`RELATED_IMAGE_SPIFFE_HELPER`).
> CSI driver must be deployed for Workload API socket access.
>
> **RBAC**: New MutatingWebhookConfiguration. Operator ServiceAccount needs
> `create/get/update` on `mutatingwebhookconfigurations`. No new cluster-wide
> secret access.
>
> **Upgrade**: Existing workloads without the annotation are unaffected. New
> webhook is additive — no migration needed.

**Expected scores:** completeness_score: 90, quality_score: 86, overall_score: 88,
overall_status: PASS. `ztwim_ecosystem.platform_matrix_addressed`: false with gap
"Platform matrix: Does this require SCC changes for the helper sidecar? Is there a
CREATE_ONLY_MODE interaction?"

##### Example 2: Contradictory Spec (BLOCKED)

**Input spec text:**
> **Title**: Add multi-trust-domain support to SpireServer
>
> **Description**: Allow SpireServer to manage multiple trust domains simultaneously.
> Each trust domain gets its own StatefulSet. The SpireServer CRD remains a singleton
> named "cluster" but multiple SpireServer CRs will be created, one per domain.
> Federation between domains should be automatic and bidirectional. Federation
> configuration must remain immutable once set (existing CEL constraint).

**Expected scores:** overall_score: 35, overall_status: BLOCKED.
Blockers: "Singleton contradiction: spec says both singleton and multiple CRs",
"Federation directionality contradiction: spec says bidirectional but existing CEL
constraint is one-way (cannot remove federation once set)".
All `ztwim_ecosystem` booleans false, with gaps covering API lifecycle contradiction,
RBAC blast radius for multiple StatefulSets, federation security contradiction,
install/uninstall for multi-CR, platform matrix, observability, and upgrade path.
