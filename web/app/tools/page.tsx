import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { publishedTools } from '@/lib/tools'
import { GuideCta, GuideFooter, GuideNav } from '../guides/chrome'
import g from '../guides/guides.module.css'

export const revalidate = 3600

export const metadata: Metadata = {
  title: 'Free Rust tools: force wipe countdown, server checker and more',
  description:
    'Free tools for Rust players: a force wipe countdown in your time zone, a live server checker, and a one-click server join link maker.',
  alternates: { canonical: '/tools' },
}

export default function ToolsIndex() {
  const tools = publishedTools()
  if (tools.length === 0) notFound()
  return (
    <div className={g.page}>
      <GuideNav />
      <p className={g.kicker}>Free tools</p>
      <h1 className={g.title}>Free tools for Rust players</h1>
      <p className={g.lede}>Quick answers for wipe night. No account needed.</p>
      <ul className={g.list} style={{ marginTop: 24 }}>
        {tools.map((t) => (
          <li key={t.slug} className={g.item}>
            <Link href={`/tools/${t.slug}`}>
              <h2>{t.title}</h2>
              <p>{t.description}</p>
            </Link>
          </li>
        ))}
      </ul>
      <GuideCta />
      <GuideFooter />
    </div>
  )
}
