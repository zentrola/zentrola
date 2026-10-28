import { longID, stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 一级资源列表与操作日志
export function handleCollections(ctx: MockContext): MockResult {
  const { url, path, method, segments, reply, pageReply } = ctx
  const {
    members,
    models,
    providers,
    providerMappings,
    credentialPrices,
    operations,
    groups,
    resources,
    groupQueries,
    modelQueries,
    providerQueries,
    memberListQueries,
    usageQueries,
    operationQueries,
  } = ctx.state
  if (segments.length === 1 && method === 'GET') {
    if (path === '/members') {
      memberListQueries.push(new URLSearchParams(url.searchParams))
      return pageReply([...members].sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1)))
    }
    if (path === '/models') {
      modelQueries.push(new URLSearchParams(url.searchParams))
      const status = url.searchParams.get('status')
      return pageReply(status ? models.filter((model) => model.status === status) : models)
    }
    if (path === '/providers') {
      providerQueries.push(new URLSearchParams(url.searchParams))
      const providerType = url.searchParams.get('type')
      const withModelCount = providers.map((provider) => ({
        ...provider,
        modelCount: (providerMappings.get(provider.id) || []).filter((mapping) =>
          models.some((model) => model.id === mapping.modelId && model.status === 'ACTIVE'),
        ).length,
      }))
      return pageReply(
        providerType
          ? withModelCount.filter((provider) => provider.type === providerType)
          : withModelCount,
      )
    }
    if (path === '/groups') {
      groupQueries.push(new URLSearchParams(url.searchParams))
      const status = url.searchParams.get('status')
      const filtered = status ? groups.filter((group) => group.status === status) : groups
      return pageReply([...filtered].sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1)))
    }
    if (path === '/resources')
      return pageReply(
        resources.map(({ credential: _, quotaDetails: __, ...row }) => ({
          ...row,
          isEnabled: row.isEnabled ?? true,
          subscriptionPrice: credentialPrices.get(row.id)?.subscriptionPrice ?? null,
        })),
      )
    if (path === '/operation-logs') {
      operationQueries.push(new URLSearchParams(url.searchParams))
      const keyword = (url.searchParams.get('keyword') || '').trim().toLowerCase()
      const type = url.searchParams.get('type')
      const result = url.searchParams.get('result')
      const limit = Number(url.searchParams.get('limit') || '20')
      const after = url.searchParams.get('after')
      const matching = operations
        .filter(
          (operation) =>
            (!type || operation.type === type) &&
            (!result || operation.result === result) &&
            (!keyword ||
              [
                operation.operatorName,
                operation.type,
                operation.targetType,
                operation.targetName,
                operation.targetId,
                operation.requestId,
                operation.result,
              ].some((value) =>
                String(value ?? '')
                  .toLowerCase()
                  .includes(keyword),
              )),
        )
        .sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1))
      const remaining = matching.filter(
        (operation) => !after || BigInt(operation.id) < BigInt(after),
      )
      const items = remaining.slice(0, limit)
      return reply({
        items,
        nextCursor: remaining.length > limit ? items.at(-1)!.id : null,
        total: matching.length,
      })
    }
    if (path === '/usage') {
      usageQueries.push(url.searchParams)
      return pageReply([
        {
          id: '99',
          requestId: 'req_live_compatible',
          principalId: longID,
          principalName: '林知远',
          modelId: '71',
          resourceId: '88',
          clientProtocol: 'OPENAI_RESPONSES',
          requestAt: stamp,
          completedAt: stamp,
          status: 'FAILED',
          inputTokens: 0,
          outputTokens: null,
          cachedInputTokens: 0,
          latencyMs: 218,
          attemptNo: 1,
          errorType: 'UPSTREAM_BILLING_BLOCKED',
        },
      ])
    }
  }
  return undefined
}
