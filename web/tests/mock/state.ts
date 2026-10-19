import type {
  ActiveModel,
  Member,
  Model,
  Operation,
  Provider,
  ProviderEndpoint,
} from '../../src/types'
import { stamp, longID } from './constants'

// 种子数据用前端类型约束；服务商的 modelCount 由模拟接口按映射实时计算。
type SeedProvider = Omit<Provider, 'modelCount' | 'endpoints'> & {
  endpoints: Array<Omit<ProviderEndpoint, 'networkScope'>>
}

// 内存模拟后端的全部可变数据；各资源处理器通过 MockContext 读写。
export function createMockState() {
  let nextID = 100n
  const next = () => (90071992547409900n + nextID++).toString()
  const members: any[] = [
    { id: longID, name: '林知远', remark: '平台研发组', status: 'ACTIVE', createdAt: stamp },
    {
      id: '90071992547409932',
      name: '陈清和',
      remark: '基础架构',
      status: 'ACTIVE',
      createdAt: stamp,
    },
    {
      id: '90071992547409933',
      name: '周予安',
      remark: '前端研发',
      status: 'DISABLED',
      createdAt: stamp,
    },
  ] satisfies Array<Omit<Member, 'monthlyTokenLimit'>>
  const models: any[] = [
    {
      id: '71',
      code: 'deepseek-v4-flash',
      name: 'DeepSeek V4 Flash',
      status: 'ACTIVE',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'DeepSeek',
      createdAt: stamp,
      updatedAt: stamp,
    },
    {
      id: '72',
      code: 'claude-sonnet',
      name: 'Claude Sonnet',
      status: 'ACTIVE',
      inputModalities: ['TEXT', 'IMAGE'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: null,
      publisherProviderName: null,
      createdAt: stamp,
      updatedAt: stamp,
    },
  ] satisfies Model[]
  const providers: any[] = [
    {
      id: '81',
      name: 'DeepSeek',
      code: 'deepseek-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: null,
      endpoints: [
        { protocolType: 'OPENAI', baseUrl: 'https://api.deepseek.com' },
        {
          protocolType: 'ANTHROPIC',
          baseUrl: 'https://api.deepseek.com/anthropic',
        },
      ],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: true,
      authAdapters: ['API_KEY'],
      createdAt: stamp,
      updatedAt: stamp,
    },
  ] satisfies SeedProvider[]
  const providerMappings = new Map<string, any[]>([
    [
      '81',
      [
        {
          id: '92',
          providerId: '81',
          modelId: '71',
          upstreamModelCode: 'deepseek-v4-flash',
          priority: 100,
          createdAt: stamp,
          updatedAt: stamp,
        },
      ],
    ],
  ])
  const credentialPrices = new Map<
    string,
    { modelPrices: any[]; subscriptionPrices: any[]; subscriptionPrice: any | null }
  >()
  const operations: any[] = [
    {
      id: '203',
      operatorName: 'admin',
      type: 'RESOURCE_CONNECTION_TEST',
      targetType: 'RESOURCE',
      targetId: '88',
      targetName: '测试服务商凭证',
      requestId: 'req_connection_test',
      result: 'SUCCESS',
      errorCode: null,
      before: null,
      after: { test: { ok: true, code: 'OK', httpStatus: 200, latencyMs: 140 } },
      createdAt: stamp,
    },
    {
      id: '202',
      operatorName: 'admin',
      type: 'LOGIN_FAILED',
      targetType: 'ADMIN_USER',
      targetId: '1',
      targetName: 'admin',
      requestId: 'req_login_failed',
      result: 'FAILED',
      errorCode: 'UNAUTHENTICATED',
      before: { lockedUntil: null, failedLoginCount: 0 },
      after: { lockedUntil: null, failedLoginCount: 1 },
      createdAt: stamp,
    },
    {
      id: '201',
      operatorName: 'admin',
      type: 'PROVIDER_STATUS_CHANGE',
      targetType: 'PROVIDER',
      targetId: '81',
      targetName: '测试服务商',
      requestId: 'req_provider_status',
      result: 'SUCCESS',
      errorCode: null,
      before: { status: 'ACTIVE' },
      after: { status: 'DISABLED' },
      createdAt: stamp,
    },
  ] satisfies Operation[]
  const groups: any[] = [],
    resources: any[] = [],
    keys: any[] = [],
    groupQueries: URLSearchParams[] = [],
    modelQueries: URLSearchParams[] = [],
    providerQueries: URLSearchParams[] = [],
    memberListQueries: URLSearchParams[] = [],
    memberSuggestionQueries: URLSearchParams[] = [],
    applicationSuggestionQueries: URLSearchParams[] = [],
    usageQueries: URLSearchParams[] = [],
    statisticQueries: URLSearchParams[] = [],
    operationQueries: URLSearchParams[] = [],
    tokenQuotaQueries: URLSearchParams[] = [],
    modelSyncRequests: string[] = [],
    providerInitializeRequests: any[] = [],
    activeModelRows: any[] = [
      { modelName: 'GPT 6', modelCode: 'gpt-6', providerName: 'OpenAI' },
      { modelName: 'Claude Sonnet 5', modelCode: 'claude-sonnet-5', providerName: 'CC Vibe' },
      { modelName: 'DeepSeek V4 Flash', modelCode: 'deepseek-v4-flash', providerName: '深度求索' },
    ] satisfies ActiveModel[]
  const providerInputs: any[] = []
  const providerActivationInputs: any[] = []
  const safeProviderProxy = (input: any) => {
    if (!input.proxyEnabled) return { proxyEnabled: false, proxyUrl: null, proxyHeaders: [] }
    const parsed = new URL(input.proxyUrl)
    if (parsed.username) parsed.username = '******'
    if (parsed.password) parsed.password = '******'
    return {
      proxyEnabled: true,
      proxyUrl: parsed.toString().replace(/\/$/, ''),
      proxyHeaders: (input.proxyHeaders || []).map((header: any) => ({
        key: header.key,
        configured: true,
      })),
    }
  }
  const relationships = new Map<string, Set<string>>()
  // 由测试切换的异常场景开关。
  const flags = {
    unauthorized: false,
    failedTest: false,
    conflict: false,
    priceEndpointUnavailable: false,
  }
  return {
    next,
    members,
    models,
    providers,
    providerMappings,
    credentialPrices,
    operations,
    groups,
    resources,
    keys,
    groupQueries,
    modelQueries,
    providerQueries,
    memberListQueries,
    memberSuggestionQueries,
    applicationSuggestionQueries,
    usageQueries,
    statisticQueries,
    operationQueries,
    tokenQuotaQueries,
    modelSyncRequests,
    providerInitializeRequests,
    activeModelRows,
    providerInputs,
    providerActivationInputs,
    safeProviderProxy,
    relationships,
    flags,
  }
}

export type MockState = ReturnType<typeof createMockState>
