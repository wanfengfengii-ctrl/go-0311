#!/usr/bin/env bash
# Smoke test for the unitized curtain-wall silicone hoist-gate backend.
#
# This script builds the server, starts it against a temporary SQLite database,
# drives a real lock + command + query flow over the local HTTP API, and then
# tears everything down. It performs no external network access and does not
# merely invoke `go test`.
set -euo pipefail

PORT="${PORT:-18123}"
BASE="http://127.0.0.1:${PORT}"
WORKDIR_ROOT="$(mktemp -d)"
BIN="${WORKDIR_ROOT}/server"
DB="${WORKDIR_ROOT}/smoke.db"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${WORKDIR_ROOT}"
}
trap cleanup EXIT

echo "building server binary..."
go build -o "${BIN}" ./cmd/server

echo "starting server on ${BASE}..."
DB_PATH="${DB}" LISTEN_ADDR="127.0.0.1:${PORT}" "${BIN}" &
SERVER_PID=$!

# The server exits immediately if it cannot bind; fail fast in that case.
sleep 0.2
if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
  echo "server failed to start (port ${PORT} may be in use)" >&2
  exit 1
fi

# Wait for the health endpoint to become ready.
ready=""
for _ in $(seq 1 50); do
  if resp="$(curl -s "http://127.0.0.1:${PORT}/healthz" 2>/dev/null)"; then
    ready="${resp}"
    break
  fi
  sleep 0.1
done
if [[ -z "${ready}" ]]; then
  echo "server did not become healthy" >&2
  exit 1
fi
echo "health: ${ready}"

# Probe health deterministically (capture response, never pipe to grep -q).
health_resp="$(curl -s "${BASE}/healthz")"
if [[ "${health_resp}" != *'"status":"ok"'* ]]; then
  echo "unexpected health response: ${health_resp}" >&2
  exit 1
fi

# Lock a task over the public API.
lock_body='{"task_id":"T-1","building":"A","facade_zone":"E","panel":"P-017","design_version":"dv-1","compatibility_ver":"cv-1","compat_valid_until":100000,"surface_summary":"s","batch":{"base_batch":"B","catalyst_batch":"C","primer_batch":"P"},"joints":[{"joint_id":"J-1","direction":"E","start":0,"end":3000,"width":20,"depth":10,"bond_area_um2":200,"segments":[{"seq":1,"start":0,"end":1000},{"seq":2,"start":1000,"end":2000},{"seq":3,"start":2000,"end":3000}],"trial_mapping":{"seg1":"sample-1"}}],"thresholds":{},"locked_at":100}'
lock_resp="$(curl -s -X POST -H 'Content-Type: application/json' -d "${lock_body}" "${BASE}/v1/tasks/lock")"
if [[ "${lock_resp}" != *'"generation":1'* ]]; then
  echo "lock failed: ${lock_resp}" >&2
  exit 1
fi
echo "lock: ${lock_resp}"

# Submit a clean command.
cmd_body='{"operation_id":"op-clean","kind":"clean","logical_time":101}'
cmd_resp="$(curl -s -X POST -H 'Content-Type: application/json' -d "${cmd_body}" "${BASE}/v1/tasks/T-1/commands")"
if [[ "${cmd_resp}" != *'"stage":"CLEANED"'* ]]; then
  echo "command failed: ${cmd_resp}" >&2
  exit 1
fi
echo "command: ${cmd_resp}"

# Query the task projection and confirm the stage persisted.
task_resp="$(curl -s "${BASE}/v1/tasks/T-1")"
if [[ "${task_resp}" != *'"stage":"CLEANED"'* ]]; then
  echo "task query failed: ${task_resp}" >&2
  exit 1
fi
echo "task: ${task_resp}"

echo "smoke test passed"
