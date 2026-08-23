# Putting QueueUp on the internet

Two deployments: the relay (a Go program that must be reachable by the agent
and the web app) and the website (Next.js, on Netlify; Vercel works identically
if you ever prefer it). Total cost at this scale: roughly the price of one
Fly.io shared VM, a few dollars a month; the website tier is free.

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

## The website, on Netlify

One-time, from the project root:

```bash
npm i -g netlify-cli
netlify login
netlify init          # create a new site; the repo's netlify.toml points it at web/
```

Set the one environment variable (the only place the website learns where the
relay is):

```bash
netlify env:set RELAY_URL https://queueup-relay.fly.dev
```

Then, and after every change:

```bash
netlify deploy --prod
```

Check: the site loads on your phone, and creating an account works, which
proves the site can reach the relay.

Notes:

- Netlify's Next.js runtime is detected automatically; nothing to configure
  beyond `netlify.toml`, which is in the repo.
- The live status screen streams over a connection that hosting platforms cap
  after a while. The page is built for that: the browser reconnects by itself
  and filters duplicates, so at worst you get an invisible reconnect. The same
  is true on Vercel.
- Prefer Vercel instead? `cd web && vercel --prod`, set the same RELAY_URL in
  the project settings. Everything else is identical.

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
