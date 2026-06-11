OpenShift TLS Profile Compliance for ZTWIM and SPIRE Operands

## *Enforce cluster-wide TLS profile (min TLS version, ciphers, and group preferences) on all ZTWIM and SPIRE TLS-serving endpoints depending on tls adherence policy*

**Date:** 		May 7, 2026  
**Scope:**		Zero Trust Workload Identity Manager (ZTWIM) Operator  
**Status:** 		Proposed  **Status Changed Date:** May 7, 2026  
**Authors:** 		[Nandan Hegde](mailto:nandan.islur@gmail.com)  
**Other docs:**

- [TLS Profile Compliance \- Implementation ref](https://docs.google.com/document/d/1NoHP2nZdg-xUkcB2y2BtI8469-iicdSG7xJwRad4kF0/edit?tab=t.0#heading=h.p836qlye75wr)  
- [Openshift Enhancement Proposal](https://github.com/openshift/enhancements/pull/1910/changes)  
- [Reference Implementation](https://github.com/openshift/cluster-control-plane-machine-set-operator/blob/85f92f79174d6df783f631eb3187f0e11e89cc96/pkg/tls/tls.go)

# What

The ZTWIM operator and its SPIRE operands do not honor the cluster-wide TLS security profile (apiserver.config.openshift.io/cluster). All TLS-serving surfaces either rely on Go defaults or hardcoded MinVersion: tls.VersionTLS12 with no cipher suite and curve prefernce restrictions. This ADR proposes a two-layer fix: use controller-runtime-common/pkg/tls for the operator itself, and inject resolved TLS settings into upstream SPIRE operands via configuration variables. This ADR covers following TLS-serving endpoints compliant with the centralized OpenShift TLS profile:

**Downstream (ZTWIM operator):**

* Metrics server (:8443)  
* Webhook server

**Upstream operands managed by ZTWIM:**

* SPIRE Server — gRPC TCP API (:8081), federation bundle endpoint (:8443), prometheus server (:8082)  
* OIDC Discovery Provider — HTTPS (:8443)  
* SPIRE Controller Manager — webhook server (:9443)


**No changes needed:**

* SPIRE Agent Socket (UDS only)

# Why

* OCP 4.23 / 5.0 release blocker. When `tlsAdherence: StrictAllComponents` is enforced, non-compliant components block cluster upgrades.  
* PQC readiness. ML-KEM key encapsulation requires TLS 1.3 and is negotiated as a **named group** (`supported_groups` / `key_share`), not via cipher suites. Honoring only `minTLSVersion` and `ciphers` is necessary but insufficient for PQC KEX control.  
* Customer custom profiles are silently ignored. Customers who disable specific ciphers or curves via custom TLS profiles get no enforcement from ZTWIM/SPIRE.

## **Goals**

* All ZTWIM/SPIRE TLS server endpoints honor `apiserver.config.openshift.io/cluster` TLS profile settings: `minTLSVersion`, `ciphers`, and `groups`.  
* Profile changes (profile type, custom fields, and `tlsAdherencePolicy`) propagate automatically (watch + rolling restart).  
* Zero behavior change for default Intermediate profile (TLS 1.2, default ciphers) or when `tlsAdherencePolicy` is not strict adherence. 
* CSV can declare `tls-profiles: "true"`.

## **Non-Goals**

* Changing TLS client settings (out of scope per PQC guidance).  
* Changing the cluster default profile (remains Intermediate / TLS 1.2).  
* Hot-reload of TLS configuration within SPIRE processes.  
* Adding TLS to Unix domain socket or plaintext health endpoints.
* Enforcing strict group **preference order** on Go-based servers — Go treats `CurvePreferences` as an allowed-set filter and applies its own internal ordering (documented OpenShift API behavior).

# How

### **Two-Layer Architecture**

SPIRE is an upstream CNCF project that does not understand OpenShift TLS profiles. The solution splits into two layers:

* **Layer 1 (Direct):** ZTWIM operator uses `controller-runtime-common/pkg/tls` to fetch and watch the cluster TLS profile and `tlsAdherence` for its own metrics and webhook servers. Resolved settings are applied via `NewTLSConfigFromProfile` (`MinVersion`, `CipherSuites`, and `CurvePreferences` from `profile.Groups`). On profile or adherence change, the `SecurityProfileWatcher` cancels the context, causing a graceful restart.  
* **Layer 2 (Injected):** ZTWIM operator reconcilers resolve the cluster TLS profile into concrete `minTLSVersion`, `cipherSuites`, and `groups` values, then inject them as config variables into SPIRE operand containers. ConfigMap hash changes trigger rolling restarts.

### **Downstream Changes (ZTWIM Operator)**

| Change | File | What |
| :---- | :---- | :---- |
| **Add dependency** | go.mod | Add `github.com/openshift/controller-runtime-common` |
| **Register scheme** | cmd/.../main.go init() | `configv1.Install(scheme)` |
| **Fetch + apply profile** | cmd/.../main.go main() | Resolve cluster TLS (adherence + profile) → `NewTLSConfigFromProfile` (min version, ciphers, groups) → append to metrics / webhook `TLSOpts` before `NewManager` |
| **Watch + restart operator** | cmd/.../main.go after mgr creation | `SecurityProfileWatcher`: `OnProfileChange` / `OnAdherencePolicyChange` → cancel context → operator pod restart |
| **APIServer in cache** | pkg/client/client.go | Include `&configv1.APIServer{}` in cache & `informerResources` so `GetClient()` can serve the APIServer object during reconcile |
| **Resolve profile for operands** | pkg/controller/tls/tls.go | Helpers to resolve min TLS string, cipher list, and group list from APIServer/cluster (same semantics as operator). Called from each operand config generator during reconcile |
| **Inject minTLSVersion, cipherSuites, and groups into operand config** | pkg/controller/spire-server/statefulset.go, spire-oidc-discovery-provider/deployments.go, spire-controller-manager deployment | Config change causes hash update and rolling restart |
| **RBAC** | config/rbac/role.yaml | get/list/watch on `config.openshift.io/apiservers` |
| **CSV annotation** | config/manifests/bases/...clusterserviceversion.yaml | `tls-profiles: "true"` |

### **Upstream Changes (SPIRE, SPIRE Controller Manager)**

These are downstream patches initially, with upstream PRs to follow.

| Change | File | What |
| :---- | :---- | :---- |
| **Configurable TLS on SPIRE Server gRPC (TCP)** | spire/pkg/server/endpoints/endpoints.go | Read `MinVersion`, `CipherSuites`, and group/`CurvePreferences` from HCL config instead of hardcoding `tls.VersionTLS12` |
| **Configurable TLS on federation bundle HTTPS (all auth modes)** | spire/pkg/server/endpoints/bundle/server.go | Read `MinVersion`, `CipherSuites`, and group/`CurvePreferences` from HCL config instead of hardcoding `tls.VersionTLS12` |
| **Configurable TLS on Prometheus scrape server (HTTPS)** | spire/pkg/common/telemetry/prometheus.go | Read `MinVersion`, `CipherSuites`, and group/`CurvePreferences` from HCL config instead of hardcoding `tls.VersionTLS12` |
| **Configurable TLS in OIDC provider** | spire/support/oidc-discovery-provider/main.go | Read TLS settings including groups from config |
| **Configurable TLS in ctrl-mgr webhook** | spire-controller-manager/cmd/main.go | Read TLS settings including groups from config |

*All upstream changes follow the same pattern: read from config, parse, apply. Fall back to current behavior (TLS 1.2, Go default ciphers/groups) when config is unset. [Experimental require\_pq\_kem config variable](https://github.com/spiffe/spire/blob/f634aaa8625a84438a51fdd628e1f218ec1862c6/doc/spire_agent.md?plain=1#L85) in SPIRE Server takes precedence  if set.*

### **Change Propagation Flow**

* Cluster admin changes [apiserver.config.openshift.io/cluster](http://apiserver.config.openshift.io/cluster) resource 
* ZTWIM operator `SecurityProfileWatcher` fires → operator restarts with new profile on its own TLS servers  
* Reconcilers resolve profile (min version, ciphers, groups) → update ConfigMap; hash change restarts operand workloads  
* New operand pods start with updated TLS settings from config

### **Determining Effective TLS Configuration**

| TLS adherence (`APIServer.spec.tlsAdherence`) | TLS security profile fetched (`APIServer.spec.tlsSecurityProfile`) | Final TLS used (operator via `NewTLSConfigFromProfile` + group mapping) |
| :---- | :---- | :---- |
| `StrictAllComponents` (or any value except `""` / `NoOpinion` / `LegacyAdheringComponentsOnly`, including unknown strings for forward compatibility) | Old / Intermediate / Modern / Custom (fetch OK) | That profile: min TLS + cipher list + groups from the profile map or custom spec. Modern → TLS 1.3 min; ciphers not set in `tls.Config` (Go TLS 1.3 behavior). Built-in profiles → default groups `X25519MLKEM768`, `X25519`, `secp256r1`, `secp384r1`. Custom with `groups` set → those groups only. Custom with `groups` omitted → no group filter (Go defaults). |
|  | Else (fetch failed or profile unset) | Intermediate: TLS 1.2 min; Intermediate cipher list; Intermediate default groups |
| `NoOpinion` (`""`) OR `LegacyAdheringComponentsOnly` OR adherence fetch failed (treated same as `NoOpinion`) | Any (including unset, Modern, custom, or profile fetch failed) | Existing way. Under ZTWIM context, defaults to Intermediate: TLS 1.2 min; Intermediate ciphers; Intermediate default groups once group support is enabled |

**Summary:** TLS policy from the APIServer resource is enforced only when adherence is strict (or forward-compatible unknown). Otherwise the Intermediate fallback profile applies. All three dimensions — min version, ciphers, and groups — follow the same adherence gate.

### **Verification**

* tls-scanner against all TLS endpoints for each profile type (Old, Intermediate, Modern, Custom)  
* Operator reconciliation on `tlsAdherencePolicy` value change  
* Unit tests for profile resolution and ConfigMap/StatefulSet generation (including `Groups` in resolved spec)  
* Integration test: change profile from Intermediate to Modern, verify all endpoints comply  
* Custom profile test: disable specific ciphers, verify not offered by any endpoint
* PQC custom profile test: `groups: [X25519MLKEM768]` with `minTLSVersion: VersionTLS13`, verify hybrid KEX when client supports it

### **Migration**

Phased rollout:

* Phase 1 (operator compliance) and   
* Phase 2 (config injection into operands) where the Operands honor the central tls profile.

## **Alternatives**

| Alternative | Verdict | Reason |
| :---- | :---- | :---- |
| **Sidecar TLS proxy (Envoy/HAProxy)** | Rejected | Breaks SPIFFE mTLS verification — SPIRE must see client certs directly. Adds latency and operational complexity. |
| **Fork SPIRE permanently** | Rejected | Unsustainable maintenance burden. SPIRE is actively developed; security patches must be manually backported. |
| **Hardcode TLS 1.3 everywhere** | Rejected | Breaks customers on Intermediate/Old profiles. Violates PQC guidance: no default changes in OCP 4.22. |
| **Wait for upstream SPIRE support** | Rejected | SPIRE is CNCF — OpenShift-specific features unlikely. This is a release blocker; we cannot wait. |
| **Config update + minimal upstream patch** | Chosen | Minimal divergence, leverages SPIRE's `-expandEnv`, easy to upstream as general configurable TLS (version, ciphers, groups). Small downstream patch until upstream merges. |

## **Risks**

| Risk | Likelihood | Impact | Mitigation |
| :---- | :---- | :---- | :---- |
| **Rolling restart on profile or tlsAdherence change causes brief SPIRE API unavailability** | Expected | Seconds of unavailability per pod | StatefulSet rolling update; PodDisruptionBudget; agents auto-reconnect |
| **Cipher suite mismatch — Go does not support all OpenSSL names** | Expected | Some ciphers silently skipped | Log unsupported ciphers; use standard mapping from PQC reference code |
| **Group name / CurveID mismatch — Go does not support all enum values** | Expected | Some groups silently skipped | Log unsupported groups; align with `TLSGroup` enum and Go version (e.g. `X25519MLKEM768` requires Go 1.24+) |
| **Go ignores group list ordering** | Expected | Admin preference order not strictly enforced on Go servers | Document behavior; list is an allowed-set filter for Go; ordering matters more for non-Go terminators |
| **FIPS mode vs PQC default groups** | Expected | Built-in profiles include non-FIPS `X25519MLKEM768` | Omit PQC / non-FIPS groups when running in FIPS mode per API documentation |

# 

# Reviews

Anybody may review the document and provide feedback.  Acceptance and rejection is reserved for those people noted in the appropriate "Accept / Reject" section of each ADR.

| Reviewed by | Date  | Notes |
| :---- | :---- | :---- |
| [Raushan Kumar Singh](mailto:rausingh@redhat.com) | May 19, 2026 | Review done. LGTM |
|  |  |  |
|  |  |  |

