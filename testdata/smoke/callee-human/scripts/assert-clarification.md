---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Script
spec:
  description: Verifies that the Human response reached shared workflow state.
  shell: bash
  onNonZero: fail
  timeout: 10s
  env:
    ACTUAL_CLARIFICATION: '{{ .State.clarification }}'
    EXPECTED_CLARIFICATION: grounded smoke reply
---
set -euo pipefail

if [[ "${ACTUAL_CLARIFICATION}" != "${EXPECTED_CLARIFICATION}" ]]; then
  printf 'clarification mismatch: got %q, want %q\n' \
    "${ACTUAL_CLARIFICATION}" "${EXPECTED_CLARIFICATION}" >&2
  exit 1
fi

printf 'clarification state verified\n'
