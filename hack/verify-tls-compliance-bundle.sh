#!/usr/bin/env bash
# Verify OLM bundle manifests include TLS compliance artifacts (FR-009, FR-010).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUNDLE_DIR="${ROOT}/bundle/manifests"
CSV="${BUNDLE_DIR}/zero-trust-workload-identity-manager.clusterserviceversion.yaml"
CRD="${BUNDLE_DIR}/operator.openshift.io_zerotrustworkloadidentitymanagers.yaml"

if [[ ! -f "${CSV}" ]]; then
	echo "ERROR: bundle CSV not found at ${CSV}; run 'make bundle' first" >&2
	exit 1
fi

if ! grep -q 'features\.operators\.openshift\.io/tls-profiles: "true"' "${CSV}"; then
	echo "ERROR: bundle CSV missing tls-profiles: true annotation (FR-009)" >&2
	exit 1
fi

if ! grep -q 'requirePQKEM:' "${CRD}"; then
	echo "ERROR: bundled CRD missing requirePQKEM field" >&2
	exit 1
fi

if ! grep -q 'apiservers' "${CSV}"; then
	echo "ERROR: bundle CSV missing APIServer resource rule (FR-010)" >&2
	exit 1
fi

for verb in get list watch; do
	if ! grep -A6 'apiservers' "${CSV}" | grep -q "\\- ${verb}"; then
		echo "ERROR: bundle CSV missing APIServer RBAC verb '${verb}' (FR-010)" >&2
		exit 1
	fi
done

echo "TLS compliance bundle verification passed"
