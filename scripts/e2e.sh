#!/usr/bin/env bash
#
# The full end-to-end check, scripted: relay + agent (simulator) + a headless
# "phone" made of curl, through a SCHEDULED join with a mid-job reboot.
# Exits 0 only if the job survives the reboot and finishes.
#
#   ./scripts/e2e.sh
#
set -uo pipefail
cd "$(dirname "$0")/.."

PORT=${PORT:-8098}
RELAY="http://127.0.0.1:$PORT"
WORK=$(mktemp -d)
export QUEUEUP_DB="$WORK/relay.db"
export QUEUEUP_ADDR="127.0.0.1:$PORT"

AGENT_PID=""; RELAY_PID=""
cleanup() {
  [ -n "$AGENT_PID" ] && kill "$AGENT_PID" 2>/dev/null
  [ -n "$RELAY_PID" ] && kill "$RELAY_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT
say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
fail() { printf '\n\033[31mFAILED: %s\033[0m\n' "$1"; exit 1; }

say "Building"
go build -o "$WORK/relay" ./cmd/relay || fail "relay build"
go build -o "$WORK/agent" ./cmd/agent || fail "agent build"

say "Relay up"
"$WORK/relay" serve >"$WORK/relay.log" 2>&1 & RELAY_PID=$!
for _ in $(seq 1 50); do curl -sf --max-time 2 "$RELAY/healthz" >/dev/null && break; sleep 0.1; done
curl -sf --max-time 2 "$RELAY/healthz" >/dev/null || fail "relay did not come up"

TOKEN=$("$WORK/relay" create-account e2e@example.com | awk '/token:/ {print $2}')
[ -n "$TOKEN" ] || fail "no account token"
auth() { curl -s --max-time 10 -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' "$@"; }

say "Pairing"
"$WORK/agent" pair --relay "$RELAY" --config "$WORK/agent.json" --name "E2E PC" >"$WORK/pair.log" 2>&1 &
PAIR_PID=$!
CODE=""
for _ in $(seq 1 50); do
  CODE=$(grep -Eo '^ +[A-Z0-9]( [A-Z0-9]){5}$' "$WORK/pair.log" 2>/dev/null | tr -d ' ' | head -1)
  [ -n "$CODE" ] && break; sleep 0.1
done
[ -n "$CODE" ] || fail "no pairing code appeared"
auth -X POST "$RELAY/api/pair" -d "{\"code\":\"$CODE\"}" >/dev/null
wait $PAIR_PID || fail "pairing did not complete"
DEVICE_ID=$(python3 -c "import json;print(json.load(open('$WORK/agent.json'))['device_id'])")

start_agent() {
  "$WORK/agent" run --relay "$RELAY" --config "$WORK/agent.json" \
    --sim --scenario testdata/scenarios/long_queue_slow.json --speed 3 --confirm 3s \
    >"$WORK/agent-$1.log" 2>&1 & AGENT_PID=$!
}
start_agent 1
sleep 2

say "Scheduling a join a few seconds out (stored as UTC)"
FIRE=$(python3 -c "from datetime import datetime,timedelta,timezone;print((datetime.now(timezone.utc)+timedelta(seconds=4)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
SCHED=$(auth -X POST "$RELAY/api/schedules" \
  -d "{\"device_id\":\"$DEVICE_ID\",\"server\":\"51.83.128.10:28015\",\"server_name\":\"E2E Server\",\"fire_at\":\"$FIRE\"}")
echo "$SCHED" | grep -q '"pending"' || fail "schedule not created: $SCHED"

say "Waiting for it to fire and reach the queue"
JOB_ID=""
for _ in $(seq 1 120); do
  JOB_ID=$(auth "$RELAY/api/jobs?limit=1" | python3 -c "
import json,sys
jobs=json.load(sys.stdin).get('jobs',[])
print(jobs[0]['id'] if jobs and jobs[0]['state']=='queued' else '')")
  [ -n "$JOB_ID" ] && break; sleep 0.5
done
[ -n "$JOB_ID" ] || fail "the scheduled join never reached the queue"
echo "job $JOB_ID is in the queue"

say "Killing the PC mid-queue (the forced-update reboot)"
kill -9 "$AGENT_PID"; AGENT_PID=""
sleep 2
STATE=$(auth "$RELAY/api/jobs/$JOB_ID" | python3 -c "import json,sys;print(json.load(sys.stdin)['job']['state'])")
[ "$STATE" = "queued" ] || fail "the job was lost in the reboot (state=$STATE)"
echo "relay still holds the job: $STATE"

say "PC comes back"
start_agent 2

say "Waiting for the join to finish"
FINAL=""
for _ in $(seq 1 120); do
  FINAL=$(auth "$RELAY/api/jobs/$JOB_ID" | python3 -c "import json,sys;print(json.load(sys.stdin)['job']['state'])")
  if [ "$FINAL" = "done" ] || [ "$FINAL" = "failed" ]; then break; fi
  sleep 0.5
done

auth "$RELAY/api/jobs/$JOB_ID" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for e in d['events']:
    print('  ', e['state'].ljust(22), e['detail'])
"
[ "$FINAL" = "done" ] || fail "final state was $FINAL"
say "PASS: scheduled join survived a mid-job reboot and finished"
