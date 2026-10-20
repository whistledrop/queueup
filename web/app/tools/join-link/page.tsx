import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { releasedTool } from '@/lib/tools'
import { GuideCta, GuideFooter, GuideNav } from '../../guides/chrome'
import g from '../../guides/guides.module.css'
import Maker from './maker'

export const revalidate = 3600

const SLUG = 'join-link'

export function generateMetadata(): Metadata {
  const t = releasedTool(SLUG)
  if (!t) return {}
  return {
    title: 'Rust server join link maker: one-click Steam connect links',
    description: t.description,
    alternates: { canonical: `/tools/${SLUG}` },
  }
}

export default function JoinLink() {
  if (!releasedTool(SLUG)) notFound()
  return (
    <div className={g.page}>
      <GuideNav />
      <p className={g.kicker}><Link href="/tools" style={{ color: 'inherit', textDecoration: 'none' }}>Free tools</Link></p>
      <h1 className={g.title}>Rust server join link maker</h1>
      <p className={g.lede}>
        Paste a Rust server&apos;s address and get a link that starts Rust and joins it in one click,
        a version for a desktop shortcut, and the F1 console command.
      </p>

      <Maker />

      <div className={g.body}>
        <h2 id="how">How the link works</h2>
        <p>
          The link is <strong>steam://run/252490//+connect</strong> followed by the server&apos;s
          address. 252490 is Rust&apos;s ID on Steam, and everything after the double slash is passed to
          the game, telling it which server to connect to. It is the same link QueueUp uses to start
          Rust into a server for you.
        </p>
        <h2 id="shortcut">Making a desktop shortcut</h2>
        <ol>
          <li>Right-click an empty part of your desktop and choose New, then Shortcut.</li>
          <li>Paste the shortcut version of the link as the location.</li>
          <li>Name it after the server and click Finish.</li>
        </ol>
        <p>
          Steam needs to be running and signed in. Close Rust first if it is already open: the link
          is reliable when it starts the game, much less so when the game is already running.
        </p>
        <h2 id="phone">Joining when you are not at your PC</h2>
        <p>
          A link still needs somebody at the PC to click it. QueueUp clicks it for you: tap join on
          your phone, and your PC starts Rust into the server and waits in the queue while you are
          somewhere else. On wipe night it can wait for the server to come back and join the moment
          it does.
        </p>
      </div>

      <GuideCta />
      <GuideFooter />
    </div>
  )
}
