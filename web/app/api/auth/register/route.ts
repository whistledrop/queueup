import { signIn } from '../login/route'

export const dynamic = 'force-dynamic'

export async function POST(request: Request) {
  return signIn(request, '/api/auth/register')
}
