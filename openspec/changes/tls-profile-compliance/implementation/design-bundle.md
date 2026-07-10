# Implementation Design Bundle

**Change:** tls-profile-compliance
**Phase:** Phase 4 — Upstream cross-repo
**Current Task:** T4_1
**Task Title:** Upstream SPIRE/ctrl-mgr TLS patches

---

## Task payload (current task)

### Task T4_1: Upstream SPIRE/ctrl-mgr TLS patches
- **Objective:** Track and coordinate configurable TLS patches in fork repos (out of tree).
- **Target file(s):** `openshift/spiffe-spire`, `openshift/spiffe-spire-controller-manager`
- **Non-goals / forbidden edits:** No SPIRE logic in ZTWIM operator repo (constitution Principle VI).
- **Implementation notes:** Deliver patches per ADR upstream table; upstream unit tests in fork.
- **Acceptance criteria:** FR-011; operand processes read minTLSVersion, cipherSuites, require_pq_kem from config.

---

## Verification

| Hook | Command / artifact | Task ID |
|------|-------------------|---------|
| Coordination doc | `implementation/upstream-tls-patches-T4_1.md` | T4_1 |
| Operator repo unchanged | No SPIRE source edits in operator repo | T4_1 |
