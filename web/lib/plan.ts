// The free plan's numbers, in one place on the web side.
//
// The relay is the authority: store.FreeJoinsPerMonth is what actually decides
// whether a join is allowed, and /api/billing reports the real remaining count
// per account. This constant exists only so a page can say "10 joins a month"
// before it has asked anybody anything, and it must be changed in step with
// the relay if that number ever moves.
export const FREE_JOINS_PER_MONTH = 10
