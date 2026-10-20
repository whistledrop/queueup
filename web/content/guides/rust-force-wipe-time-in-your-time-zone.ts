import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-force-wipe-time-in-your-time-zone",
  date: "2026-10-24",
  title: "Rust force wipe time in your time zone",
  description: "Force wipe usually lands around 7pm UK time. Here is roughly what that means in Europe, the US and Australia, and why the exact minute varies.",
  body: `
Rust's monthly update, and so force wipe, usually arrives in the **evening UK time, around 7pm**. It is set by Facepunch, who are based in the UK, which is why the time is easiest to give in UK terms.

Here is roughly what that is elsewhere. Treat these as a guide rather than a timetable: the update can land a little earlier or later, and servers come back in waves after it.

## Approximate local times

- **UK and Ireland:** around 7pm
- **Central Europe (France, Germany, Spain, Poland):** around 8pm
- **Eastern Europe (Finland, Romania, Greece):** around 9pm
- **US Eastern:** around 2pm
- **US Central:** around 1pm
- **US Pacific:** around 11am
- **Eastern Australia:** early Friday morning, around 4 to 6am

## Why it is "around"

Three things move it:

1. **Clocks change on different dates.** The UK, Europe and the US switch between summer and winter time on different weekends, so for a week or two each spring and autumn the gap between them is an hour different.
2. **The update itself.** Facepunch release when it is ready. Most months it lands at the usual time; occasionally it does not.
3. **Your server.** After the update, each server has to restart and start its new map. Big official servers are usually quick; community servers may open at a time they have announced in advance.

> The most reliable time for your server is the one in its Discord. Use the times above to plan your evening, and the Discord to plan the minute.

## When to actually be ready

Be ready **before** the update lands rather than after. Have Steam open and signed in, and make sure Rust has already downloaded the update if Steam offers it early, so you are not stuck waiting for a download while everybody else is loading in.

If you will not be at your PC at that time, which is likely if it is the middle of your working day, QueueUp lets you join from your phone instead, and can be scheduled to queue the moment your server comes back.

## The short version

- Force wipe usually lands around 7pm UK time on the first Thursday of the month.
- That is around 2pm US Eastern and around 8pm in Central Europe.
- Check your server's Discord for its exact opening time.
`,
}

export default guide
