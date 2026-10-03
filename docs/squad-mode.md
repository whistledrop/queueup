# Squad mode

Design, agreed 3 October 2026. To build the week after the 5 November force
wipe, once there is one real wipe night of evidence behind it.

## What it is

A squad is a few QueueUp accounts who want to get into the same server on the
same night. One person makes it, the others join by link, and on wipe night one
tap takes everybody who has said yes.

Each member keeps their own account, their own subscription and their own PC.
That is not a limitation to be designed around, it is the engine: a four-player
squad is four subscriptions, and the reason a player nags their mates to
install QueueUp is not a discount, it is that the squad cannot go in together
without them.

## The one rule that makes it safe

**Nobody's PC is ever used without them agreeing to that specific join.**

Not a setting they turned on once and forgot. Not a permission the squad owner
holds. For every single coordinated join, each member taps Ready, and only
members who tapped Ready have a job created for them.

The consent expires with the join. There is no standing authority to revoke,
which means there is no standing authority to abuse, and nobody ever has to
wonder what their mates can do to their computer while they are out.

## Data model

Four new tables. Nothing existing changes shape.

```
squads
  id           TEXT PRIMARY KEY
  name         TEXT              -- "Trio Squad"
  owner_id     TEXT              -- accounts.id
  invite_code  TEXT UNIQUE       -- the join link
  created_at   INTEGER

squad_members
  squad_id     TEXT
  account_id   TEXT
  joined_at    INTEGER
  PRIMARY KEY (squad_id, account_id)

squad_joins                      -- one coordinated attempt
  id           TEXT PRIMARY KEY
  squad_id     TEXT
  server_id    TEXT
  server_name  TEXT
  server_addr  TEXT
  created_by   TEXT              -- accounts.id
  wait_for_wipe INTEGER          -- the existing wipe mode, for the whole squad
  fire_at      INTEGER           -- 0 means now
  state        TEXT              -- gathering | running | done | cancelled
  created_at   INTEGER
  started_at   INTEGER

squad_ready                      -- consent, per member, per join
  squad_join_id TEXT
  account_id    TEXT
  ready_at      INTEGER
  job_id        TEXT             -- filled when their job is created
  PRIMARY KEY (squad_join_id, account_id)
```

`squad_ready` is the consent record and the audit trail at once. If anybody
ever asks why their PC launched Rust, there is a row with a timestamp saying
they asked for it.

## The flow

1. Someone picks a server and a time, and creates a squad join. Wipe mode is
   set once, for the squad, using exactly the logic that already exists.
2. Every member's phone shows it: *Dave wants to take Trio Squad into Rustopia
   EU at 7pm.* With one button: **Ready**.
3. Members tap Ready. They can un-ready any time before it fires.
4. At fire time, a normal job is created for every member who is ready and
   whose PC is online.
5. Everyone watches one screen.

From there it is the product that already exists. A squad join is N ordinary
jobs that happen to start together.

## Partial success is success

The screen counts who got in. **"3 of 4 in."** Never "1 failed".

Per member, one of:

| State | What it means | Shown as |
|---|---|---|
| Not ready | Hasn't opted in | Quiet grey. Not a problem, they're just not playing |
| Ready | Opted in, waiting | Ready |
| PC offline | Readied, but their PC isn't connected | "Liam's PC is off" — so somebody can text him |
| Launching / queued / in | The ordinary job states | As the solo app shows them |
| Didn't make it | Their job failed | The reason, in the same plain words the solo app uses |

Three rules of tone:

- The headline counts who got **in**, not who didn't.
- Somebody who never readied up is not a failure. They are not playing tonight,
  which is an ordinary thing for a person to not be doing.
- Nobody's problem blocks anybody else. Four jobs, four fates.

The one case worth real care is the member whose PC was off. That is the only
state where somebody else can fix it — by sending a text — so it says whose PC
and nothing else.

## Why this needs no new referral machinery

The squad invite link **is** the referral link.

Somebody who taps it without an account gets: *Dave invited you to Trio Squad*
→ create account → the paywall with Dave's code already applied, so £1.99 →
link your PC → you're in the squad.

Which means Dave earns his £1.99 month under the rule that already exists:
his mate paid, and his mate linked a PC of their own. No second reward system,
no new fraud surface, and the thing that motivates the invite is not the money
at all.

## What v1 does not do

- No squad payment. Everybody pays their own, which is the point.
- No chat. They are already in a Discord.
- No roles beyond the owner being able to rename the squad and remove people.
- No squad history or stats. Later, if anybody asks.

## Anti-cheat

Nothing changes. Squad mode is a coordination layer on the relay that creates
ordinary jobs; every agent does exactly what it does today, with the same four
permitted actions and nothing else. No new contact with the game of any kind.

## Build order

1. **Squads, members, invite link.** Shippable alone, and it starts driving
   referrals the day it lands.
2. **The squad screen**, live, read-only. Who is in, whose PC is on.
3. **Squad join and ready-up**, firing now.
4. **Scheduled squad joins**, which is wipe night and the reason for all of it.

## Open, for Logan

- **Squad size cap.** Suggest 8. Rust teams run 2 to 8 and an uncapped squad is
  a load question nobody has asked for.
- **More than one squad per person?** Suggest yes for membership, but only one
  active squad join at a time, because one PC does one thing.
- **If the owner cancels a join after it has fired, does everybody's job
  cancel?** Suggest no. Starting together is a favour; stopping is personal.
  Once you are queueing, only you can stop your own.
