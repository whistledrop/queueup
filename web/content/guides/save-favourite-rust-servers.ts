import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "save-favourite-rust-servers",
  date: "2026-11-09",
  title: "How to save your favourite Rust servers",
  description: "Stop searching for the same Rust server every wipe. How to favourite servers in the Rust browser, keep their addresses handy and join them in one tap.",
  body: `
Searching the server browser for the same server every wipe night is slow, and similar names make it easy to click the wrong one. Save the servers you play instead.

## Favourite it in game

In Rust's server browser, servers you mark as **favourites** appear in their own **Favorites** tab, so you can find them without searching. While you are connected to a server, you can also add it to your favourites from the browser so it is there next time.

Favourites also make it obvious when a server is offline for a wipe: it stops answering in your list, then comes back.

## Keep the address

Favourites live in the game on one PC. For a copy you can use anywhere, keep each server's address (it looks like 123.45.67.89:28015). With it you can connect from the F1 console or a one-click Steam link. See [how to connect by IP](/guides/connect-to-a-rust-server-by-ip).

## Know which address you actually want

Some communities run several servers with almost identical names: a main, a long-wipe, a 2x. Check the address in the server's Discord matches the one you have saved, especially after the community moves hosts.

## Save them in QueueUp too

QueueUp keeps a list of your saved servers on your phone, so on wipe night you open the app and tap the one you want instead of searching. If a community server moves to a new address between wipes, QueueUp follows it by name, so your saved server keeps working.
`,
}

export default guide
