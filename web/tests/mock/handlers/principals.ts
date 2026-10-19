import type { MockContext, MockResult } from '../context'

// 用户 / 应用联想与配额状态
export function handlePrincipals(ctx: MockContext): MockResult {
  const { url, path, method, reply, pageReply } = ctx
  const {
    members,
    groups,
    memberSuggestionQueries,
    applicationSuggestionQueries,
    tokenQuotaQueries,
  } = ctx.state
  if (path === '/members/suggestions' && method === 'GET') {
    memberSuggestionQueries.push(new URLSearchParams(url.searchParams))
    const name = url.searchParams.get('name') || ''
    return pageReply(members.filter((member) => member.name.includes(name)))
  }
  if (path === '/applications/suggestions' && method === 'GET') {
    applicationSuggestionQueries.push(new URLSearchParams(url.searchParams))
    const name = url.searchParams.get('name') || ''
    return pageReply(
      [{ id: '90071992547409944', name: '报表应用' }].filter((app) => app.name.includes(name)),
    )
  }
  if (path === '/token-quotas' && method === 'GET') {
    const scopeType = url.searchParams.get('scopeType')
    const ids = (url.searchParams.get('scopeIds') || '').split(',').filter(Boolean)
    tokenQuotaQueries.push(new URLSearchParams(url.searchParams))
    if (ids.length > 100) return reply(null, 400, 'INVALID_ARGUMENT')
    const source = scopeType === 'GROUP' ? groups : members
    return reply(
      ids.flatMap((id) => {
        const item = source.find((entry) => entry.id === id)
        if (!item?.monthlyTokenLimit) return []
        const usedTokens = item.quotaUsedTokens || '600'
        const limit = BigInt(item.monthlyTokenLimit)
        const used = BigInt(usedTokens)
        return [
          {
            scopeType,
            scopeId: id,
            monthlyTokenLimit: item.monthlyTokenLimit,
            usedTokens,
            remainingTokens: String(limit > used ? limit - used : 0n),
            usedPercent: Number((used * 10000n) / limit) / 100,
            level: used >= limit ? 'EXHAUSTED' : used * 100n >= limit * 80n ? 'WARNING' : 'NOTICE',
            periodStart: '2026-09-01T00:00:00Z',
            periodEnd: '2026-10-01T00:00:00Z',
          },
        ]
      }),
    )
  }
  return undefined
}
