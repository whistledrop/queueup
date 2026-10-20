// The small piece of Markdown the guides are written in, turned into React.
//
// Deliberately small: ## and ### headings, paragraphs, bulleted and numbered
// lists, > tips, **bold** and [links](/somewhere). That is all an article
// needs, and a renderer this size cannot be talked into writing raw HTML onto
// the page, which a full Markdown library with HTML passthrough can.

import Link from 'next/link'
import type { ReactNode } from 'react'
import s from './guides.module.css'

/** A heading's text as an anchor id, so a section can be linked to directly. */
export function anchor(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-')
}

function inline(text: string, keyBase: string): ReactNode[] {
  const out: ReactNode[] = []
  // **bold** or [label](href)
  const re = /\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g
  let last = 0
  let m: RegExpExecArray | null
  let i = 0
  while ((m = re.exec(text))) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const key = `${keyBase}-${i++}`
    if (m[1] !== undefined) {
      out.push(<strong key={key}>{m[1]}</strong>)
    } else {
      const href = m[3]
      out.push(
        href.startsWith('/') ? (
          <Link key={key} href={href}>{m[2]}</Link>
        ) : (
          <a key={key} href={href} target="_blank" rel="noopener noreferrer">{m[2]}</a>
        ),
      )
    }
    last = re.lastIndex
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

export default function Markdown({ source }: { source: string }) {
  const blocks = source.trim().split(/\n\s*\n/)
  return (
    <>
      {blocks.map((block, n) => {
        const lines = block.split('\n').map((l) => l.trim()).filter(Boolean)
        const key = `b${n}`
        const first = lines[0]
        if (first.startsWith('### ')) {
          const t = first.slice(4)
          return <h3 key={key} id={anchor(t)}>{inline(t, key)}</h3>
        }
        if (first.startsWith('## ')) {
          const t = first.slice(3)
          return <h2 key={key} id={anchor(t)}>{inline(t, key)}</h2>
        }
        if (lines.every((l) => l.startsWith('- '))) {
          return (
            <ul key={key}>
              {lines.map((l, i) => <li key={i}>{inline(l.slice(2), `${key}-${i}`)}</li>)}
            </ul>
          )
        }
        if (lines.every((l) => /^\d+\.\s/.test(l))) {
          return (
            <ol key={key}>
              {lines.map((l, i) => <li key={i}>{inline(l.replace(/^\d+\.\s/, ''), `${key}-${i}`)}</li>)}
            </ol>
          )
        }
        if (lines.every((l) => l.startsWith('> '))) {
          return (
            <aside key={key} className={s.tip}>
              {inline(lines.map((l) => l.slice(2)).join(' '), key)}
            </aside>
          )
        }
        return <p key={key}>{inline(lines.join(' '), key)}</p>
      })}
    </>
  )
}
