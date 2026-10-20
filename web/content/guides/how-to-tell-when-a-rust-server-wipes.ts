import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "how-to-tell-when-a-rust-server-wipes",
  date: "2026-10-26",
  title: "How to tell when a Rust server wipes",
  description: "Five ways to find out when a Rust server wipes next and when it last wiped, so you are there on the night rather than a day late.",
  body: `
Turning up a day after a wipe means turning up to a server where everybody else has a head start. Here is how to find out when a server wipes, before you need to know.

## 1. Read the server name

Most community servers put their schedule in the name, because it is the first thing anybody sees in the browser. Look for things like:

- **"Weekly"**, **"Biweekly"** or **"Monthly"**: how often the map resets.
- **A day**, like "Thu" or "Mon/Thu": which day it wipes.
- **"BP monthly"** or **"BP force"**: blueprints only reset on force wipe.

## 2. Check when it last wiped

The server browser shows how long ago a server wiped. If a weekly server wiped six days ago, it wipes tomorrow. If it wiped an hour ago, it is fresh right now, and worth joining.

## 3. Join the server's Discord

Almost every community server announces wipes in its Discord, often with an exact time and a countdown. This is the most reliable source, because it comes from the people who press the button. Turn notifications on for the announcements channel and mute the rest.

## 4. Remember force wipe

Every server wipes on force wipe, the first Thursday of every month, whatever its normal schedule. If you play anywhere, that date is always worth knowing. See [when Rust force wipe is](/guides/when-is-rust-force-wipe).

## 5. Watch for the restart

On the night itself, a server going offline is the wipe starting. It disappears from the browser or stops answering, then comes back with a fresh map and a burst of players. The first few minutes after it reappears are when it fills up.

> QueueUp can do this watching for you: schedule a join for wipe night and your PC waits for the server to go down and come back, then queues the moment it does.

## The short version

- The name usually tells you the schedule.
- The browser tells you when it last wiped.
- Discord tells you the exact time.
- Force wipe is always the first Thursday of the month.
`,
}

export default guide
