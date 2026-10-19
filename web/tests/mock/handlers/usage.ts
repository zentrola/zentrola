import { longID } from '../constants'
import type { MockContext, MockResult } from '../context'

// 用量看板与统计
export function handleUsage(ctx: MockContext): MockResult {
  const { url, path, method, reply, pageReply } = ctx
  const { statisticQueries } = ctx.state
  if (path === '/usage/dashboard')
    return reply({
      activeMemberCount: 2,
      modelCount: 2,
      providerCount: 1,
      totalTokens: 12840,
      tokenRanking: [
        { principalId: longID, name: '林知远', tokens: 8200 },
        { principalId: '90071992547409932', name: '陈清和', tokens: 4640 },
      ],
      clientModelRanking: [
        {
          modelId: '71',
          modelCode: 'deepseek-v4-flash',
          modelName: 'DeepSeek V4 Flash',
          requests: 18,
          tokens: 9100,
        },
        {
          modelId: '72',
          modelCode: 'claude-sonnet',
          modelName: 'Claude Sonnet',
          requests: 7,
          tokens: 3740,
        },
      ],
      providerRanking: [
        { providerId: '81', providerName: 'DeepSeek', calls: 20, tokens: 9200 },
        { providerId: '82', providerName: 'Anthropic', calls: 7, tokens: 3740 },
      ],
    })
  if (path === '/usage/statistics' && method === 'GET') {
    statisticQueries.push(new URLSearchParams(url.searchParams))
    const dimension = url.searchParams.get('dimension')
    const shared = {
      successful: 17,
      inputTokens: 6100,
      outputTokens: 2100,
      cachedInputTokens: 800,
      tokens: 8200,
      overallTokens: 12840,
      averageLatencyMs: 236,
    }
    if (dimension === 'model')
      return pageReply([
        {
          ...shared,
          entityId: '71',
          name: 'DeepSeek V4 Flash',
          code: 'deepseek-v4-flash',
          count: 18,
        },
      ])
    if (dimension === 'provider')
      return pageReply([
        {
          ...shared,
          entityId: '81',
          name: 'DeepSeek',
          code: 'deepseek',
          count: 20,
        },
      ])
    return pageReply([{ ...shared, entityId: longID, name: '林知远', code: '', count: 18 }])
  }
  return undefined
}
