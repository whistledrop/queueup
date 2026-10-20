import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "why-rust-servers-restart-on-wipe-day",
  date: "2026-11-11",
  title: "Why Rust servers go offline and restart on wipe day",
  description: "On wipe day your Rust server vanishes from the browser, comes back, and sometimes vanishes again. What is happening during a wipe restart and how to time your join.",
  body: `
If you watch your server around wipe time, it does something odd: it disappears, maybe comes back, maybe disappears again, and then suddenly fills up. Here is what is going on.

## Step 1: the old map shuts down

At wipe time the server is stopped. Everybody on it is disconnected, and it stops answering in the server browser. If you were in the queue, you are out of it.

## Step 2: the update (on force wipe)

On force wipe, the server software has to update to Facepunch's new monthly version before it can start. Big hosts are quick; some community servers take longer.

## Step 3: the new map is generated or loaded

A fresh map has to be created or loaded before the server can accept players. Depending on the map size and the hardware, that can take a few minutes.

## Step 4: it comes back, sometimes twice

Admins sometimes restart a server again just after it comes up, to fix a setting or a plugin, especially on modded servers. That is the "flap" you see: up, down, up again.

## Step 5: the rush

Once it is properly back, everybody who was waiting connects at once, and a popular server can go from empty to full with a queue in minutes.

## Timing your join

- **Do not connect to the old map just before the wipe**: you will be kicked when it shuts down.
- **Watch for it coming back** and connect straight away. If it flaps, connect again.
- **If you join a few minutes late**, expect a queue. See [why the first minutes matter](/guides/why-the-first-minutes-after-a-wipe-matter).

QueueUp's scheduled joins are built around exactly this sequence: your PC waits for the server to go down, ignores the flaps, and joins the moment it is genuinely back.
`,
}

export default guide
