import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { connection } from 'next/server'
import { guideBySlug, publishedGuides, readingMinutes } from '@/lib/guides'
import { SITE_NAME, SITE_URL } from '@/lib/site'
import Markdown from '../markdown'
import { GuideCta, GuideFooter, GuideNav } from '../chrome'
import s from '../guides.module.css'

// Made on each visit. A guide whose day has not come is a 404, exactly as if
// it did not exist, and it must stop being one at midnight UK on its day
// without a deploy. Built pages could not promise that: a "not found" built
// or stored before the day stayed stored after it (2026-10-22, the tools).

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  await connection()
  const g = guideBySlug((await params).slug)
  if (!g) return {}
  return {
    title: g.title,
    description: g.description,
    alternates: { canonical: `/guides/${g.slug}` },
    openGraph: {
      type: 'article',
      title: g.title,
      description: g.description,
      url: `/guides/${g.slug}`,
      publishedTime: g.date,
      modifiedTime: g.updated ?? g.date,
    },
  }
}

function longDate(iso: string): string {
  return new Date(iso + 'T12:00:00Z').toLocaleDateString('en-GB', {
    day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC',
  })
}

export default async function GuidePage({ params }: Props) {
  await connection()
  const g = guideBySlug((await params).slug)
  if (!g) notFound()

  // Up to three others, newest first, so a reader who found one answer finds
  // the next question's answer too.
  const more = publishedGuides().filter((o) => o.slug !== g.slug).slice(0, 3)

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'Article',
    headline: g.title,
    description: g.description,
    datePublished: g.date,
    dateModified: g.updated ?? g.date,
    mainEntityOfPage: `${SITE_URL}/guides/${g.slug}`,
    author: { '@type': 'Organization', name: SITE_NAME, url: SITE_URL },
    publisher: { '@type': 'Organization', name: SITE_NAME, url: SITE_URL },
  }

  return (
    <div className={s.page}>
      <script
        type="application/ld+json"
        // Our own data, serialised; the escape stops a "</script>" in a title
        // ever closing the tag early.
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, '\\u003c') }}
      />
      <GuideNav />
      <article>
        <p className={s.kicker}>
          <Link href="/guides" style={{ color: 'inherit', textDecoration: 'none' }}>Guides</Link>
        </p>
        <h1 className={s.title}>{g.title}</h1>
        <p className={s.lede}>{g.description}</p>
        <p className={s.meta}>
          {longDate(g.updated ?? g.date)} · {readingMinutes(g)} min read
        </p>
        <div className={s.body}>
          <Markdown source={g.body} />
        </div>
      </article>
      <GuideCta />
      {more.length > 0 && (
        <section className={s.more}>
          <h2>More guides</h2>
          <ul className={s.list}>
            {more.map((o) => (
              <li key={o.slug} className={s.item}>
                <Link href={`/guides/${o.slug}`}>
                  <h2>{o.title}</h2>
                  <p>{o.description}</p>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      <GuideFooter />
    </div>
  )
}
