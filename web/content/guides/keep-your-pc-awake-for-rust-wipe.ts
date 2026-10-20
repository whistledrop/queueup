import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "keep-your-pc-awake-for-rust-wipe",
  date: "2026-11-01",
  title: "How to keep your PC awake for a Rust wipe",
  description: "A sleeping PC drops you out of the Rust queue and cannot rejoin. How to stop Windows sleeping, restarting for updates, or turning your screen off at the wrong moment.",
  body: `
Nothing ends a Rust queue faster than your PC going to sleep. The connection drops, your place is gone, and a sleeping PC cannot rejoin by itself. If you are leaving your PC to queue, or to run anything while you are away, it needs to stay awake.

## Turn off sleep

**Windows 11:** Settings, System, Power & battery, then Screen and sleep. Set "When plugged in, put my device to sleep after" to **Never**.

**Windows 10:** Settings, System, Power & sleep. Under Sleep, set "When plugged in, PC goes to sleep after" to **Never**.

Turning the **screen** off is fine. A blank screen does not stop the PC working; only sleep does.

## Stop Windows restarting for updates

Windows Update likes to restart at night, which is exactly when wipes happen.

- In Settings, open **Windows Update**, then **Advanced options**, and set your **active hours** to cover your evenings. Windows will not restart for updates during them.
- Before a big wipe, check Windows Update and install anything waiting **earlier in the day**, so it is not pending when you need the PC.

## Check the power plan

On some PCs, especially laptops and small form factor builds, the power plan can still put drives or the network adapter to sleep. If your PC seems awake but drops connections, open **Power Options** in the Control Panel, choose **Change plan settings** and then **Change advanced power settings**, and make sure nothing under hard disk, wireless adapter or USB is set to switch off while plugged in.

## Laptops

A laptop needs to be **plugged in**, and closing the lid usually sends it to sleep. Either leave the lid open or change what closing the lid does in Power Options.

## Why this matters for remote joining

Tools like QueueUp can start Rust on your PC from your phone, but they cannot wake a PC that is asleep. With sleep off, your PC is ready whenever you tap join. If Windows does restart, QueueUp starts again as soon as you are signed back in to Windows, and picks the join back up.
`,
}

export default guide
