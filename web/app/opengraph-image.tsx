import { ImageResponse } from 'next/og'

// The card a link to queueuprust.com shows in Discord, iMessage, X and the
// rest. Drawn from text rather than a screenshot so it stays sharp, and so the
// one claim on it is the same as the page's.

export const alt = 'QueueUp: join Rust server queues from your phone'
export const size = { width: 1200, height: 630 }
export const contentType = 'image/png'

export default function Image() {
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: '72px 80px',
          background: '#0e0f12',
          color: '#edeef0',
          fontFamily: 'sans-serif',
        }}
      >
        <div style={{ display: 'flex', fontSize: 44, fontWeight: 600, letterSpacing: -1 }}>
          Queue<span style={{ color: '#ffffff' }}>Up</span>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 22 }}>
          <div style={{ display: 'flex', fontSize: 76, fontWeight: 600, lineHeight: 1.05, letterSpacing: -2 }}>
            Load into Rust servers from anywhere.
          </div>
          <div style={{ display: 'flex', fontSize: 32, color: '#8b8f98' }}>
            Tap join on your phone. Your PC queues. You walk in and play.
          </div>
        </div>
        <div style={{ display: 'flex', fontSize: 26, color: '#8b8f98' }}>
          Free for 10 joins a month · queueuprust.com
        </div>
      </div>
    ),
    size,
  )
}
