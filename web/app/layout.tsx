import type { Metadata, Viewport } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'
import PromoCatcher from './promoCatcher'
import { SITE_DESCRIPTION, SITE_NAME, SITE_TITLE, SITE_URL } from '@/lib/site'

// Inter, the face Linear's look is built on: neutral, very readable small,
// and tight and confident at display sizes.
const font = Inter({ subsets: ['latin'], display: 'swap' })

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
    <html lang="en" className={font.className}>
      <body>
        <PromoCatcher />
        {children}
      </body>
    </html>
  )
}
