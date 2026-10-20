import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "connect-to-a-rust-server-by-ip",
  date: "2026-10-30",
  title: "How to connect to a Rust server by IP address",
  description: "Joining a Rust server by IP is quicker than searching the browser. How to do it from the F1 console, and how to make a link that does it in one click.",
  body: `
If you know a server's address, you do not need the server browser at all. You can connect directly, which is quicker, and works even when the browser is slow to load on wipe night.

## Find the address

A Rust server address looks like **123.45.67.89:28015**: an IP, a colon, and a port. Most community servers post theirs in Discord or on their website, often as a "connect" line you can copy.

## Connect with the F1 console

1. Start Rust and wait for the main menu.
2. Press **F1** to open the console.
3. Type **client.connect** followed by the address, for example: client.connect 123.45.67.89:28015
4. Press **Enter**.

Rust connects straight to that server, or puts you in its queue if it is full.

> Paste the address exactly as given, including the port after the colon. Without the right port you will connect to nothing.

## Connect with one click

Steam can launch Rust straight into a server from a link. The link is:

**steam://run/252490//+connect 123.45.67.89:28015/**

with the server's address in place of the example. 252490 is Rust's ID on Steam. Opened from a browser or a desktop shortcut, it starts Rust and connects. Close Rust first if it is already running: the link is not reliable on a game that is already open, which is why QueueUp closes Rust before it joins a server for you.

## Why use the IP?

- **Speed.** No waiting for the browser to populate.
- **Accuracy.** Similar server names are easy to confuse; an address is not.
- **Wipe night.** When thousands of people are refreshing the browser at once, a direct connect skips all of it.

## If it does not connect

- Check the port is included and correct.
- Make sure the server is actually up: on wipe night it may still be restarting.
- If you see a full server, you are in its queue. Stay put; leaving sends you to the back.
`,
}

export default guide
