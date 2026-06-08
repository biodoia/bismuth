#!/usr/bin/env bash
# Smoke test end-to-end: avvia il server, hit endpoints, kill.
# Tutto in un solo processo, niente background.

set -e
cd "$(dirname "$0")/.."

NINEROUTER_URL=http://127.0.0.1:9999 \
NINEROUTER_KEY=smoketest \
./bin/bismuth serve --config config.yaml &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null" EXIT

# wait for ready
for i in $(seq 1 20); do
  if curl -fs http://127.0.0.1:9000/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if ! curl -fs http://127.0.0.1:9000/healthz >/dev/null; then
  echo "FAIL: server didn't come up"
  kill $SERVER_PID 2>/dev/null
  wait $SERVER_PID 2>/dev/null
  exit 1
fi

echo "=== /healthz ==="
curl -s http://127.0.0.1:9000/healthz
echo ""

echo "=== /api/v1/roles (count) ==="
curl -s http://127.0.0.1:9000/api/v1/roles | python3 -c "import json,sys; d=json.load(sys.stdin); print('roles:', len(d.get('roles',[])))"

echo "=== /api/v1/agents (initial) ==="
curl -s http://127.0.0.1:9000/api/v1/agents | python3 -m json.tool

echo "=== /api/v1/tasks (initial) ==="
curl -s http://127.0.0.1:9000/api/v1/tasks | python3 -m json.tool

echo "=== spawn fake implementer (cli=bash) ==="
SPAWN=$(curl -s -X POST http://127.0.0.1:9000/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"role":"implementer","cli":"bash","task":"echo ciao-da-bismuth"}')
echo "$SPAWN" | python3 -m json.tool
AGENT_ID=$(echo "$SPAWN" | python3 -c "import json,sys; print(json.load(sys.stdin).get('agent_id',''))")
echo "agent_id=$AGENT_ID"

sleep 1.5

echo "=== /api/v1/agents (after spawn) ==="
curl -s http://127.0.0.1:9000/api/v1/agents | python3 -m json.tool

echo "=== /api/v1/tasks (after spawn) ==="
curl -s http://127.0.0.1:9000/api/v1/tasks | python3 -m json.tool

echo "=== /api/v1/events?limit=10 ==="
curl -s "http://127.0.0.1:9000/api/v1/events?limit=10" | python3 -c "import json,sys; d=json.load(sys.stdin); print('event count:', len(d.get('events',[]))); [print(' seq=%d' % e.get('Seq',0), 'type=' + e.get('Type',''), 'agent=' + e.get('AgentID','')[:20]) for e in d.get('events',[])]"

echo "=== read agent pane ==="
curl -s "http://127.0.0.1:9000/api/v1/agents/${AGENT_ID}/read?n=50" | python3 -m json.tool | head -20

echo "=== kill agent ==="
curl -s -X POST "http://127.0.0.1:9000/api/v1/agents/${AGENT_ID}/kill" | python3 -m json.tool

echo "=== DONE ==="
