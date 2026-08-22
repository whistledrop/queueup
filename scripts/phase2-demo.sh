#!/usr/bin/env bash
#
# Phase 2 end to end, on this machine, with nothing but curl.
#
# Starts a relay, pairs a "PC", sends it a join from the command line the way
# the web app will, watches the status stream, then yanks the PC's power
# mid-queue and shows the job resuming on its own.
#
#   ./scripts/phase2-demo.sh
#
set -uo pipefail
cd "$(dirname "$0")/.."

PORT=${PORT:-8099}
RELAY="http://127.0.0.1:$PORT"
WORK=$(mktemp -d)
export QUEUEUP_DB="$WORK/relay.db"
export QUEUEUP_ADMIN_TOKEN="demo-admin-token"
export QUEUEUP_ADDR="127.0.0.1:$PORT"

RELAY_PID=""
AGENT_PID=""
cleanup() {
  [ -n "$AGENT_PID" ] && kill "$AGENT_PID" 2>/dev/null
  [ -n "$RELAY_PID" ] && kill "$RELAY_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

say "Building"
go build -o "$WORK/relay" ./cmd/relay || exit 1
go build -o "$WORK/agent" ./cmd/agent || exit 1
echo "ok"

say "Starting the relay"
"$WORK/relay" serve >"$WORK/relay.log" 2>&1 &
RELAY_PID=$!
for _ in $(seq 1 50); do
  curl -sf --max-time 5 "$RELAY/healthz" >/dev/null && break
  sleep 0.1
done
curl -s --max-time 10 "$RELAY/healthz"; echo

say "Creating an account"
ACCOUNT_OUT=$("$WORK/relay" create-account you@example.com)
ACCOUNT_TOKEN=$(echo "$ACCOUNT_OUT" | awk '/token:/ {print $2}')
echo "account token: ${ACCOUNT_TOKEN:0:12}..."

say "Pairing this 'PC' with the account"
# The agent asks for a code and waits. In real life the user reads the code off
# their PC screen and types it into the web app.
"$WORK/agent" pair --relay "$RELAY" --config "$WORK/agent.json" --name "Demo PC" \
  >"$WORK/pair.log" 2>&1 &
PAIR_PID=$!
CODE=""
for _ in $(seq 1 50); do
  CODE=$(grep -Eo '^ +[A-Z0-9]( [A-Z0-9]){5}$' "$WORK/pair.log" 2>/dev/null | tr -d ' ' | head -1)
  [ -n "$CODE" ] && break
  sleep 0.1
done
echo "the PC is showing the code: $CODE"
echo "typing it into the web app (this is the curl the web app will make):"
echo "  curl -X POST $RELAY/api/pair -d '{\"code\":\"$CODE\"}'"
curl -s --max-time 10 -X POST "$RELAY/api/pair" \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$CODE\"}" | head -c 400; echo
wait $PAIR_PID
tail -3 "$WORK/pair.log"

DEVICE_ID=$(python3 -c "import json;print(json.load(open('$WORK/agent.json'))['device_id'])")

say "Starting the agent (simulator mode, long queue scenario)"
"$WORK/agent" run --relay "$RELAY" --config "$WORK/agent.json" \
  --sim --scenario testdata/scenarios/long_queue.json --speed 4 --confirm 3s \
  >"$WORK/agent.log" 2>&1 &
AGENT_PID=$!
sleep 1.5
curl -s --max-time 10 "$RELAY/api/devices" -H "Authorization: Bearer $ACCOUNT_TOKEN" | head -c 400; echo

say "Sending a join from the 'web app'"
JOB=$(curl -s --max-time 10 -X POST "$RELAY/api/jobs" \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"device_id\":\"$DEVICE_ID\",\"server\":\"51.83.128.10:28015\"}")
JOB_ID=$(echo "$JOB" | python3 -c "import json,sys;print(json.load(sys.stdin)['id'])")
echo "job: $JOB_ID"

say "Live status (this is exactly what the phone will listen to)"
curl -sN --max-time 12 "$RELAY/api/jobs/$JOB_ID/events" \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  | grep --line-buffered '^data:' \
  | python3 -u -c "
import sys, json
for line in sys.stdin:
    try:
        e = json.loads(line[5:])
    except Exception:
        continue
    pos = f\"  (position {e['position']})\" if e.get('position') else ''
    print(f\"  {e['state']:<22} {e['detail']}{pos}\")
    if e['state'] in ('done', 'failed'):
        break
"

say "Now the resilience bit: the PC reboots mid-job"
# Stop the first agent first. One PC means one agent: two of them sharing a
# token would just fight over the same job.
kill "$AGENT_PID" 2>/dev/null; AGENT_PID=""
sleep 1
"$WORK/agent" run --relay "$RELAY" --config "$WORK/agent.json" \
  --sim --scenario testdata/scenarios/long_queue.json --speed 4 --confirm 3s \
  >"$WORK/agent2.log" 2>&1 &
AGENT_PID=$!
sleep 1.5
JOB2=$(curl -s --max-time 10 -X POST "$RELAY/api/jobs" \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"device_id\":\"$DEVICE_ID\",\"server\":\"51.83.128.10:28015\"}")
JOB2_ID=$(echo "$JOB2" | python3 -c "import json,sys;print(json.load(sys.stdin)['id'])")
echo "job: $JOB2_ID"
echo "waiting until it is sitting in the queue..."
for _ in $(seq 1 60); do
  STATE=$(curl -s --max-time 10 "$RELAY/api/jobs/$JOB2_ID" -H "Authorization: Bearer $ACCOUNT_TOKEN" \
    | python3 -c "import json,sys;print(json.load(sys.stdin)['job']['state'])")
  [ "$STATE" = "queued" ] && break
  sleep 0.25
done
echo "state is now: $STATE"
echo "killing the agent, the way a forced Windows update would"
kill -9 "$AGENT_PID" 2>/dev/null; AGENT_PID=""
sleep 2

echo "the relay still has the job:"
curl -s --max-time 10 "$RELAY/api/jobs/$JOB2_ID" -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  | python3 -c "import json,sys;j=json.load(sys.stdin)['job'];print(f\"  state={j['state']} device_online={j['device_online']}\")"

echo "Windows comes back and the agent auto-starts..."
"$WORK/agent" run --relay "$RELAY" --config "$WORK/agent.json" \
  --sim --scenario testdata/scenarios/long_queue.json --speed 4 --confirm 3s \
  >"$WORK/agent3.log" 2>&1 &
AGENT_PID=$!

for _ in $(seq 1 80); do
  STATE=$(curl -s --max-time 10 "$RELAY/api/jobs/$JOB2_ID" -H "Authorization: Bearer $ACCOUNT_TOKEN" \
    | python3 -c "import json,sys;print(json.load(sys.stdin)['job']['state'])")
  if [ "$STATE" = "done" ] || [ "$STATE" = "failed" ]; then break; fi
  sleep 0.25
done

say "The full timeline of the job that survived the reboot"
curl -s --max-time 10 "$RELAY/api/jobs/$JOB2_ID" -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  | python3 -c "
import json,sys
d = json.load(sys.stdin)
for e in d['events']:
    pos = f\"  (position {e['position']})\" if e.get('position') else ''
    print(f\"  {e['state']:<22} {e['detail']}{pos}\")
print()
print('  final state:', d['job']['state'])
"

say "Admin view"
curl -s --max-time 10 "$RELAY/admin/status" -H "Authorization: Bearer $QUEUEUP_ADMIN_TOKEN" \
  | python3 -c "
import json,sys
d = json.load(sys.stdin)
print('  connected agents:', d['connected_agents'])
for dev in d['devices']:
    print(f\"  PC {dev['name']}: online={dev['online']} version={dev['agent_version']} sim={dev['simulator']}\")
print('  recent jobs:', len(d['recent_jobs']))
"
echo
