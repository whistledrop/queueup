import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { connection } from 'next/server'
import { releasedTool } from '@/lib/tools'
import { GuideCta, GuideFooter, GuideNav } from '../../guides/chrome'
import g from '../../guides/guides.module.css'
import Checker from './checker'

// Checked on every visit, not built once: a page built before its release
// day is a "not found", and the host kept serving that stored copy after the
// day came (2026-10-22, the server checker). Asking at request time means it
// appears at midnight UK on its day, whenever the site was last deployed.

const SLUG = 'server-checker'

export async function generateMetadata(): Promise<Metadata> {
  await connection()
  const t = releasedTool(SLUG)
  if (!t) return {}
  return {
    title: 'Rust server checker: players, queue and last wipe',
    description: t.description,
    alternates: { canonical: `/tools/${SLUG}` },
  }
}

export default async function ServerChecker() {
  await connection()
  if (!releasedTool(SLUG)) notFound()
  return (
    <div className={g.page}>
      <GuideNav />
      <p className={g.kicker}><Link href="/tools" style={{ color: 'inherit', textDecoration: 'none' }}>Free tools</Link></p>
      <h1 className={g.title}>Rust server checker</h1>
      <p className={g.lede}>
        Search any Rust server and check it live: whether it is up, how full it is, how many are in
        the queue, and how long since it last restarted, which on most servers means since it wiped.
      </p>

      <Checker />

      <div className={g.body}>
        <h2 id="how">Where the numbers come from</h2>
        <p>
          The search uses Steam&apos;s public list of Rust servers. When you pick one, QueueUp asks the
          server itself, so the players, the queue and the start time are as of that moment, not
          minutes old.
        </p>
        <h2 id="queue">What the queue number means</h2>
        <p>
          It is how many people are waiting to get in right now. A full server lets the next person in
          each time somebody leaves, so a long queue straight after a wipe moves slowly: almost
          nobody leaves a server they have only just got into.
        </p>
        <h2 id="wipe">Telling a fresh wipe from a restart</h2>
        <p>
          Every server restarts when it wipes, so a server that started a few hours ago has usually
          just wiped. Servers also restart for updates and fixes, so check its name or Discord to be
          sure of its schedule. Force wipe is always the first Thursday of the month: see the{' '}
          <Link href="/tools/force-wipe-countdown">force wipe countdown</Link>.
        </p>
      </div>

      <GuideCta />
      <GuideFooter />
    </div>
  )
}
