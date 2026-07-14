# Deviations Observed — tls-compliance

## D1: `replace` for controller-runtime (T1_1)

**Expected:** Pure `require` + `make vendor` without `replace` (plan open question #2).

**Observed:** `github.com/openshift/controller-runtime-common` pulls `sigs.k8s.io/controller-runtime v0.23.3`, which breaks `github.com/spiffe/spire-controller-manager v0.6.4` (webhook API incompatibility). Upgrading spire-controller-manager to v0.6.6 requires Go 1.26.4 (not available in this repo's toolchain).

**Mitigation:** Added to `go.mod`:

```go
replace sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.22.4
```

**Impact:** controller-runtime-common and library-go compile and tests pass against pinned v0.22.4.

## D2: FR-008 warning events (by design)

Per plan user feedback — no Kubernetes Warning events when `requirePQKEM=true` and strict adherence coexist. Not implemented.

## D3: T5_2 manual only

tls-scanner validation checklist produced at `implementation/tls-scanner-checklist.md`; not executed against a live cluster in this implementation session.
