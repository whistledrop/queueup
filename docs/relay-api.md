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

## Signing in

The web app signs in with an email and a password and gets back a session token.
It keeps that token in an http-only cookie on its own server, so the browser
never holds a credential that could command somebody's PC.

```bash
curl -X POST http://127.0.0.1:8080/api/auth/register \
  -d '{"email":"you@example.com","password":"at least eight characters"}'
# {"session_token":"...","account":{"id":"acct_...","email":"you@example.com"}}

curl -X POST http://127.0.0.1:8080/api/auth/login \
  -d '{"email":"you@example.com","password":"at least eight characters"}'

curl http://127.0.0.1:8080/api/auth/me -H "Authorization: Bearer $SESSION"
curl -X POST http://127.0.0.1:8080/api/auth/logout -H "Authorization: Bearer $SESSION"
```

A session token and the long-lived account token below are interchangeable on
every route. Sessions are what the web app uses; the account token stays for
scripts and for this document.

Sign-in refuses a wrong password and an unknown email address with the same
message on purpose, so the page cannot be used to find out who has an account.

## Finding servers

```bash
curl "http://127.0.0.1:8080/api/servers/search?q=rust&limit=25" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{"source":"stub",
 "servers":[{"id":"stub-1","name":"Rustopia EU Main","address":"51.83.128.10:28015",
             "online":true,"players":198,"max_players":200,"queue":312,
             "region":"EU","favourite":false}]}
```

`source` tells you which backend answered. See `docs/server-search.md`: the
default is a built-in example list, because BattleMetrics now needs a paid
subscription.

Star a server so it appears on the dashboard:

```bash
curl -X POST http://127.0.0.1:8080/api/favourites -H "Authorization: Bearer $TOKEN" \
  -d '{"server_id":"stub-1","name":"Rustopia EU Main","address":"51.83.128.10:28015"}'

curl http://127.0.0.1:8080/api/favourites -H "Authorization: Bearer $TOKEN"
curl -X DELETE http://127.0.0.1:8080/api/favourites/stub-1 -H "Authorization: Bearer $TOKEN"
```

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

Start one. The normal way is by server id, from the search results, and the
relay looks the address up:

```bash
curl -X POST http://127.0.0.1:8080/api/jobs \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  -d '{"device_id":"'"$DEVICE_ID"'","server_id":"stub-1"}'
```

A raw address still works, for testing or when you already know it:

```bash
curl -X POST http://127.0.0.1:8080/api/jobs \
  -H "Authorization: Bearer $ACCOUNT_TOKEN" \
  -d '{"device_id":"'"$DEVICE_ID"'","server":"51.83.128.10:28015"}'
```

When a job carries a server id, the address is looked up again in the moment
before the job is handed to the PC, because Rust server addresses change between
wipes. If it moved, the phone is told so.

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

## Scheduled joins

Schedules live on the relay and fire on the relay's clock. The phone can be off,
the PC can be mid-reboot: the join still starts, and whatever cannot happen is
notified instead of silently dropped.

Times travel as RFC3339 and are stored as UTC. The browser converts from the
user's local time; a phone in Spain and a PC in the UK agree by construction.

```bash
curl -X POST http://127.0.0.1:8080/api/schedules \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"device_id":"'"$DEVICE_ID"'","server_id":"stub-1",
       "fire_at":"2026-09-03T18:00:00Z","wait_for_server_up":true}'

curl http://127.0.0.1:8080/api/schedules -H "Authorization: Bearer $TOKEN"
curl -X POST http://127.0.0.1:8080/api/schedules/$SCHED_ID/cancel -H "Authorization: Bearer $TOKEN"
```

`wait_for_server_up: true` is wipe mode: from the scheduled moment the relay
polls the server directly (Valve's query protocol, one UDP packet, no API or
key involved) and the instant it answers again after the restart, the agent is
told to connect. Down/up flapping during the restart is tolerated; the agent's
own jitter and rate cap decide when to actually launch.

## Notifications

Web push, with email as fallback when SMTP is configured. Run `relay gen-vapid`
once and set the two keys, or notifications quietly stay off and everything
still lands in the job timeline.

```bash
curl http://127.0.0.1:8080/api/push/config -H "Authorization: Bearer $TOKEN"
# {"enabled":true,"public_key":"...","subscriptions":1}

# The body is the browser's PushSubscription.toJSON(), verbatim:
curl -X POST http://127.0.0.1:8080/api/push/subscribe -H "Authorization: Bearer $TOKEN" \
  -d '{"endpoint":"https://...","keys":{"p256dh":"...","auth":"..."}}'

curl -X POST http://127.0.0.1:8080/api/push/test -H "Authorization: Bearer $TOKEN"
```

What gets sent, and when:

| Moment | Notification |
|---|---|
| Entered the queue | "In the queue", with the position |
| Position crosses 100, 50, 10 | "Position N" (only the crossings; every change would be spam) |
| In the server | "You're in" |
| Join failed | The plain-language reason |
| PC went offline mid-job | "Your PC went offline" |
| PC came back mid-job | "Your PC is back online" |
| Scheduled join started | "Scheduled join started" |
| Schedule fired but the PC is off | "Your PC is offline", immediately, while there is still time to fix it |

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
