import type { MetadataRoute } from 'next'
import { publishedGuides } from '@/lib/guides'
import { publishedTools } from '@/lib/tools'
import { SITE_URL } from '@/lib/site'

// Rebuilt at most hourly, so the day's new guide is in it the same day without
// anybody deploying anything.
export const revalidate = 3600

export default function sitemap(): MetadataRoute.Sitemap {
  const guides = publishedGuides()
  const tools = publishedTools()
  return [
    { url: SITE_URL, changeFrequency: 'weekly', priority: 1 },
    { url: `${SITE_URL}/guides`, changeFrequency: 'daily', priority: 0.8,
      lastModified: guides[0] ? new Date(guides[0].date) : undefined },
    { url: `${SITE_URL}/help`, changeFrequency: 'monthly', priority: 0.5 },
    { url: `${SITE_URL}/get`, changeFrequency: 'monthly', priority: 0.5 },
    { url: `${SITE_URL}/privacy`, changeFrequency: 'yearly', priority: 0.2 },
    { url: `${SITE_URL}/terms`, changeFrequency: 'yearly', priority: 0.2 },
    ...(tools.length > 0
      ? [{ url: `${SITE_URL}/tools`, changeFrequency: 'weekly' as const, priority: 0.8 }]
      : []),
    ...tools.map((t) => ({
      url: `${SITE_URL}/tools/${t.slug}`,
      lastModified: new Date(t.date),
      changeFrequency: 'weekly' as const,
      priority: 0.8,
    })),
    ...guides.map((g) => ({
      url: `${SITE_URL}/guides/${g.slug}`,
      lastModified: new Date(g.updated ?? g.date),
      changeFrequency: 'monthly' as const,
      priority: 0.7,
    })),
  ]
}
