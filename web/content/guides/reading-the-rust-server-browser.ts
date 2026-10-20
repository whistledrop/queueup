import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "reading-the-rust-server-browser",
  date: "2026-11-12",
  title: "How to read the Rust server browser",
  description: "Player counts, wipe times, ping and server names in the Rust browser, explained, so you can pick a good server in a minute instead of ten.",
  body: `
The Rust server browser packs a lot into one line per server. Here is what to look at, and what it tells you.

## The name

Community server names are mostly information:

- **Region**, like EU, US or AU.
- **Wipe schedule**, like Weekly, Biweekly or Monthly, often with a day.
- **Rates**, like 2x or 5x on modded servers.
- **Group limit**, like Solo, Duo, Trio or Quad, or "No limit".

## Players

The player count shows how many are on now against the maximum. A server near its maximum on wipe night will have a queue.

An empty server is not always a bad sign: it might be just before its wipe. A busy server long after its wipe usually means a healthy community.

## When it last wiped

The browser shows how long ago each server wiped. For a fresh start, look for servers that wiped in the last few hours; for a weekly server, a wipe six days ago means a new one is about due. See [how to tell when a server wipes](/guides/how-to-tell-when-a-rust-server-wipes).

## Ping

Ping is the delay between you and the server. Lower is better. Under about 60 feels sharp, up to about 120 is playable, and much higher starts to make fights feel unfair.

## Official, community and modded tabs

The tabs separate Facepunch's own servers from community and modded ones. See [official vs community vs modded servers](/guides/official-vs-community-vs-modded-rust-servers).

## Save the good ones

Once you find a server you like, favourite it so you are not doing this again next wipe. See [how to save your favourite servers](/guides/save-favourite-rust-servers).
`,
}

export default guide
