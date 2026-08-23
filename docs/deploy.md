# Putting QueueUp on the internet

Two deployments: the relay (a Go program that must be reachable by the agent
and the web app) and the website (Next.js, goes to Vercel like your other
projects). Total cost at this scale: roughly the price of one Fly.io shared VM,
a few dollars a month.

## The relay, on Fly.io

One-time setup, from the project folder:

```bash
fly launch --no-deploy --name queueup-relay --region lhr
fly volumes create queueup_data --size 1 --region lhr
```

Edit the generated `fly.toml` so it has:

```toml
[mounts]
  source = "queueup_data"
  destination = "/data"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = false   # agents hold connections open; never stop
  min_machines_running = 1
```

`auto_stop_machines = false` matters: the whole design rests on agents keeping
one connection open, so the relay must never be put to sleep.

Secrets (never in files, never in the repo):

```bash
go run ./cmd/relay gen-vapid    # run locally, once, keep the output
fly secrets set \
  QUEUEUP_ADMIN_TOKEN=pick-something-long \
  QUEUEUP_SERVER_SOURCE=steam \
  QUEUEUP_STEAM_API_KEY=your-free-key \
  QUEUEUP_VAPID_PUBLIC=... \
  QUEUEUP_VAPID_PRIVATE=... \
  QUEUEUP_PUSH_SUBJECT=mailto:you@yourdomain.com
```

Then, and after every change:

```bash
fly deploy
```

Check it: `https://queueup-relay.fly.dev/healthz` should say ok.

Accounts are created from the machine itself:

```bash
fly ssh console -C "relay create-account you@example.com"
```

## The website, on Vercel

```bash
cd web
vercel --prod
```

Set one environment variable in the Vercel project settings:

```
RELAY_URL=https://queueup-relay.fly.dev
```

That is the only place the website learns where the relay is.

## The agent, on the PC

Build it from any machine (`./scripts/build-agent.sh`), copy
`dist/QueueUpAgent.exe` anywhere permanent on the PC (not Downloads), then in a
terminal on the PC:

```
QueueUpAgent.exe pair --relay https://queueup-relay.fly.dev
QueueUpAgent.exe install-autostart
```

Autostart uses the per-user registry Run key: no administrator rights, runs in
the user's session (which Steam and the game need anyway), removed with
`uninstall-autostart`. From the next sign-in the agent runs as a tray icon.
