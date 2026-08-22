# Relay API

Everything the web app will do in phase 3, as curl commands you can run today.
`./scripts/phase2-demo.sh` runs this whole sequence for you.

Two kinds of caller:

- **The agent**, on the player's PC. It authenticates with a **device token**,
  issued when the PC is paired. It only ever dials out.
- **The web app**, acting for a signed-in user. It authenticates with an
  **account token**. In phase 3 this becomes a proper email login; the routes
  below do not change.

Both send the token as `Authorization: Bearer <token>`.

## Setup

```bash
export QUEUEUP_DB=queueup.db
export QUEUEUP_ADMIN_TOKEN=pick-something-long
export QUEUEUP_ADDR=127.0.0.1:8080

go run ./cmd/relay create-account you@example.com   # prints the account token, once
go run ./cmd/relay serve
```

There are no secrets in the repo. Everything comes from environment variables.

## Pairing a PC

The PC has no credentials at this point, and that is deliberate. Ownership is
proved by the user reading a code off their own screen and typing it in where
they are already signed in.

**1. The agent asks for a code** (no authentication):

```bash
curl -X POST http://127.0.0.1:8080/pair/start -d '{"name":"Gaming PC"}'
# {"device_id":"dev_...","code":"HXVE9G","claim_token":"...","expires_at":"..."}
```

In practice you never run this by hand. The agent does it and shows the code:

```bash
go run ./cmd/agent pair --relay http://127.0.0.1:8080
```

**2. The user types the code into the web app** (account token):

```bash
curl -X POST http://127.0.0.1:8080/api/pair \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  -d '{"code":"HXVE9G"}'
```

**3. The agent collects its token.** It is polling `/pair/result` already, and
saves the token to its settings file. The token can be collected exactly once,
and the code expires after ten minutes.

## Devices

```bash
curl http://127.0.0.1:8080/api/devices -H "Authorization: Bearer $ACCOUNT_TOKEN"
```

```json
{"devices":[{"id":"dev_...","name":"Gaming PC","online":true,
             "agent_version":"0.2.0-phase2","os":"windows","simulator":false,
             "last_seen_at":"...","paired_at":"...","revoked":false}]}
```

Unlink a PC. The agent is disconnected immediately and its token stops working:

```bash
curl -X POST http://127.0.0.1:8080/api/devices/$DEVICE_ID/revoke \
  -H "Authorization: Bearer $ACCOUNT_TOKEN"
```

## Joins

Start one:

```bash
curl -X POST http://127.0.0.1:8080/api/jobs \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  -d '{"device_id":"'"$DEVICE_ID"'","server":"51.83.128.10:28015"}'
```

`wait_for_server_up: true` arms the "join as soon as the server comes back after
wipe" mode. In phase 2 the relay reports the server as up straight away; phase 4
replaces that stub with real polling.

If the PC is switched off, the job is still created. It sits in `pending` and
starts the moment the agent reconnects. Nothing is lost.

One PC runs one job at a time. A second returns `409` with a message explaining
why.

Read a job and its whole timeline:

```bash
curl http://127.0.0.1:8080/api/jobs/$JOB_ID -H "Authorization: Bearer $ACCOUNT_TOKEN"
```

Cancel it. The agent stops and closes the game:

```bash
curl -X POST http://127.0.0.1:8080/api/jobs/$JOB_ID/cancel \
  -H "Authorization: Bearer $ACCOUNT_TOKEN"
```

## Live status

This is what the phone's status screen listens to. Server-sent events, so the
browser reconnects on its own:

```bash
curl -N http://127.0.0.1:8080/api/jobs/$JOB_ID/events \
  -H "Authorization: Bearer $ACCOUNT_TOKEN"
```

```
id: 4
data: {"id":4,"state":"queued","position":212,"detail":"In queue, position 212","at":"..."}

id: 5
data: {"id":5,"state":"queued","position":148,"detail":"In queue, position 148","at":"..."}
```

The history is replayed first, then live updates follow. Pass `?since=<id>` to
skip what you already have, which is how a phone catches up after going through
a tunnel.

**Why not a WebSocket here?** Status only ever flows one way, browsers reconnect
server-sent events automatically, and it means only one long-lived protocol to
debug at three in the morning. Commands go over ordinary POSTs.

## Admin view

```bash
curl http://127.0.0.1:8080/admin/status -H "Authorization: Bearer $QUEUEUP_ADMIN_TOKEN"
```

Every PC, whether it is connected, what version it is running, its last few raw
Rust log lines, and the 25 most recent jobs. Those log lines are held in memory
only and are never written to the database.

## Job states

| State | What the user sees |
|---|---|
| `pending` | Waiting to be picked up by your PC |
| `waiting_for_server_up` | Waiting for the server to come back up |
| `launching` | Launching Rust |
| `connecting` | Rust is starting and connecting |
| `queued` | In queue, position N |
| `in_server` | You're in the server |
| `retrying` | Something went wrong, trying again in N seconds |
| `done` | Finished, either joined or cancelled |
| `failed` | Gave up, with a plain-language reason |
