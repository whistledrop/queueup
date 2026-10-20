import type { MetadataRoute } from 'next'
import { publishedGuides } from '@/lib/guides'
import { SITE_URL } from '@/lib/site'

// Rebuilt at most hourly, so the day's new guide is in it the same day without
// anybody deploying anything.
export const revalidate = 3600

export default function sitemap(): MetadataRoute.Sitemap {
  const guides = publishedGuides()
  return [
    { url: SITE_URL, changeFrequency: 'weekly', priority: 1 },
    { url: `${SITE_URL}/guides`, changeFrequency: 'daily', priority: 0.8,
      lastModified: guides[0] ? new Date(guides[0].date) : undefined },
    { url: `${SITE_URL}/help`, changeFrequency: 'monthly', priority: 0.5 },
    { url: `${SITE_URL}/get`, changeFrequency: 'monthly', priority: 0.5 },
    { url: `${SITE_URL}/privacy`, changeFrequency: 'yearly', priority: 0.2 },
    { url: `${SITE_URL}/terms`, changeFrequency: 'yearly', priority: 0.2 },
    ...guides.map((g) => ({
      url: `${SITE_URL}/guides/${g.slug}`,
      lastModified: new Date(g.updated ?? g.date),
      changeFrequency: 'monthly' as const,
      priority: 0.7,
    })),
  ]
}
