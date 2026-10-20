import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { upcomingForceWipes } from '@/lib/forcewipe'
import { releasedTool } from '@/lib/tools'
import { GuideCta, GuideFooter, GuideNav } from '../../guides/chrome'
import g from '../../guides/guides.module.css'
import s from '../tools.module.css'
import { Countdown, LocalTime } from './countdown'

export const revalidate = 3600

const SLUG = 'force-wipe-countdown'

export function generateMetadata(): Metadata {
  const t = releasedTool(SLUG)
  if (!t) return {}
  return {
    title: 'Rust force wipe countdown: when is the next force wipe?',
    description: t.description,
    alternates: { canonical: `/tools/${SLUG}` },
  }
}

const ukDate = (d: Date) =>
  d.toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Europe/London' })

export default function ForceWipeCountdown() {
  if (!releasedTool(SLUG)) notFound()
  const wipes = upcomingForceWipes(new Date(), 13)
  const next = wipes[0]

  return (
    <div className={g.page}>
      <GuideNav />
      <p className={g.kicker}><Link href="/tools" style={{ color: 'inherit', textDecoration: 'none' }}>Free tools</Link></p>
      <h1 className={g.title}>When is the next Rust force wipe?</h1>
      <p className={g.lede}>
        Rust force wipes on the first Thursday of every month, when Facepunch releases the monthly
        update. The next one is <strong>{ukDate(next)}</strong>.
      </p>

      <div className={s.panel}>
        <Countdown wipeISO={next.toISOString()} />
        <p className={s.when}>
          Usually around <strong>7pm UK time</strong>. That is <strong><LocalTime iso={next.toISOString()} /></strong> where you are.
        </p>
        <p className={s.note}>
          The update goes out once, worldwide, so every region gets it at the same moment. Official
          servers come back soon after; some community servers open their new map later, at a time
          they announce in their Discord.
        </p>
      </div>

      <div className={g.body}>
        <h2 id="dates">Force wipe dates for the next year</h2>
        <div className={s.tableWrap}>
          <table className={s.table}>
            <thead>
              <tr><th>Date</th><th>Usual time, your time zone</th></tr>
            </thead>
            <tbody>
              {wipes.map((w) => (
                <tr key={w.toISOString()}>
                  <td>{ukDate(w)}</td>
                  <td><LocalTime iso={w.toISOString()} withDay={false} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h2 id="regions">Is force wipe at a different time in EU, NA and OCE?</h2>
        <p>
          No. There is one update, released once for the whole world. What changes is only the clock
          on your wall: around 7pm in the UK is around 8pm in Central Europe, around 2pm on the US East
          Coast, around 11am on the US West Coast, and early Friday morning in eastern Australia.
        </p>
        <p>
          What can differ is when <strong>your server</strong> opens. Every server has to update before it
          can run, and many reopen straight away, but some community servers, especially in Oceania
          and parts of North America, keep their new map closed until an evening time that suits their
          players. Their Discord will say.
        </p>

        <h2 id="what">What happens on force wipe</h2>
        <p>
          Every server resets its map, and almost every server resets blueprints too. Your Steam
          account, skins and items are untouched. For the full picture, read{' '}
          <Link href="/guides/when-is-rust-force-wipe">when Rust force wipe is, and what it resets</Link>.
        </p>
        <aside className={g.tip}>
          The update time is Facepunch&apos;s call, not a fixed rule. 7pm UK is when it usually lands,
          not a promise; treat it as the moment to be ready by.
        </aside>
      </div>

      <GuideCta />
      <GuideFooter />
    </div>
  )
}
