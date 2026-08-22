// One proxy for every relay call the browser makes.
//
// It exists so the session token can stay in an http-only cookie: the browser
// asks this app, this app adds the token and asks the relay. It also passes
// streaming responses straight through, which is what the live status screen
// needs.

import type { NextRequest } from 'next/server'
import { relayURL, sessionToken } from '@/lib/relay'

export const dynamic = 'force-dynamic'

async function forward(request: NextRequest, path: string[]) {
  const token = await sessionToken()

  const url = new URL(relayURL() + '/' + path.join('/'))
  url.search = request.nextUrl.search

  const headers = new Headers()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  for (const h of ['content-type', 'accept']) {
    const v = request.headers.get(h)
    if (v) headers.set(h, v)
  }

  const hasBody = request.method !== 'GET' && request.method !== 'HEAD'
  let upstream: Response
  try {
    upstream = await fetch(url, {
      method: request.method,
      headers,
      body: hasBody ? await request.text() : undefined,
      cache: 'no-store',
      signal: request.signal,
    })
  } catch {
    return Response.json(
      { error: "We can't reach QueueUp right now. Try again in a moment." },
      { status: 503 },
    )
  }

  const out = new Headers()
  for (const h of ['content-type', 'cache-control']) {
    const v = upstream.headers.get(h)
    if (v) out.set(h, v)
  }
  // Server-sent events must not be buffered on the way through.
  if (out.get('content-type')?.includes('text/event-stream')) {
    out.set('cache-control', 'no-cache, no-transform')
    out.set('connection', 'keep-alive')
  }
  return new Response(upstream.body, { status: upstream.status, headers: out })
}

type Ctx = { params: Promise<{ path: string[] }> }

export async function GET(request: NextRequest, ctx: Ctx) {
  return forward(request, (await ctx.params).path)
}
export async function POST(request: NextRequest, ctx: Ctx) {
  return forward(request, (await ctx.params).path)
}
export async function DELETE(request: NextRequest, ctx: Ctx) {
  return forward(request, (await ctx.params).path)
}
