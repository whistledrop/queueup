import LiveStatus from './live'

export const dynamic = 'force-dynamic'

export default async function JobPage({ params }: { params: Promise<{ id: string }> }) {
  // params is a promise in Next 16.
  const { id } = await params
  return <LiveStatus jobId={id} />
}
