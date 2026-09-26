import type { Metadata, Viewport } from 'next'
import { Geist } from 'next/font/google'
import './globals.css'

// Geist, at weight 500 for headings rather than bold. The combination of a
// grotesque at medium weight with tight letter-spacing is most of what makes a
// page read as a real product rather than a template.
const geist = Geist({ subsets: ['latin'], display: 'swap' })

export const metadata: Metadata = {
  title: 'QueueUp',
  description: 'Join a Rust server queue from your phone while your PC waits at home.',
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
  themeColor: '#ffffff',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={geist.className}>
      <body>{children}</body>
    </html>
  )
}
