# QueueUp

Join a Rust server queue on wipe day from your phone, while your PC does the
waiting at home.

**This is an unofficial third-party tool. It is not affiliated with, endorsed by,
or connected to Facepunch Studios. It does not modify the game, automate play, or
interfere with the game in any way. It launches the game the same way clicking a
link does, reads the game's own log file to see what is happening, and can close
the game the same way you would.**

---

## Where the project is right now

**Phases 1 and 2 are done.** The agent works, the relay works, and a PC can be
paired to an account and sent a join from anywhere. There is no website yet, so
for now the "web app" is a curl command.

| Phase | What it is | Status |
|---|---|---|
| 1 | Agent brain + fake Rust simulator, running locally | **done** |
| 2 | Relay server, agent holds an outbound connection, survives reboots | **done** |
| 3 | Web app: login, pairing, server search, join now, live status | not started |
| 4 | Scheduling, wipe-restart detection, notifications | not started |
| 5 | Hardening, then the real wipe-day test on your PC | not started |

## Try it right now

You need Go installed (`brew install go` on a Mac). Then, from this folder:

```
go run ./cmd/agent --sim --scenario testdata/scenarios/long_queue.json --server 51.83.128.10:28015 --speed 6 --confirm 2s
```

That plays out a full wipe-day join against a fake Rust client, with no game and
no Windows PC involved:

```
[   0.0s] launching              Launching Rust (attempt 1).
[   0.0s] connecting             Rust is starting and connecting.
[   0.6s] queued                 In queue, position 212
[   0.8s] queued                 In queue, position 148
[   1.2s] queued                 In queue, position 61
[   1.6s] queued                 In queue, position 12
[   1.8s] queued                 In queue, position 1
[   2.2s] in_server              You're in the server.
[   4.5s] done                   You're in. Slot is being held.
```

Every line there is a message that will end up on your phone in phase 3.

Run `./scripts/demo.sh` to watch all seven scenarios back to back, including the
nasty ones: a crash mid-queue, a banned account, Steam not signed in, and a
server that flaps up and down through a wipe restart.

## Try the whole system

`./scripts/phase2-demo.sh` runs the real thing on your machine: it starts a
relay, pairs a "PC" to an account, sends it a join the way the web app will,
streams the live status, then kills the PC mid-queue and shows the job carrying
on by itself when it comes back.

Everything it does is a curl command, all of them written out in
`docs/relay-api.md`. That file is the contract the website will be built against
in phase 3.

## The pieces

```
cmd/agent          the agent. Phase 1: a command-line tool. Later: a Windows tray app.
cmd/fakerust       the fake Rust client, runnable on its own

internal/job       the state machine: idle -> waiting -> launching -> connecting
                   -> queued -> in_server -> done, plus retrying and failed
internal/logparse  turns Rust log lines into events
internal/logtail   follows the log file as the game writes it
internal/game      the ONLY code that touches Rust (see the rule below)
internal/serverstat is the target server up, and how full is it
internal/runner    wires it all together and runs a job
internal/fakerust  the simulator's guts
internal/scenario  the scripted test situations

configs/patterns.json      the Rust log wording. Edit this, not the code.
testdata/scenarios/*.json  the seven test situations
docs/steam-uri-test.md     the manual test you need to run on your PC
```

## The anti-cheat rule

Rust runs Easy Anti-Cheat. QueueUp is allowed to do exactly four things to the
game and nothing else:

1. launch it through the Steam link,
2. read its log file,
3. check whether it is running,
4. ask it to close.

No reading or writing the game's memory. No injecting anything. No fake keyboard
or mouse input into the game. No touching game files. All four permitted actions
are things you could do by hand with Windows on its own.

This is enforced by keeping everything that touches the game inside
`internal/game`, which is about 200 lines and easy to audit. If a feature ever
seems to need more than the four above, that feature does not get built.

## Two things that make this testable without the game

**The fake Rust client.** `cmd/fakerust` behaves exactly like the real game from
the agent's point of view: it is a process you start, it writes lines into a
`Player.log`, it can be asked to close, and it can die unexpectedly. The agent
cannot tell the difference. All development and every automated test runs against
it, so nothing needs your PC, Steam, or a real wipe.

**The log wording lives in a config file.** Rust patches change the wording of
log lines, and when that happens the parser stops recognising them. Every pattern
lives in `configs/patterns.json`, with an example line next to it. Fixing a
patch-day break means editing that file and restarting the agent. No rebuilding,
no code changes, about five minutes.

Right now every pattern in that file is a **guess**, because I have not seen a
real `Player.log` yet. The agent prints a loud warning while that is true. See
`docs/steam-uri-test.md` for how to capture one.

## Tests

```
go test ./...          # everything
go test -race ./...    # everything, checking for concurrency bugs
```

Two to know about:

`TestScenarioLinesParseAsDeclared`. Every log line the fake client writes carries
a note saying what it is supposed to mean, and that test checks the real parser
agrees. When the real `Player.log` arrives and the patterns get rewritten, this
test is what proves the simulator was updated too.

`TestJobResumesAfterTheAgentRestarts` in `internal/e2e`. A real relay, a real
agent over a real WebSocket, a join that reaches the queue, and then the agent is
killed outright, the way a forced Windows update kills it. The test fails unless
the job survives and finishes on its own after the agent comes back.

## Setup on the gaming PC (things you do by hand, once)

QueueUp deliberately does not try to automate any of these. They are one-time
Windows and Steam settings, and a tool that changed them for you would be doing
something you did not ask for on a machine you cannot see.

1. **Windows signs in automatically after a restart.** Otherwise a forced update
   leaves the PC sitting on the lock screen with nothing running.
2. **Steam starts with Windows, and stays signed in.** Tick "remember my
   password" in Steam. QueueUp never sees or stores this.
3. **Sleep and hibernate are off.** The PC is expected to be awake all the time.
   QueueUp does not wake sleeping machines and is not designed to.
4. **Rust has been run at least once on that machine**, so its log folder exists.
5. **The agent starts at login.** Phase 2 runs it from a terminal; the installer
   in a later phase sets this up properly.

## What I need from you

1. **Run the manual Steam link test** in `docs/steam-uri-test.md`, on your gaming
   PC, before you lose access to it. Everything depends on it.
2. **A real `Player.log`** from a full session: launch, sit in a queue, join,
   leave. Instructions are in the same file.
3. **The BattleMetrics ID of your test server**, for phase 4.

## Things you will be asked and the answers

**Does this need my Steam password?** No. QueueUp never asks for it, never stores
it, and has no way to use it. Steam stays signed in on your own PC exactly as it
is now.

**Does anything connect *into* my PC?** No. The agent makes one outgoing
connection to the relay and keeps it open, the same way a chat app does. There is
no port forwarding and nothing to change on your router.

**Is this a cheat?** No. See the anti-cheat rule above.

## Tech choices, and why

**Agent: Go.** It compiles to a single `.exe` with nothing to install alongside it
— no .NET runtime, no Python, no installer wrestling on a machine you cannot see.
Just as important, Go cross-compiles: the Windows build is produced from your Mac
with one command, which matters a lot while your test PC is in another country.
It has good, boring support for Windows tray icons and background services when
phase 2 needs them.

**Relay: Go, deployed to Fly.io or Railway.** The relay's whole job is holding
thousands of idle WebSocket connections open cheaply, which is the thing Go is
best at. Sharing the language with the agent means one set of message definitions
and no translation layer between them.

**Web app (phase 3): Next.js.** Mobile-first, deploys to Vercel in one step,
handles web push, and it is what you already run for your other projects, so
there is nothing new for you to learn or pay for.

**Database: SQLite first, Postgres when it needs it.** A single file on the relay
is enough for accounts, devices and jobs, and it removes an entire moving part.
The SQLite driver is pure Go, so there is no C compiler involved and the relay
still cross-compiles anywhere with one command. Everything is standard SQL, so
switching later is a driver change rather than a rewrite.
