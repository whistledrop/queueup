# Where server search comes from, and what it costs

This is a decision you need to make, and it involves money. Nothing is blocked
while you decide: the app works today with a built-in example list.

## What changed

The brief assumed BattleMetrics has a public API. It does not any more.

As of 22 August 2026, every BattleMetrics API endpoint returns this without a
paid subscription, including a plain lookup of a single server:

```
403 Forbidden
{"errors":[{"detail":"Access denied. A subscription is required to use the API."}]}
```

So the free public API the brief was written around is gone.

## Your three options

QueueUp treats the source as a plug-in, chosen with one environment variable on
the relay. Switching between them is a config change and a restart. No code
changes, no rebuild.

### 1. Built-in list (`stub`), the current default

Six made-up servers. No account, no key, no internet needed.

- **Cost:** nothing
- **Good for:** developing, demos, and every automated test
- **Not good for:** actual use, because the servers are not real

The web app shows a notice saying the list is not real when this is on, so it
cannot be mistaken for the finished thing.

```bash
QUEUEUP_SERVER_SOURCE=stub
```

### 2. Steam's own server list (`steam`), my recommendation to start

Steam publishes the same server list the in-game browser uses. The key is free
and takes about a minute to get from https://steamcommunity.com/dev/apikey.

- **Cost:** nothing
- **Gives us:** server names, addresses, player counts, current map
- **Does not give us:** queue length. Steam does not report it.
- **Good for:** launching. Search works, joining works, and the queue position
  the player actually cares about comes from their own game log anyway.

```bash
QUEUEUP_SERVER_SOURCE=steam
QUEUEUP_STEAM_API_KEY=your-free-key
```

The missing queue length matters in one place: showing "312 people are queuing
for this server" in search results, before they commit. Phase 4 can fill that
gap by querying servers directly with the Valve query protocol, which needs no
key at all. That work is planned anyway for wipe-restart detection.

### 3. BattleMetrics (`battlemetrics`), the paid option

Still the best data if you pay for it. It is the only one of the three that
reports queue length directly, which is exactly the number a wipe-day tool wants,
and it also has wipe history.

- **Cost:** a paid subscription. Check their current pricing at
  https://www.battlemetrics.com/developers before committing.
- **Good for:** once there is revenue, or if the queue number turns out to be the
  thing that sells the product.

```bash
QUEUEUP_SERVER_SOURCE=battlemetrics
QUEUEUP_BATTLEMETRICS_TOKEN=your-token
```

## What I would do

Start on `steam`, because it is free and search is the only thing that needs it.
Add direct server queries in phase 4 for the live numbers. Revisit BattleMetrics
only if paying for it clearly earns its money.

## The part that does not change

Whichever source is used, a server's identity is its **id**, never its address.
Rust server addresses change between wipes. QueueUp stores the id and looks the
address up again in the moment before it hands the job to your PC, so a server
that moved is still joined correctly. There is a test for exactly this
(`TestAddressIsRefreshedWhenTheJobIsHandedOver`), and when it happens the phone
says "That server has changed address. Using the new one."
