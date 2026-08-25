#!/usr/bin/env bash
# End-to-end validation: port-forwards to talam-server and polls until it has
# ingested findings from the demo mesh, correlated them into incidents, AND
# gotten a real explanation back from the configured LLM provider (Claude, by
# default) — proving the whole pipeline (collector -> analyzer -> ingest ->
# correlation -> LLM gateway) actually works against a live cluster, not just
# that the binaries started.
set -euo pipefail

NAMESPACE="${NAMESPACE:-talam-system}"
LOCAL_PORT="${LOCAL_PORT:-8443}"
TIMEOUT_SECS="${TIMEOUT_SECS:-180}"

echo "==> Waiting for talam-agent to be deployed by the operator..."
for i in $(seq 1 30); do
  if kubectl -n "$NAMESPACE" get deploy -l talam.dev/component=agent 2>/dev/null | grep -q talam-agent; then
    break
  fi
  sleep 2
done
kubectl -n "$NAMESPACE" wait --for=condition=available --timeout=120s deploy -l talam.dev/component=agent

echo "==> Port-forwarding talam-server:${LOCAL_PORT}..."
kubectl -n "$NAMESPACE" port-forward svc/talam-server "${LOCAL_PORT}:8443" >/tmp/talam-port-forward.log 2>&1 &
PF_PID=$!
trap 'kill $PF_PID 2>/dev/null || true' EXIT

for i in $(seq 1 20); do
  if curl -sf "http://localhost:${LOCAL_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

STATS=$(curl -sf "http://localhost:${LOCAL_PORT}/v1/stats")
PROVIDER=$(echo "$STATS" | grep -o '"llmProvider":"[^"]*"' | cut -d'"' -f4)
echo "==> talam-server is up. Resolved LLM provider: ${PROVIDER:-<none>}"

echo "==> Waiting up to ${TIMEOUT_SECS}s for incidents with a real LLM explanation..."
deadline=$((SECONDS + TIMEOUT_SECS))
explained_count=0
incidents_json=""
while [ $SECONDS -lt $deadline ]; do
  incidents_json=$(curl -sf "http://localhost:${LOCAL_PORT}/v1/incidents" || echo "[]")
  total=$(echo "$incidents_json" | grep -o '"id":"' | wc -l | tr -d ' ')
  explained_count=$(echo "$incidents_json" | grep -o '"explanation":"[^"]\+' | wc -l | tr -d ' ')
  errored_count=$(echo "$incidents_json" | grep -o '"explainError":"[^"]\+' | wc -l | tr -d ' ')
  echo "    incidents=${total} explained=${explained_count} explainErrors=${errored_count}"
  if [ "$explained_count" -ge 1 ]; then
    break
  fi
  if [ "$errored_count" -ge 1 ] && [ "$total" -ge 1 ] && [ "$explained_count" -eq 0 ]; then
    echo "!! At least one incident has an explainError and none have succeeded yet — check credentials." >&2
  fi
  sleep 5
done

if [ "$explained_count" -lt 1 ]; then
  echo "FAIL: no incident got a real LLM explanation within ${TIMEOUT_SECS}s." >&2
  echo "--- last /v1/incidents response ---" >&2
  echo "$incidents_json" >&2
  echo "--- talam-server logs ---" >&2
  kubectl -n "$NAMESPACE" logs deploy/talam-server --tail=100 >&2 || true
  exit 1
fi

echo
echo "==> PASS. At least one incident was explained by a real LLM call."
echo "==> Sample incident(s):"
curl -sf "http://localhost:${LOCAL_PORT}/v1/incidents" | python3 -m json.tool 2>/dev/null | head -80 || echo "$incidents_json"

echo
echo "==> Proposals generated so far:"
curl -sf "http://localhost:${LOCAL_PORT}/v1/proposals" | python3 -m json.tool 2>/dev/null | head -80 || true

echo
echo "Dashboard: kubectl -n ${NAMESPACE} port-forward svc/talam-server 8443:8443, then open http://localhost:8443"
