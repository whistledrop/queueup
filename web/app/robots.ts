import type { MetadataRoute } from 'next'
import { SITE_URL } from '@/lib/site'

// Everything public is open to search engines. The signed-in app, the admin
// screen and the API are not pages anybody should land on from a search, and
// several are one-time links out of emails.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: '*',
      allow: '/',
      disallow: [
        '/api/', '/admin', '/settings', '/jobs', '/schedule', '/servers', '/subscribe',
        '/continue', '/finish', '/reset', '/unsubscribe', '/plan', '/rewards', '/ask',
        '/feedback', '/forgot', '/login', '/download',
      ],
    },
    sitemap: `${SITE_URL}/sitemap.xml`,
  }
}
