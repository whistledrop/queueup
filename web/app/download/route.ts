import { NextResponse } from 'next/server'

// /download is our own address for the agent, so the download button never has
// to know where the file is actually hosted. Move it off GitHub later and this
// one line changes; every link, doc and screenshot keeps working.
const AGENT_URL =
  'https://github.com/whistledrop/queueup/releases/latest/download/QueueUpAgent.exe'

export function GET() {
  return NextResponse.redirect(AGENT_URL, 302)
}
