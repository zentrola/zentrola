import { stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 服务商目录、初始化与模型同步
export function handleProviders(ctx: MockContext): MockResult {
  const { path, method, body, segments, reply } = ctx
  const {
    next,
    models,
    providers,
    providerMappings,
    modelSyncRequests,
    providerInitializeRequests,
    flags,
  } = ctx.state
  if (path === '/usage/writer') return reply({ pending: 0, failed: 0 })
  if (path === '/providers/initialize-options' && method === 'GET') {
    const names = [
      ['openai-official', 'OpenAI', 'https://openai.com'],
      ['anthropic-official', 'Anthropic', 'https://www.anthropic.com'],
      ['google-gemini-official', 'Google', 'https://ai.google.dev/gemini-api'],
      ['deepseek-official', '深度求索', 'https://www.deepseek.com'],
      ['zhipu-official', '智谱 AI', 'https://www.zhipuai.cn'],
      ['kimi-official', '月之暗面', 'https://www.moonshot.cn'],
      ['qwen-official', '通义千问', 'https://bailian.console.aliyun.com/'],
      ['xai-official', 'xAI', 'https://x.ai'],
    ]
    return reply(names.map(([code, name, website]) => ({ code, name, website })))
  }
  if (segments[0] === 'providers' && segments.length === 2 && method === 'GET') {
    const row = providers.find((provider) => provider.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    const providerModels =
      row.type === 'OFFICIAL'
        ? models.filter((model) => model.publisherProviderId === row.id)
        : models.filter((model) => model.status === 'ACTIVE')
    const mappings = providerMappings.get(row.id) || []
    return reply({
      ...row,
      modelCount: mappings.filter((mapping) =>
        models.some((model) => model.id === mapping.modelId && model.status === 'ACTIVE'),
      ).length,
      mappings,
      models: providerModels,
    })
  }
  if (segments[0] === 'providers' && segments[2] === 'sync-models' && method === 'POST') {
    modelSyncRequests.push(segments[1])
    return reply({
      ok: !flags.failedTest,
      code: flags.failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
      httpStatus: flags.failedTest ? 401 : 200,
      latencyMs: 160,
      discovered: flags.failedTest ? 0 : 2,
      created: flags.failedTest ? 0 : 1,
      updated: 0,
      mapped: flags.failedTest ? 0 : 1,
      source: 'PROVIDER',
    })
  }
  if (path === '/providers/initialize' && method === 'POST') {
    providerInitializeRequests.push(body)
    const templates = [
      ['openai-official', 'OpenAI', 'OpenAI', 'https://api.openai.com/v1', 'https://openai.com'],
      [
        'anthropic-official',
        'Anthropic',
        'Anthropic',
        'https://api.anthropic.com',
        'https://www.anthropic.com',
      ],
      [
        'google-gemini-official',
        'Google',
        'Google',
        'https://generativelanguage.googleapis.com/v1beta/openai',
        'https://ai.google.dev/gemini-api',
      ],
      [
        'deepseek-official',
        '深度求索',
        'DeepSeek',
        'https://api.deepseek.com',
        'https://www.deepseek.com',
      ],
      [
        'zhipu-official',
        '智谱 AI',
        'Zhipu AI',
        'https://open.bigmodel.cn/api/paas/v4',
        'https://www.zhipuai.cn',
      ],
      [
        'kimi-official',
        '月之暗面',
        'Moonshot AI',
        'https://api.moonshot.cn/v1',
        'https://www.moonshot.cn',
      ],
      [
        'qwen-official',
        '通义千问',
        'Qwen',
        'https://dashscope.aliyuncs.com/compatible-mode/v1',
        'https://bailian.console.aliyun.com/',
      ],
      ['xai-official', 'xAI', 'xAI', 'https://api.x.ai/v1', 'https://x.ai'],
    ]
    let created = 0,
      updated = 0
    const selectedTemplates = templates.filter(([code]) => body.providerCodes.includes(code))
    for (const [code, nameZH, nameEN, baseUrl, website] of selectedTemplates) {
      const name = body.locale === 'zh-CN' ? nameZH : nameEN
      const existing = providers.find((provider) => provider.code === code)
      if (existing) {
        if (existing.name !== name || existing.website !== website) {
          existing.name = name
          existing.website = website
          updated++
        }
        continue
      }
      providers.push({
        id: next(),
        code,
        name,
        type: 'OFFICIAL',
        status: 'DISABLED',
        website,
        endpoints: [{ protocolType: 'OPENAI', baseUrl }],
        proxyEnabled: false,
        proxyUrl: null,
        proxyHeaders: [],
        modelSyncSupported: code === 'deepseek-official' || code === 'qwen-official',
        authAdapters: code === 'openai-official' ? ['API_KEY', 'OPENAI_CODEX'] : ['API_KEY'],
        createdAt: stamp,
        updatedAt: stamp,
      })
      created++
    }
    return reply({
      total: selectedTemplates.length,
      created,
      updated,
      existing: selectedTemplates.length - created,
    })
  }
  return undefined
}
