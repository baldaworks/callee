#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
agent_root="${repo_root}/testdata/smoke/callee-human"

mode="all"
keep_temp=0
temp_dir=""
active_pid=""
active_fifo_fd=""

usage() {
  cat <<'EOF'
Usage: scripts/smoke-test-callee-human.sh [questions|loop|all] [--keep-temp]

Runs PTY-backed smoke tests against the current Callee checkout:
- questions: verifies a root Human waits for input and returns it as the artifact
- loop: verifies Role return continues to Human, the reply reaches Script state,
  and the Loop starts Role visit 2
- all: runs both tests

The loop mode requires an installed and authenticated Codex CLI. Both modes
require util-linux script(1), mkfifo, and Go. Use --keep-temp to preserve
artifacts and diagnostics under /tmp.
EOF
}

fail() {
  keep_temp=1
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  if [[ -n "${active_pid}" ]]; then
    kill "${active_pid}" >/dev/null 2>&1 || true
    wait "${active_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${active_fifo_fd}" ]]; then
    eval "exec ${active_fifo_fd}>&-"
    eval "exec ${active_fifo_fd}<&-"
  fi
  if [[ -n "${temp_dir}" && "${keep_temp}" -eq 0 ]]; then
    rm -rf "${temp_dir}"
  elif [[ -n "${temp_dir}" ]]; then
    echo "Kept temp files in ${temp_dir}"
  fi
}

trap cleanup EXIT

while (($# > 0)); do
  case "$1" in
    questions|loop|all)
      mode="$1"
      ;;
    --keep-temp)
      keep_temp=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      fail "unknown argument: $1"
      ;;
  esac
  shift
done

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_cmd go
require_cmd mkfifo
require_cmd script
if [[ "${mode}" == "loop" || "${mode}" == "all" ]]; then
  require_cmd codex
fi

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/callee-human-smoke.XXXXXX")"
callee_bin="${temp_dir}/callee"

echo "Building Callee from the current checkout..."
go -C "${repo_root}" build -o "${callee_bin}" ./cmd/callee
"${callee_bin}" --agent-root "${agent_root}" agent view workflows/human-loop --json >/dev/null

shell_join() {
  local joined=()
  local quoted=""
  for arg in "$@"; do
    printf -v quoted '%q' "$arg"
    joined+=("${quoted}")
  done
  local old_ifs="${IFS}"
  IFS=' '
  printf '%s' "${joined[*]}"
  IFS="${old_ifs}"
}

line_has_fields() {
  local path="$1"
  shift

  [[ -f "${path}" ]] || return 1

  local line=""
  local field=""
  local matched=0
  while IFS= read -r line; do
    matched=1
    for field in "$@"; do
      if [[ "${line}" != *"${field}"* ]]; then
        matched=0
        break
      fi
    done
    if ((matched)); then
      return 0
    fi
  done <"${path}"

  return 1
}

print_diagnostics() {
  local path="$1"
  echo "Last diagnostics from ${path}:" >&2
  if [[ -f "${path}" ]]; then
    tail -40 "${path}" >&2
  else
    echo "(missing file)" >&2
  fi
}

wait_for_fields() {
  local path="$1"
  local timeout_seconds="$2"
  local label="$3"
  shift 3

  local deadline=$((SECONDS + timeout_seconds))
  while ((SECONDS < deadline)); do
    if line_has_fields "${path}" "$@"; then
      echo "PASS: ${label}"
      return 0
    fi
    sleep 1
  done

  print_diagnostics "${path}"
  fail "${label}"
}

assert_fields() {
  local path="$1"
  local label="$2"
  shift 2

  if line_has_fields "${path}" "$@"; then
    echo "PASS: ${label}"
    return 0
  fi

  print_diagnostics "${path}"
  fail "${label}"
}

start_agent_run() {
  local agent_id="$1"
  local message="$2"
  local run_dir="$3"
  local fifo_path="${run_dir}/stdin.fifo"
  local artifact_path="${run_dir}/artifact.txt"
  local diagnostics_path="${run_dir}/diagnostics.txt"
  local command_string=""

  mkdir -p "${run_dir}"
  mkfifo "${fifo_path}"
  exec {active_fifo_fd}<>"${fifo_path}"

  command_string="$(shell_join \
    "${callee_bin}" \
    --agent-root "${agent_root}" \
    agent run "${agent_id}" \
    --message "${message}" \
    --repl-timeout 1m)"
  command_string+=" > $(printf '%q' "${artifact_path}")"
  command_string+=" 2> $(printf '%q' "${diagnostics_path}")"

  script -qefc "${command_string}" /dev/null <&"${active_fifo_fd}" &
  active_pid=$!
}

close_fifo_fd() {
  if [[ -n "${active_fifo_fd}" ]]; then
    eval "exec ${active_fifo_fd}>&-"
    eval "exec ${active_fifo_fd}<&-"
    active_fifo_fd=""
  fi
}

wait_for_successful_exit() {
  local timeout_seconds="$1"
  local diagnostics_path="$2"
  local label="$3"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    if ! kill -0 "${active_pid}" >/dev/null 2>&1; then
      local status=0
      wait "${active_pid}" || status=$?
      active_pid=""
      close_fifo_fd
      if ((status == 0)); then
        echo "PASS: ${label}"
        return 0
      fi
      print_diagnostics "${diagnostics_path}"
      fail "${label} (exit ${status})"
    fi
    sleep 1
  done

  print_diagnostics "${diagnostics_path}"
  fail "${label} (timed out)"
}

stop_active_run() {
  kill "${active_pid}" >/dev/null 2>&1 || true
  wait "${active_pid}" >/dev/null 2>&1 || true
  active_pid=""
  close_fifo_fd
}

run_questions_smoke() {
  local run_dir="${temp_dir}/questions"
  local artifact_path="${run_dir}/artifact.txt"
  local diagnostics_path="${run_dir}/diagnostics.txt"
  local response="smoke-test clarification"

  echo "Running questions smoke test..."
  start_agent_run "humans/questions" "verify Human input" "${run_dir}"
  wait_for_fields "${diagnostics_path}" 15 "questions enters Human prompt" \
    "running agent" "id=humans/questions" "kind=Human" "visit=1"
  printf '%s\n' "${response}" >&"${active_fifo_fd}"
  wait_for_successful_exit 30 "${diagnostics_path}" "questions run completes"

  assert_fields "${diagnostics_path}" "questions Human returns successfully" \
    "agent finished" "id=humans/questions" "kind=Human" \
    "outcome=return" "status=completed" "visit=1"
  assert_fields "${diagnostics_path}" "questions run emits final metrics" \
    "agent run finished" "agent_duration=" "agent_wait_duration=" \
    "agent_token_usage=unavailable" "status=completed"

  if [[ "$(<"${artifact_path}")" != "${response}" ]]; then
    echo "Artifact contents:" >&2
    cat "${artifact_path}" >&2 || true
    fail "questions artifact preserves operator response"
  fi
  echo "PASS: questions artifact preserves operator response"
}

run_loop_smoke() {
  local run_dir="${temp_dir}/loop"
  local diagnostics_path="${run_dir}/diagnostics.txt"
  local response="grounded smoke reply"

  echo "Running Loop smoke test..."
  start_agent_run "workflows/human-loop" "verify Human Loop round-trip" "${run_dir}"
  wait_for_fields "${diagnostics_path}" 300 "Loop reaches first Human prompt" \
    "running agent" "id=questions" "kind=Human" \
    "ref=humans/questions" "visit=1"
  assert_fields "${diagnostics_path}" "Role return continues the Loop" \
    "agent finished" "id=normalizer" "kind=Role" \
    "outcome=return" "status=completed" "visit=1"

  printf '%s\n' "${response}" >&"${active_fifo_fd}"
  wait_for_fields "${diagnostics_path}" 30 "Loop Human returns successfully" \
    "agent finished" "id=questions" "kind=Human" \
    "outcome=return" "status=completed" "visit=1"
  wait_for_fields "${diagnostics_path}" 30 "Script validates Human shared state" \
    "agent finished" "id=state_check" "kind=Script" \
    "outcome=return" "status=completed" "visit=1"
  wait_for_fields "${diagnostics_path}" 30 "Loop resumes with Role visit 2" \
    "running agent" "id=normalizer" "kind=Role" "visit=2"

  stop_active_run
}

case "${mode}" in
  questions)
    run_questions_smoke
    ;;
  loop)
    run_loop_smoke
    ;;
  all)
    run_questions_smoke
    run_loop_smoke
    ;;
esac

echo "Smoke tests completed successfully."
