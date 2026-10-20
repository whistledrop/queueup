// The guides: articles for people searching about Rust wipes, queues and
// servers, published one a day.
//
// Each one lives in content/guides with a publish date. Nothing is scheduled
// and nothing needs deploying on the day: a guide simply does not exist, to
// the index, the sitemap or its own address, until its date arrives in the UK.
// The pages are rebuilt at most hourly, so a guide dated today appears within
// the hour after midnight.

import { GUIDES } from '@/content/guides'

export type Guide = {
  slug: string
  title: string
  /** One or two sentences: the search result snippet and the share card. */
  description: string
  /** Publish date, YYYY-MM-DD, UK time. */
  date: string
  /** Set when a published guide is materially revised. */
  updated?: string
  /** Markdown: ## and ### headings, paragraphs, - and 1. lists, **bold**, [links](/x), > tips. */
  body: string
}

/** Today's date in the UK, as YYYY-MM-DD. */
export function ukToday(now = new Date()): string {
  // en-CA formats as YYYY-MM-DD.
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/London' }).format(now)
}

/** Every guide whose day has come, newest first. */
export function publishedGuides(now = new Date()): Guide[] {
  const today = ukToday(now)
  return GUIDES.filter((g) => g.date <= today).sort((a, b) => (a.date < b.date ? 1 : -1))
}

/** One published guide, or null for one that does not exist or is not out yet. */
export function guideBySlug(slug: string, now = new Date()): Guide | null {
  return publishedGuides(now).find((g) => g.slug === slug) ?? null
}

/** Roughly how long a guide takes to read. */
export function readingMinutes(g: Guide): number {
  return Math.max(2, Math.round(g.body.split(/\s+/).length / 220))
}
