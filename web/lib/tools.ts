// The free tools: things Rust players search for and come back to every
// wipe. Released like the guides: each exists, to its page, the tools list
// and the sitemap, only from its own date in the UK.

import { ukToday } from '@/lib/guides'

export type Tool = {
  slug: string
  title: string
  /** The search result snippet and share card. */
  description: string
  /** Release date, YYYY-MM-DD, UK time. */
  date: string
}

export const TOOLS: Tool[] = [
  {
    slug: 'force-wipe-countdown',
    title: 'Rust force wipe countdown',
    description:
      'When is the next Rust force wipe? A live countdown in your own time zone, and every force wipe date for the next year.',
    date: '2026-10-21',
  },
  {
    slug: 'server-checker',
    title: 'Rust server checker',
    description:
      'Check any Rust server live: online or down, players, queue length, and how long since it last restarted or wiped.',
    date: '2026-10-22',
  },
  {
    slug: 'join-link',
    title: 'Rust server join link maker',
    description:
      'Turn a Rust server address into a one-click Steam link, or a desktop shortcut that joins it.',
    date: '2026-10-23',
  },
]

/** Every tool whose day has come, in release order. */
export function publishedTools(now = new Date()): Tool[] {
  const today = ukToday(now)
  return TOOLS.filter((t) => t.date <= today)
}

/** One released tool, or null. */
export function releasedTool(slug: string, now = new Date()): Tool | null {
  return publishedTools(now).find((t) => t.slug === slug) ?? null
}
