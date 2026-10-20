import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { guideBySlug, publishedGuides, readingMinutes } from '@/lib/guides'
import { SITE_NAME, SITE_URL } from '@/lib/site'
import Markdown from '../markdown'
import { GuideCta, GuideFooter, GuideNav } from '../chrome'
import s from '../guides.module.css'

// Prerendered at build for every guide already out, and rebuilt at most
// hourly. A guide whose day comes after the build is made on its first visit;
// one whose day has not come is a 404, exactly as if it did not exist.
export const revalidate = 3600

export function generateStaticParams() {
  return publishedGuides().map((g) => ({ slug: g.slug }))
}

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
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
