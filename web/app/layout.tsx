import type { Metadata, Viewport } from 'next'
import { Geist } from 'next/font/google'
import './globals.css'
import PromoCatcher from './promoCatcher'
import { SITE_DESCRIPTION, SITE_NAME, SITE_TITLE, SITE_URL } from '@/lib/site'

// Geist, at weight 500 for headings rather than bold. The combination of a
// grotesque at medium weight with tight letter-spacing is most of what makes a
// page read as a real product rather than a template.
const geist = Geist({ subsets: ['latin'], display: 'swap' })

export const metadata: Metadata = {
  // Every relative address in metadata (canonicals, share cards) resolves
  // against the real site, not whichever host served the page.
  metadataBase: new URL(SITE_URL),
  title: { default: SITE_TITLE, template: `%s | ${SITE_NAME}` },
  description: SITE_DESCRIPTION,
  applicationName: SITE_NAME,
  openGraph: {
    type: 'website',
    siteName: SITE_NAME,
    title: SITE_TITLE,
    description: SITE_DESCRIPTION,
    url: '/',
    locale: 'en_GB',
  },
  twitter: { card: 'summary_large_image', title: SITE_TITLE, description: SITE_DESCRIPTION },
  // The manifest and apple icon make "Add to Home Screen" produce a real app
  // icon, and on iPhone the home-screen install is what unlocks push
  // notifications at all.
  manifest: '/manifest.json',
  icons: {
    icon: '/icon-192.png',
    apple: '/apple-touch-icon.png',
  },
  appleWebApp: {
    capable: true,
    title: 'QueueUp',
    statusBarStyle: 'black-translucent',
  },
}

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  themeColor: '#0e0f12',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={geist.className}>
      <body>
        <PromoCatcher />
        {children}
      </body>
    </html>
  )
}
