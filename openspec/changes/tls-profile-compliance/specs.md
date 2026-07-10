# Feature Specification: TLS Profile Compliance and Hybrid Post-Quantum Key Exchange

**Feature Branch**: `tls-profile-compliance`

**Created**: 2026-07-02

**Status**: Draft

**Input**: ADR-TLS-Compliance — enforce cluster-wide TLS profile and enable hybrid post-quantum key exchange on all operator and managed identity TLS-serving endpoints (OCPSTRAT-2611, OCPSTRAT-3145, OCPSTRAT-3123, OCPSTRAT-2361)

## Personas

| Persona | Responsibility |
| ------- | -------------- |
| **Cluster administrator** | Configures cluster-wide TLS security profile and TLS adherence mode; may enable strict hybrid post-quantum key exchange on the operator |
| **Platform security engineer** | Validates TLS compliance across endpoints; runs conformance scans before upgrades |
| **Operator maintainer** | Implements and verifies operator and operand TLS behavior; coordinates upstream operand changes |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator TLS servers honor the cluster profile (Priority: P1)

The cluster administrator expects the identity operator's own TLS-serving endpoints (metrics scrape and admission webhook) to negotiate TLS according to the cluster-wide TLS security profile and adherence rules, matching other OpenShift platform components.

**Why this priority**: Release-blocker for central TLS consistency; the operator must comply before managed operands can be credibly enforced.

**Independent Test**: Deploy the operator on a cluster with a known TLS profile and strict adherence; probe only the operator metrics and webhook TLS endpoints and confirm negotiated version and ciphers match the profile.

**Acceptance Scenarios**:

1. **Given** the cluster TLS profile is Intermediate (TLS 1.2 minimum) and TLS adherence is not strict, **When** a client connects to the operator metrics TLS endpoint, **Then** the connection negotiates at least TLS 1.2 and behavior matches pre-change defaults.
2. **Given** the cluster TLS profile is Modern and TLS adherence is strict for all components, **When** a client connects to the operator admission webhook TLS endpoint, **Then** the connection honors the Modern profile minimum version and cipher policy.
3. **Given** the cluster administrator changes the cluster TLS profile or TLS adherence setting, **When** the change is applied, **Then** the operator process restarts and subsequent connections to operator metrics and webhook endpoints reflect the updated profile.

---

### User Story 2 - Managed identity operands honor the cluster profile under strict adherence (Priority: P1)

When the cluster requires strict TLS adherence, the cluster administrator expects every managed identity operand TLS server endpoint to honor the cluster TLS security profile—not silently ignore custom or platform profiles.

**Why this priority**: Core OCPSTRAT-2611 outcome; non-compliance blocks cluster upgrades under strict adherence.

**Independent Test**: Enable strict adherence and a non-default profile; scan or handshake-test each operand TLS endpoint (server API, federation, metrics, OIDC discovery, admission webhook) independently of the operator's own servers.

**Acceptance Scenarios**:

1. **Given** TLS adherence is strict for all components and the cluster profile is Custom with specific ciphers disabled, **When** clients handshake with each managed operand TLS endpoint, **Then** disabled ciphers are not offered on any endpoint.
2. **Given** TLS adherence is strict and the cluster profile is Intermediate, **When** clients connect to server API, federation, server metrics, OIDC discovery, and controller admission webhook endpoints, **Then** each endpoint negotiates TLS consistent with the Intermediate profile.
3. **Given** TLS adherence is legacy-only or omitted (platform default non-strict mode), **When** the operator reconciles managed operands, **Then** operands retain default TLS behavior equivalent to Intermediate minimum without central profile injection into operand configuration.

---

### User Story 3 - Cluster TLS changes propagate via rolling restart (Priority: P2)

The cluster administrator changes the cluster TLS profile or adherence mode and expects the identity stack to pick up the change without manual pod deletion or config editing.

**Why this priority**: Operational requirement for living clusters; profile drift otherwise leaves stale TLS settings.

**Independent Test**: Change cluster profile from Intermediate to Modern with strict adherence; observe operator restart followed by operand rolling updates; verify endpoints before and after.

**Acceptance Scenarios**:

1. **Given** strict adherence and an active identity deployment, **When** the cluster TLS profile changes, **Then** the operator restarts and managed operands roll out with updated TLS settings derived from the new profile.
2. **Given** strict adherence, **When** TLS adherence changes from non-strict to strict, **Then** operand configuration is updated to inject the central profile and operands restart to apply it.
3. **Given** a rolling restart is in progress, **When** individual operand pods restart, **Then** agents and clients reconnect automatically without requiring manual intervention beyond normal upgrade tolerance.

---

### User Story 4 - Administrator enables strict hybrid post-quantum key exchange (Priority: P2)

The cluster administrator opts in to strict hybrid post-quantum key exchange (X25519MLKEM768) for the entire managed identity stack using a single flag on the operator custom resource, overriding central TLS profile injection for operands.

**Why this priority**: OCPSTRAT-3145 deliverable; must be opt-in, cluster-wide, and consistent across server, agent, OIDC, and controller components.

**Independent Test**: Set the operator flag to require post-quantum key exchange; verify generated operand configuration and live handshakes on server–agent and workload identity TLS paths; confirm non-PQC clients fail when the flag is on.

**Acceptance Scenarios**:

1. **Given** the operator custom resource has post-quantum key exchange required set to true, **When** reconciliation completes, **Then** all managed server, agent, OIDC discovery, and controller operand configurations reflect strict hybrid post-quantum mode and central TLS profile fields are not injected into those configs.
2. **Given** post-quantum key exchange is required, **When** a supporting client handshakes with server API or agent-to-server TLS, **Then** the negotiated key exchange uses hybrid X25519MLKEM768 and minimum TLS 1.3.
3. **Given** post-quantum key exchange is required, **When** a client that does not support X25519MLKEM768 attempts to connect, **Then** the handshake fails.
4. **Given** post-quantum key exchange is required and TLS adherence is simultaneously strict, **When** reconciliation runs, **Then** the operator emits a warning event stating application-level post-quantum policy overrides central profile injection for operands.

---

### User Story 5 - TLS compliance is verified before release (Priority: P3)

The platform security engineer validates that all TLS-serving endpoints meet organizational TLS consistency expectations using automated scanning across profile types.

**Why this priority**: Primary verification mechanism cited in the ADR; prevents regressions across phases.

**Independent Test**: Run TLS conformance scans against the full endpoint matrix for at least Intermediate and one additional profile type; confirm pass/fail per endpoint without reading operator source.

**Acceptance Scenarios**:

1. **Given** a test cluster with strict adherence and Intermediate profile, **When** TLS conformance scans run against all operator and operand TLS endpoints, **Then** every endpoint reports compliant negotiated parameters for that profile.
2. **Given** phased delivery, **When** Phase 1 (operator-only compliance) is complete, **Then** scans cover operator metrics and webhook endpoints in continuous integration; full Old/Intermediate/Modern/Custom matrix runs as a release qualification gate before GA.
3. **Given** post-quantum key exchange is enabled, **When** scans or handshake probes run, **Then** results confirm X25519MLKEM768 negotiation on applicable TLS paths.

---

### Edge Cases

- **When** central TLS profile fetch fails under strict adherence, **then** operands fall back to Intermediate-equivalent defaults (TLS 1.2 minimum, empty cipher list) and may opportunistically negotiate post-quantum key exchange if both peers support it.
- **When** TLS adherence is unknown (forward-compatible value), **then** the system treats it as strict for profile injection per platform forward-compatibility guidance.
- **When** post-quantum key exchange is disabled or unset and adherence is non-strict, **then** no central profile is injected into operands and Go/runtime defaults apply with graceful classical fallback.
- **When** the cluster profile changes during operand rollout, **then** rolling update completes with brief per-pod unavailability; agents auto-reconnect without orphaning workloads.
- **When** post-quantum key exchange is enabled before all agents and workloads support hybrid negotiation, **then** handshake failures occur on affected paths until components are upgraded (documented opt-in risk).
- **When** TLS client settings are configured by administrators, **then** they are unchanged—this feature affects TLS servers only.
- **When** workload identity uses Unix domain sockets only, **then** no TLS profile or post-quantum changes apply to that path.
- **When** post-quantum key exchange groups via central profile are unavailable (future platform feature), **then** strict hybrid post-quantum via the operator flag remains the interim enforcement mechanism; central profile group preferences are deferred to a future phase.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The operator MUST honor the cluster TLS security profile and TLS adherence setting on its own TLS-serving endpoints (metrics and admission webhook).
- **FR-002**: The operator MUST restart gracefully when the cluster TLS profile or TLS adherence setting changes, so operator TLS endpoints reflect the updated configuration.
- **FR-003**: When TLS adherence is strict for all components and post-quantum key exchange is not required on the operator custom resource, the operator MUST inject resolved cluster TLS profile settings (minimum version, cipher suites, and group preferences when platform support exists) into all managed operand configurations.
- **FR-004**: When TLS adherence is not strict (including legacy-only, omitted, empty, or platform default non-strict modes), the operator MUST NOT inject the central TLS profile into managed operand configurations; operands MUST retain default TLS behavior equivalent to Intermediate minimum with runtime opportunistic post-quantum negotiation where supported.
- **FR-005**: The operator MUST expose a single optional boolean on the operator custom resource (`requirePQKEM`) as the sole administrator control for strict hybrid post-quantum key exchange across all managed operands.
- **FR-006**: When `requirePQKEM` is true, the operator MUST propagate strict hybrid post-quantum policy to server, agent, OIDC discovery, and controller manager operand configurations, enforcing TLS 1.3 minimum and hybrid X25519MLKEM768 only, rejecting clients that cannot negotiate it.
- **FR-007**: When `requirePQKEM` is true, the operator MUST NOT inject central TLS profile minimum version, cipher suite, or group settings into managed operand configurations.
- **FR-008**: When `requirePQKEM` is true and TLS adherence is simultaneously strict, the operator MUST emit a warning event clarifying that application-level post-quantum policy overrides central profile injection for operands.
- **FR-009**: When cluster TLS configuration changes under strict adherence (and post-quantum is not required), the operator MUST update operand configuration and trigger rolling restarts so all operand TLS endpoints reflect the new profile.
- **FR-0010**: Managed operand TLS server endpoints MUST include server API, federation bundle HTTPS, server metrics HTTPS, OIDC discovery HTTPS, and controller admission webhook TLS; Unix-domain workload sockets MUST remain out of scope.
- **FR-0011**: Managed operands MUST read injected TLS and post-quantum settings from their configuration (via upstream-compatible configurable TLS support) rather than hardcoded TLS 1.2 defaults when settings are present.
- **FR-0012**: The operator bundle MUST declare TLS profile support so the platform recognizes the operator as TLS-profile-aware.
- **FR-0013**: The operator MUST have permission to read cluster APIServer configuration required to resolve TLS profile and adherence during reconciliation.
- **FR-0014**: On clusters with default Intermediate profile and non-strict TLS adherence, system behavior MUST remain unchanged from pre-feature defaults for both operator and operands.

### Key Entities

- **Cluster APIServer TLS configuration**: Cluster-scoped settings defining TLS security profile (Old, Intermediate, Modern, Custom) and TLS adherence mode (`StrictAllComponents`, `LegacyAdheringComponentsOnly`, or omitted default).
- **Operator custom resource (`ZeroTrustWorkloadIdentityManager`)**: Cluster-scoped operator configuration including optional `requirePQKEM` flag controlling strict hybrid post-quantum key exchange for all managed operands.
- **Managed operand configurations**: Generated configuration for server, agent, OIDC discovery, and controller components, including optional central profile TLS fields or post-quantum experimental settings.
- **TLS-serving endpoint**: Any HTTPS or TLS TCP listener exposed by the operator or managed operands; excludes Unix-domain sockets.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a cluster with strict adherence and Intermediate profile, a cluster administrator can confirm within one reconciliation cycle that operator metrics and webhook TLS endpoints negotiate TLS 1.2 or higher consistent with the profile.
- **SC-002**: On a cluster with strict adherence and a Custom profile disabling specific ciphers, handshake probes to all six in-scope operand TLS endpoint classes show none of the disabled ciphers offered.
- **SC-003**: After changing cluster TLS profile from Intermediate to Modern under strict adherence, all in-scope TLS endpoints reflect Modern policy within the duration of a normal rolling restart (no manual pod deletion required).
- **SC-004**: With `requirePQKEM` enabled, handshake probes confirm X25519MLKEM768 on server–agent TLS paths, and clients without hybrid support fail to complete the handshake.
- **SC-005**: With `requirePQKEM` disabled and non-strict adherence on a default cluster, observable TLS behavior matches pre-feature baselines (no unexpected cipher or version changes).
- **SC-006**: When `requirePQKEM` is true and strict adherence is active, a warning event visible to the cluster administrator explains that post-quantum policy overrides central profile injection.
- **SC-007**: TLS conformance scanning passes for operator endpoints in CI for each delivery phase; full Old/Intermediate/Modern/Custom matrix passes at release qualification before GA.
- **SC-008**: Cluster upgrade readiness under strict adherence is not blocked by identity operator or managed operand TLS non-compliance when this feature is fully delivered.

## Assumptions

- **A-001**: Cluster administrators manage TLS profile, TLS adherence, and `requirePQKEM` via platform and operator APIs; platform security engineers own conformance scanning.
- **A-002**: Target platform release is OpenShift 4.23 / 5.0 where strict TLS adherence can block upgrades for non-compliant components.
- **A-003**: Default cluster TLS profile remains Intermediate (TLS 1.2 minimum); default TLS adherence remains non-strict (legacy-only behavior) unless the administrator changes it—preserving zero behavior change on vanilla clusters.
- **A-004**: Strict hybrid post-quantum key exchange requires all participating agents and workloads to support hybrid X25519MLKEM768 before `requirePQKEM` is enabled; administrators upgrade agents before enabling the flag.
- **A-005**: Upstream managed identity operands will accept configurable minimum TLS version, cipher suites, and experimental post-quantum settings via configuration patches delivered in phased rollout (operator injection first, upstream read support coordinated separately).
- **A-006**: TLS conformance scans run in CI for operator endpoints during Phases 1–3; the full profile-type matrix (Old, Intermediate, Modern, Custom) runs as a manual release qualification gate before GA.
- **A-007**: Central profile TLS group preferences (`TLSGroupPreferences` platform feature) are deferred to a future phase once platform API support graduates; `requirePQKEM` serves as interim strict post-quantum enforcement until then.
- **A-008**: Post-quantum scope is key exchange (KEM) only; post-quantum certificate signatures (ML-DSA, SLH-DSA) are out of scope.
- **A-009**: Pure ML-KEM-only mode is unavailable; only hybrid X25519MLKEM768 is supported when post-quantum is required.
- **A-010**: TLS configuration changes apply via process restart and rolling workload updates—no in-process hot reload within operand processes.
- **A-011**: OIDC discovery TLS and post-quantum enforcement is achieved by operator configuration injection plus upstream operand support to read injected settings; the operator does not modify upstream source code in the operator repository for OIDC listener logic.

## Out of Scope

- TLS client configuration changes
- Changing the cluster default TLS profile
- In-process hot reload of TLS settings
- Post-quantum digital signatures on certificates
- Pure ML-KEM-only key exchange mode
- Unix-domain SPIRE agent workload API sockets

## Phased Delivery *(informational — planning detail)*

| Phase | Deliverable |
| ----- | ----------- |
| Phase 1 | Operator own TLS servers (metrics, webhook) comply with cluster profile |
| Phase 2 | Central profile injection into all managed operand TLS endpoints under strict adherence |
| Phase 3 | Operator CR `requirePQKEM` and propagation to all operand configs |
| Phase 4 (future) | Central profile TLS group preferences when platform feature graduates |
