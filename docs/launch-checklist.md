# Launch checklist

Everything that remains, in order. The build is finished; every item here needs
either your accounts or your hands on the PC, which is why it is yours.

## 1. Deploy the relay (~15 minutes, from this Mac)

Install the Fly.io CLI and sign in (free account, card needed for the ~$3/mo VM):

```bash
brew install flyctl
fly auth signup        # or: fly auth login
```

Then follow `docs/deploy.md` top to bottom. It is copy-paste: create the app
and volume, set the secrets (including the notification keys from
`go run ./cmd/relay gen-vapid` and your free Steam key from
https://steamcommunity.com/dev/apikey), then `fly deploy`.

Check: `https://<your-app>.fly.dev/healthz` says ok.

## 2. Deploy the website (~5 minutes)

```bash
npm i -g netlify-cli
netlify login
netlify init
netlify env:set RELAY_URL https://<your-app>.fly.dev
netlify deploy --prod
```

Check: the site loads on your phone and you can create an account. (Vercel
works too; see docs/deploy.md.)

## 3. Get the agent onto the PC (~10 minutes, remote is fine)

On this Mac: `./scripts/build-agent.sh`, then get `dist/QueueUpAgent.exe` to
the PC however you normally move files. Put it somewhere permanent.

In a terminal on the PC:

```
QueueUpAgent.exe pair --relay https://<your-app>.fly.dev --web https://<your-site>.vercel.app
QueueUpAgent.exe install-autostart
```

Type the code into the website. Done: from the next sign-in the agent runs as
a tray icon.

## 4. One-time Windows settings on the PC (~10 minutes)

The four prerequisites in TESTING.md: auto sign-in (netplwiz), Steam starts
with Windows and stays signed in, sleep off, and Rust run at least once.

## 5. The dress rehearsal (~20 minutes, from anywhere)

TESTING.md Part A: the eight checks, ending with a real mid-queue PC restart.
All of it uses the built-in pretend Rust; the game is not involved.

## 6. First contact with the real game (~30 minutes, at the PC)

TESTING.md Part B, and this is the important one:

- the five steam:// launch links (docs/steam-uri-test.md), Rust closed and open
- capture a real Player.log and send it to me, so the 13 guessed log patterns
  become facts
- one real join of a quiet server from your phone

## 7. Wipe day

TESTING.md Part C. Schedule it the night before, watch your phone.

---

Item 6 is the only place a surprise can still be hiding, and both possible
surprises are one-file fixes on my side: the launch link lives in a single
function, and the log wording lives in patterns.json, which can be updated on
the PC by dropping the new file next to the agent's settings, no rebuild.
