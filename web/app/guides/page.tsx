import type { Metadata } from 'next'
import Link from 'next/link'
import { publishedGuides } from '@/lib/guides'
import { GuideCta, GuideFooter, GuideNav } from './chrome'
import s from './guides.module.css'

// Rebuilt at most hourly, so each day's guide joins the list on its day.
export const revalidate = 3600

export const metadata: Metadata = {
  title: 'Rust guides: wipes, queues and servers',
  description:
    'Plain answers about Rust: when force wipe is, how server queues work, wipe schedules, and getting into a server the moment it wipes.',
  alternates: { canonical: '/guides' },
}

export default function GuidesIndex() {
  const guides = publishedGuides()
  return (
    <div className={s.page}>
      <GuideNav />
      <p className={s.kicker}>Guides</p>
      <h1 className={s.title}>Rust wipes, queues and servers</h1>
      <p className={s.lede}>
        Plain answers to the things Rust players search for on wipe day, from the people who built
        a tool for exactly that.
      </p>
      <ul className={s.list} style={{ marginTop: 24 }}>
        {guides.map((g) => (
          <li key={g.slug} className={s.item}>
            <Link href={`/guides/${g.slug}`}>
              <h2>{g.title}</h2>
              <p>{g.description}</p>
            </Link>
          </li>
        ))}
      </ul>
      <GuideCta />
      <GuideFooter />
    </div>
  )
}
