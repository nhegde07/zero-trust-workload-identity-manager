# Feature Specification: Cluster-Wide TLS Profile Compliance and Hybrid Post-Quantum Key Exchange

**Feature Branch**: `ocpstrat-2611-tls-compliance`

**Created**: 2026-07-09

**Status**: Draft

**Input**: OCPSTRAT-2611 (Central TLS Consistency) with related scope from OCPSTRAT-3145 (Hybrid ML-KEM), OCPSTRAT-3123 (TLS Group Preferences — deferred), OCPSTRAT-2361. Source: ADR-TLS-Compliance.md.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cluster administrator enforces platform TLS profile (Priority: P1)

As a **cluster administrator**, I need the Zero Trust Workload Identity Manager and all SPIRE-managed TLS-serving endpoints to honor the cluster-wide TLS security profile so that my cluster remains upgrade-eligible when strict TLS adherence is enforced platform-wide.

**Why this priority**: Non-compliant components block cluster upgrades under strict TLS adherence policy. This is the primary release-blocker motivation (OCPSTRAT-2611).

**Independent Test**: Configure the cluster APIServer with a known TLS profile and strict adherence; verify every ZTWIM and SPIRE TLS server endpoint negotiates only the profile-permitted TLS version and cipher suites using standard TLS scanning tools.

**Acceptance Scenarios**:

1. **Given** the cluster APIServer has `tlsAdherencePolicy` set to strict-all-components and a Modern TLS profile, **When** the workload identity operator and SPIRE operands are running, **Then** the operator metrics endpoint, SPIRE server registration API, federation bundle endpoint, SPIRE server metrics endpoint, OIDC discovery endpoint, and admission webhook endpoint each negotiate TLS settings consistent with the Modern profile.
2. **Given** the cluster uses the default Intermediate TLS profile and adherence is not strict, **When** the operator and operands are running, **Then** TLS behavior is unchanged from the pre-feature baseline (TLS 1.2 minimum, no regression in connectivity).
3. **Given** a cluster administrator changes the cluster TLS profile or adherence policy on the APIServer resource, **When** the change is applied, **Then** the workload identity operator restarts and SPIRE operand workloads roll out with updated TLS settings reflecting the new profile within one reconciliation cycle per component.
4. **Given** strict adherence is active and the central TLS profile fetch fails or the profile is unset, **When** operands reconcile, **Then** the Intermediate profile defaults apply (TLS 1.2 minimum, empty cipher suite list per platform semantics).

---

### User Story 2 - Cluster administrator opts into strict hybrid post-quantum key exchange (Priority: P2)

As a **cluster administrator**, I need a single cluster-wide flag on the workload identity manager custom resource to enforce hybrid post-quantum key exchange (X25519MLKEM768) across all SPIRE operands so that workload identity traffic can meet quantum-safe requirements without per-component configuration.

**Why this priority**: Supports OCPSTRAT-3145 PQC readiness. Opt-in by design to avoid breaking legacy clients.

**Independent Test**: Set the cluster-wide PQC flag to enabled on the workload identity manager resource; verify SPIRE server–agent and workload mTLS connections negotiate hybrid PQC only, and clients without PQC support are rejected.

**Acceptance Scenarios**:

1. **Given** the workload identity manager resource has the post-quantum key exchange flag set to true, **When** SPIRE server, agent, OIDC discovery provider, and controller manager configurations are reconciled, **Then** each operand enforces TLS 1.3 minimum with hybrid X25519MLKEM768 key exchange only and rejects peers that cannot negotiate it.
2. **Given** the post-quantum key exchange flag is true, **When** the cluster APIServer also has strict TLS adherence with a custom central profile, **Then** the application-level PQC policy takes precedence — central profile minimum TLS version, cipher suites, and key exchange groups are not injected into SPIRE operand configuration — and a warning event is emitted documenting the override.
3. **Given** the post-quantum key exchange flag is absent or false, **When** strict adherence is active, **Then** the central TLS profile settings are injected into SPIRE operand configuration as in User Story 1.
4. **Given** the post-quantum key exchange flag is absent or false and adherence is not strict, **When** both peers support hybrid PQC, **Then** connections may opportunistically negotiate X25519MLKEM768 with graceful fallback to classical curves when the peer does not support PQC.

---

### User Story 3 - Platform compliance team validates TLS posture (Priority: P2)

As a **platform compliance engineer**, I need to validate TLS posture across all ZTWIM and SPIRE TLS-serving endpoints using standard OpenShift TLS scanning tooling so that organizational TLS consistency requirements can be audited and reported.

**Why this priority**: Verification is the primary acceptance mechanism cited in the source ADR; without measurable validation, compliance claims cannot be substantiated.

**Independent Test**: Run TLS scanner against each documented endpoint for each profile type (Old, Intermediate, Modern, Custom) and confirm scan results match expected TLS version, cipher suite, and key exchange group constraints.

**Acceptance Scenarios**:

1. **Given** each supported TLS profile type is configured on the cluster APIServer with strict adherence, **When** TLS scanner probes the operator metrics endpoint, SPIRE server gRPC API, federation bundle HTTPS endpoint, SPIRE server metrics endpoint, OIDC discovery HTTPS endpoint, and admission webhook HTTPS endpoint, **Then** each endpoint's negotiated TLS parameters match the active profile constraints.
2. **Given** a custom TLS profile disables specific cipher suites, **When** TLS scanner probes all covered endpoints, **Then** the disabled cipher suites are not offered by any endpoint.
3. **Given** the post-quantum key exchange flag is enabled, **When** TLS scanner or equivalent TLS probing connects to SPIRE mTLS endpoints, **Then** X25519MLKEM768 is negotiated and non-PQC clients receive a handshake failure.

---

### User Story 4 - Cluster administrator receives predictable behavior on profile migration (Priority: P3)

As a **cluster administrator**, I need TLS profile changes to propagate through controlled restarts rather than silent in-process reload so that I can plan for brief service interruption and verify the new configuration took effect.

**Why this priority**: Operational predictability; avoids false confidence from stale in-process TLS state.

**Independent Test**: Change cluster TLS profile from Intermediate to Modern; observe operator pod restart followed by operand rolling updates; confirm endpoints reflect Modern profile after rollout completes.

**Acceptance Scenarios**:

1. **Given** the cluster TLS profile is changed on the APIServer resource, **When** the operator's security profile watcher detects the change, **Then** the operator process exits gracefully and restarts with the updated profile applied to its own TLS servers.
2. **Given** the operator has reconciled updated TLS settings into SPIRE operand configuration, **When** configuration content changes, **Then** operand StatefulSets, DaemonSets, and Deployments undergo rolling restarts and new pods start with the updated TLS settings.
3. **Given** SPIRE agents lose server connectivity during a rolling restart, **When** the server pod becomes available again, **Then** agents automatically reconnect without manual intervention.

---

### Edge Cases

- **When** the post-quantum key exchange flag is enabled and a SPIRE agent or workload client does not support X25519MLKEM768, **then** the TLS handshake fails and the connection is not established; the operator surfaces this as a connectivity failure observable through operand status and logs.
- **When** strict adherence is active and the central TLS profile references cipher suites not supported by the runtime, **then** unsupported ciphers are skipped with a logged warning and remaining supported ciphers from the profile are applied.
- **When** the post-quantum key exchange flag is toggled from true to false while strict adherence is active, **then** central TLS profile injection resumes on the next reconciliation and operand workloads roll out with profile-driven settings.
- **When** the cluster APIServer TLS adherence policy changes from strict to non-strict, **then** operand TLS settings revert to Intermediate defaults without requiring manual CR edits.
- **When** a custom TLS profile disables ciphers that SPIRE requires for operation, **then** the operator continues reconciliation but affected endpoints may fail health checks; status conditions reflect degraded operand availability.
- **When** SPIRE agent Unix domain socket communication is used (non-TLS), **then** no TLS profile or PQC changes apply to that communication path.
- **When** TLS key exchange group preferences via central profile are not yet available on the platform (future platform capability), **then** group-based profile enforcement is deferred; hybrid PQC opt-in via the application flag serves as interim enforcement until group preferences graduate.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST apply the cluster-wide TLS security profile from the APIServer cluster configuration to all TLS-serving endpoints exposed by the workload identity operator itself (metrics and admission webhook servers).
- **FR-002**: System MUST inject resolved minimum TLS version and permitted cipher suites from the cluster TLS profile into SPIRE server, SPIRE agent, OIDC discovery provider, and SPIRE controller manager operand configurations when strict TLS adherence is active and the post-quantum key exchange flag is not enabled.
- **FR-003**: System MUST watch the cluster APIServer resource for TLS profile and adherence policy changes and propagate changes to the operator and operands via controlled restarts (operator process restart, then operand rolling updates).
- **FR-004**: System MUST preserve existing TLS behavior when the cluster uses the default Intermediate profile and TLS adherence is not strict — no connectivity or cipher negotiation regression.
- **FR-005**: System MUST expose a single optional boolean field on the cluster-scoped workload identity manager custom resource (`requirePQKEM`) that enables strict hybrid post-quantum key exchange (X25519MLKEM768) across all SPIRE operands when set to true.
- **FR-006**: When the post-quantum key exchange flag is true, system MUST enforce TLS 1.3 minimum and hybrid X25519MLKEM768 only on SPIRE server–agent and workload mTLS paths, rejecting peers without PQC support.
- **FR-007**: When the post-quantum key exchange flag is true, system MUST NOT inject central TLS profile minimum version, cipher suites, or key exchange groups into SPIRE operand configuration; application-level PQC policy takes precedence.
- **FR-008**: When the post-quantum key exchange flag is true and strict TLS adherence is also active, system MUST emit a warning event informing the administrator that application-level PQC policy overrides central profile injection.
- **FR-009**: System MUST declare TLS profile compliance capability in the operator's cluster service version metadata so the platform can identify the operator as TLS-profile-aware.
- **FR-010**: System MUST grant the operator read access to the cluster APIServer configuration resource required to resolve TLS profiles and adherence policy.
- **FR-011**: System MUST cover TLS server endpoints for: operator metrics, SPIRE server registration API, SPIRE federation bundle HTTPS, SPIRE server metrics HTTPS, OIDC discovery HTTPS, and SPIRE controller manager admission webhook HTTPS.
- **FR-012**: System MUST NOT modify TLS client-side settings for outbound connections from operator or SPIRE components.
- **FR-013**: System MUST NOT implement in-process hot-reload of TLS settings; configuration changes MUST take effect only after operator or operand restart.
- **FR-014**: System MUST fall back to Intermediate profile defaults (TLS 1.2 minimum, empty cipher suite list) when strict adherence is active but profile resolution fails or the profile is unset.
- **FR-015**: System MUST support configurable TLS settings on SPIRE operand TLS servers (minimum version and cipher suites read from injected configuration with fallback to TLS 1.2 and runtime defaults when unset).
- **FR-016**: When the post-quantum key exchange flag is false or absent and adherence is not strict, system MUST allow opportunistic hybrid PQC negotiation when both peers support it, with graceful fallback to classical key exchange otherwise.

### Key Entities

- **Cluster APIServer configuration**: Platform resource holding cluster-wide TLS security profile (Old, Intermediate, Modern, Custom), minimum TLS version, cipher suites, and (future) key exchange group preferences; also holds TLS adherence policy (strict-all-components, no-opinion, legacy-adhering-components-only).
- **Workload identity manager resource**: Cluster-scoped singleton custom resource governing ZTWIM deployment; gains optional `requirePQKEM` boolean field for cluster-wide PQC enforcement.
- **TLS security profile**: Named profile defining minimum TLS version and permitted cipher suites; Custom profiles allow administrator-defined cipher restrictions.
- **TLS adherence policy**: Platform policy determining whether components must strictly honor the central TLS profile or may use component defaults.
- **SPIRE operand configurations**: Server, agent, OIDC discovery provider, and controller manager configuration consumed at startup; receive injected TLS settings or PQC policy from the operator during reconciliation.
- **Covered TLS endpoints**: The six TLS-serving server endpoints listed in FR-011; Unix domain socket communication is explicitly excluded.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: When strict TLS adherence is active with any supported profile (Old, Intermediate, Modern, Custom), a cluster administrator can scan all six covered TLS endpoints and observe negotiated TLS version and cipher suites matching the active profile constraints within 30 minutes of profile application and operand rollout completion.
- **SC-002**: When the cluster uses default Intermediate profile with non-strict adherence, existing SPIRE agent-to-server and workload mTLS connections continue to succeed without administrator intervention after operator upgrade.
- **SC-003**: When a cluster administrator changes the APIServer TLS profile or adherence policy, all covered endpoints reflect the new effective TLS configuration after operator restart and operand rolling update complete — observable via TLS scanning without manual pod deletion.
- **SC-004**: When the post-quantum key exchange flag is enabled, TLS probing confirms X25519MLKEM768 negotiation on SPIRE mTLS endpoints and handshake failure for clients that do not support hybrid PQC.
- **SC-005**: When a custom TLS profile disables specific cipher suites, TLS scanning confirms those cipher suites are not offered by any of the six covered endpoints.
- **SC-006**: When the post-quantum key exchange flag is enabled concurrently with strict adherence, a warning event is visible to the cluster administrator documenting that PQC policy overrides central profile injection.
- **SC-007**: Clusters with strict TLS adherence can complete platform upgrade readiness checks without ZTWIM/SPIRE TLS non-compliance blocking the upgrade path.

## Assumptions

- **A-001**: Primary actors are **cluster administrators** (configure APIServer TLS profile, adherence policy, and optional PQC flag) and **platform compliance engineers** (validate TLS posture via scanning tools).
- **A-002**: Phases 1–3 from the source ADR are in scope: (1) operator TLS compliance, (2) central profile injection into SPIRE operands, (3) `requirePQKEM` CRD field and PQC propagation. Phase 4 (TLS key exchange group preferences via central profile when platform feature gate graduates) is explicitly **out of scope** for this feature (OCPSTRAT-3123 deferred).
- **A-003**: SPIRE operand TLS configurability requires coordinated updates to SPIRE upstream components (server, agent, OIDC provider, controller manager); these are delivered as downstream patches with upstream contribution planned, not permanent forks.
- **A-004**: Controller manager admission webhook PQC/TLS configurability ships when the upstream SPIRE controller manager patch is available; if unavailable at initial release, webhook TLS follows central profile injection (without PQC override) until the patch lands — documented as a known limitation, not a blocker for Phases 1–2.
- **A-005**: OIDC discovery provider TLS settings are applied in the SPIRE OIDC provider component (upstream fork), coordinated through the release repository submodule workflow — not via a separate operator-side code path.
- **A-006**: ML-DSA and other post-quantum **signature** algorithms are out of scope; only key exchange (KEM) via hybrid X25519MLKEM768 is in scope. Pure ML-KEM (non-hybrid) mode is not supported.
- **A-007**: TLS client configuration for outbound connections from operator and SPIRE components remains unchanged per platform PQC guidance.
- **A-008**: Brief SPIRE API unavailability during rolling restarts on profile change is acceptable; agents auto-reconnect and PodDisruptionBudgets limit disruption.
- **A-009**: All SPIRE agents and workloads must run on a runtime version supporting X25519MLKEM768 before administrators enable the post-quantum key exchange flag; enabling PQC on mixed-version fleets may cause expected handshake failures.
- **A-010**: Related initiative OCPSTRAT-3145 (hybrid ML-KEM) scope is satisfied by the single `requirePQKEM` cluster-wide flag; per-operand PQC flags are explicitly rejected as out of scope.
