import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-connection-errors-explained",
  date: "2026-11-13",
  title: "Common Rust connection errors and what to do",
  description: "Steam auth timeouts, anti-cheat errors, full servers and connections that just hang. What the common Rust connection errors usually mean and the first thing to try.",
  body: `
Connection errors are most common exactly when you least want them: wipe night, when everybody is connecting at once. Here are the usual ones and the first thing to try.

## Steam auth timeout

**Usually means:** the server could not confirm your Steam session in time.

**Try:** restarting Steam completely (exit from the system tray, then open it again), then connecting again. If Steam's own servers are having a bad moment, waiting a few minutes helps.

## Easy Anti-Cheat errors

Errors mentioning **EAC** usually mean the anti-cheat service did not start or could not talk to the server.

**Try:** closing Rust and starting it again from Steam, then restarting your PC if it persists. After a monthly update, the anti-cheat sometimes needs a restart to finish updating. Verifying the game files also helps (right-click Rust in your Library, Properties, Installed Files, Verify integrity of game files).

## Server full

Usually not an error at all: you are in the queue. Stay put. See [Rust server full? What it means](/guides/rust-server-full-what-it-means).

## Connection attempt failed, or it hangs on connecting

**Usually means:** the server is not answering, often because it is still restarting after a wipe.

**Try:** waiting a minute and connecting again. Check the address and port if you typed them yourself.

## Disconnected from the queue

Your place is lost if your connection drops, your PC sleeps or Rust closes. See [keeping your PC awake](/guides/keep-your-pc-awake-for-rust-wipe).

## Steam not signed in

Rust needs Steam running and signed in. If Steam is waiting at its login screen, nothing will connect.

## When you are not there to click

Several of these fix themselves with a retry. QueueUp's joins retry on their own when Rust crashes or a connection drops, and tell your phone in plain words when something needs you, such as Steam being signed out.
`,
}

export default guide
