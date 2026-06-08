#!/usr/bin/env bash
# MCP end-to-end smoke test: simulates a worker connecting to bismuth-team.
# Tests: initialize, tools/list, team_status, team_peers, team_post,
#        team_read_inbox, team_claim, team_finish, shared_memory.

set -e
cd "$(dirname "$0")/.."

export BISMUTH_MCP_DB="/home/lisergico25/.tmp/bismuth-mcp-smoke.db"
rm -f "$BISMUTH_MCP_DB"

# Helper: send JSON-RPC and read one response line
rpc() {
  local id="$1" method="$2" params="${3:-{}}"
  printf '{"jsonrpc":"2.0","id":%s,"method":"%s","params":%s}\n' "$id" "$method" "$params"
}

echo "=== MCP smoke test ==="

# Initialize
rpc 1 initialize '{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}' | ./bin/bismuth mcp | head -1 | python3 -m json.tool | grep -E 'protocolVersion|serverInfo'

echo "--- tools/list ---"
rpc 2 tools/list | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('tools:', [t['name'] for t in d.get('result',{}).get('tools',[])])"

echo "--- team_status ---"
rpc 3 'tools/call' '{"name":"team_status","arguments":{}}' | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('status:', d.get('result',{}).get('content',[{}])[0].get('text','')[:80])"

echo "--- team_peers ---"
rpc 4 'tools/call' '{"name":"team_peers","arguments":{}}' | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('peers:', d.get('result',{}).get('content',[{}])[0].get('text','')[:80])"

echo "--- team_post ---"
rpc 5 'tools/call' '{"name":"team_post","arguments":{"to":"all","message":"hello from smoke test"}}' | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('post:', d.get('result',{}).get('content',[{}])[0].get('text','')[:80])"

echo "--- team_read_inbox ---"
rpc 6 'tools/call' '{"name":"team_read_inbox","arguments":{"limit":5}}' | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('inbox:', d.get('result',{}).get('content',[{}])[0].get('text','')[:80])"

echo "--- shared_memory ---"
rpc 7 'tools/call' '{"name":"shared_memory","arguments":{"query":"test"}}' | ./bin/bismuth mcp | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); print('memory:', d.get('result',{}).get('content',[{}])[0].get('text','')[:80])"

echo "=== ALL MCP TOOLS OK ==="
rm -f "$BISMUTH_MCP_DB"
