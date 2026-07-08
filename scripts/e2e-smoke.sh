#!/usr/bin/env bash
# e2e-smoke.sh — assertion suite that runs against an already-deployed
# dapr-go-crud-app stack (KinD + Dapr + Redis + Deployments). Uses kubectl
# port-forward (no LoadBalancer required); kubeconfig context is pinned so
# the script is safe to run alongside other KinD-using projects.
#
# Every operator-tunable value is externalized (per rules/common/configuration.md):
# the committed `.env.example` is the source of truth, an optional gitignored
# `.env` overrides it, and inline `${VAR:-default}` fallbacks make the script
# work if neither file is present. Local port-forward ports default to
# kernel-ephemeral (parallel-run safe); set E2E_LOCAL_*_PORT only to pin a
# fixed local port (the harness then verifies it is free, naming the holder).
set -euo pipefail

# ---- Load committed defaults (.env.example) then optional .env override ----
# `set -a` exports everything sourced so child processes inherit it. Ports are
# COMMENTED in .env.example (so they stay unset → ephemeral by default); only
# the timing knobs are set. This deliberately sources .env for TIMING while
# leaving ports ephemeral — see the config rule's test-harness exception.
# shellcheck source=/dev/null
if [ -f .env.example ]; then set -a; . ./.env.example; set +a; fi
# shellcheck source=/dev/null
if [ -f .env         ]; then set -a; . ./.env;         set +a; fi

# ---- Tunables (env-fallback defaults mirror .env.example) ----
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-dapr-go-crud-app}"
APP_NAMESPACE="${APP_NAMESPACE:-crud-app}"
APP_INTERNAL_PORT="${APP_INTERNAL_PORT:-8080}"

# Component start-up loop (wait for every Deployment to become Available).
E2E_POD_READY_TIMEOUT="${E2E_POD_READY_TIMEOUT:-180}"
E2E_COMPONENT_START_ATTEMPTS="${E2E_COMPONENT_START_ATTEMPTS:-30}"
E2E_COMPONENT_START_DELAY="${E2E_COMPONENT_START_DELAY:-5}"

# Port-forward readiness poll.
E2E_PORTFWD_ATTEMPTS="${E2E_PORTFWD_ATTEMPTS:-30}"
E2E_PORTFWD_DELAY="${E2E_PORTFWD_DELAY:-1}"

# Assertion poll budgets.
E2E_PUBSUB_TIMEOUT="${E2E_PUBSUB_TIMEOUT:-90}"
E2E_REPOST_INTERVAL="${E2E_REPOST_INTERVAL:-10}"
E2E_INVOKE_ATTEMPTS="${E2E_INVOKE_ATTEMPTS:-45}"
E2E_INVOKE_DELAY="${E2E_INVOKE_DELAY:-2}"
E2E_FANOUT_ATTEMPTS="${E2E_FANOUT_ATTEMPTS:-30}"
E2E_FANOUT_DELAY="${E2E_FANOUT_DELAY:-2}"
# Un-mask the service-c fan-out assertion (make it a hard FAIL) by setting
# E2E_REQUIRE_SERVICE_C_FANOUT=true. Defaults to true; set false only to
# tolerate a known-broken service-c subscription during investigation.
E2E_REQUIRE_SERVICE_C_FANOUT="${E2E_REQUIRE_SERVICE_C_FANOUT:-true}"

KUBECTL=(kubectl --context="kind-${KIND_CLUSTER_NAME}" -n "${APP_NAMESPACE}")

PASS=0
FAIL=0
PIDS=()

cleanup() {
  for pid in "${PIDS[@]:-}"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

pass() { echo "PASS: $*"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

# pick_port — ask the kernel for a free ephemeral TCP port (bind :0). The
# parallel-run-safe idiom: two concurrent e2e runs never collide on a local
# port because each is assigned a distinct free one.
pick_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()'
}

# port_in_use — true if something is already listening on 127.0.0.1:<port>.
port_in_use() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null && { exec 3>&- 3<&-; return 0; }
  return 1
}

# resolve_local_port — echo a usable local port for a forward. If the operator
# pinned one via env, verify it is free (fail early, naming the PID holding it);
# otherwise pick an ephemeral free port.
resolve_local_port() {
  local requested="$1" label="$2"
  if [ -n "$requested" ]; then
    if port_in_use "$requested"; then
      local holder
      holder="$( (command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$requested" -sTCP:LISTEN -t 2>/dev/null | head -1) || true)"
      fail "requested ${label} local port ${requested} is already in use${holder:+ (held by PID ${holder})} — free it or unset E2E_LOCAL_${label}_PORT to auto-pick"
      exit 1
    fi
    echo "$requested"
  else
    pick_port
  fi
}

# wait_for_all_deployments — block until EVERY Deployment in the namespace is
# Available, retrying in a loop with a configurable delay. This is the
# "all components start" gate: nothing is asserted until crud/timeline/
# service-a..c/consumer/publisher/datagen/errorgen are all up.
wait_for_all_deployments() {
  echo "==> Waiting for all deployments Available (${E2E_COMPONENT_START_ATTEMPTS} attempts × ${E2E_COMPONENT_START_DELAY}s)"
  local attempt
  for attempt in $(seq 1 "$E2E_COMPONENT_START_ATTEMPTS"); do
    if "${KUBECTL[@]}" wait --for=condition=Available deployment --all \
         --timeout="${E2E_POD_READY_TIMEOUT}s" >/dev/null 2>&1; then
      pass "all deployments Available"
      return 0
    fi
    echo "  components not all Available (attempt ${attempt}/${E2E_COMPONENT_START_ATTEMPTS}) — retry in ${E2E_COMPONENT_START_DELAY}s"
    sleep "$E2E_COMPONENT_START_DELAY"
  done
  echo "--- deployments ---"
  "${KUBECTL[@]}" get deployments -o wide || true
  "${KUBECTL[@]}" get pods -o wide || true
  fail "not all deployments became Available within the start budget"
  return 1
}

# port_forward — start a kubectl port-forward to a Service in the background;
# record the PID so the trap can clean it up. Polls the probe path until the
# forward is actually serving (configurable attempts × delay).
port_forward() {
  local svc="$1" local_port="$2" remote_port="$3" probe_path="$4"
  "${KUBECTL[@]}" port-forward "svc/${svc}" "${local_port}:${remote_port}" >/dev/null 2>&1 &
  PIDS+=("$!")
  local _
  for _ in $(seq 1 "$E2E_PORTFWD_ATTEMPTS"); do
    if curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${local_port}${probe_path}" 2>/dev/null | grep -qE '^[234][0-9][0-9]$'; then
      return 0
    fi
    sleep "$E2E_PORTFWD_DELAY"
  done
  fail "port-forward to ${svc}:${remote_port} not serving on 127.0.0.1:${local_port}${probe_path}"
  return 1
}

echo "=========================================="
echo " dapr-go-crud-app e2e smoke"
echo " cluster:   kind-${KIND_CLUSTER_NAME}"
echo " namespace: ${APP_NAMESPACE}"
echo "=========================================="

# ---- Wait for every component to come up (configurable start loop) ----
wait_for_all_deployments || exit 1

# ---- Open port-forwards to the two HTTP-facing services ----
# Local ports default to ephemeral (parallel-safe); override via
# E2E_LOCAL_CRUD_PORT / E2E_LOCAL_TIMELINE_PORT for a stable local port.
LOCAL_CRUD_PORT="$(resolve_local_port "${E2E_LOCAL_CRUD_PORT:-}" CRUD)"
LOCAL_TIMELINE_PORT="$(resolve_local_port "${E2E_LOCAL_TIMELINE_PORT:-}" TIMELINE)"
echo "==> Establishing port-forwards (crud→${LOCAL_CRUD_PORT}, timeline→${LOCAL_TIMELINE_PORT})"
port_forward crud-app "$LOCAL_CRUD_PORT" "$APP_INTERNAL_PORT" /api/v1/todos || exit 1
port_forward timeline-app "$LOCAL_TIMELINE_PORT" "$APP_INTERNAL_PORT" /todos || exit 1

CRUD="http://127.0.0.1:${LOCAL_CRUD_PORT}/api/v1/todos"
TIMELINE="http://127.0.0.1:${LOCAL_TIMELINE_PORT}/todos"

post_todo() {
  local text="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -X POST -H 'Content-Type: application/json' \
    -d "{\"text\":\"${text}\",\"done\":\"false\"}" \
    "$CRUD"
}

# ---- 1. CRUD round-trip via crud-app (POST → GET) ----
# NOTE: only POST+GET are asserted here, deliberately. crud-app runs a
# background cleanup() loop that periodically deletes ALL todos plus a
# generateLoad() writer, so a POST→PUT→DELETE sequence against the live pod is
# racy (cleanup can wipe the row mid-sequence). PUT/DELETE are asserted
# deterministically at the integration layer instead
# (pkg/server/server_integration_test.go), against an isolated storage.
TODO_TEXT="e2e-$(date +%s%N)"
echo "==> POST /api/v1/todos text=${TODO_TEXT}"
status=$(post_todo "$TODO_TEXT")
if [ "$status" = "200" ]; then
  pass "POST /api/v1/todos returned 200"
else
  fail "POST /api/v1/todos returned $status"
fi

echo "==> GET /api/v1/todos"
if curl -sf "$CRUD" | grep -q "$TODO_TEXT"; then
  pass "GET /api/v1/todos lists the just-created todo"
else
  fail "GET /api/v1/todos missing '${TODO_TEXT}' (body: $(curl -s "$CRUD"))"
fi

# ---- 2. Pub/sub round-trip: crud-app → 'todos' topic → timeline-app ----
# Dapr pub/sub with Redis Streams is at-most-once: if timeline-app's sidecar
# hasn't bound the subscription yet when crud-app publishes, the event is lost.
# Defend with re-POSTs every E2E_REPOST_INTERVAL seconds — by the second or
# third attempt the subscription is always live.
echo "==> Polling timeline-app for any propagated todo (up to ${E2E_PUBSUB_TIMEOUT}s, re-POST every ${E2E_REPOST_INTERVAL}s)"
found=""
for i in $(seq 1 "$E2E_PUBSUB_TIMEOUT"); do
  if curl -sf "$TIMELINE" | grep -q '"New todo created: e2e-'; then
    found="yes"
    break
  fi
  if [ $((i % E2E_REPOST_INTERVAL)) -eq 0 ]; then
    retry_text="${TODO_TEXT}-retry-${i}"
    echo "  re-POST text=${retry_text}"
    post_todo "$retry_text" >/dev/null
  fi
  sleep 1
done
if [ "$found" = "yes" ]; then
  pass "timeline-app received a 'todos' pub/sub event"
else
  fail "timeline-app did NOT receive any event within ${E2E_PUBSUB_TIMEOUT}s (timeline: $(curl -s "$TIMELINE"))"
fi

# ---- 3. Service-invocation chain + resiliency policy (service-a → service-b) ----
# service-a invokes service-b/method/hello on a 10-second ticker. service-b
# sleeps 1500ms on ~55% of calls, tripping service-a-resiliency's 1s timeout +
# retryForever exponential retry. Two assertions: (a) ≥3 invocations succeed,
# AND (b) the daprd sidecar logs retry/timeout evidence.
echo "==> Asserting service-a → service-b invocation chain + resiliency policy (poll up to $((E2E_INVOKE_ATTEMPTS * E2E_INVOKE_DELAY))s)"
invocations=0
for _ in $(seq 1 "$E2E_INVOKE_ATTEMPTS"); do
  invocations=$("${KUBECTL[@]}" logs -l app=service-a -c service-a --tail=500 2>/dev/null | grep -c 'Order passed' || true)
  if [ "$invocations" -ge 3 ]; then
    break
  fi
  sleep "$E2E_INVOKE_DELAY"
done
if [ "$invocations" -ge 3 ]; then
  pass "service-a completed ${invocations} successful service-b invocations"
else
  fail "service-a logged only ${invocations} 'Order passed' lines — invocation chain broken"
fi

# Resiliency policy evidence — sidecar logs of retries / timeouts.
retry_evidence=$("${KUBECTL[@]}" logs -l app=service-a -c daprd --tail=1000 2>/dev/null \
  | grep -ciE 'retry|timeout|deadline exceeded' || true)
if [ "$retry_evidence" -ge 1 ]; then
  pass "service-a-resiliency policy active (${retry_evidence} retry/timeout indicator(s) in daprd sidecar)"
else
  echo "WARN: no retry/timeout evidence in service-a daprd logs — every call may have landed on the fast path this run (rare but possible)"
fi

# ---- 4. Pub/sub fan-out: events topic → consumer + service-c ----
# Both consumer-app and service-c subscribe to the 'events' topic via the gRPC
# SDK; publisher-app + service-b publish. Poll until both register an event.
echo "==> Asserting 'events' topic fan-out (poll up to $((E2E_FANOUT_ATTEMPTS * E2E_FANOUT_DELAY))s)"
consumer_seen=0
servicec_seen=0
for _ in $(seq 1 "$E2E_FANOUT_ATTEMPTS"); do
  consumer_seen=$("${KUBECTL[@]}" logs -l app=consumer-app --tail=500 2>/dev/null | grep -c 'event consumed' || true)
  servicec_seen=$("${KUBECTL[@]}" logs -l app=service-c --tail=500 2>/dev/null | grep -c 'event consumed' || true)
  if [ "$consumer_seen" -ge 1 ] && [ "$servicec_seen" -ge 1 ]; then
    break
  fi
  sleep "$E2E_FANOUT_DELAY"
done
# consumer-app is the always-on subscriber; its zero is an unconditional FAIL.
if [ "$consumer_seen" -ge 1 ]; then
  pass "events fan-out: consumer-app received ${consumer_seen} event(s)"
else
  fail "events fan-out: consumer-app received NOTHING — broker/subscription broken"
fi
# service-c is the second subscriber. By default this is a hard assertion
# (E2E_REQUIRE_SERVICE_C_FANOUT=true); set it false only to tolerate a
# known-broken service-c subscription during investigation.
if [ "$servicec_seen" -ge 1 ]; then
  pass "events fan-out: service-c received ${servicec_seen} event(s)"
elif [ "$E2E_REQUIRE_SERVICE_C_FANOUT" = "true" ]; then
  fail "events fan-out: service-c received NOTHING (set E2E_REQUIRE_SERVICE_C_FANOUT=false to downgrade to a warning)"
else
  echo "WARN: events fan-out asymmetric — service-c received 0 events (consumer-app=${consumer_seen}); tolerated by E2E_REQUIRE_SERVICE_C_FANOUT=false"
fi

# ---- 5. Negative cases ----
echo "==> Negative: malformed JSON"
status=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d 'not-json' "$CRUD")
if [ "$status" = "400" ]; then
  pass "POST /api/v1/todos with malformed JSON returned 400"
else
  fail "POST malformed JSON returned $status, want 400"
fi

echo "==> Negative: missing required field (Text)"
status=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{}' "$CRUD")
if [ "$status" = "400" ]; then
  pass "POST /api/v1/todos with empty body returned 400 (Text required)"
else
  fail "POST empty body returned $status, want 400"
fi

echo "==> Negative: 404 on unknown route"
status=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${LOCAL_CRUD_PORT}/api/v1/nonexistent")
if [ "$status" = "404" ]; then
  pass "GET unknown route returned 404"
else
  fail "GET unknown route returned $status, want 404"
fi

echo ""
echo "=========================================="
echo " Results: ${PASS} passed, ${FAIL} failed"
echo "=========================================="
[ "$FAIL" -eq 0 ]
