import { test, expect, type Locator, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'

const stamp = '2026-09-06T07:30:00Z'
const longID = '90071992547409931'
async function expectOnlyModalBodyScrolls(dialog: Locator) {
  const layout = await dialog.evaluate((element) => {
    const body = element.querySelector('.modal-body') as HTMLElement
    const head = element.querySelector('.modal-head') as HTMLElement
    const footer = element.querySelector('.modal-footer') as HTMLElement
    const before = {
      headTop: head.getBoundingClientRect().top,
      footerTop: footer.getBoundingClientRect().top,
      dialogTop: element.getBoundingClientRect().top,
    }
    body.scrollTop = body.scrollHeight
    return {
      scrollable: body.scrollHeight > body.clientHeight,
      scrollTop: body.scrollTop,
      bodyOverflow: getComputedStyle(body).overflowY,
      dialogOverflow: getComputedStyle(element).overflowY,
      headTopUnchanged: head.getBoundingClientRect().top === before.headTop,
      footerTopUnchanged: footer.getBoundingClientRect().top === before.footerTop,
      dialogTopUnchanged: element.getBoundingClientRect().top === before.dialogTop,
    }
  })
  expect(layout).toEqual({
    scrollable: true,
    scrollTop: expect.any(Number),
    bodyOverflow: 'auto',
    dialogOverflow: 'hidden',
    headTopUnchanged: true,
    footerTopUnchanged: true,
    dialogTopUnchanged: true,
  })
  expect(layout.scrollTop).toBeGreaterThan(0)
}
async function fixture(page: Page) {
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
  ]
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
  ]
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
  ]
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
  let priceEndpointUnavailable = false
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
  ]
  const groups: any[] = [],
    resources: any[] = [],
    keys: any[] = [],
    groupQueries: URLSearchParams[] = [],
    modelQueries: URLSearchParams[] = [],
    providerQueries: URLSearchParams[] = [],
    memberListQueries: URLSearchParams[] = [],
    memberSuggestionQueries: URLSearchParams[] = [],
    usageQueries: URLSearchParams[] = [],
    statisticQueries: URLSearchParams[] = [],
    modelSyncRequests: string[] = [],
    providerInitializeRequests: any[] = [],
    activeModelRows: any[] = [
      { modelName: 'GPT 6', modelCode: 'gpt-6', providerName: 'OpenAI' },
      { modelName: 'Claude Sonnet 5', modelCode: 'claude-sonnet-5', providerName: 'CC Vibe' },
      { modelName: 'DeepSeek V4 Flash', modelCode: 'deepseek-v4-flash', providerName: '深度求索' },
    ]
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
  let unauthorized = false,
    failedTest = false,
    conflict = false
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname.replace('/api/v1', ''),
      method = req.method()
    const body = req.postDataJSON(),
      segments = path.split('/').filter(Boolean)
    const reply = (data: unknown, status = 200, code = 'OK') =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify({ code, data, requestId: 'req_browser_fixture' }),
      })
    if (path === '/auth/setup') return reply({ required: false })
    if (path === '/auth/login')
      return reply({
        token: 'fixture-admin-token',
        expiresAt: new Date(Date.now() + 3600000).toISOString(),
      })
    if (unauthorized) return reply(null, 401, 'UNAUTHENTICATED')
    if (path === '/me') return reply({ id: '1', username: 'admin', displayName: '管理员' })
    if (path === '/auth/logout') return reply({ clearToken: true })
    if (path === '/gateway/active-models') return reply(activeModelRows)
    if (path === '/billing/usage-costs/summary' && method === 'GET')
      return reply({
        from: url.searchParams.get('from'),
        to: url.searchParams.get('to'),
        requests: 2,
        attempts: 2,
        successful: 2,
        inputTokens: 2100,
        cachedInputTokens: 100,
        outputTokens: 900,
        rated: 1,
        shared: 1,
        unrated: 0,
        totals: [{ currency: 'USD', amount: '0.00800000', rated: 1 }],
      })
    if (path === '/billing/usage-costs' && method === 'GET')
      return reply({
        items: [
          {
            id: '901',
            requestId: 'req_api_cost',
            attemptNo: 1,
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            modelId: '71',
            modelName: 'DeepSeek V4 Flash',
            providerId: '81',
            providerName: 'DeepSeek',
            providerModelId: '92',
            resourceId: '88',
            resourceName: 'DeepSeek API Key',
            clientProtocol: 'OPENAI_RESPONSES',
            status: 'SUCCESS',
            errorType: null,
            startedAt: stamp,
            completedAt: '2026-09-06T07:30:01Z',
            latencyMs: 1000,
            inputTokens: 1200,
            cachedInputTokens: 100,
            outputTokens: 300,
            billingType: 'API_KEY',
            ratingStatus: 'RATED',
            ratingId: '911',
            ratingRevision: 2,
            currency: 'USD',
            totalCost: '0.00800000',
            ratedAt: stamp,
          },
          {
            id: '902',
            requestId: 'req_subscription_cost',
            attemptNo: 1,
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            modelId: '72',
            modelName: 'Claude Sonnet',
            providerId: '81',
            providerName: 'DeepSeek',
            providerModelId: '93',
            resourceId: '89',
            resourceName: '团队订阅',
            clientProtocol: 'ANTHROPIC_MESSAGES',
            status: 'SUCCESS',
            errorType: null,
            startedAt: stamp,
            completedAt: '2026-09-06T07:30:02Z',
            latencyMs: 2000,
            inputTokens: 900,
            cachedInputTokens: 0,
            outputTokens: 600,
            billingType: 'SUBSCRIPTION',
            ratingStatus: 'SUBSCRIPTION_SHARED',
            ratingId: null,
            ratingRevision: null,
            currency: 'CNY',
            totalCost: null,
            ratedAt: null,
          },
        ],
        nextCursor: null,
        total: 2,
      })
    if (path === '/billing/usage-costs/901' && method === 'GET')
      return reply({
        id: '901',
        requestId: 'req_api_cost',
        attemptNo: 1,
        principalId: longID,
        principalName: '林知远',
        principalType: 'MEMBER',
        modelId: '71',
        modelName: 'DeepSeek V4 Flash',
        providerId: '81',
        providerName: 'DeepSeek',
        providerModelId: '92',
        resourceId: '88',
        resourceName: 'DeepSeek API Key',
        clientProtocol: 'OPENAI_RESPONSES',
        status: 'SUCCESS',
        errorType: null,
        startedAt: stamp,
        completedAt: '2026-09-06T07:30:01Z',
        latencyMs: 1000,
        inputTokens: 1200,
        cachedInputTokens: 100,
        outputTokens: 300,
        billingType: 'API_KEY',
        ratingStatus: 'RATED',
        ratingId: '911',
        ratingRevision: 2,
        currency: 'USD',
        totalCost: '0.00800000',
        ratedAt: stamp,
        ratings: [
          {
            id: '910',
            revision: 1,
            modelPriceId: '920',
            priceEffectiveAt: '2026-09-01T00:00:00Z',
            inputPrice: '0.00000200',
            cachedInputPrice: '0.00000100',
            outputPrice: '0.00000800',
            inputCost: '0.00240000',
            cachedInputCost: '0.00010000',
            outputCost: '0.00240000',
            totalCost: '0.00490000',
            currency: 'USD',
            createdAt: stamp,
          },
          {
            id: '911',
            revision: 2,
            modelPriceId: '921',
            priceEffectiveAt: '2026-09-01T00:00:00Z',
            inputPrice: '0.00000300',
            cachedInputPrice: '0.00000100',
            outputPrice: '0.00001400',
            inputCost: '0.00360000',
            cachedInputCost: '0.00010000',
            outputCost: '0.00420000',
            totalCost: '0.00790000',
            currency: 'USD',
            createdAt: stamp,
          },
        ],
        subscription: null,
      })
    if (path === '/billing/usage-costs/902' && method === 'GET')
      return reply({
        id: '902',
        requestId: 'req_subscription_cost',
        attemptNo: 1,
        principalId: longID,
        principalName: '林知远',
        principalType: 'MEMBER',
        modelId: '72',
        modelName: 'Claude Sonnet',
        providerId: '81',
        providerName: 'DeepSeek',
        providerModelId: '93',
        resourceId: '89',
        resourceName: '团队订阅',
        clientProtocol: 'ANTHROPIC_MESSAGES',
        status: 'SUCCESS',
        errorType: null,
        startedAt: stamp,
        completedAt: '2026-09-06T07:30:02Z',
        latencyMs: 2000,
        inputTokens: 900,
        cachedInputTokens: 0,
        outputTokens: 600,
        billingType: 'SUBSCRIPTION',
        ratingStatus: 'SUBSCRIPTION_SHARED',
        ratingId: null,
        ratingRevision: null,
        currency: 'CNY',
        totalCost: null,
        ratedAt: null,
        ratings: [],
        subscription: {
          priceId: '930',
          currency: 'CNY',
          periodAmount: '1000.00000000',
          billingPeriod: 'MONTH',
          effectiveAt: '2026-09-01T00:00:00Z',
          documentId: '601',
          periodStart: '2026-09-01T00:00:00Z',
          periodEnd: '2026-10-01T00:00:00Z',
          principalAmount: '700.00000000',
        },
      })
    if (path === '/billing/usage-costs/export' && method === 'GET')
      return route.fulfill({
        status: 200,
        contentType: 'text/csv; charset=utf-8',
        headers: { 'Content-Disposition': 'attachment; filename="zentrola-usage-costs.csv"' },
        body: '\ufeffusage_record_id,total_cost\r\n901,0.00800000\r\n',
      })
    if (path === '/billing/current-attribution' && method === 'GET')
      return reply({
        from: url.searchParams.get('from'),
        to: url.searchParams.get('to'),
        totals: [
          {
            billingType: 'SUBSCRIPTION',
            currency: 'CNY',
            totalAmount: '1000.00000000',
            allocatedAmount: '1000.00000000',
            unallocatedAmount: '0',
            totalTokens: 1500,
          },
          {
            billingType: 'API_KEY',
            currency: 'USD',
            totalAmount: '0.00800000',
            allocatedAmount: '0.00800000',
            unallocatedAmount: '0',
            totalTokens: 1500,
          },
        ],
        items: [
          {
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            billingType: 'SUBSCRIPTION',
            currency: 'CNY',
            amount: '1000.00000000',
            tokens: 1500,
          },
          {
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            billingType: 'API_KEY',
            currency: 'USD',
            amount: '0.00800000',
            tokens: 1500,
          },
        ],
        total: 2,
      })
    if (path === '/billing/statistics' && method === 'GET')
      return reply({
        from: url.searchParams.get('from'),
        to: url.searchParams.get('to'),
        totals: url.searchParams.get('from')?.startsWith('2026-10-01')
          ? []
          : [
              {
                billingType: 'SUBSCRIPTION',
                currency: 'CNY',
                totalAmount: '710.00000000',
                allocatedAmount: '710.00000000',
                unallocatedAmount: '0',
                totalTokens: 1000000,
              },
              {
                billingType: 'API_KEY',
                currency: 'USD',
                totalAmount: '5.12500000',
                allocatedAmount: '5.12500000',
                unallocatedAmount: '0',
                totalTokens: 2500000,
              },
            ],
        items: url.searchParams.get('from')?.startsWith('2026-10-01')
          ? []
          : [
              {
                principalId: longID,
                principalName: '林知远',
                principalType: 'MEMBER',
                billingType: 'SUBSCRIPTION',
                currency: 'CNY',
                amount: '497.00000000',
                tokens: 700000,
              },
            ],
        total: url.searchParams.get('from')?.startsWith('2026-10-01') ? 0 : 1,
      })
    if (path === '/billing/documents' && method === 'GET')
      return reply({
        items: [
          {
            id: '602',
            billingType: 'SUBSCRIPTION',
            documentType: 'ADJUSTMENT',
            status: 'CONFIRMED',
            credentialId: '88',
            credentialName: '个人订阅主账号',
            originalDocumentId: '601',
            periodStart: '2026-09-01T00:00:00Z',
            periodEnd: '2026-10-01T00:00:00Z',
            totalTokens: 0,
            totalAmount: '-200.00000000',
            currency: 'CNY',
            createdAt: stamp,
          },
          {
            id: '601',
            billingType: 'SUBSCRIPTION',
            documentType: 'CHARGE',
            status: 'CONFIRMED',
            credentialId: '88',
            credentialName: '个人订阅主账号',
            originalDocumentId: null,
            periodStart: '2026-09-01T00:00:00Z',
            periodEnd: '2026-10-01T00:00:00Z',
            totalTokens: 1000000,
            totalAmount: '1000.00000000',
            currency: 'CNY',
            createdAt: stamp,
          },
        ],
        nextCursor: null,
        total: 2,
      })
    if (path === '/billing/documents/602' && method === 'GET')
      return reply({
        id: '602',
        billingType: 'SUBSCRIPTION',
        documentType: 'ADJUSTMENT',
        status: 'CONFIRMED',
        credentialId: '88',
        credentialName: '个人订阅主账号',
        originalDocumentId: '601',
        periodStart: '2026-09-01T00:00:00Z',
        periodEnd: '2026-10-01T00:00:00Z',
        totalTokens: 0,
        totalAmount: '-200.00000000',
        currency: 'CNY',
        createdAt: stamp,
        original: {
          id: '601',
          billingType: 'SUBSCRIPTION',
          documentType: 'CHARGE',
          status: 'CONFIRMED',
          credentialId: '88',
          credentialName: '个人订阅主账号',
          originalDocumentId: null,
          periodStart: '2026-09-01T00:00:00Z',
          periodEnd: '2026-10-01T00:00:00Z',
          totalTokens: 1000000,
          totalAmount: '1000.00000000',
          currency: 'CNY',
          createdAt: stamp,
        },
        adjustments: [],
        ratingCount: 0,
        items: [
          {
            id: '701',
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            usageTokens: 0,
            allocationRatio: '0.7000000000000000',
            amount: '-140.00000000',
          },
        ],
      })
    if (path === '/billing/unrated-usage' && method === 'GET')
      return reply({
        items: [
          {
            usageRecordId: '801',
            principalId: longID,
            principalName: '林知远',
            principalType: 'MEMBER',
            credentialId: '89',
            credentialName: 'DeepSeek API Key',
            providerModelId: '92',
            startedAt: stamp,
            inputTokens: null,
            cachedInputTokens: null,
            outputTokens: 5,
            reason: 'INCOMPLETE_TOKENS',
            waitingSince: stamp,
          },
        ],
        nextCursor: null,
        total: 1,
      })
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
    const pageReply = (items: any[]) => reply({ items, nextCursor: null, total: items.length })
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
    if (path === '/members/suggestions' && method === 'GET') {
      memberSuggestionQueries.push(new URLSearchParams(url.searchParams))
      const name = url.searchParams.get('name') || ''
      return pageReply(members.filter((member) => member.name.includes(name)))
    }
    if (path === '/token-quotas' && method === 'GET') {
      const scopeType = url.searchParams.get('scopeType')
      const ids = (url.searchParams.get('scopeIds') || '').split(',').filter(Boolean)
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
              level:
                used >= limit ? 'EXHAUSTED' : used * 100n >= limit * 80n ? 'WARNING' : 'NOTICE',
              periodStart: '2026-09-01T00:00:00Z',
              periodEnd: '2026-10-01T00:00:00Z',
            },
          ]
        }),
      )
    }
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
            subscriptionPrice: credentialPrices.get(row.id)?.subscriptionPrice ?? null,
          })),
        )
      if (path === '/operation-logs') return pageReply(operations)
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
        ok: !failedTest,
        code: failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
        httpStatus: failedTest ? 401 : 200,
        latencyMs: 160,
        discovered: failedTest ? 0 : 2,
        created: failedTest ? 0 : 1,
        updated: 0,
        mapped: failedTest ? 0 : 1,
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
          modelSyncSupported:
            code === 'deepseek-official' || code === 'qwen-official' || code === 'xai-official',
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
    if (method === 'POST' && segments.length === 1) {
      const id = next()
      const { modelIds = [], groupIds = [], mappings = [], ...fields } = body
      const subscriptionAccountRef =
        path === '/resources' &&
        body.authType === 'SUBSCRIPTION' &&
        body.authAdapter === 'OPENAI_CODEX'
          ? JSON.parse(body.credential).tokens.account_id
          : null
      const row = {
        ...fields,
        id,
        name: subscriptionAccountRef
          ? `${fields.name} · ${subscriptionAccountRef.slice(-6)}`
          : fields.name,
        ...(path === '/groups' ? { code: body.code || `group-${id}` } : {}),
        status:
          path === '/members' || path === '/models' || path === '/providers'
            ? 'DISABLED'
            : 'ACTIVE',
        createdAt: stamp,
        updatedAt: stamp,
        credentialConfigured: path === '/resources',
        runtimeStatus: path === '/resources' ? 'HEALTHY' : undefined,
        blockedReason: null,
        blockedAt: null,
        lastErrorAt: null,
        lastHttpStatus: null,
        lastErrorCode: null,
        modelSyncSupported: path === '/providers' ? false : undefined,
        authAdapters: path === '/providers' ? ['API_KEY'] : undefined,
        publisherProviderName:
          path === '/models'
            ? (providers.find((provider) => provider.id === body.publisherProviderId)?.name ?? null)
            : undefined,
        ...(path === '/resources'
          ? {
              authType: body.authType || 'API_KEY',
              authAdapter: body.authAdapter || 'API_KEY',
              subscriptionType: body.authType === 'SUBSCRIPTION' ? 'PERSONAL' : null,
              planCode: body.authType === 'SUBSCRIPTION' ? 'plus' : null,
              externalAccountRef: subscriptionAccountRef,
              priority: body.priority ?? 100,
              effectiveAt: body.effectiveAt ?? null,
              expiresAt: body.expiresAt ?? null,
              quotaStatus: 'UNKNOWN',
              quotaCheckedAt: null,
              quotaResetsAt: null,
            }
          : {}),
      }
      if (conflict) return reply(null, 409, 'CONFLICT')
      const target =
        path === '/members'
          ? members
          : path === '/groups'
            ? groups
            : path === '/models'
              ? models
              : path === '/providers'
                ? providers
                : resources
      if (
        path === '/resources' &&
        row.authType === 'SUBSCRIPTION' &&
        row.externalAccountRef &&
        resources.some(
          (resource) =>
            resource.providerId === row.providerId &&
            resource.authAdapter === row.authAdapter &&
            resource.externalAccountRef === row.externalAccountRef,
        )
      )
        return reply(null, 409, 'SUBSCRIPTION_ACCOUNT_ALREADY_EXISTS')
      target.push(row)
      if (path === '/groups') {
        relationships.set(`groups/${id}/models`, new Set(modelIds))
      }
      if (path === '/providers') {
        providerInputs.push(structuredClone(body))
        Object.assign(row, safeProviderProxy(body))
        providerMappings.set(
          id,
          mappings.map((mapping: any) => ({
            ...mapping,
            id: next(),
            providerId: id,
            createdAt: stamp,
            updatedAt: stamp,
          })),
        )
      }
      if (path === '/members') {
        for (const group of groups) {
          const relation = relationships.get(`groups/${group.id}/members`) || new Set<string>()
          relationships.set(`groups/${group.id}/members`, relation)
          if (groupIds.includes(group.id)) relation.add(id)
        }
      }
      const { credential: _, ...safe } = row
      return reply(safe, 201)
    }
    if (segments[0] === 'models' && segments.length === 2 && method === 'PUT') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = models.find((model) => model.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      Object.assign(row, body, {
        publisherProviderName:
          providers.find((provider) => provider.id === body.publisherProviderId)?.name ?? null,
      })
      return reply(row)
    }
    if (segments[0] === 'providers' && segments.length === 2 && method === 'PUT') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = providers.find((provider) => provider.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      const { mappings = [], ...fields } = body
      providerInputs.push(structuredClone(body))
      const { proxyUrl: _, proxyHeaders: __, ...safeFields } = fields
      Object.assign(row, safeFields, safeProviderProxy(body))
      providerMappings.set(
        row.id,
        mappings.map((mapping: any) => ({
          ...mapping,
          id: next(),
          providerId: row.id,
          createdAt: stamp,
          updatedAt: stamp,
        })),
      )
      return reply(row)
    }
    if (segments[0] === 'groups' && segments.length === 2 && method === 'PUT') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = groups.find((group) => group.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      const { modelIds = [], ...fields } = body
      Object.assign(row, fields)
      relationships.set(`groups/${row.id}/models`, new Set(modelIds))
      return reply(row)
    }
    if (
      (segments[0] === 'members' || segments[0] === 'groups') &&
      segments[2] === 'token-quota' &&
      segments[3] === 'add' &&
      method === 'POST'
    ) {
      const source = segments[0] === 'groups' ? groups : members
      const row = source.find((entry) => entry.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      row.monthlyTokenLimit = String(BigInt(row.monthlyTokenLimit || '0') + BigInt(body.amount))
      return reply(row)
    }
    if (
      (segments[0] === 'members' || segments[0] === 'groups') &&
      segments[2] === 'token-quota' &&
      segments.length === 3 &&
      method === 'DELETE'
    ) {
      const source = segments[0] === 'groups' ? groups : members
      const row = source.find((entry) => entry.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      row.monthlyTokenLimit = null
      return reply(row)
    }
    if (segments[0] === 'members' && segments.length === 2 && method === 'PUT') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = members.find((member) => member.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      const { groupIds = [], ...fields } = body
      Object.assign(row, fields)
      for (const group of groups) {
        const relation = relationships.get(`groups/${group.id}/members`) || new Set<string>()
        relationships.set(`groups/${group.id}/members`, relation)
        if (groupIds.includes(group.id)) relation.add(row.id)
        else relation.delete(row.id)
      }
      return reply(row)
    }
    if (segments[0] === 'members' && segments.length === 2 && method === 'DELETE') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const index = members.findIndex((member) => member.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      members.splice(index, 1)
      return reply({ deleted: true })
    }
    if (segments[0] === 'groups' && segments.length === 2 && method === 'DELETE') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const index = groups.findIndex((group) => group.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      groups.splice(index, 1)
      relationships.delete(`groups/${segments[1]}/members`)
      relationships.delete(`groups/${segments[1]}/models`)
      return reply({ deleted: true })
    }
    if (segments[0] === 'models' && segments.length === 2 && method === 'DELETE') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const index = models.findIndex((model) => model.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      models.splice(index, 1)
      for (const [key, ids] of relationships) {
        if (key.endsWith('/models')) ids.delete(segments[1])
      }
      for (const mappings of providerMappings.values()) {
        for (let mappingIndex = mappings.length - 1; mappingIndex >= 0; mappingIndex--) {
          if (mappings[mappingIndex].modelId === segments[1]) mappings.splice(mappingIndex, 1)
        }
      }
      return reply({ deleted: true })
    }
    if (segments[0] === 'providers' && segments.length === 2 && method === 'DELETE') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const index = providers.findIndex((provider) => provider.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      providers.splice(index, 1)
      providerMappings.delete(segments[1])
      for (let resourceIndex = resources.length - 1; resourceIndex >= 0; resourceIndex--) {
        if (resources[resourceIndex].providerId === segments[1]) resources.splice(resourceIndex, 1)
      }
      return reply({ deleted: true })
    }
    if (segments[0] === 'resources' && segments.length === 2 && method === 'DELETE') {
      const index = resources.findIndex((resource) => resource.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      resources.splice(index, 1)
      return reply({ deleted: true })
    }
    if (segments[0] === 'resources' && segments.length === 2 && method === 'GET') {
      const resource = resources.find((candidate) => candidate.id === segments[1])
      if (!resource) return reply(null, 404, 'NOT_FOUND')
      const { credential: _, quotaDetails: __, ...safe } = resource
      return reply(safe)
    }
    if (segments[0] === 'resources' && segments[2] === 'prices') {
      if (priceEndpointUnavailable) return reply(null, 404, 'NOT_FOUND')
      const resource = resources.find((candidate) => candidate.id === segments[1])
      if (!resource) return reply(null, 404, 'NOT_FOUND')
      const prices = credentialPrices.get(resource.id) ?? {
        modelPrices: [],
        subscriptionPrices: [],
        subscriptionPrice: null,
      }
      if (segments.length === 3 && method === 'GET') return reply(prices)
      if (segments[3] === 'models' && segments.length === 5 && method === 'PUT') {
        const saved = {
          ...body,
          id: next(),
          providerCredentialId: resource.id,
          providerModelId: segments[4],
          effectiveAt: body.effectiveAt,
          createdAt: stamp,
        }
        prices.modelPrices = [saved, ...prices.modelPrices.filter((price) => price.id !== saved.id)]
        credentialPrices.set(resource.id, prices)
        return reply(saved)
      }
      if (segments[3] === 'models' && segments.length === 6 && method === 'PUT') {
        const index = prices.modelPrices.findIndex((price) => price.id === segments[5])
        if (index < 0) return reply(null, 404, 'NOT_FOUND')
        const saved = { ...prices.modelPrices[index], ...body }
        prices.modelPrices[index] = saved
        credentialPrices.set(resource.id, prices)
        return reply(saved)
      }
      if (segments[3] === 'models' && segments.length === 6 && method === 'DELETE') {
        const before = prices.modelPrices.length
        prices.modelPrices = prices.modelPrices.filter((price) => price.id !== segments[5])
        if (prices.modelPrices.length === before) return reply(null, 404, 'NOT_FOUND')
        credentialPrices.set(resource.id, prices)
        return reply({ deleted: true })
      }
      if (segments[3] === 'subscription' && segments.length === 4 && method === 'PUT') {
        const saved = {
          ...body,
          id: next(),
          providerCredentialId: resource.id,
          createdAt: stamp,
        }
        prices.subscriptionPrices = [saved, ...prices.subscriptionPrices]
        prices.subscriptionPrice = [...prices.subscriptionPrices]
          .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
          .find((price) => price.effectiveAt <= new Date().toISOString())
        credentialPrices.set(resource.id, prices)
        return reply(saved)
      }
      if (segments[3] === 'subscription' && segments.length === 5 && method === 'PUT') {
        const index = prices.subscriptionPrices.findIndex((price) => price.id === segments[4])
        if (index < 0) return reply(null, 404, 'NOT_FOUND')
        const saved = { ...prices.subscriptionPrices[index], ...body }
        prices.subscriptionPrices[index] = saved
        prices.subscriptionPrice = [...prices.subscriptionPrices]
          .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
          .find((price) => price.effectiveAt <= new Date().toISOString())
        credentialPrices.set(resource.id, prices)
        return reply(saved)
      }
      if (segments[3] === 'subscription' && segments.length === 5 && method === 'DELETE') {
        const before = prices.subscriptionPrices.length
        prices.subscriptionPrices = prices.subscriptionPrices.filter(
          (price) => price.id !== segments[4],
        )
        if (prices.subscriptionPrices.length === before) return reply(null, 404, 'NOT_FOUND')
        prices.subscriptionPrice = [...prices.subscriptionPrices]
          .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
          .find((price) => price.effectiveAt <= new Date().toISOString())
        credentialPrices.set(resource.id, prices)
        return reply({ deleted: true })
      }
    }
    if (segments[0] === 'providers' && segments[2] === 'activate' && method === 'POST') {
      providerActivationInputs.push(structuredClone(body))
      const provider = providers.find((candidate) => candidate.id === segments[1])
      const resource = resources.find((candidate) => candidate.id === body.resourceId)
      const mapping = (providerMappings.get(segments[1]) || []).find(
        (candidate) => candidate.id === body.providerModelMappingId,
      )
      const model = models.find((candidate) => candidate.id === mapping?.modelId)
      if (
        !provider ||
        !resource ||
        resource.providerId !== provider.id ||
        (resource.authType !== 'SUBSCRIPTION' && (!body.protocol || !mapping || !model))
      )
        return reply(null, 400, 'INVALID_ARGUMENT')
      if (!failedTest) provider.status = 'ACTIVE'
      return reply({
        ok: !failedTest,
        code: failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
        latencyMs: 12,
        providerModelMappingId: mapping?.id,
        testedModelId: model?.id,
        testedModelCode: mapping?.upstreamModelCode || model?.code,
      })
    }
    if (segments[2] === 'status') {
      const list =
        segments[0] === 'members'
          ? members
          : segments[0] === 'groups'
            ? groups
            : segments[0] === 'models'
              ? models
              : segments[0] === 'providers'
                ? providers
                : resources
      const row = list.find((x) => x.id === segments[1])
      if (
        segments[0] === 'members' &&
        body.status === 'ACTIVE' &&
        !keys.some((key) => key.memberID === segments[1])
      )
        return reply(null, 409, 'MEMBER_ACCESS_KEY_REQUIRED')
      row.status = body.status
      return reply(row)
    }
    if (segments[0] === 'members' && segments[2] === 'groups' && method === 'GET') {
      return pageReply(
        groups.filter((group) => relationships.get(`groups/${group.id}/members`)?.has(segments[1])),
      )
    }
    if (segments[0] === 'members' && segments[2] === 'keys') {
      if (method === 'GET') {
        const limit = Number(url.searchParams.get('limit') || '50')
        const after = url.searchParams.get('after')
        const matching = keys.filter(
          (key) =>
            key.memberID === segments[1] &&
            (url.searchParams.get('expiry') !== 'unexpired' ||
              (key.status === 'ACTIVE' &&
                !key.revokedAt &&
                (!key.expiresAt || Date.parse(key.expiresAt) > Date.now()))),
        )
        const items = matching
          .filter((key) => !after || BigInt(key.id) < BigInt(after))
          .sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1))
          .slice(0, limit)
          .map(({ key: _, memberID: __, ...safe }) => safe)
        return reply({
          items,
          nextCursor: items.length === limit ? items.at(-1)!.id : null,
          total: matching.length,
        })
      }
      expect(members.some((member) => member.id === segments[1])).toBe(true)
      const row = {
        ...body,
        id: next(),
        memberID: segments[1],
        key: 'vk-fixture-key-shown-once',
        maskedKey: 'vk-fixture1********ture',
        status: 'ACTIVE',
        createdAt: stamp,
        revokedAt: null,
      }
      keys.push(row)
      return reply(row, 201)
    }
    if (segments[0] === 'access-keys') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = keys.find((k) => k.id === segments[1])
      row.status = 'REVOKED'
      row.revokedAt = stamp
      return reply({ revoked: true })
    }
    if (segments[0] === 'groups' && segments.length >= 3) {
      if (conflict && (method === 'PUT' || method === 'DELETE')) return reply(null, 409, 'CONFLICT')
      const key = segments.slice(0, 3).join('/'),
        ids = relationships.get(key) || new Set<string>()
      relationships.set(key, ids)
      if (method === 'PUT') ids.add(segments[3])
      if (method === 'DELETE') ids.delete(segments[3])
      if (method === 'GET')
        return pageReply(
          (segments[2] === 'members' ? members : models).filter((x) => ids.has(x.id)),
        )
      return reply({ updated: true })
    }
    if (
      segments[0] === 'resources' &&
      segments[2] === 'credential' &&
      segments[3] === 'export' &&
      method === 'GET'
    ) {
      const resource = resources.find((candidate) => candidate.id === segments[1])
      if (!resource) return reply(null, 404, 'NOT_FOUND')
      if (
        resource.authType !== 'SUBSCRIPTION' ||
        resource.authAdapter !== 'OPENAI_CODEX' ||
        resource.subscriptionType !== 'PERSONAL'
      )
        return reply(null, 409, 'CREDENTIAL_EXPORT_UNSUPPORTED')
      expect(req.headers().authorization).toBe('Bearer fixture-admin-token')
      return route.fulfill({
        status: 200,
        contentType: 'application/octet-stream',
        headers: { 'Content-Disposition': 'attachment; filename="auth.json"' },
        body: resource.credential,
      })
    }
    if (segments[2] === 'credential') {
      resources.find((r) => r.id === segments[1]).credential = body.credential
      return reply({ updated: true })
    }
    if (segments[2] === 'quotas' && method === 'GET') {
      const resource = resources.find((candidate) => candidate.id === segments[1])
      if (!resource) return reply(null, 404, 'NOT_FOUND')
      return reply(resource.quotaDetails || [])
    }
    if (
      segments[2] === 'rate-limit-reset-credit' &&
      segments[3] === 'consume' &&
      method === 'POST'
    ) {
      const resource = resources.find((candidate) => candidate.id === segments[1])
      if (!resource) return reply(null, 404, 'NOT_FOUND')
      expect(body.idempotencyKey).toMatch(
        /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
      )
      expect(body.creditId).toBe('credit-1')
      resource.quotaStatus = 'AVAILABLE'
      resource.quotaDetails = (resource.quotaDetails || []).map((quota: any) => ({
        ...quota,
        usedPercent: 0,
        status: 'AVAILABLE',
      }))
      return reply({
        outcome: 'reset',
        resetCredits: { availableCount: 0, credits: [] },
      })
    }
    if (segments[2] === 'test-connection') {
      const resource = resources.find((candidate) => candidate.id === segments[1])
      const providerModelMappingId = url.searchParams.get('providerModelMappingId')
      const testedMapping = [...providerMappings.values()]
        .flat()
        .find((mapping) => mapping.id === providerModelMappingId)
      const testedModel = models.find((model) => model.id === testedMapping?.modelId)
      if (resource?.authType === 'SUBSCRIPTION' && !failedTest) {
        resource.quotaStatus = 'AVAILABLE'
        resource.quotaCheckedAt = stamp
        resource.quotaResetsAt = stamp
        resource.quotaDetails = [
          {
            code: 'codex.primary',
            name: null,
            status: 'AVAILABLE',
            unit: 'PERCENT',
            limitValue: null,
            usedValue: null,
            remainingValue: null,
            usedPercent: 25,
            windowDurationSeconds: 18000,
            resetsAt: stamp,
            reachedType: null,
            observedAt: stamp,
          },
          {
            code: 'codex.secondary',
            name: null,
            status: 'AVAILABLE',
            unit: 'PERCENT',
            limitValue: null,
            usedValue: null,
            remainingValue: null,
            usedPercent: 6,
            windowDurationSeconds: 604800,
            resetsAt: stamp,
            reachedType: null,
            observedAt: stamp,
          },
          {
            code: 'codex.spark.primary',
            name: 'Codex Spark',
            status: 'AVAILABLE',
            unit: 'PERCENT',
            limitValue: null,
            usedValue: null,
            remainingValue: null,
            usedPercent: 0,
            windowDurationSeconds: 604800,
            resetsAt: stamp,
            reachedType: null,
            observedAt: stamp,
          },
        ]
      }
      return reply({
        ok: !failedTest,
        code: failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
        httpStatus: failedTest ? 401 : 200,
        latencyMs: 140,
        providerModelMappingId: testedMapping?.id,
        testedModelId: testedModel?.id,
        testedModelCode: testedMapping?.upstreamModelCode || testedModel?.code,
        resetCredits:
          resource?.authAdapter === 'OPENAI_CODEX'
            ? {
                availableCount: 1,
                credits: [
                  {
                    id: 'credit-1',
                    resetType: 'codexRateLimits',
                    status: 'available',
                    grantedAt: stamp,
                    expiresAt: stamp,
                    title: 'Rate-limit reset',
                    description: 'Reset an eligible Codex rate-limit window.',
                  },
                ],
              }
            : undefined,
      })
    }
    return reply(null, 404, 'NOT_FOUND')
  })
  return {
    members,
    models,
    providers,
    providerMappings,
    credentialPrices,
    setPriceEndpointUnavailable(value: boolean) {
      priceEndpointUnavailable = value
    },
    providerInputs,
    providerActivationInputs,
    groups,
    resources,
    keys,
    relationships,
    groupQueries,
    modelQueries,
    providerQueries,
    memberListQueries,
    memberSuggestionQueries,
    usageQueries,
    statisticQueries,
    modelSyncRequests,
    providerInitializeRequests,
    activeModelRows,
    expire: () => {
      unauthorized = true
    },
    failTest: () => {
      failedTest = true
    },
    conflict: (on: boolean) => {
      conflict = on
    },
  }
}
async function signIn(page: Page, destination: 'home' | 'members' = 'members') {
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('fixture-password')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await expect(page.getByRole('heading', { name: '仪表盘', exact: true })).toBeVisible()
  if (destination === 'home') return
  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
}
const modal = (page: Page) => page.locator('dialog').last()
test('应用管理签发仅展示一次的 App Key', async ({ page }) => {
  const state = await fixture(page)
  state.groups.push({
    id: '51',
    code: 'engineering',
    name: '研发组',
    status: 'ACTIVE',
    createdAt: stamp,
  })
  const applications: any[] = []
  await page.route('**/api/v1/applications**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const reply = (data: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'OK', data }),
      })
    if (path === '/applications' && request.method() === 'GET')
      return reply({ items: applications, total: applications.length, nextCursor: null })
    if (path === '/applications' && request.method() === 'POST') {
      const input = request.postDataJSON()
      const created = {
        id: '71',
        name: input.name,
        remark: input.remark,
        status: 'DISABLED',
        createdAt: stamp,
      }
      applications.push(created)
      return reply(created, 201)
    }
    if (path === '/applications/71/keys' && request.method() === 'POST')
      return reply(
        {
          id: '81',
          key: 'ak-once-only-secret',
          maskedKey: 'ak-******',
          name: request.postDataJSON().name,
          expiresAt: null,
        },
        201,
      )
    if (path === '/applications/71/keys' && request.method() === 'GET') {
      const keys = [
        {
          id: '82',
          name: '已过期',
          maskedKey: 'ak-expired******',
          status: 'ACTIVE',
          expiresAt: '2020-01-01T00:00:00Z',
          revokedAt: null,
          createdAt: stamp,
        },
        {
          id: '81',
          name: '生产调用',
          maskedKey: 'ak-active******',
          status: 'ACTIVE',
          expiresAt: null,
          revokedAt: null,
          createdAt: stamp,
        },
        {
          id: '80',
          name: '已撤销',
          maskedKey: 'ak-revoked******',
          status: 'REVOKED',
          expiresAt: null,
          revokedAt: '2026-09-20T09:12:26Z',
          createdAt: stamp,
        },
      ]
      const items = url.searchParams.get('expiry') === 'unexpired' ? keys.slice(1, 2) : keys
      return reply({ items, total: items.length, nextCursor: null })
    }
    return reply(null, 404)
  })

  await signIn(page, 'home')
  const memberNavigation = page.getByRole('link', { name: '用户管理', exact: true })
  const applicationNavigation = page.getByRole('link', { name: '应用管理', exact: true })
  await expect(memberNavigation).toBeVisible()
  await expect(applicationNavigation).toBeVisible()
  await applicationNavigation.click()
  await expect(page).toHaveURL(/#\/applications$/)
  await expect(applicationNavigation).toHaveClass(/router-link-active/)
  await expect(memberNavigation).not.toHaveClass(/router-link-active/)
  await expect(page.getByRole('tab', { name: '应用管理' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '应用管理', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader').first()).toHaveText('ID')
  await page.getByRole('button', { name: '创建应用' }).click()
  const dialog = modal(page)
  await dialog.getByLabel('应用名称').fill('自动化服务')
  await dialog.getByRole('checkbox', { name: '选择分组 研发组' }).check()
  await dialog.getByRole('button', { name: '创建', exact: true }).click()
  const row = page.getByRole('row').filter({ hasText: '自动化服务' })
  await expect(row).toBeVisible()
  await row.hover()
  await expect(row.getByRole('cell').first()).toHaveCSS('background-color', 'rgb(248, 250, 252)')
  await expect(row.getByRole('cell').last()).toHaveCSS('background-color', 'rgb(248, 250, 252)')
  const applicationMoreActions = row.locator('summary')
  await expect(applicationMoreActions).toHaveAttribute('aria-label', '自动化服务 的更多操作')
  await expect(applicationMoreActions).toHaveText('⋮')
  await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
  const tableScroll = page.locator('.table-scroll').first()
  const scrollHeightBeforeMenu = await tableScroll.evaluate((element) => element.scrollHeight)
  await applicationMoreActions.click()
  const quotaAction = row.getByRole('button', { name: '增加配额', exact: true })
  const deleteAction = row.getByRole('button', { name: '删除', exact: true })
  await expect(quotaAction).toBeVisible()
  await expect(deleteAction).toBeVisible()
  const menuBox = (await row.locator('.row-action-more-menu').boundingBox())!
  const quotaActionBox = (await quotaAction.boundingBox())!
  const deleteActionBox = (await deleteAction.boundingBox())!
  expect(deleteActionBox.y).toBeGreaterThan(quotaActionBox.y)
  expect(quotaActionBox.x).toBeGreaterThanOrEqual(menuBox.x)
  expect(deleteActionBox.x + deleteActionBox.width).toBeLessThanOrEqual(menuBox.x + menuBox.width)
  expect(await tableScroll.evaluate((element) => element.scrollHeight)).toBe(scrollHeightBeforeMenu)
  await applicationMoreActions.press('Escape')
  await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
  await expect(row.getByRole('cell').first().locator('code')).toHaveText('71')
  await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({
    path: '../.cache/web-visual/application-id-first.png',
    animations: 'disabled',
  })
  await expect(row.getByRole('link', { name: '审计记录' })).toHaveCount(0)
  await row.getByRole('button', { name: 'App Key', exact: true }).click()
  await modal(page).getByLabel('Key 名称').fill('生产调用')
  await modal(page).getByRole('button', { name: '签发 App Key' }).click()
  await expect(modal(page).getByRole('textbox', { name: '完整 App Key 仅展示这一次' })).toHaveValue(
    'ak-once-only-secret',
  )
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
  await expect(page.getByText('ak-once-only-secret')).toHaveCount(0)
  await row.getByRole('button', { name: '查看 App Key 记录' }).click()
  const expiryFilter = modal(page).getByRole('combobox', { name: '查看范围' })
  await expect(expiryFilter).toHaveValue('unexpired')
  await expect(modal(page).locator('tbody tr td:first-child')).toHaveText(['生产调用'])
  await expiryFilter.selectOption('all')
  await expect(modal(page).locator('tbody tr td:first-child')).toHaveText([
    '已过期',
    '生产调用',
    '已撤销',
  ])
  await expect(
    modal(page).getByRole('row').filter({ hasText: '已撤销' }).locator('td').nth(2),
  ).toHaveText('-')
  await modal(page).getByRole('button', { name: '关闭' }).click()
  await memberNavigation.click()
  await expect(page).toHaveURL(/#\/members$/)
  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
  await page.goto('/#/applications')
  await expect(applicationNavigation).toHaveClass(/router-link-active/)
  await expect(page.getByRole('row').filter({ hasText: '自动化服务' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('.workspace')).toHaveCSS('margin-left', '0px')
  await page.getByRole('button', { name: '打开导航' }).click()
  await expect(memberNavigation).toBeVisible()
  await expect(applicationNavigation).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
test('仪表盘初始化向导依次高亮配置入口', async ({ page }) => {
  await fixture(page)
  await signIn(page, 'home')

  await page.getByRole('button', { name: '初始化向导', exact: true }).click()
  const guide = page.locator('.initialization-tour')
  await expect(guide).toBeVisible()
  await expect(guide.getByRole('heading', { name: '添加服务商' })).toBeVisible()
  await expect(guide).toHaveAttribute('data-target', 'provider')
  await expect(page.locator('#nav-providers')).toHaveClass(/initialization-tour-target/)
  await expect(page.locator('#nav-models')).not.toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'model')
  await expect(guide.getByRole('heading', { name: '添加模型' })).toBeVisible()
  await expect(page.locator('#nav-models')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'group')
  await expect(guide.getByRole('heading', { name: '创建用户分组' })).toBeVisible()
  await expect(page.locator('#nav-groups')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'member')
  await expect(guide.getByRole('heading', { name: '添加用户' })).toBeVisible()
  await expect(page.locator('#nav-members')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'usage')
  await expect(guide.getByRole('heading', { name: '查看用量分析' })).toBeVisible()
  await expect(guide.getByText('第 5 步，共 5 步', { exact: true })).toBeVisible()
  await expect(page.locator('#nav-usage')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '完成', exact: true }).click()
  await expect(guide).toHaveCount(0)
  await expect(page.locator('.initialization-tour-target')).toHaveCount(0)
})

test('首页展示本月指标、应用接入、配置脚本和分项排行榜', async ({ page }) => {
  const state = await fixture(page)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  state.resources.push({
    id: '88',
    name: 'DeepSeek Key',
    providerId: '81',
    status: 'ACTIVE',
    runtimeStatus: 'HEALTHY',
    blockedReason: null,
    blockedAt: null,
    lastErrorAt: null,
    lastHttpStatus: null,
    lastErrorCode: null,
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  const dashboardRequest = page.waitForRequest((request) =>
    new URL(request.url()).pathname.endsWith('/usage/dashboard'),
  )
  await signIn(page, 'home')

  const dashboardURL = new URL((await dashboardRequest).url())
  const expectedRange = await page.evaluate(() => {
    const now = new Date()
    const year = now.getUTCFullYear()
    const month = now.getUTCMonth()
    return {
      from: new Date(Date.UTC(year, month, 1)).toISOString(),
      to: new Date(Date.UTC(year, month + 1, 1)).toISOString(),
    }
  })
  expect(dashboardURL.searchParams.get('from')).toBe(expectedRange.from)
  expect(dashboardURL.searchParams.get('to')).toBe(expectedRange.to)

  await expect(page.locator('.token-total dd')).toHaveText('12.8K')
  await expect(page.locator('.token-total dd')).toHaveAttribute('title', '12,840')
  await expect(page.getByRole('heading', { name: '本月概览', exact: true })).toBeVisible()
  const expectedMonthLabel = await page.evaluate(() =>
    new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: 'long',
      timeZone: 'UTC',
    }).format(new Date()),
  )
  await expect(page.locator('.dashboard-section-head > .dashboard-period')).toHaveText(
    expectedMonthLabel,
  )
  await expect(page.getByText('Token 消耗', { exact: true })).toBeVisible()
  await expect(
    page.locator('.provider-total').getByRole('img', { name: '已启用服务商均可用' }),
  ).toHaveClass(/is-healthy/)
  expect(
    await page.locator('.provider-metric-label').evaluate((element) => {
      const label = element.querySelector('dt')!.getBoundingClientRect()
      const badge = element.querySelector('.provider-health-badge')!.getBoundingClientRect()
      return { above: badge.top < label.top, right: badge.left > label.right }
    }),
  ).toEqual({ above: true, right: true })
  await expect(page.getByText('最新统计', { exact: true })).toHaveCount(0)
  await expect(page.locator('.provider-health-alert')).toHaveCount(0)
  await expect(
    page.getByText('激活用户数').locator('..').getByText('2', { exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText('支持模型').locator('..').getByText('2', { exact: true }),
  ).toBeVisible()
  await expect(page.locator('.provider-total').getByText('1', { exact: true })).toBeVisible()
  await expect(page.getByText('掌握今日模型服务状态、Token 消耗与团队使用趋势。')).toHaveCount(0)
  await expect(page.getByText('当前已启用')).toHaveCount(0)
  await expect(page.getByText('输入 + 输出')).toHaveCount(0)
  expect(
    await page
      .locator('.metric-strip > div')
      .evaluateAll((elements) =>
        elements.every((element) => getComputedStyle(element).textAlign === 'center'),
      ),
  ).toBe(true)
  expect(
    await page
      .locator('.metric-strip')
      .evaluate(
        (element) =>
          getComputedStyle(element).gridTemplateColumns.split(' ').filter(Boolean).length,
      ),
  ).toBe(4)
  expect(
    await page
      .locator('.ranking-grid')
      .evaluate(
        (element) =>
          getComputedStyle(element).gridTemplateColumns.split(' ').filter(Boolean).length,
      ),
  ).toBe(3)
  await expect(page.getByText('用户 Token 消耗排行')).toBeVisible()
  await expect(page.getByText('模型请求排行')).toBeVisible()
  await expect(page.getByText('服务商调用排行')).toBeVisible()
  const activeModels = page.getByRole('region', { name: '当前模型服务商' })
  await expect(activeModels.getByText('3 个模型', { exact: true })).toBeVisible()
  await expect(activeModels.locator('.active-model-row')).toHaveCount(3)
  await expect(activeModels.getByText('Claude Sonnet 5', { exact: true })).toBeVisible()
  await expect(activeModels.getByText('claude-sonnet-5', { exact: true })).toBeVisible()
  await expect(activeModels.getByText('CC Vibe', { exact: true })).toBeVisible()
  await expect(activeModels.getByText('LingkeAI', { exact: true })).toHaveCount(0)
  await expect(activeModels.locator('.active-model-identity').first()).toHaveCSS('display', 'flex')
  await expect(activeModels.locator('.active-model-code')).toHaveCount(3)
  await expect(activeModels.locator('.active-provider-name')).toHaveCount(3)
  await expect(activeModels.locator('.active-provider-tag')).toHaveCount(0)
  await expect(page.locator('.access-panel').getByText('OpenAI', { exact: true })).toBeVisible()
  await expect(page.locator('.access-panel').getByText('Anthropic', { exact: true })).toBeVisible()
  await expect(page.getByText('适用于 Codex 等 OpenAI 兼容客户端')).toBeVisible()
  await expect(page.getByText('适用于 Claude Code 等 Anthropic 兼容客户端')).toBeVisible()
  await expect(page.getByText('林知远', { exact: true })).toBeVisible()
  await expect(
    page.locator('.model-ranking').getByText('DeepSeek V4 Flash', { exact: true }),
  ).toBeVisible()
  await expect(page.locator('.token-ranking [title="8,200 Token"]')).toHaveText('8.2K Token')
  const firstModelUsage = page.locator('.model-ranking .rank-label span').first()
  await expect(firstModelUsage).toHaveText('18 次 / 9.1K Token')
  await expect(firstModelUsage).toHaveAttribute('title', '18 次 / 9,100 Token')
  await expect(page.locator('.model-ranking')).not.toContainText('deepseek-v4-flash')
  const firstProviderUsage = page.locator('.provider-ranking .rank-label span').first()
  await expect(firstProviderUsage).toHaveText('20 次 / 9.2K Token')
  await expect(firstProviderUsage).toHaveAttribute('title', '20 次 / 9,200 Token')
  for (const [name, dimension] of [
    ['用户 Token 消耗排行', 'member'],
    ['模型请求排行', 'model'],
    ['服务商调用排行', 'provider'],
  ])
    await expect(page.getByRole('link', { name, exact: true })).toHaveAttribute(
      'href',
      new RegExp(`dimension=${dimension}`),
    )
  await expect(page.getByText(/\/v1$/, { exact: true })).toBeVisible()
  await expect(page.getByText(/\/anthropic$/, { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/home.png', fullPage: true })

  const accessPanel = page.getByRole('region', { name: '应用接入' })
  const openaiRow = accessPanel.locator('.access-addresses > div').filter({ hasText: 'OpenAI' })
  const openaiUrl = await openaiRow.locator('code').innerText()
  await openaiRow.getByRole('button', { name: '复制 OpenAI 接入地址' }).click()
  await expect(page.locator('.toast-success')).toContainText('已复制')
  await expect(accessPanel.getByRole('button', { name: '接入指南' })).toHaveCount(1)
  await accessPanel.getByRole('button', { name: '接入指南' }).click()
  await expect(modal(page).getByRole('heading', { name: '应用接入指南' })).toBeVisible()
  await expect(modal(page).getByRole('tab', { name: '使用脚本' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(modal(page).locator('.setup-step-number')).toHaveText(['1', '2'])
  await expect(modal(page).getByRole('heading', { name: '选择协议并复制配置脚本' })).toBeVisible()
  await expect(modal(page).getByRole('tab', { name: 'OpenAI' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(modal(page).locator('.setup-script').first()).toContainText(
    `export OPENAI_BASE_URL='${openaiUrl}'`,
  )
  await expect(modal(page).locator('.setup-script').first()).toContainText(
    'export OPENAI_API_KEY="$ZENTROLA_KEY"',
  )
  await expect(modal(page)).toContainText('输入过程不会回显')
  await expect(modal(page).locator('.setup-script')).toHaveCount(2)
  await modal(page).getByRole('button', { name: '复制 macOS / Linux 脚本' }).click()
  await expect(page.locator('.toast-success')).toContainText('已复制')
  await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
    `$env:OPENAI_BASE_URL = '${openaiUrl}'`,
  )
  await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
    '$env:OPENAI_API_KEY = $zentrolaKey',
  )
  await modal(page).getByRole('tab', { name: 'CC Switch' }).click()
  await expect(modal(page).getByRole('tab', { name: 'CC Switch' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(modal(page).locator('.setup-script').first()).toBeHidden()
  await expect(modal(page).getByRole('link', { name: '访问官网' })).toHaveAttribute(
    'href',
    'https://ccswitch.io/',
  )
  await expect(modal(page).getByText(/\.msi 安装包或 Portable \.zip/)).toBeVisible()
  await page.screenshot({ path: '../.cache/web-visual/home-setup-cc-switch.png', fullPage: true })
  await modal(page).getByRole('tab', { name: '使用脚本' }).click()
  await page.screenshot({ path: '../.cache/web-visual/home-setup.png', fullPage: true })
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()

  const anthropicRow = accessPanel
    .locator('.access-addresses > div')
    .filter({ hasText: 'Anthropic' })
  const anthropicUrl = await anthropicRow.locator('code').innerText()
  await accessPanel.getByRole('button', { name: '接入指南' }).click()
  await modal(page).getByRole('tab', { name: 'Anthropic' }).click()
  await expect(modal(page).getByRole('tab', { name: 'Anthropic' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(modal(page).locator('.setup-script').first()).toContainText(
    `export ANTHROPIC_BASE_URL='${anthropicUrl}'`,
  )
  await expect(modal(page).locator('.setup-script').first()).toContainText(
    'export ANTHROPIC_API_KEY="$ZENTROLA_KEY"',
  )
  await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
    `$env:ANTHROPIC_BASE_URL = '${anthropicUrl}'`,
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(modal(page)).toBeVisible()
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true)
  await page.screenshot({ path: '../.cache/web-visual/home-setup-mobile.png', fullPage: true })
})

test('没有当前模型服务商时隐藏整个区块', async ({ page }) => {
  const state = await fixture(page)
  state.activeModelRows.splice(0)
  const activeModelsResponse = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith('/gateway/active-models'),
  )
  await signIn(page, 'home')
  await activeModelsResponse

  await expect(page.getByRole('region', { name: '当前模型服务商' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '用户 Token 消耗排行' })).toBeVisible()
})

test('当前模型服务商在大量记录时限制卡片高度并内部滚动', async ({ page }) => {
  const state = await fixture(page)
  state.activeModelRows.splice(
    0,
    state.activeModelRows.length,
    ...Array.from({ length: 100 }, (_, index) => ({
      modelName: `Model ${index + 1}`,
      modelCode: `model-${String(index + 1).padStart(3, '0')}`,
      providerName: `Provider ${index + 1}`,
    })),
  )
  await signIn(page, 'home')

  const activeModels = page.getByRole('region', { name: '当前模型服务商' })
  await expect(activeModels.getByText('100 个模型', { exact: true })).toBeVisible()
  await expect(activeModels.locator('.active-model-row')).toHaveCount(100)
  const dimensions = await activeModels.locator('.active-models-table-wrap').evaluate((element) => {
    const style = getComputedStyle(element)
    return {
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      maxHeight: style.maxHeight,
      overflowY: style.overflowY,
      headerPosition: getComputedStyle(element.querySelector('th')!).position,
    }
  })
  expect(dimensions.maxHeight).toBe('280px')
  expect(dimensions.overflowY).toBe('auto')
  expect(dimensions.headerPosition).toBe('sticky')
  expect(dimensions.clientHeight).toBeLessThanOrEqual(280)
  expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight)
  await page.screenshot({ path: '../.cache/web-visual/home-active-models-100.png', fullPage: true })
})

test('凭证内可按生效时间保留 API Key 单价历史并修改订阅费用', async ({ page }) => {
  const state = await fixture(page)
  state.resources.push(
    {
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    },
    {
      id: '89',
      providerId: '81',
      name: 'DeepSeek 订阅',
      authType: 'SUBSCRIPTION',
      authAdapter: 'OPENAI_CODEX',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    },
  )
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
  await modal(page)
    .getByRole('row')
    .filter({ hasText: 'DeepSeek API Key' })
    .getByRole('button', { name: '计费设置' })
    .click()
  await expect(modal(page).getByRole('heading', { name: '凭证计费设置' })).toBeVisible()
  await modal(page).getByRole('button', { name: '配置价格' }).click()
  let priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
  await expect(priceDialog).toBeVisible()
  await priceDialog.getByRole('button', { name: '取消' }).click()
  await expect(priceDialog).toBeHidden()
  await expect(modal(page).getByRole('heading', { name: '凭证计费设置' })).toBeVisible()
  await modal(page).getByRole('button', { name: '配置价格' }).click()
  priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
  await expect(priceDialog.getByLabel('币种')).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '普通输入' })).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '输出' })).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '缓存输入' })).toHaveValue('')
  await expect(priceDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
  await priceDialog.getByLabel('币种').selectOption('CNY')
  await priceDialog.getByRole('textbox', { name: '普通输入' }).fill('1.25')
  await priceDialog.getByRole('textbox', { name: '输出' }).fill('2.5')
  await priceDialog.getByRole('textbox', { name: '缓存输入' }).fill('0.5')
  await priceDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
  await priceDialog.getByRole('button', { name: '保存' }).click()
  await expect(priceDialog).toBeHidden()
  await expect(modal(page)).toContainText('¥ 1.25 / 2.5 / 0.5')
  await modal(page).getByRole('button', { name: '新增价格' }).click()
  priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
  await expect(priceDialog.getByLabel('币种')).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '普通输入' })).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '输出' })).toHaveValue('')
  await expect(priceDialog.getByRole('textbox', { name: '缓存输入' })).toHaveValue('')
  await expect(priceDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
  await priceDialog.getByLabel('币种').selectOption('CNY')
  await priceDialog.getByRole('textbox', { name: '普通输入' }).fill('1.25')
  await priceDialog.getByRole('textbox', { name: '输出' }).fill('3')
  await priceDialog.getByRole('textbox', { name: '缓存输入' }).fill('0.5')
  await priceDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
  await priceDialog.getByRole('button', { name: '保存' }).click()
  expect(state.credentialPrices.get('88')?.modelPrices[0].outputPrice).toBe('3')
  expect(state.credentialPrices.get('88')?.modelPrices).toHaveLength(2)
  await expect(modal(page).getByLabel('价格历史').locator('.price-history-item')).toHaveCount(2)
  await expect(modal(page).getByText('未配置', { exact: true })).toHaveCount(0)
  await expect(modal(page).getByLabel('价格历史')).not.toContainText('08:00')
  await expect(modal(page).getByText('¥ 1.25 / 2.5 / 0.5', { exact: true })).toBeVisible()
  const olderPrice = modal(page).getByLabel('价格历史').locator('.price-history-item').nth(1)
  await olderPrice.getByRole('button', { name: '修改' }).click()
  priceDialog = page.getByRole('dialog', { name: '修改 DeepSeek V4 Flash 的价格' })
  await expect(priceDialog).toBeVisible()
  await priceDialog.getByRole('textbox', { name: '输出' }).fill('2.75')
  await priceDialog.getByRole('button', { name: '保存' }).click()
  await expect(modal(page).getByText('¥ 1.25 / 2.75 / 0.5', { exact: true })).toBeVisible()
  await modal(page)
    .getByLabel('价格历史')
    .locator('.price-history-item')
    .nth(1)
    .getByRole('button', { name: '删除' })
    .click()
  await page.getByRole('dialog', { name: '删除价格' }).getByRole('button', { name: '删除' }).click()
  await expect(modal(page).getByLabel('价格历史').locator('.price-history-item')).toHaveCount(1)

  await modal(page).getByRole('button', { name: '返回凭证' }).click()
  await modal(page)
    .getByRole('row')
    .filter({ hasText: 'DeepSeek 订阅' })
    .getByRole('button', { name: '计费设置' })
    .click()
  await expect(modal(page).getByText('价格列表', { exact: true })).toBeVisible()
  await expect(modal(page).locator('.price-subscription-empty')).toHaveText('-')
  await modal(page).getByRole('button', { name: '配置订阅费用' }).click()
  let subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
  await expect(subscriptionDialog).toBeVisible()
  await subscriptionDialog.getByRole('button', { name: '取消' }).click()
  await expect(subscriptionDialog).toBeHidden()
  await modal(page).getByRole('button', { name: '配置订阅费用' }).click()
  subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
  await expect(subscriptionDialog.getByLabel('计费周期起始日（UTC）')).toHaveCount(0)
  await expect(subscriptionDialog.getByLabel('币种')).toHaveValue('')
  await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('')
  await expect(subscriptionDialog.getByLabel('计费周期')).toHaveValue('')
  await expect(subscriptionDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
  await subscriptionDialog.getByLabel('币种').selectOption('CNY')
  await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('100')
  await subscriptionDialog.getByLabel('计费周期').selectOption('MONTH')
  await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
  await subscriptionDialog.getByRole('button', { name: '保存' }).click()
  await expect(subscriptionDialog).toBeHidden()
  expect(state.credentialPrices.get('89')?.subscriptionPrice.periodAmount).toBe('100')
  await modal(page).getByRole('button', { name: '新增价格' }).click()
  subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
  await expect(subscriptionDialog.getByLabel('币种')).toHaveValue('')
  await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('')
  await expect(subscriptionDialog.getByLabel('计费周期')).toHaveValue('')
  await expect(subscriptionDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
  await subscriptionDialog.getByLabel('币种').selectOption('CNY')
  await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('100')
  await subscriptionDialog.getByLabel('计费周期').selectOption('MONTH')
  await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
  await subscriptionDialog.getByRole('button', { name: '保存' }).click()
  await expect(subscriptionDialog.getByRole('alert')).toHaveText(
    '该生效日期已存在，请选择其他日期。',
  )
  expect(state.credentialPrices.get('89')?.subscriptionPrices).toHaveLength(1)
  await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('120')
  await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
  await subscriptionDialog.getByRole('button', { name: '保存' }).click()
  expect(state.credentialPrices.get('89')?.subscriptionPrices).toHaveLength(2)
  const subscriptionHistory = modal(page).getByLabel('价格历史')
  await expect(subscriptionHistory.locator('.price-history-item')).toHaveCount(2)
  const olderSubscriptionPrice = subscriptionHistory.locator('.price-history-item').nth(1)
  await olderSubscriptionPrice.getByRole('button', { name: '修改' }).click()
  subscriptionDialog = page.getByRole('dialog', { name: '修改「DeepSeek 订阅」的订阅费用' })
  await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('100')
  await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('110')
  await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
  await subscriptionDialog.getByRole('button', { name: '保存' }).click()
  await expect(subscriptionDialog.getByRole('alert')).toHaveText(
    '该生效日期已存在，请选择其他日期。',
  )
  await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
  await subscriptionDialog.getByRole('button', { name: '保存' }).click()
  expect(state.credentialPrices.get('89')?.subscriptionPrice.periodAmount).toBe('120')
  await subscriptionHistory
    .locator('.price-history-item')
    .nth(1)
    .getByRole('button', { name: '删除' })
    .click()
  await page
    .getByRole('dialog', { name: '删除订阅费用' })
    .getByRole('button', { name: '删除' })
    .click()
  await expect(subscriptionHistory.locator('.price-history-item')).toHaveCount(1)
  await modal(page).getByRole('button', { name: '返回凭证' }).click()
  const subscriptionRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek 订阅' })
  await expect(subscriptionRow).toContainText('当前订阅费用 · ¥120 / 月')
  await expect(subscriptionRow).toContainText('生效日期（UTC） · 2026-10-02')
})

test('价格接口未更新时不误报模型映射缺失', async ({ page }) => {
  const state = await fixture(page)
  state.setPriceEndpointUnavailable(true)
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
  await modal(page)
    .getByRole('row')
    .filter({ hasText: 'DeepSeek API Key' })
    .getByRole('button', { name: '计费设置' })
    .click()

  await expect(modal(page)).toContainText('凭证仍然存在，但价格接口不可用')
  await expect(modal(page)).toContainText('DeepSeek V4 Flash')
  await expect(modal(page)).not.toContainText('该服务商尚未配置模型映射')
  await expect(modal(page).getByRole('button', { name: '配置价格' })).toBeDisabled()
})

test('价格历史较多时默认折叠并可独立展开', async ({ page }) => {
  const state = await fixture(page)
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.credentialPrices.set('88', {
    modelPrices: Array.from({ length: 6 }, (_, index) => ({
      id: String(900 + index),
      providerCredentialId: '88',
      providerModelId: '92',
      currency: 'CNY',
      inputPrice: '5',
      outputPrice: '4',
      cachedInputPrice: String(index + 1),
      effectiveAt: `2026-09-${String(index + 1).padStart(2, '0')}T00:00:00Z`,
      createdAt: stamp,
    })),
    subscriptionPrices: [],
    subscriptionPrice: null,
  })
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
  await modal(page)
    .getByRole('row')
    .filter({ hasText: 'DeepSeek API Key' })
    .getByRole('button', { name: '计费设置' })
    .click()

  await expect(modal(page).getByLabel('价格历史')).toHaveCount(0)
  await modal(page).getByRole('button', { name: '展开 DeepSeek V4 Flash 的价格' }).click()
  const history = modal(page).getByLabel('价格历史')
  await expect(history.locator('.price-history-item')).toHaveCount(3)
  await modal(page).getByRole('button', { name: '查看全部 6 条价格' }).click()
  await expect(history.locator('.price-history-item')).toHaveCount(6)
  await expect(history).toHaveCSS('max-height', '320px')
  await modal(page).getByRole('button', { name: '收起价格历史' }).click()
  await expect(history.locator('.price-history-item')).toHaveCount(3)
})

test('首页在启用服务商不可用时提醒管理员', async ({ page }) => {
  const state = await fixture(page)
  state.resources.push({
    id: '88',
    name: 'DeepSeek Key',
    providerId: '81',
    status: 'ACTIVE',
    runtimeStatus: 'BLOCKED',
    blockedReason: 'BILLING',
    blockedAt: stamp,
    lastErrorAt: stamp,
    lastHttpStatus: 402,
    lastErrorCode: 'UPSTREAM_BILLING_BLOCKED',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })

  await signIn(page, 'home')

  await expect(
    page.locator('.provider-total').getByRole('img', { name: '已启用服务商均不可用' }),
  ).toHaveClass(/is-unavailable/)
  const alert = page.locator('.provider-health-alert')
  await expect(alert).toContainText('DeepSeek 当前不可用')
  await expect(alert).toContainText('余额或计费异常 · HTTP 402')
  await expect(alert.getByRole('link', { name: '查看服务商' })).toHaveAttribute(
    'href',
    '#/providers?runtimeStatus=ABNORMAL',
  )
})

test('首页用黄色角标表示部分启用服务商不可用', async ({ page }) => {
  const state = await fixture(page)
  state.providers.push({
    id: '82',
    name: 'Anthropic',
    code: 'anthropic-official',
    type: 'OFFICIAL',
    status: 'ACTIVE',
    website: null,
    endpoints: [{ protocolType: 'ANTHROPIC', baseUrl: 'https://api.anthropic.com' }],
    proxyEnabled: false,
    proxyUrl: null,
    proxyHeaders: [],
    modelSyncSupported: false,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.resources.push(
    {
      id: '88',
      name: 'DeepSeek Key',
      providerId: '81',
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      blockedReason: null,
      blockedAt: null,
      lastErrorAt: null,
      lastHttpStatus: null,
      lastErrorCode: null,
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    },
    {
      id: '89',
      name: 'Anthropic Key',
      providerId: '82',
      status: 'ACTIVE',
      runtimeStatus: 'BLOCKED',
      blockedReason: 'AUTHENTICATION',
      blockedAt: stamp,
      lastErrorAt: stamp,
      lastHttpStatus: 401,
      lastErrorCode: 'UPSTREAM_AUTH_FAILED',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    },
  )

  await signIn(page, 'home')

  await expect(
    page.locator('.provider-total').getByRole('img', {
      name: '1 / 2 个已启用服务商可用',
    }),
  ).toHaveClass(/is-degraded/)
  await expect(page.locator('.provider-health-alert')).toContainText('Anthropic 当前不可用')
  await page.getByRole('link', { name: '查看服务商' }).click()
  const runtimeFilter = page.getByRole('combobox', { name: '运行状态' })
  const deepSeekName = page.locator('tbody .person strong', { hasText: /^DeepSeek$/ })
  const anthropicName = page.locator('tbody .person strong', { hasText: /^Anthropic$/ })
  await expect(runtimeFilter).toHaveValue('ABNORMAL')
  await expect(anthropicName).toBeVisible()
  await expect(deepSeekName).toHaveCount(0)

  await runtimeFilter.selectOption('HEALTHY')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(deepSeekName).toBeVisible()
  await expect(anthropicName).toHaveCount(0)
})

test('操作日志详情展示原始 JSON、差异高亮和追踪信息', async ({ page }) => {
  await fixture(page)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await signIn(page, 'home')
  await page.getByRole('link', { name: '操作日志', exact: true }).click()
  await expect(page.getByRole('searchbox', { name: '请输入操作人名称' })).toHaveAttribute(
    'placeholder',
    '请输入操作人名称',
  )

  const operationRow = page.getByRole('row').filter({ hasText: '服务商状态变更' })
  await expect(operationRow.locator('td').nth(3)).toContainText('服务商')
  await expect(operationRow.locator('.operation-target-name')).toHaveText('测试服务商')
  await expect(operationRow.locator('.operation-target-id')).toHaveText('服务商 · ID 81')
  await operationRow.getByRole('button', { name: '详情', exact: true }).click()

  const dialog = modal(page)
  await expect(dialog.getByRole('heading', { name: '日志详情', exact: true })).toBeVisible()
  const before = dialog.locator('section[aria-label="修改前"]')
  const after = dialog.locator('section[aria-label="修改后"]')
  await expect(before.locator('code')).toContainText('"status": "ACTIVE"')
  await expect(after.locator('code')).toContainText('"status": "DISABLED"')
  await expect(before.locator('.json-line.changed')).toContainText('"status": "ACTIVE"')
  await expect(after.locator('.json-line.changed')).toContainText('"status": "DISABLED"')
  await expect(before.getByRole('button', { name: /复制/ })).toHaveCount(0)
  await expect(after.getByRole('button', { name: /复制/ })).toHaveCount(0)
  const requestID = dialog.locator('.operation-trace dd').last()
  await expect(requestID).toHaveText('req_provider_status')
  await expect(dialog.getByText('PROVIDER_STATUS_CHANGE', { exact: true })).toHaveCount(0)
  await expect(dialog.getByText('admin', { exact: true })).toHaveCount(0)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/operation-diff.png', fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect
    .poll(async () => {
      const beforeBox = await before.boundingBox()
      const afterBox = await after.boundingBox()
      return Boolean(beforeBox && afterBox && afterBox.y > beforeBox.y)
    })
    .toBe(true)
  await page.screenshot({ path: '../.cache/web-visual/operation-diff-mobile.png', fullPage: true })
})

test('操作日志单侧快照显示为日志内容且登录失败隐藏内部状态快照', async ({ page }) => {
  await fixture(page)
  await signIn(page, 'home')
  await page.getByRole('link', { name: '操作日志', exact: true }).click()

  const connectionRow = page.getByRole('row').filter({ hasText: '测试服务商连接' })
  await connectionRow.getByRole('button', { name: '详情', exact: true }).click()
  await expect(modal(page).locator('section[aria-label="日志内容"]')).toBeVisible()
  await expect(modal(page).getByRole('heading', { name: '修改后', exact: true })).toHaveCount(0)
  await expect(modal(page).getByRole('button', { name: /复制/ })).toHaveCount(0)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/operation-log-content.png', fullPage: true })
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

  const loginRow = page.getByRole('row').filter({ hasText: '登录失败' })
  await loginRow.getByRole('button', { name: '详情', exact: true }).click()
  await expect(modal(page).locator('.json-snapshot')).toHaveCount(0)
  await expect(modal(page)).not.toContainText('failedLoginCount')
  await expect(modal(page)).not.toContainText('lockedUntil')
  await expect(modal(page)).toContainText('账号或密码错误')
  await expect(modal(page).getByText('req_login_failed', { exact: true })).toBeVisible()
  await page.screenshot({ path: '../.cache/web-visual/operation-login-failed.png', fullPage: true })
})

test('成员列表按需查看 Key 并处理删除和失败恢复', async ({ page }) => {
  const state = await fixture(page)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  state.keys.push(
    {
      id: '801',
      memberID: longID,
      name: '工作站',
      maskedKey: 'vk-work1234********1234',
      status: 'ACTIVE',
      expiresAt: null,
      revokedAt: null,
      createdAt: '2019-01-01T00:00:00Z',
    },
    {
      id: '802',
      memberID: longID,
      name: '临时测试',
      maskedKey: 'zt_vk_temp********temp',
      status: 'ACTIVE',
      expiresAt: '2020-01-01T00:00:00Z',
      revokedAt: null,
      createdAt: '2019-02-01T00:00:00Z',
    },
    {
      id: '803',
      memberID: longID,
      name: '已撤销的 Key',
      maskedKey: 'vk-revoked********1234',
      status: 'REVOKED',
      expiresAt: null,
      revokedAt: '2026-09-20T09:12:26Z',
      createdAt: '2019-03-01T00:00:00Z',
    },
  )
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '林知远' })
  await expect(page.getByRole('columnheader').first()).toHaveText('ID')
  await expect(page.getByRole('button', { name: '管理 Key' })).toHaveCount(0)
  await expect(page.getByRole('columnheader', { name: '访问密钥', exact: true })).toHaveCount(0)
  await expect(row.getByRole('cell').first().locator('code')).toHaveText(longID)
  await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
  expect(
    await row
      .getByRole('cell')
      .first()
      .locator('code')
      .evaluate((element) => {
        return element.scrollWidth > element.clientWidth
      }),
  ).toBe(true)
  await expect(row.getByRole('cell').first().locator('.technical-value')).toHaveAttribute(
    'title',
    longID,
  )
  await expect(row).not.toContainText('vk-work1234')
  await expect(row).not.toContainText('zt_vk_temp')
  await expect(row.getByText('工作站', { exact: true })).toHaveCount(0)
  await expect(row).not.toContainText('长期有效')
  await expect(row).not.toContainText('2020年1月1日')
  await expect(row).not.toContainText('已过期')
  await expect(row.getByRole('button', { name: '撤销', exact: true })).toHaveCount(0)
  const viewKeys = row.getByRole('button', { name: '查看 Key 记录', exact: true })
  await expect(viewKeys.locator('svg')).toHaveCount(1)
  await viewKeys.click()
  await expect(modal(page)).toHaveAccessibleName('林知远的 Key 记录')
  await expect(modal(page)).toContainText('完整 Key 仅在分配成功时展示一次')
  const expiryFilter = modal(page).getByRole('combobox', { name: '查看范围' })
  await expect(expiryFilter).toHaveValue('unexpired')
  await expect(modal(page).locator('tbody tr td:first-child')).toHaveText(['工作站'])
  await expiryFilter.selectOption('all')
  await expect(modal(page).locator('tbody tr td:first-child')).toHaveText([
    '已撤销的 Key',
    '临时测试',
    '工作站',
  ])
  await expect(modal(page).getByRole('columnheader')).toHaveText([
    '显示名称',
    '访问密钥',
    '过期日期',
    '操作',
  ])
  await expect(modal(page).locator('code')).toHaveText([
    'vk-revoked********1234',
    'zt_vk_temp********temp',
    'vk-work1234********1234',
  ])
  const maskedKey = modal(page).getByRole('row').filter({ hasText: '工作站' })
  await expect(maskedKey.getByRole('button', { name: '复制', exact: true })).toHaveCount(0)
  await expect(maskedKey.locator('td').nth(2)).toHaveText('-')
  await expect(modal(page)).toContainText('2020年1月1日')
  await expect(
    modal(page).getByRole('row').filter({ hasText: '临时测试' }).locator('td').nth(2),
  ).toHaveText('2020年1月1日')
  await expect(
    modal(page).getByRole('row').filter({ hasText: '已撤销的 Key' }).locator('td').nth(2),
  ).toHaveText('-')
  await expect(
    modal(page)
      .getByRole('row')
      .filter({ hasText: '临时测试' })
      .getByRole('button', { name: '撤销' }),
  ).toHaveCount(0)
  const historyDialog = page.getByRole('dialog', { name: '林知远的 Key 记录', exact: true })
  const historyRevoke = historyDialog
    .getByRole('row')
    .filter({ hasText: '工作站' })
    .getByRole('button', { name: '撤销' })
  await historyRevoke.click()
  await expect(modal(page)).toContainText('确认撤销「工作站」')
  await modal(page).getByRole('button', { name: '取消', exact: true }).click()
  expect(state.keys[0].status).toBe('ACTIVE')
  await historyRevoke.click()
  state.conflict(true)
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  await expect(page.locator('.toast')).not.toContainText('req_browser_fixture')
  expect(state.keys[0].status).toBe('ACTIVE')
  state.conflict(false)
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(1)
  await expect(page.locator('.toast-success')).toContainText('Key 已撤销')
  await expect(historyDialog.locator('.notice')).toHaveCount(0)
  await expect(historyRevoke).toHaveCount(0)
  await expect(maskedKey).toContainText('已撤销')
  expect(state.keys[0].status).toBe('REVOKED')
  expect(state.keys[1].status).toBe('ACTIVE')
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/member-key-list.png', fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await modal(page).evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
  await page.screenshot({ path: '../.cache/web-visual/member-key-list-mobile.png', fullPage: true })
  await page.keyboard.press('Escape')
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(viewKeys).toBeFocused()
  await page.setViewportSize({ width: 1440, height: 1000 })
  const memberTableScroll = page.locator('.table-scroll')
  expect(
    await memberTableScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)
  const disabledMemberRow = page.getByRole('row').filter({ hasText: '周予安' })
  const disabledMemberStatus = disabledMemberRow.getByRole('switch', {
    name: '周予安的激活状态',
  })
  await expect(disabledMemberStatus).toBeEnabled()
  await expect(disabledMemberStatus).not.toHaveAttribute('title')
  await expect(disabledMemberStatus).toHaveText('')
  await expect(disabledMemberRow.getByRole('button', { name: '密钥', exact: true })).toBeEnabled()
  const assignKey = row.getByRole('button', { name: '密钥', exact: true })
  await expect(assignKey.locator('svg')).toHaveCount(0)
  await assignKey.click()
  await modal(page).getByLabel('Key 名称').fill('自动化')
  await expect(modal(page).getByLabel('到期日期（可选）')).toHaveAttribute('type', 'date')
  await modal(page).getByLabel('到期日期（可选）').fill('2020-01-01')
  await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('不能早于今天')
  await expect(modal(page).locator('.alert.error')).toHaveCount(0)
  await modal(page).getByLabel('到期日期（可选）').fill('2099-12-31')
  await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
  await modal(page).getByRole('button', { name: '复制', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText('已复制')
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(row).not.toContainText('vk-fixture1')
  await expect(row.getByRole('cell').first().locator('code')).toHaveText(longID)
  await expect(row).not.toContainText('zt_vk_temp')
  await expect(row).not.toContainText('2020年1月1日')
  expect(state.keys).toHaveLength(4)
  await expect(row).not.toContainText('2099年12月31日')
  expect(
    await page.evaluate((value) => {
      const expiry = new Date(value)
      return [
        expiry.getFullYear(),
        expiry.getMonth() + 1,
        expiry.getDate(),
        expiry.getHours(),
        expiry.getMinutes(),
        expiry.getSeconds(),
        expiry.getMilliseconds(),
      ]
    }, state.keys.at(-1).expiresAt),
  ).toEqual([2099, 12, 31, 23, 59, 59, 999])
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/members-keys.png', fullPage: true })
  await row.locator('summary').click()
  await row.getByRole('button', { name: '删除', exact: true }).click()
  await modal(page).getByRole('button', { name: '取消' }).click()
  expect(state.members).toHaveLength(3)
  await row.locator('summary').click()
  await row.getByRole('button', { name: '删除', exact: true }).click()
  state.conflict(true)
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  expect(state.members).toHaveLength(3)
  state.conflict(false)
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(row).toHaveCount(0)
  await expect(page.locator('.toast-success')).toContainText('用户已删除')
  await expect(page.locator('.notice')).toHaveCount(0)
  expect(state.members).toHaveLength(2)
})

test('成员列表不预查密钥且分配密钥后可激活', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '周予安' })
  const status = row.getByRole('switch', { name: '周予安的激活状态' })

  await expect(status).toBeEnabled()
  await status.click()
  await expect(page.locator('.toast')).toContainText('请先为该用户生成调用 Key，再启用用户')
  await expect(status).not.toBeChecked()
  await row.getByRole('button', { name: '密钥', exact: true }).click()
  await modal(page).getByLabel('Key 名称').fill('工作站')
  await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()

  await expect(status).toBeEnabled()
  await status.click()
  await expect(status).toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  await expect(status).toHaveText('')
  expect(state.members.find((member) => member.name === '周予安')?.status).toBe('ACTIVE')
})

test('用户月度额度展示已用与剩余并支持增量调整和取消限制', async ({ page }) => {
  const state = await fixture(page)
  state.members[0].monthlyTokenLimit = '1000'
  state.members[0].quotaUsedTokens = '600'
  await signIn(page)

  const row = page.getByRole('row').filter({ hasText: '林知远' })
  const quotaCell = row.getByRole('cell').nth(3)
  await expect(quotaCell).toContainText(/600\s*\/\s*1,000/)
  await expect(quotaCell).toContainText('剩余 400')

  const moreActions = row.locator('summary')
  await expect(moreActions).toHaveAttribute('aria-label', '林知远 的更多操作')
  await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
  await moreActions.click()
  const otherRow = page.getByRole('row').filter({ hasText: '陈清和' })
  await otherRow.locator('summary').click()
  await expect(row.locator('.row-action-more-menu')).toBeHidden()
  await expect(otherRow.locator('.row-action-more-menu')).toBeVisible()
  await expect(page.locator('.row-action-more-menu:popover-open')).toHaveCount(1)
  await moreActions.click()
  await row.getByRole('button', { name: '增加配额', exact: true }).click()
  const quotaDialog = modal(page)
  await expect(quotaDialog.getByRole('group', { name: '常用额度' }).getByRole('button')).toHaveText(
    ['100 万', '1000 万', '1 亿'],
  )
  await quotaDialog.getByRole('button', { name: '1 亿', exact: true }).click()
  await expect(quotaDialog.getByLabel('增加 Token 数')).toHaveValue('100000000')
  await expect(quotaDialog.getByText('约 1亿 Token', { exact: true })).toBeVisible()
  await quotaDialog.getByLabel('增加 Token 数').fill('10000000')
  await expect(quotaDialog.getByText('约 1000万 Token', { exact: true })).toBeVisible()
  await quotaDialog.getByLabel('增加 Token 数').fill('500')
  await expect(quotaDialog.getByLabel('调整原因（选填）')).toHaveAttribute(
    'placeholder',
    '例如：项目扩容',
  )
  await modal(page).getByRole('button', { name: '增加配额', exact: true }).click()

  await expect(page.locator('.toast-success')).toContainText('配额已增加')
  await expect(modal(page)).toHaveCount(0)
  await expect(quotaCell).toContainText(/600\s*\/\s*1,500/)
  await expect(quotaCell).toContainText('剩余 900')
  expect(state.members[0].monthlyTokenLimit).toBe('1500')

  await moreActions.click()
  await row.getByRole('button', { name: '取消额度限制', exact: true }).click()
  await expect(modal(page)).toContainText('取消后将不再按月度 Token 用量拒绝请求')
  await modal(page).getByRole('button', { name: '取消额度限制', exact: true }).click()

  await expect(page.locator('.toast-success')).toContainText('额度限制已取消')
  await expect(modal(page)).toHaveCount(0)
  await expect(quotaCell).toHaveText('-')
  expect(state.members[0].monthlyTokenLimit).toBeNull()
})

test('成员 Key 弹层加载失败可重试', async ({ page }) => {
  await fixture(page)
  let fail = true
  await page.route(`**/api/v1/members/${longID}/keys?*`, async (route) => {
    if (!fail) return route.fallback()
    return route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'UNAVAILABLE', data: null }),
    })
  })
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '林知远' })
  await expect(row.getByRole('alert')).toHaveCount(0)
  await row.getByRole('button', { name: '查看 Key 记录' }).click()
  await expect(modal(page).getByRole('alert')).toBeVisible()
  fail = false
  await modal(page).getByRole('button', { name: '重试' }).click()
  await expect(modal(page).getByRole('alert')).toHaveCount(0)
})

test('主列表不请求密钥，弹层按需分页并支持失败重试和切换每页条数', async ({ page }) => {
  const state = await fixture(page)
  for (let i = 0; i < 51; i++) {
    state.keys.push({
      id: (90071992547409931n + BigInt(i)).toString(),
      memberID: longID,
      name: `设备 ${i}`,
      maskedKey: `vk-${String(i).padStart(8, '0')}********${String(i).padStart(4, '0')}`,
      key: `secret-fixture-full-key-${i}`,
      status: 'ACTIVE',
      expiresAt: null,
      revokedAt: null,
      createdAt: stamp,
    })
  }
  const requests: URLSearchParams[] = []
  let failMore = true
  await page.route(`**/api/v1/members/${longID}/keys?*`, async (route) => {
    const query = new URL(route.request().url()).searchParams
    requests.push(query)
    if (query.has('after') && failMore)
      return route.fulfill({ status: 503, json: { code: 'SERVICE_UNAVAILABLE' } })
    return route.fallback()
  })
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '林知远' })
  expect(requests).toHaveLength(0)
  await row.getByRole('button', { name: '查看 Key 记录' }).click()
  const dialog = modal(page)
  await expect(dialog.locator('tbody tr')).toHaveCount(20)
  await expect(dialog.locator('tbody tr').first()).toContainText('设备 50')
  await expect(dialog.getByText('每页', { exact: true })).toHaveCount(0)
  await expect(dialog.locator('.pagination-controls')).toHaveCSS('justify-content', 'flex-end')
  await expect(dialog.getByText('每页显示', { exact: true })).toBeVisible()
  await expect(dialog.getByText('条', { exact: true })).toBeVisible()
  await expect(dialog.getByLabel('每页')).toHaveValue('20')
  await expect(dialog.getByLabel('每页').locator('option')).toHaveText(['20', '50', '100'])
  expect(requests.at(-1)!.get('limit')).toBe('20')
  await dialog.getByRole('button', { name: '下一页' }).click()
  await expect(dialog.getByRole('alert')).toContainText('服务暂时不可用')
  await expect(dialog.locator('tbody tr')).toHaveCount(20)
  expect(requests.at(-1)!.get('after')).toBe('90071992547409962')
  failMore = false
  await dialog.getByRole('button', { name: '重试' }).click()
  await expect(dialog.locator('tbody tr')).toHaveCount(20)
  await expect(dialog.locator('tbody tr').first()).toContainText('设备 30')
  await expect(dialog.locator('tbody tr').last()).toContainText('设备 11')
  await expect(dialog.getByText('第 2 / 3 页', { exact: true })).toBeVisible()
  await expect(dialog.getByText('共 51 条', { exact: true })).toBeVisible()
  await dialog.getByLabel('每页').selectOption('100')
  await expect(dialog.locator('tbody tr')).toHaveCount(51)
  await expect(dialog.getByText('第 1 / 1 页', { exact: true })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '下一页' })).toBeDisabled()
  expect(requests.at(-1)!.get('limit')).toBe('100')
  expect(await page.content()).not.toContain('secret-fixture-full-key')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(row.getByRole('button', { name: '查看 Key 记录' })).toBeEnabled()
})

test('模型新增编辑、模态校验、冲突恢复和窄屏表单', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page)
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await expect.poll(() => state.providerQueries.at(-1)?.get('type')).toBe('OFFICIAL')
  await expect(page.getByRole('columnheader')).toHaveText([
    'ID',
    '模型名称',
    '模型厂商',
    '启用状态',
    '输入类型',
    '输出类型',
    '备注',
    '操作',
  ])
  const existingModelRow = page.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
  await expect(existingModelRow.locator('.person strong')).toHaveText('DeepSeek V4 Flash')
  await expect(existingModelRow.locator('.person small')).toHaveText('deepseek-v4-flash')
  await expect(existingModelRow.getByRole('cell')).toHaveCount(8)
  await expect(existingModelRow.getByRole('cell').first().locator('code')).toHaveText('71')
  await expect(
    existingModelRow.getByRole('cell').first().locator('.technical-value-copy'),
  ).toHaveCount(0)
  await expect(existingModelRow.getByRole('cell').nth(2)).toHaveText('DeepSeek')
  const publisherFilter = page.getByRole('combobox', { name: '模型厂商', exact: true })
  await expect(publisherFilter.locator('..')).toHaveCSS('flex-direction', 'row')
  await expect(publisherFilter.locator('option')).toHaveText(['全部', 'DeepSeek'])
  await publisherFilter.selectOption('81')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.locator('tbody tr')).toHaveCount(1)
  await expect(existingModelRow).toBeVisible()
  await expect(page.getByRole('row').filter({ hasText: 'Claude Sonnet' })).toHaveCount(0)
  await page.getByRole('button', { name: '重置', exact: true }).click()
  await expect(publisherFilter).toHaveValue('')
  await expect(page.locator('tbody tr')).toHaveCount(2)
  await page.getByRole('button', { name: '添加模型' }).click()
  const dialog = modal(page)
  const modelFormRows = dialog.locator('.model-form-row')
  await expect(modelFormRows).toHaveCount(6)
  await expect(modelFormRows.locator('.model-form-label')).toHaveText([
    '模型编码',
    '模型名称',
    '模型厂商',
    '输入类型',
    '输出类型',
    '备注',
  ])
  await expect(modelFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await dialog.getByLabel('模型名称', { exact: true }).fill('测试官方模型')
  await dialog.getByLabel('模型编码', { exact: true }).fill('official-test-v1')
  await dialog.getByLabel('模型厂商', { exact: true }).selectOption('81')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('至少选择一项')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  await dialog.getByRole('group', { name: '输入类型' }).getByLabel('文本', { exact: true }).check()
  await dialog.getByRole('group', { name: '输入类型' }).getByLabel('图片', { exact: true }).check()
  await dialog.getByRole('group', { name: '输出类型' }).getByLabel('文本', { exact: true }).check()
  await dialog.getByLabel('备注', { exact: true }).fill('图文理解用途')
  state.conflict(true)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  await expect(dialog.getByLabel('模型编码', { exact: true })).toHaveValue('official-test-v1')
  state.conflict(false)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  const row = page.getByRole('row').filter({ hasText: 'official-test-v1' })
  await expect(row).not.toContainText('多模态')
  await expect(row.getByText('图片', { exact: true })).toBeVisible()
  await expect(row.getByText('文本', { exact: true })).toHaveCount(2)
  await expect(row).toContainText('图文理解用途')
  const created = state.models.find((model) => model.code === 'official-test-v1')
  expect(created.inputModalities).toEqual(['TEXT', 'IMAGE'])
  expect(created.outputModalities).toEqual(['TEXT'])
  expect(created.status).toBe('DISABLED')
  expect(created.publisherProviderId).toBe('81')
  expect(created.publisherProviderName).toBe('DeepSeek')
  await expect(row.getByRole('cell').nth(2)).toHaveText('DeepSeek')
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(dialog.getByLabel('发布方编码', { exact: true })).toHaveCount(0)
  await expect(dialog.getByLabel('模型厂商', { exact: true })).toHaveValue('81')
  await mkdir('../.cache/web-visual', { recursive: true })
  await dialog.screenshot({ path: '../.cache/web-visual/model-edit-desktop.png' })
  await expect(
    dialog.getByRole('group', { name: '输入类型' }).getByLabel('图片', { exact: true }),
  ).toBeChecked()
  await dialog.getByLabel('模型编码', { exact: true }).fill('official-test-v2')
  await expect(dialog.getByRole('status')).toContainText('客户端需要使用新编码')
  await dialog.getByRole('group', { name: '输出类型' }).getByLabel('音频', { exact: true }).check()
  await page.setViewportSize({ width: 390, height: 480 })
  await expectOnlyModalBodyScrolls(dialog)
  await expect(dialog.locator('.modal-footer').getByRole('button', { name: '保存' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeVisible()
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/model-edit-mobile.png' })
  await dialog.getByLabel('模型厂商', { exact: true }).selectOption('')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  expect(created.code).toBe('official-test-v2')
  expect(created.status).toBe('DISABLED')
  expect(created.outputModalities).toEqual(['TEXT', 'AUDIO'])
  expect(created.publisherProviderId).toBeNull()
  expect(created.publisherProviderName).toBeNull()
  await page.setViewportSize({ width: 1440, height: 1000 })
  const modelSearch = page.getByRole('searchbox', { name: '模型名称' })
  await expect(modelSearch).toHaveAttribute('placeholder', '请输入模型名称')
  await expect(page.locator('.list-search-label')).toHaveText('模型名称')
  const modelQueryCount = state.modelQueries.length
  await modelSearch.fill('图文理解用途')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  expect(state.modelQueries).toHaveLength(modelQueryCount + 1)
  await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(1)
  await page.screenshot({ path: '../.cache/web-visual/model-catalog-desktop.png' })
  const updatedRow = page.getByRole('row').filter({ hasText: 'official-test-v2' })
  await expect(updatedRow.getByRole('cell').nth(2)).toHaveText('-')
  await updatedRow.locator('summary').click()
  await updatedRow.getByRole('button', { name: '删除', exact: true }).click()
  const deleteDialog = modal(page)
  await expect(deleteDialog).toContainText('相关服务商映射和分组授权将同时失效')
  await deleteDialog.getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(0)
  expect(state.models.some((model) => model.id === created.id)).toBe(false)
})

test('服务商同步入口只由后端能力参数控制', async ({ page }) => {
  await fixture(page)
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(page.getByRole('columnheader').first()).toHaveText('ID')
  await expect(row.getByRole('cell').first().locator('code')).toHaveText('81')
  await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
  const syncModelsButton = row.getByRole('button', {
    name: '同步 DeepSeek 的官方模型',
    exact: true,
  })
  await expect(syncModelsButton).toBeVisible()
  await expect(row.locator('.provider-name-actions').getByRole('button')).toHaveCount(1)
  await expect(syncModelsButton.locator('svg')).toBeVisible()
  await expect(syncModelsButton.locator('path')).toHaveAttribute('d', /\S+/)
  await syncModelsButton.click()
  const dialog = page.getByRole('dialog', { name: 'DeepSeek / 模型同步结果' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('status')).toContainText('模型目录同步完成')
  await expect(dialog.getByText('官方接口', { exact: true })).toBeVisible()
})

test('服务商协议地址可显式选择私有网络', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '编辑服务商' })
  await dialog.getByLabel('OpenAI 网络位置', { exact: true }).selectOption('PRIVATE')
  await dialog.getByLabel('OpenAI 协议地址', { exact: true }).fill('http://192.168.1.20:8080/v1')
  await expect(dialog.getByText(/私有网络地址可以访问 Zentrola 所在网络/)).toBeVisible()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()

  expect(state.providerInputs.at(-1)?.endpoints).toContainEqual({
    protocolType: 'OPENAI',
    baseUrl: 'http://192.168.1.20:8080/v1',
    networkScope: 'PRIVATE',
  })
  await expect(row.getByText('私有网络', { exact: true })).toBeVisible()
})

test('服务商代理支持 SOCKS5 并忽略 HTTP Header', async ({ page }) => {
  const state = await fixture(page)
  state.providers[0].proxyEnabled = true
  state.providers[0].proxyUrl = 'http://proxy.example.com:8080'
  state.providers[0].proxyHeaders = [{ key: 'X-Proxy-Token', configured: true }]
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '编辑服务商' })
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  await dialog.getByRole('switch', { name: '启用代理', exact: true }).check()
  await expect(dialog.getByLabel('KEY', { exact: true })).toHaveValue('X-Proxy-Token')
  const proxyURL = dialog.getByLabel('代理服务器地址', { exact: true })
  await proxyURL.fill('ftp://proxy.example.com:21')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('请输入有效的 HTTP、HTTPS 或 SOCKS5 代理地址')

  await dialog.getByRole('switch', { name: '修改代理凭据', exact: true }).check()
  await proxyURL.fill('socks5://proxy-user:proxy-password@proxy.example.com:1080')
  await expect(
    dialog.getByText('SOCKS5 使用地址中的用户名和密码认证，不支持自定义 HTTP Header。', {
      exact: true,
    }),
  ).toBeVisible()
  await expect(dialog.getByRole('button', { name: '添加 Header', exact: true })).toHaveCount(0)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()

  expect(state.providerInputs.at(-1)).toEqual(
    expect.objectContaining({
      proxyEnabled: true,
      proxyUrl: 'socks5://proxy-user:proxy-password@proxy.example.com:1080',
      proxyHeaders: [],
    }),
  )
  expect(state.providers[0].proxyUrl).not.toContain('proxy-password')
})

test('编辑服务商代理时默认保留凭据并可显式修改', async ({ page }) => {
  const state = await fixture(page)
  state.providers[0].proxyEnabled = true
  state.providers[0].proxyUrl = 'http://******:******@geo.miyaip.app:8001'
  state.providers[0].proxyHeaders = []
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: '编辑服务商' })
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  const credentialSwitch = dialog.getByRole('switch', {
    name: '修改代理凭据',
    exact: true,
  })
  const proxyProtocol = dialog.getByLabel('代理协议', { exact: true })
  const proxyURLInput = dialog.getByLabel('代理服务器地址', { exact: true })
  await expect(credentialSwitch).not.toBeChecked()
  await expect(proxyProtocol).toHaveValue('http')
  await proxyURLInput.fill('https://******:******@geo.miyaip.app:8002')
  await expect(proxyProtocol).toHaveValue('https')
  await expect(proxyURLInput).toHaveValue('******:******@geo.miyaip.app:8002')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  expect(state.providerInputs.at(-1)).toEqual(
    expect.objectContaining({
      proxyUrl: 'https://******:******@geo.miyaip.app:8002',
      updateProxyCredentials: false,
    }),
  )

  await row.getByRole('button', { name: '编辑', exact: true }).click()
  dialog = page.getByRole('dialog', { name: '编辑服务商' })
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  const proxyURL = dialog.getByLabel('代理服务器地址', { exact: true })
  await expect(dialog.getByLabel('代理协议', { exact: true })).toHaveValue('https')
  await dialog.getByRole('switch', { name: '修改代理凭据', exact: true }).check()
  await expect(proxyURL).toHaveValue('geo.miyaip.app:8002')
  await proxyURL.fill('socks5://new-user:new-password@geo.miyaip.app:8002')
  await expect(proxyURL).toHaveValue('new-user:new-password@geo.miyaip.app:8002')
  await expect(dialog.getByLabel('代理协议', { exact: true })).toHaveValue('socks5')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  expect(state.providerInputs.at(-1)).toEqual(
    expect.objectContaining({
      proxyUrl: 'socks5://new-user:new-password@geo.miyaip.app:8002',
      updateProxyCredentials: true,
    }),
  )
})

test('连接测试允许选择模型并显示实际测试模型', async ({ page }) => {
  const state = await fixture(page)
  state.providers[0].name = 'Google'
  state.providers[0].code = 'google-gemini-official'
  state.providers[0].endpoints = [
    {
      protocolType: 'OPENAI',
      baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai',
    },
  ]
  state.models.splice(
    0,
    state.models.length,
    {
      id: '73',
      code: 'gemini-3.6-flash',
      name: 'Gemini 3.6 Flash',
      status: 'ACTIVE',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'Google',
      createdAt: stamp,
      updatedAt: stamp,
    },
    {
      id: '74',
      code: 'gemini-3.6-pro',
      name: 'Gemini 3.6 Pro',
      status: 'ACTIVE',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'Google',
      createdAt: stamp,
      updatedAt: stamp,
    },
  )
  state.providerMappings.set('81', [
    {
      id: '92',
      providerId: '81',
      modelId: '73',
      upstreamModelCode: 'gemini-3.6-flash',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    },
    {
      id: '93',
      providerId: '81',
      modelId: '74',
      upstreamModelCode: 'gemini-3.6-pro',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    },
  ])
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'Google API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    status: 'ACTIVE',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })

  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '测试 Google 的连接', exact: true }).click()

  const selection = page.getByRole('dialog', { name: 'Google / 选择测试模型' })
  await expect(selection).toBeVisible()
  await page.setViewportSize({ width: 390, height: 280 })
  await expectOnlyModalBodyScrolls(selection)
  await expect(
    selection.getByRole('radio', { name: '使用 Gemini 3.6 Flash 测试连接' }),
  ).toBeChecked()
  await selection.getByRole('radio', { name: '使用 Gemini 3.6 Pro 测试连接' }).check()
  const requestPromise = page.waitForRequest((request) => {
    const url = new URL(request.url())
    return request.method() === 'POST' && url.pathname.endsWith('/test-connection')
  })
  await selection.getByRole('button', { name: '开始测试', exact: true }).click()
  const request = await requestPromise
  expect(new URL(request.url()).searchParams.get('providerModelMappingId')).toBe('93')

  const result = page.getByRole('dialog', { name: 'Google / 连接测试结果' })
  await expect(result.getByRole('status')).toContainText('模型调用验证通过')
  await expect(result.locator('dt').filter({ hasText: /^协议$/ })).toBeVisible()
  await expect(result.getByText('gemini-3.6-pro', { exact: true })).toBeVisible()
})

test('服务商没有可用模型映射时隐藏连接测试入口', async ({ page }) => {
  const state = await fixture(page)
  state.providerMappings.set('81', [])
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    status: 'ACTIVE',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })

  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(
    providerRow.getByRole('button', { name: '测试 DeepSeek 的连接', exact: true }),
  ).toHaveCount(0)
  const credentialButton = providerRow.getByRole('button', {
    name: '管理 DeepSeek 的认证凭据',
    exact: true,
  })
  await expect(credentialButton).toBeVisible()
  await credentialButton.click()
  await expect(
    modal(page).getByRole('button', { name: '验证 DeepSeek API Key 的可用性', exact: true }),
  ).toHaveCount(0)
})

test('服务商启用开关分别提示缺少模型映射和认证凭据', async ({ page }) => {
  const state = await fixture(page)
  const disabledProvider = (id: string, name: string) => ({
    ...structuredClone(state.providers[0]),
    id,
    name,
    code: `provider-${id}`,
    status: 'DISABLED',
  })
  state.providers.push(
    disabledProvider('82', '模型凭据均未配置'),
    disabledProvider('83', '仅配置凭据'),
    disabledProvider('84', '仅配置模型'),
    disabledProvider('85', '配置齐全'),
  )
  state.providers.find((provider) => provider.id === '85')!.type = 'THIRD_PARTY'
  state.providerMappings.set('82', [])
  state.providerMappings.set('83', [])
  state.providerMappings.set('84', [
    {
      id: '94',
      providerId: '84',
      modelId: '71',
      upstreamModelCode: 'deepseek-v4-flash',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    },
  ])
  state.resources.push({
    id: '89',
    name: '仅凭据 API Key',
    providerId: '83',
    authType: 'API_KEY',
    priority: 100,
    status: 'ACTIVE',
    runtimeStatus: 'HEALTHY',
    quotaStatus: 'UNKNOWN',
    blockedReason: null,
    blockedAt: null,
    lastErrorAt: null,
    lastHttpStatus: null,
    lastErrorCode: null,
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.providerMappings.set(
    '85',
    state.providerMappings.get('84')!.map((mapping) => ({
      ...mapping,
      id: '95',
      providerId: '85',
    })),
  )
  state.resources.push({ ...state.resources.at(-1)!, id: '90', providerId: '85' })

  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  const switchFor = (name: string) =>
    page
      .getByRole('row')
      .filter({ hasText: name })
      .getByRole('switch', { name: `${name}的启用状态` })
  await expect(switchFor('模型凭据均未配置')).toBeDisabled()
  await expect(switchFor('模型凭据均未配置')).toHaveAttribute('title', '请先配置模型和凭证。')
  await expect(switchFor('仅配置凭据')).toBeDisabled()
  await expect(switchFor('仅配置凭据')).toHaveAttribute('title', '请先配置模型。')
  await expect(switchFor('仅配置模型')).toBeDisabled()
  await expect(switchFor('仅配置模型')).toHaveAttribute('title', '请先配置凭证。')
  const configuredRow = page.getByRole('row').filter({ hasText: '配置齐全' })
  await expect(configuredRow.locator('.provider-runtime-state')).toHaveText('待启用')
  await expect(switchFor('配置齐全')).toBeEnabled()
  await switchFor('配置齐全').click()
  const activation = page.getByRole('dialog', { name: '配置齐全 / 选择启用时验证的连接' })
  await expect(activation).toBeVisible()
  await page.setViewportSize({ width: 390, height: 420 })
  await expectOnlyModalBodyScrolls(activation)
  await expect(activation.getByRole('radio', { name: '使用 Anthropic 协议测试连接' })).toBeChecked()
  await expect(
    activation.getByRole('radio', { name: '使用 DeepSeek V4 Flash 测试连接' }),
  ).toBeChecked()
  await expect(switchFor('配置齐全')).not.toBeChecked()
  await activation.getByRole('button', { name: '测试并启用' }).click()
  await expect(page.getByRole('dialog', { name: '配置齐全 / 启用结果' })).toContainText(
    '连接测试通过，服务商已启用',
  )
  expect(state.providerActivationInputs.at(-1)).toEqual({
    resourceId: '90',
    protocol: 'ANTHROPIC',
    providerModelMappingId: '95',
  })
  await expect(configuredRow.locator('.provider-runtime-state')).toHaveText('正常')
  await page
    .getByRole('dialog', { name: '配置齐全 / 启用结果' })
    .getByRole('button', { name: '关闭', exact: true })
    .last()
    .click()
  const credentialOnlyRow = page.getByRole('row').filter({ hasText: '仅配置凭据' })
  await expect(credentialOnlyRow.locator('.provider-runtime-state')).toHaveText('未配置模型')
  await expect(credentialOnlyRow.locator('.provider-runtime-state')).toHaveAttribute(
    'title',
    '为 仅配置凭据 配置模型映射后即可启用',
  )
  await mkdir('../.cache/web-visual', { recursive: true })
  await credentialOnlyRow.screenshot({ path: '../.cache/web-visual/provider-missing-model.png' })
  await credentialOnlyRow.locator('.provider-runtime-state').click()
  await expect(modal(page).getByRole('tab', { name: '模型配置' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
})

test('启用服务商使用弹窗选定的协议，连接失败时保持停用', async ({ page }) => {
  const state = await fixture(page)
  state.providers[0].status = 'DISABLED'
  state.models[1].publisherProviderId = '81'
  state.providerMappings.get('81')!.push({
    id: '93',
    providerId: '81',
    modelId: '72',
    upstreamModelCode: 'alternate-model',
    priority: 100,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.failTest()
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  const status = page.getByRole('switch', { name: 'DeepSeek的启用状态' })
  await status.click()
  const selection = page.getByRole('dialog', { name: 'DeepSeek / 选择启用时验证的连接' })
  await selection.getByRole('radio', { name: '使用 OpenAI 协议测试连接' }).check()
  await selection.getByRole('radio', { name: '使用 Claude Sonnet 测试连接' }).check()
  await selection.getByRole('button', { name: '测试并启用' }).click()
  await expect(page.getByRole('dialog', { name: 'DeepSeek / 启用结果' })).toContainText(
    '上游认证失败',
  )
  expect(state.providerActivationInputs).toEqual([
    { resourceId: '88', protocol: 'OPENAI', providerModelMappingId: '93' },
  ])
  await expect(status).not.toBeChecked()
  expect(state.providers[0].status).toBe('DISABLED')
})

test('服务商操作引导依次高亮配置入口', async ({ page }) => {
  const state = await fixture(page)
  state.providers[0].website = 'https://www.deepseek.com'
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    authType: 'API_KEY',
    authAdapter: 'API_KEY',
    status: 'ACTIVE',
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })

  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '操作引导', exact: true }).click()

  const guide = page.locator('.initialization-tour')
  await expect(guide).toHaveAttribute('data-target', 'create')
  await expect(guide.getByRole('heading', { name: '添加服务商' })).toBeVisible()
  await expect(page.locator('#provider-guide-create')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'website')
  await expect(guide.getByRole('heading', { name: '前往官网获取凭证' })).toBeVisible()
  const websiteLink = page.locator('#provider-guide-website')
  await expect(websiteLink).toHaveClass(/initialization-tour-target/)
  await expect(websiteLink).toHaveAttribute('href', 'https://www.deepseek.com')
  await websiteLink.evaluate((element) => {
    element.addEventListener(
      'click',
      (event) => {
        event.preventDefault()
        element.setAttribute('data-guide-clicked', 'true')
      },
      { once: true },
    )
  })
  await page.locator('.initialization-highlight').click()
  await expect(websiteLink).toHaveAttribute('data-guide-clicked', 'true')

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'credential')
  await expect(guide.getByRole('heading', { name: '添加凭证' })).toBeVisible()
  await expect(page.locator('#provider-guide-credential')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'sync')
  await expect(guide.getByRole('heading', { name: '同步模型' })).toBeVisible()
  await expect(page.locator('#provider-guide-sync')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'test')
  await expect(guide.getByRole('heading', { name: '测试连接' })).toBeVisible()
  await expect(page.locator('#provider-guide-test')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '下一步', exact: true }).click()
  await expect(guide).toHaveAttribute('data-target', 'edit')
  await expect(guide.getByRole('heading', { name: '编辑服务商配置' })).toBeVisible()
  await expect(guide.getByText('第 6 步，共 6 步', { exact: true })).toBeVisible()
  await expect(page.locator('#provider-guide-edit')).toHaveClass(/initialization-tour-target/)

  await guide.getByRole('button', { name: '完成', exact: true }).click()
  await expect(guide).toHaveCount(0)
})

test('没有服务商时操作引导只显示添加服务商', async ({ page }) => {
  const state = await fixture(page)
  state.providers.splice(0)

  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page.getByRole('button', { name: '操作引导', exact: true }).click()

  const guide = page.locator('.initialization-tour')
  await expect(guide).toHaveAttribute('data-target', 'create')
  await expect(guide.getByText('第 1 步，共 1 步', { exact: true })).toBeVisible()
  await expect(guide.getByRole('button', { name: '完成', exact: true })).toBeVisible()
  await expect(page.locator('#provider-guide-create')).toHaveClass(/initialization-tour-target/)
})

test('服务商编辑模型列表在大量模型时可独立滚动', async ({ page }) => {
  const state = await fixture(page)
  state.models.push(
    ...Array.from({ length: 30 }, (_, index) => ({
      id: String(1000 + index),
      code: `deepseek-history-${index}`,
      name: `DeepSeek History ${index}`,
      status: 'ACTIVE',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'DeepSeek',
      createdAt: stamp,
      updatedAt: stamp,
    })),
  )
  await signIn(page)
  await page.setViewportSize({ width: 1200, height: 900 })
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await providerRow.getByRole('button', { name: '编辑', exact: true }).click()

  const mappingList = modal(page).locator('.mapping-catalog-list')
  await expect(mappingList).toBeVisible()
  const dimensions = await mappingList.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }))
  expect(dimensions.clientHeight).toBeLessThanOrEqual(640)
  expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight)
  await mappingList.hover()
  await page.mouse.wheel(0, 600)
  await expect.poll(() => mappingList.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
  await mappingList.evaluate((element) => {
    element.scrollTop = element.scrollHeight
  })
  await expect
    .poll(() =>
      mappingList.evaluate(
        (element) => element.scrollTop + element.clientHeight >= element.scrollHeight - 1,
      ),
    )
    .toBe(true)
  const listBox = (await mappingList.boundingBox())!
  const lastRowBox = (await mappingList.locator('.mapping-catalog-row').last().boundingBox())!
  expect(lastRowBox.y + lastRowBox.height).toBeLessThanOrEqual(listBox.y + listBox.height + 1)
  const bottomGap = await mappingList.evaluate((element) => {
    const editor = element.closest<HTMLElement>('.mapping-editor')!
    return editor.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom
  })
  expect(bottomGap).toBeGreaterThanOrEqual(16)

  await page.setViewportSize({ width: 1200, height: 600 })
  const desktopWorkspace = modal(page).locator('.mapping-workspace')
  const desktopBottomGap = await desktopWorkspace.evaluate((element) => {
    const editor = element.closest<HTMLElement>('.mapping-editor')!
    return editor.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom
  })
  expect(desktopBottomGap).toBeGreaterThanOrEqual(16)

  await page.setViewportSize({ width: 700, height: 800 })
  const mappingWorkspace = modal(page).locator('.mapping-workspace')
  await expect(mappingWorkspace).toHaveCSS('grid-template-columns', /^\S+$/)
  await expect(modal(page).locator('.mapping-editor')).toHaveCSS('overflow-y', 'auto')
  const catalogBox = (await modal(page).locator('.mapping-catalog-pane').boundingBox())!
  const selectedBox = (await modal(page).locator('.mapping-selected-pane').boundingBox())!
  expect(selectedBox.y).toBeGreaterThan(catalogBox.y + catalogBox.height)
})

test('官方服务商编辑时不展示或提交停用模型映射', async ({ page }) => {
  const state = await fixture(page)
  state.models.push({
    id: '73',
    code: 'deepseek-disabled',
    name: 'DeepSeek Disabled',
    status: 'DISABLED',
    inputModalities: ['TEXT'],
    outputModalities: ['TEXT'],
    remark: '',
    publisherProviderId: '81',
    publisherProviderName: 'DeepSeek',
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.providerMappings.get('81')!.push({
    id: '93',
    providerId: '81',
    modelId: '73',
    upstreamModelCode: 'deepseek-disabled',
    priority: 100,
    createdAt: stamp,
    updatedAt: stamp,
  })

  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: 'DeepSeek' })
    .getByRole('button', { name: '编辑' })
    .click()
  const dialog = modal(page)
  await expect(dialog.locator('.mapping-count-tag')).toHaveText('1')
  await expect(dialog.getByRole('checkbox', { name: '启用 DeepSeek V4 Flash 映射' })).toBeChecked()
  await expect(dialog.getByRole('checkbox', { name: '启用 DeepSeek Disabled 映射' })).toHaveCount(0)
  await expect(dialog.getByText('DeepSeek Disabled', { exact: true })).toHaveCount(0)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  expect(state.providerInputs.at(-1)?.mappings).toEqual([
    { modelId: '71', upstreamModelCode: 'deepseek-v4-flash' },
  ])
})

test('服务商新增编辑、启停和窄屏导航折叠', async ({ page }) => {
  const state = await fixture(page)
  state.models.push({
    id: '73',
    code: 'deepseek-v3-legacy',
    name: 'DeepSeek V3 Legacy',
    status: 'DISABLED',
    inputModalities: ['TEXT'],
    outputModalities: ['TEXT'],
    remark: '',
    publisherProviderId: '81',
    publisherProviderName: 'DeepSeek',
    createdAt: stamp,
    updatedAt: stamp,
  })
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await signIn(page)

  await page.setViewportSize({ width: 1200, height: 900 })
  const providerLink = page.getByRole('link', { name: '服务商', exact: true })
  await expect(providerLink.locator('span')).toBeVisible()
  await providerLink.click()
  await expect(page.getByRole('heading', { name: '服务商', exact: true })).toBeVisible()

  const providerSearch = page.getByRole('search')
  const providerSearchInput = providerSearch.getByRole('searchbox', { name: '服务商名称' })
  await expect(providerSearchInput).toHaveAttribute('placeholder', '请输入服务商名称')
  await expect
    .poll(() => providerSearchInput.evaluate((input) => getComputedStyle(input).paddingLeft))
    .toBe('12px')
  await expect(providerSearch.locator('.search-box > svg')).toHaveCount(0)
  await expect(
    providerSearch.getByRole('button', { name: '搜索', exact: true }).locator('svg'),
  ).toHaveCount(0)
  await expect(
    page.locator('.page-heading').getByRole('button', { name: '添加服务商', exact: true }),
  ).toHaveCount(0)
  await expect(
    providerSearch.getByRole('button', { name: '创建服务商', exact: true }),
  ).toBeVisible()
  await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
  const createMenu = providerSearch.getByRole('menu', { name: '创建服务商' })
  await expect(createMenu.getByRole('menuitem', { name: /三方服务商/ })).toBeVisible()
  await expect(createMenu.getByRole('menuitem', { name: /预设厂商/ })).toBeVisible()
  await createMenu.getByRole('menuitem', { name: /预设厂商/ }).click()
  const initializeDialog = modal(page)
  await expect(initializeDialog.getByRole('heading', { name: '选择模型厂商' })).toBeVisible()
  await expect(initializeDialog.getByText('已选择 8 / 8', { exact: true })).toBeVisible()
  await expect(
    initializeDialog.getByRole('checkbox', { name: '选择 xAI', exact: true }),
  ).toBeVisible()
  await expect(
    initializeDialog.getByRole('checkbox', { name: '选择 MiniMax', exact: true }),
  ).toHaveCount(0)
  await expect(
    initializeDialog.getByRole('checkbox', { name: '选择 字节跳动', exact: true }),
  ).toHaveCount(0)
  await initializeDialog.getByRole('button', { name: '清空', exact: true }).click()
  await expect(
    initializeDialog.getByRole('button', { name: '初始化所选厂商', exact: true }),
  ).toBeDisabled()
  await initializeDialog.getByRole('checkbox', { name: '选择 深度求索', exact: true }).check()
  await initializeDialog.getByRole('checkbox', { name: '选择 月之暗面', exact: true }).check()
  await initializeDialog.getByRole('checkbox', { name: '选择 通义千问', exact: true }).check()
  await expect(initializeDialog.getByText('已选择 3 / 8', { exact: true })).toBeVisible()
  await initializeDialog.getByRole('button', { name: '初始化所选厂商', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText(
    '已补充 2 个官方服务商，并更新 1 个名称或官网，共 3 个',
  )
  await expect(page.locator('.provider-initialize-notice')).toHaveCount(0)
  const kimiRow = page.getByRole('row').filter({ hasText: '月之暗面' })
  await expect(kimiRow).toBeVisible()
  await expect(
    kimiRow.getByRole('button', { name: '同步 月之暗面 的官方模型', exact: true }),
  ).toHaveCount(0)
  const qwenRow = page.getByRole('row').filter({ hasText: '通义千问' })
  await expect(
    qwenRow.getByRole('button', { name: '同步 通义千问 的官方模型', exact: true }),
  ).toBeVisible()
  expect(state.providers.find((provider) => provider.code === 'kimi-official')?.website).toBe(
    'https://www.moonshot.cn',
  )
  await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
  await providerSearch
    .getByRole('menu', { name: '创建服务商' })
    .getByRole('menuitem', { name: /预设厂商/ })
    .click()
  const repeatInitializeDialog = modal(page)
  await repeatInitializeDialog.getByRole('button', { name: '清空', exact: true }).click()
  await repeatInitializeDialog.getByRole('checkbox', { name: '选择 深度求索', exact: true }).check()
  await repeatInitializeDialog.getByRole('checkbox', { name: '选择 月之暗面', exact: true }).check()
  await repeatInitializeDialog.getByRole('checkbox', { name: '选择 通义千问', exact: true }).check()
  await repeatInitializeDialog.getByRole('button', { name: '初始化所选厂商', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText(
    '官方服务商的当前语言名称和官网已是最新，共 3 个',
  )
  expect(state.providers).toHaveLength(3)
  expect(state.providerInitializeRequests).toEqual([
    {
      locale: 'zh-CN',
      providerCodes: ['deepseek-official', 'kimi-official', 'qwen-official'],
    },
    {
      locale: 'zh-CN',
      providerCodes: ['deepseek-official', 'kimi-official', 'qwen-official'],
    },
  ])

  const deepSeekRow = page.getByRole('row').filter({ hasText: '深度求索' })
  await expect(page.getByRole('columnheader', { name: '启用状态', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '认证凭据', exact: true })).toHaveCount(0)
  const deepSeekCredential = deepSeekRow.getByRole('button', {
    name: '管理 深度求索 的认证凭据',
    exact: true,
  })
  await expect(deepSeekCredential).toBeVisible()
  await expect(deepSeekCredential).toHaveText('凭证')
  const deepSeekProxy = deepSeekRow.getByRole('img', {
    name: '深度求索 未启用代理访问',
    exact: true,
  })
  await expect(deepSeekProxy).toBeVisible()
  await expect(deepSeekProxy.locator('svg')).toHaveCount(1)
  await expect(deepSeekRow.locator('td').last().getByRole('button', { name: '编辑' })).toBeVisible()
  const moreActions = deepSeekRow.locator('td').last().locator('summary')
  await expect(moreActions).toHaveAttribute('aria-label', '深度求索 的更多操作')
  await moreActions.click()
  await expect(deepSeekRow.locator('.row-action-more-menu')).toBeVisible()
  const moreMenuBox = (await deepSeekRow.locator('.row-action-more-menu').boundingBox())!
  expect(moreMenuBox.x).toBeGreaterThanOrEqual(8)
  expect(moreMenuBox.y).toBeGreaterThanOrEqual(8)
  await moreActions.click()
  expect(state.modelQueries).toHaveLength(0)
  await deepSeekRow.getByRole('button', { name: '编辑', exact: true }).click()
  expect(state.modelQueries).toHaveLength(0)
  await expect(modal(page).getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(
    modal(page).getByRole('checkbox', { name: '启用 DeepSeek V4 Flash 映射' }),
  ).toBeChecked()
  const existingProviderModelCode = modal(page).getByLabel('DeepSeek V4 Flash 的服务商模型编码')
  await expect(existingProviderModelCode).toHaveValue('deepseek-v4-flash')
  await expect(modal(page).getByText('Claude Sonnet', { exact: true })).toHaveCount(0)
  await expect(modal(page).getByText('DeepSeek V3 Legacy', { exact: true })).toHaveCount(0)
  await expect(
    modal(page).getByRole('checkbox', { name: '启用 DeepSeek V3 Legacy 映射' }),
  ).toHaveCount(0)
  await expect(modal(page).getByLabel('DeepSeek V3 Legacy 的服务商模型编码')).toHaveCount(0)
  await expect(modal(page).getByText('模型列表', { exact: true })).toBeVisible()
  await expect(modal(page).getByText('已启用模型映射', { exact: true })).toHaveCount(0)
  await expect(modal(page).getByText('模型名称', { exact: true })).toHaveCount(1)
  await expect(modal(page).getByText('服务商编码', { exact: true })).toBeVisible()
  await expect(modal(page).locator('.mapping-control > span')).toHaveCount(0)
  const editDialog = modal(page)
  const dialogBody = editDialog.locator('.modal-body')
  const dialogHeader = editDialog.locator('.modal-head')
  const dialogFooter = editDialog.locator('.modal-footer')
  const configTabs = editDialog.locator('.config-tabs')
  await expect(dialogFooter).toBeVisible()
  expect(await configTabs.evaluate((element) => getComputedStyle(element).overflowY)).toBe('hidden')
  await page.setViewportSize({ width: 1200, height: 600 })
  expect(await dialogBody.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  )
  const headerTop = (await dialogHeader.boundingBox())!.y
  const footerTop = (await dialogFooter.boundingBox())!.y
  await dialogBody.evaluate((element) => element.scrollTo(0, element.scrollHeight))
  expect((await dialogHeader.boundingBox())!.y).toBe(headerTop)
  expect((await dialogFooter.boundingBox())!.y).toBe(footerTop)
  await page.setViewportSize({ width: 1200, height: 900 })
  await modal(page).getByLabel('官方网站').fill('https://www.deepseek.com/')
  await existingProviderModelCode.fill('')
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  expect(state.providers[0].website).toBe('https://www.deepseek.com')
  expect(state.providerMappings.get('81')).toHaveLength(1)
  expect(state.providerMappings.get('81')?.[0].upstreamModelCode).toBe('')
  await expect(page.getByRole('columnheader', { name: '网站', exact: true })).toHaveCount(0)
  const websiteLink = deepSeekRow.getByRole('link', {
    name: '在新页面打开 深度求索 官网',
    exact: true,
  })
  await expect(websiteLink).toHaveAttribute('href', 'https://www.deepseek.com')
  await expect(websiteLink).toHaveAttribute('target', '_blank')
  await expect(websiteLink.locator('svg')).toHaveCount(1)
  await deepSeekRow.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(modal(page).getByLabel('DeepSeek V4 Flash 的服务商模型编码')).toHaveValue('')
  await modal(page).getByRole('button', { name: '取消', exact: true }).click()

  await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
  await providerSearch
    .getByRole('menu', { name: '创建服务商' })
    .getByRole('menuitem', { name: /三方服务商/ })
    .click()
  expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
  const dialog = modal(page)
  const providerFieldRows = dialog.locator('.connection-fields .provider-field-row')
  await expect(providerFieldRows).toHaveCount(4)
  await expect(providerFieldRows.locator('.provider-field-label')).toHaveText([
    '服务商名称',
    '官方网站',
    'OpenAI 协议地址',
    'Anthropic 协议地址',
  ])
  await expect(dialog.getByText('基础配置', { exact: true })).toHaveCount(0)
  await expect(
    dialog.getByText(
      '请至少填写一个协议地址：OpenAI 或 Anthropic。公网服务必须使用 HTTPS，私有网络服务可以使用 HTTP 或 HTTPS。',
      { exact: true },
    ),
  ).toBeVisible()
  await expect(dialog.locator('.connection-fields')).toHaveCSS('grid-template-columns', /^\S+$/)
  await expect(providerFieldRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(dialog.getByRole('button', { name: '添加映射', exact: true })).toHaveCount(0)
  await expect(dialog.getByText('模型映射', { exact: true })).toHaveCount(0)
  const mappingCatalogHead = dialog.locator('.mapping-catalog-pane .mapping-pane-head')
  const selectAllMappings = mappingCatalogHead.getByRole('button', {
    name: '全选',
    exact: true,
  })
  const invertMappings = mappingCatalogHead.getByRole('button', {
    name: '反选',
    exact: true,
  })
  await expect(mappingCatalogHead.getByText('模型列表', { exact: true })).toBeVisible()
  await expect(mappingCatalogHead.locator('.mapping-count-tag')).toHaveText(/\d+/)
  await expect(mappingCatalogHead.locator('.mapping-action-separator')).toHaveText('/')
  const mappingCheckbox = dialog.getByRole('checkbox', {
    name: '启用 DeepSeek V4 Flash 映射',
  })
  const claudeMappingCheckbox = dialog.getByRole('checkbox', {
    name: '启用 Claude Sonnet 映射',
  })
  await expect(dialog.getByText('尚未启用模型，请从左侧模型列表中勾选。')).toBeVisible()
  const mappingSearch = dialog.getByRole('searchbox', { name: '搜索模型映射' })
  await expect(mappingSearch).toHaveAttribute(
    'placeholder',
    '搜索系统模型名称、编码或服务商模型编码',
  )
  await expect(dialog.locator('.mapping-search-box > svg')).toHaveCount(0)
  await mappingSearch.fill('Claude')
  await expect(mappingCheckbox).toHaveCount(0)
  await expect(claudeMappingCheckbox).toBeVisible()
  await expect(dialog.getByText('显示 1 个', { exact: true })).toBeVisible()
  await mappingSearch.fill('不存在的模型')
  await expect(dialog.locator('.mapping-filter-empty')).toContainText('没有匹配的模型。')
  await dialog.getByRole('button', { name: '清除筛选', exact: true }).click()
  await expect(mappingSearch).toHaveValue('')
  await mappingSearch.fill('DeepSeek V4 Flash')
  await selectAllMappings.click()
  await expect(mappingCheckbox).toBeChecked()
  await expect(claudeMappingCheckbox).toHaveCount(0)
  await dialog.getByRole('button', { name: '清空模型映射搜索', exact: true }).click()
  await expect(claudeMappingCheckbox).toBeVisible()
  await expect(claudeMappingCheckbox).not.toBeChecked()
  await expect(mappingCheckbox).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(mappingCheckbox).toHaveCSS('border-top-style', 'none')
  const mappingList = dialog.locator('.mapping-catalog-list')
  await expect(mappingList).toHaveCSS('overflow-y', 'scroll')
  expect(
    await mappingList.evaluate((element) => getComputedStyle(element, '::-webkit-scrollbar').width),
  ).toBe('12px')
  await expect(dialog.locator('.modal-body')).toHaveCSS('overflow-y', 'hidden')
  await expect(dialog.locator('.mapping-catalog-row').first()).toHaveCSS('min-height', '52px')
  const providerModelCode = dialog.getByLabel('DeepSeek V4 Flash 的服务商模型编码')
  await selectAllMappings.click()
  await expect(mappingCheckbox).toBeChecked()
  await expect(claudeMappingCheckbox).toBeChecked()
  await invertMappings.click()
  await expect(mappingCheckbox).not.toBeChecked()
  await expect(claudeMappingCheckbox).not.toBeChecked()
  await expect(providerModelCode).toHaveCount(0)
  await mappingCheckbox.check()
  await expect(mappingCheckbox).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(mappingCheckbox).toHaveCSS('border-top-style', 'none')
  await expect(selectAllMappings).toBeEnabled()
  await expect(invertMappings).toBeEnabled()
  await expect(mappingCheckbox).toBeVisible()
  await expect(claudeMappingCheckbox).toBeVisible()
  await expect(providerModelCode).toBeEnabled()
  await expect(providerModelCode).toHaveValue('')
  await expect(providerModelCode).toHaveAttribute('placeholder', '默认为 deepseek-v4-flash')
  await mappingSearch.fill('Claude')
  await expect(mappingCheckbox).toHaveCount(0)
  await expect(providerModelCode).toBeVisible()
  await dialog.getByRole('button', { name: '清空模型映射搜索', exact: true }).click()
  await providerModelCode.fill('deepseek-v4-flash')
  await providerModelCode.blur()
  await expect(providerModelCode).toHaveValue('deepseek-v4-flash')
  await dialog.getByRole('button', { name: '移除 DeepSeek V4 Flash 映射' }).click()
  await expect(mappingCheckbox).not.toBeChecked()
  await expect(providerModelCode).toHaveCount(0)
  await dialog.getByLabel('服务商名称').fill('自定义百炼')
  await dialog.getByLabel('官方网站').fill('https://www.aliyun.com')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('至少填写一种协议接口地址')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  await dialog
    .getByLabel('OpenAI 协议地址')
    .fill('http://dashscope.aliyuncs.com/compatible-mode/v1')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('公网接口必须使用 HTTPS')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  const openAINetworkScope = dialog.getByLabel('OpenAI 网络位置', { exact: true })
  await openAINetworkScope.selectOption('PRIVATE')
  await expect(dialog.getByText(/私有网络地址可以访问 Zentrola 所在网络/)).toBeVisible()
  await expect(dialog.getByLabel('OpenAI 协议地址')).toHaveAttribute(
    'placeholder',
    'http://192.168.1.20:8080/v1',
  )
  await openAINetworkScope.selectOption('PUBLIC')
  await dialog
    .getByLabel('OpenAI 协议地址')
    .fill('https://dashscope.aliyuncs.com/compatible-mode/v1')
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  const proxySwitch = dialog.getByRole('switch', { name: '启用代理', exact: true })
  await expect(dialog.locator('.proxy-switch-row')).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await expect(dialog.locator('.proxy-switch-row .provider-field-label')).toHaveCSS(
    'text-align',
    'left',
  )
  await expect(
    dialog.getByText('仅该服务商的连接测试和模型调用使用此代理，默认直连。', {
      exact: true,
    }),
  ).toHaveCount(0)
  await expect(proxySwitch).toHaveAttribute(
    'title',
    '仅该服务商的连接测试和模型调用使用此代理，默认直连。',
  )
  const proxySwitchBoxBeforeEnable = await proxySwitch.boundingBox()
  const providerNameLabelBox = await dialog
      .locator('label[for="provider-name-input"]')
      .boundingBox(),
    providerNameInputBox = await dialog.getByLabel('服务商名称', { exact: true }).boundingBox()
  expect(proxySwitchBoxBeforeEnable).not.toBeNull()
  expect(providerNameLabelBox).not.toBeNull()
  expect(providerNameInputBox).not.toBeNull()
  expect(Math.abs(proxySwitchBoxBeforeEnable!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
  await proxySwitch.check()
  await expect(dialog.locator('.proxy-url-row')).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await expect(dialog.locator('.proxy-url-row .provider-field-label')).toHaveCSS(
    'text-align',
    'left',
  )
  await expect(
    dialog.getByText(
      '支持 http:// 或 https://，可直接包含用户名和密码；完整地址将加密保存，之后只显示脱敏值。',
      { exact: true },
    ),
  ).toHaveCount(0)
  const proxyUrlCredentialHint = dialog.getByRole('img', {
    name: '用户名和密码将加密保存',
    exact: true,
  })
  await expect(proxyUrlCredentialHint).toHaveAttribute('tabindex', '0')
  await expect(proxyUrlCredentialHint).toHaveAttribute('title', '用户名和密码将加密保存')
  const proxyURLLabelBox = await dialog
      .locator('.proxy-url-row .provider-field-label')
      .boundingBox(),
    proxyURLControlBox = await dialog.locator('.proxy-url-control').boundingBox(),
    proxyProtocolBox = await dialog.getByLabel('代理协议', { exact: true }).boundingBox(),
    proxyURLInputBox = await dialog.getByLabel('代理服务器地址', { exact: true }).boundingBox()
  expect(proxyURLLabelBox).not.toBeNull()
  expect(proxyURLControlBox).not.toBeNull()
  expect(proxyProtocolBox).not.toBeNull()
  expect(proxyURLInputBox).not.toBeNull()
  expect(Math.abs(proxyURLLabelBox!.width - providerNameLabelBox!.width)).toBeLessThanOrEqual(1)
  expect(Math.abs(proxyURLControlBox!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
  expect(Math.abs(proxyProtocolBox!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
  expect(proxyURLInputBox!.x).toBeGreaterThan(proxyProtocolBox!.x + proxyProtocolBox!.width)
  const proxySwitchBox = await proxySwitch.boundingBox()
  expect(proxySwitchBox).not.toBeNull()
  expect(Math.abs(proxySwitchBox!.x - proxySwitchBoxBeforeEnable!.x)).toBeLessThanOrEqual(1)
  expect(Math.abs(proxySwitchBox!.x - proxyURLControlBox!.x)).toBeLessThanOrEqual(1)
  await dialog
    .getByLabel('代理服务器地址', { exact: true })
    .fill('http://proxy-user:proxy-password@proxy.example.com:8080')
  const proxyHeaderHint = dialog.getByText('Header Value 加密保存，且只发送给代理服务器。', {
    exact: true,
  })
  const addProxyHeaderButton = dialog.getByRole('button', { name: '添加 Header', exact: true })
  await expect(dialog.locator('.proxy-headers-head > div')).toHaveCSS('display', 'flex')
  const proxyHeaderHintBox = await proxyHeaderHint.boundingBox()
  const addProxyHeaderButtonBox = await addProxyHeaderButton.boundingBox()
  expect(proxyHeaderHintBox).not.toBeNull()
  expect(addProxyHeaderButtonBox).not.toBeNull()
  expect(
    Math.abs(
      proxyHeaderHintBox!.y +
        proxyHeaderHintBox!.height / 2 -
        (addProxyHeaderButtonBox!.y + addProxyHeaderButtonBox!.height / 2),
    ),
  ).toBeLessThanOrEqual(3)
  await addProxyHeaderButton.click()
  await expect(dialog.locator('.proxy-header-field')).toHaveCount(2)
  await expect(dialog.locator('.proxy-header-field').first()).toHaveCSS(
    'grid-template-columns',
    /\S+ \S+/,
  )
  const proxyHeaderBox = await dialog.locator('.proxy-header-field').first().boundingBox()
  expect(proxyHeaderBox).not.toBeNull()
  expect(Math.abs(proxyHeaderBox!.x - proxyURLControlBox!.x)).toBeLessThanOrEqual(1)
  await dialog.getByLabel('KEY', { exact: true }).fill('X-Proxy-Token')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveAttribute('type', 'text')
  await dialog.getByLabel('VALUE', { exact: true }).fill('proxy-header-secret')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveValue('proxy-header-secret')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)

  const created = state.providers.find((provider) => provider.name === '自定义百炼')
  expect(created.status).toBe('DISABLED')
  expect(state.providerMappings.get(created.id)).toEqual([])

  const row = page.getByRole('row').filter({ hasText: '自定义百炼' })
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await mappingCheckbox.check()
  await providerModelCode.fill('aliyun-deepseek-v4-flash')
  await dialog.screenshot({ path: '../.cache/web-visual/provider-model-mappings.png' })
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)

  expect(state.providerInputs.at(-2)).toEqual(
    expect.objectContaining({
      endpoints: [
        {
          protocolType: 'OPENAI',
          baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
          networkScope: 'PUBLIC',
        },
      ],
      proxyEnabled: true,
      proxyUrl: 'http://proxy-user:proxy-password@proxy.example.com:8080',
      proxyHeaders: [{ key: 'X-Proxy-Token', value: 'proxy-header-secret' }],
    }),
  )
  expect(state.providerInputs.at(-1)?.mappings).toEqual([
    {
      modelId: '71',
      upstreamModelCode: 'aliyun-deepseek-v4-flash',
    },
  ])
  expect(created.proxyUrl).not.toContain('proxy-password')
  expect(state.providerMappings.get(created.id)).toEqual([
    expect.objectContaining({
      modelId: '71',
      upstreamModelCode: 'aliyun-deepseek-v4-flash',
    }),
  ])
  const providerNameCellBox = await row.getByRole('cell').nth(1).boundingBox()
  expect(providerNameCellBox).not.toBeNull()
  expect(providerNameCellBox!.width).toBeGreaterThanOrEqual(245)
  const endpoint = row.locator('.endpoint')
  await expect(endpoint).toHaveText('https://dashscope.aliyuncs.com/compatible-mode/v1')
  await expect(endpoint).toHaveAttribute(
    'title',
    'https://dashscope.aliyuncs.com/compatible-mode/v1',
  )
  await expect(endpoint.locator('.technical-value-text')).toHaveCSS('text-overflow', 'ellipsis')
  await expect(endpoint.locator('.technical-value-text')).toHaveCSS('white-space', 'nowrap')
  await endpoint.getByRole('button', { name: '复制', exact: true }).click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe('https://dashscope.aliyuncs.com/compatible-mode/v1')
  await expect(row.getByText('OpenAI', { exact: true })).toBeVisible()
  await expect(row.locator('.endpoint-protocol')).toHaveText(['OpenAI'])
  await expect(row.getByText('Anthropic', { exact: true })).toHaveCount(0)
  const proxyCellBox = await row.getByRole('cell').nth(4).boundingBox()
  const endpointCellBox = await row.getByRole('cell').nth(5).boundingBox()
  expect(proxyCellBox).not.toBeNull()
  expect(endpointCellBox).not.toBeNull()
  expect(proxyCellBox!.width).toBeLessThan(80)
  expect(endpointCellBox!.width).toBeGreaterThan(proxyCellBox!.width * 3)
  const enabledStatusBox = await row.getByRole('switch').boundingBox()
  const runtimeStatusBox = await row.locator('.provider-runtime-state').boundingBox()
  expect(enabledStatusBox).not.toBeNull()
  expect(runtimeStatusBox).not.toBeNull()
  expect(
    runtimeStatusBox!.x - (enabledStatusBox!.x + enabledStatusBox!.width),
  ).toBeGreaterThanOrEqual(16)
  await expect(row.locator('.provider-runtime .subline')).toHaveCount(0)
  await expect(row.locator('.provider-runtime-state')).toHaveAttribute(
    'title',
    '查看 自定义百炼 的凭证，当前运行状态：未配置：尚未配置服务商凭证',
  )
  await expect(
    row.getByRole('img', { name: '自定义百炼 已启用代理访问', exact: true }),
  ).toBeVisible()
  const createdStatus = row.getByRole('switch', { name: '自定义百炼的启用状态' })
  await expect(createdStatus).toHaveText('')
  await expect(createdStatus).toBeDisabled()
  await expect(createdStatus).toHaveAttribute('title', '请先配置凭证。')
  const missingCredential = row.getByRole('button', {
    name: '管理 自定义百炼 的认证凭据',
    exact: true,
  })
  await expect(missingCredential).toHaveText('凭证')
  await missingCredential.click()
  await expect(dialog.getByText('暂无认证凭据', { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '新增凭据', exact: true }).click()
  await expect(dialog).not.toHaveClass(/medium/)
  const credentialFormRows = dialog.locator('.credential-form-row')
  await expect(credentialFormRows).toHaveCount(2)
  await expect(credentialFormRows.locator('.credential-form-label')).toHaveText([
    '认证方式',
    'API Key',
  ])
  await expect(credentialFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  const apiKeyInput = dialog.getByLabel('API Key', { exact: true })
  await expect(apiKeyInput).toHaveAttribute('type', 'text')
  await apiKeyInput.fill('aliyun-fixture-credential')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('row').filter({ hasText: '自定义百炼 API Key' })).toBeVisible()
  await expect(page.locator('.toast-success')).toContainText('已保存')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(
    row.getByRole('button', { name: '同步 自定义百炼 的官方模型', exact: true }),
  ).toHaveCount(0)
  expect(state.modelSyncRequests).toHaveLength(0)
  await expect(
    row.getByRole('button', {
      name: '管理 自定义百炼 的认证凭据',
      exact: true,
    }),
  ).toHaveText('凭证')
  await expect(createdStatus).toBeEnabled()
  await expect(createdStatus).not.toHaveAttribute('title')
  await expect(
    row.getByRole('button', { name: '验证 自定义百炼 的模型调用', exact: true }),
  ).toHaveCount(0)
  await row
    .getByRole('button', {
      name: '管理 自定义百炼 的认证凭据',
      exact: true,
    })
    .click()
  const createdCredentialRow = modal(page)
    .getByRole('row')
    .filter({ hasText: '自定义百炼 API Key' })
  const testCredential = createdCredentialRow.getByRole('button', {
    name: '验证 自定义百炼 API Key 的可用性',
    exact: true,
  })
  await expect(testCredential).toBeVisible()
  await testCredential.click()
  await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  expect(state.resources.find((resource) => resource.providerId === created.id)?.status).toBe(
    'ACTIVE',
  )
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  await expect(dialog.getByLabel('代理服务器地址', { exact: true })).not.toHaveValue(
    /proxy-password/,
  )
  await expect(dialog.getByLabel('KEY', { exact: true })).toHaveValue('X-Proxy-Token')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveValue('')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveAttribute(
    'placeholder',
    '已配置；留空保持不变',
  )
  await dialog.screenshot({ path: '../.cache/web-visual/provider-proxy-config.png' })
  await dialog.getByLabel('服务商名称').fill('阿里云模型服务')
  state.conflict(true)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  const providerErrorToast = page.locator('.toast')
  await expect(providerErrorToast).toContainText('操作冲突')
  await expect(providerErrorToast).toBeVisible()
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  expect(
    await providerErrorToast.evaluate((element) =>
      element.closest('[popover]')?.matches(':popover-open'),
    ),
  ).toBe(true)
  state.conflict(false)
  await providerErrorToast.getByRole('button', { name: '关闭' }).click()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: '阿里云模型服务' })).toBeVisible()

  const renamedStatus = page.getByRole('switch', { name: '阿里云模型服务的启用状态' })
  await renamedStatus.click()
  await page
    .getByRole('dialog', { name: '阿里云模型服务 / 选择启用时验证的连接' })
    .getByRole('button', { name: '测试并启用' })
    .click()
  await expect(renamedStatus).toBeChecked()
  expect(created.status).toBe('ACTIVE')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  await expect(modal(page)).toHaveCount(0)
  await mkdir('../.cache/web-visual', { recursive: true })
  await renamedStatus.click()
  await expect(renamedStatus).not.toBeChecked()
  expect(created.status).toBe('DISABLED')
  await expect(modal(page)).toHaveCount(0)
  await page.screenshot({
    path: '../.cache/web-visual/provider-catalog-desktop.png',
    fullPage: true,
    animations: 'disabled',
  })

  await page.setViewportSize({ width: 900, height: 900 })
  await expect(providerLink.locator('span')).toBeHidden()
  await expect
    .poll(() => page.locator('.sidebar').evaluate((element) => element.clientWidth))
    .toBe(76)

  await page.setViewportSize({ width: 1200, height: 900 })
  const renamedRow = page.getByRole('row').filter({ hasText: '阿里云模型服务' })
  await renamedRow.locator('summary').click()
  await renamedRow.getByRole('button', { name: '删除', exact: true }).click()
  await expect(modal(page)).toContainText('确认删除服务商「阿里云模型服务」')
  await modal(page).screenshot({ path: '../.cache/web-visual/delete-confirm-dialog.png' })
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: '阿里云模型服务' })).toHaveCount(0)
  expect(state.providerMappings.has(created.id)).toBe(false)
  expect(state.resources.some((resource) => resource.providerId === created.id)).toBe(false)
})

test('服务商列表展示凭证聚合运行状态和错误原因', async ({ page }) => {
  const state = await fixture(page)
  state.resources.push({
    id: '88',
    providerId: '81',
    name: 'DeepSeek API Key',
    status: 'ACTIVE',
    runtimeStatus: 'BLOCKED',
    blockedReason: 'BILLING',
    blockedAt: stamp,
    lastErrorAt: stamp,
    lastHttpStatus: 402,
    lastErrorCode: 'UPSTREAM_BILLING_BLOCKED',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  await expect(page.getByRole('columnheader', { name: '运行状态', exact: true })).toBeVisible()
  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(row.getByText('不可用', { exact: true })).toBeVisible()
  await expect(row).not.toContainText('余额或计费异常 · HTTP 402')
  await expect(row.locator('.provider-runtime-state')).toHaveAttribute(
    'title',
    '查看 DeepSeek 的凭证，当前运行状态：不可用：余额或计费异常 · HTTP 402 · UPSTREAM_BILLING_BLOCKED',
  )
  await row.getByRole('button', { name: /查看 DeepSeek 的凭证/ }).click()
  await expect(modal(page).getByRole('heading', { name: '凭证配置' })).toBeVisible()
  await expect(modal(page).locator('.credential-title-line')).toContainText('DeepSeek')
  await expect(modal(page).locator('.credential-title-line')).toContainText('启用')
  await expect(modal(page).locator('.credential-overview dt')).toHaveText(['Base URL'])
  await expect(modal(page).locator('.credential-overview-endpoints dd > span')).toHaveCount(2)
  await expect(modal(page).locator('.credential-overview')).toContainText(
    'https://api.deepseek.com',
  )
  await expect(
    modal(page).getByRole('button', { name: '验证并恢复 DeepSeek API Key', exact: true }),
  ).toBeVisible()
  await expect(modal(page).getByRole('button', { name: '保存', exact: true })).toHaveCount(0)
  await expect(modal(page).locator('.modal-footer')).toHaveCount(0)
  await modal(page).screenshot({ path: '../.cache/web-visual/provider-credential-config.png' })
  const credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
  await expect(credentialRow).not.toContainText('••••••••')
  await expect(credentialRow).toContainText('最后验证时间')
  await expect(modal(page).getByRole('columnheader')).toHaveText([
    '认证凭据',
    '认证方式',
    '运行状态',
    '操作',
  ])
  const runtimeCell = credentialRow.getByRole('cell').nth(2)
  await expect(runtimeCell).toContainText('不可用')
  await expect(runtimeCell).toContainText('余额或计费异常 · HTTP 402')
  await expect(runtimeCell).toContainText('UPSTREAM_BILLING_BLOCKED')
  const credentialList = modal(page).locator('.credential-list')
  expect(
    await credentialList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)

  await page.setViewportSize({ width: 560, height: 800 })
  await expect(credentialRow).toHaveCSS('display', 'grid')
  expect(
    await credentialList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)
  await modal(page).screenshot({
    path: '../.cache/web-visual/provider-credential-config-mobile.png',
  })
})

test('个人订阅额度刷新失败时展示 Codex 可执行文件配置项', async ({ page }) => {
  const state = await fixture(page)
  state.providers.push({
    id: '82',
    name: 'OpenAI',
    code: 'openai-official',
    type: 'OFFICIAL',
    status: 'ACTIVE',
    website: 'https://openai.com',
    endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
    proxyEnabled: false,
    proxyUrl: null,
    proxyHeaders: [],
    modelSyncSupported: false,
    authAdapters: ['API_KEY', 'OPENAI_CODEX'],
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.resources.push({
    id: '89',
    providerId: '82',
    name: 'OpenAI 个人订阅 · ccount',
    authType: 'SUBSCRIPTION',
    authAdapter: 'OPENAI_CODEX',
    subscriptionType: 'PERSONAL',
    planCode: 'plus',
    externalAccountRef: 'fixture-account',
    priority: 100,
    effectiveAt: null,
    expiresAt: null,
    quotaStatus: 'UNKNOWN',
    quotaCheckedAt: null,
    quotaResetsAt: null,
    runtimeStatus: 'HEALTHY',
    blockedReason: null,
    blockedAt: null,
    lastErrorAt: null,
    lastHttpStatus: null,
    lastErrorCode: null,
    credentialConfigured: true,
    subscriptionPrice: null,
    createdAt: stamp,
    updatedAt: stamp,
  })
  await page.route('**/api/v1/resources/89/test-connection', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'OK',
        data: {
          ok: false,
          code: 'CODEX_APP_SERVER_UNAVAILABLE',
          httpStatus: null,
          latencyMs: 0,
        },
        requestId: 'req_codex_app_server_unavailable',
      }),
    }),
  )

  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: 'OpenAI' })
    .getByRole('button', { name: '管理 OpenAI 的认证凭据', exact: true })
    .click()

  const subscriptionRow = modal(page).getByRole('row').filter({ hasText: '个人订阅' })
  await expect(subscriptionRow.locator('.credential-quota-feedback')).toHaveText(
    '额度刷新失败 · Codex App Server 无法启动，请检查 Codex 可执行文件配置（CODEX_EXECUTABLE）并重启后端。',
  )
})

test('服务商支持个人订阅优先并保留 API Key 兜底', async ({ page }) => {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  const state = await fixture(page)
  state.providers.push({
    id: '82',
    name: 'OpenAI',
    code: 'openai-official',
    type: 'OFFICIAL',
    status: 'ACTIVE',
    website: 'https://openai.com',
    endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
    proxyEnabled: false,
    proxyUrl: null,
    proxyHeaders: [],
    modelSyncSupported: false,
    authAdapters: ['API_KEY', 'OPENAI_CODEX'],
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.models.push({
    id: '75',
    code: 'gpt-fixture',
    name: 'GPT Fixture',
    status: 'ACTIVE',
    inputModalities: ['TEXT'],
    outputModalities: ['TEXT'],
    remark: '',
    publisherProviderId: '82',
    publisherProviderName: 'OpenAI',
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.providerMappings.set('82', [
    {
      id: '94',
      providerId: '82',
      modelId: '75',
      upstreamModelCode: 'gpt-fixture',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    },
  ])
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  const providerRow = page.getByRole('row').filter({ hasText: 'OpenAI' })

  await providerRow
    .getByRole('button', {
      name: '管理 OpenAI 的认证凭据',
      exact: true,
    })
    .click()
  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
  const authJSON = JSON.stringify({
    auth_mode: 'chatgpt',
    tokens: {
      id_token: 'fixture-id-token',
      access_token: 'fixture-access-token',
      refresh_token: 'fixture-refresh-token',
      account_id: 'fixture-account',
    },
  })
  const subscriptionTabs = modal(page).getByRole('tablist', {
    name: '个人订阅凭据导入方式',
  })
  await expect(subscriptionTabs.getByRole('tab')).toHaveText(['上传文件', '手动输入'])
  await expect(subscriptionTabs.getByRole('tab', { name: '上传文件' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  const sourceGuide = modal(page).locator('.subscription-source-guide')
  const sourceSummary = sourceGuide.locator('summary')
  await expect(sourceSummary).toHaveText('找不到文件？查看打开目录命令')
  await expect(sourceGuide.locator('.subscription-command-list')).toBeHidden()
  await modal(page).screenshot({
    path: '../.cache/web-visual/provider-subscription-upload.png',
  })
  await sourceSummary.click()
  await expect(sourceGuide).toHaveAttribute('open', '')
  await expect(sourceGuide).toContainText('open ~/.codex')
  await expect(sourceGuide).toContainText('explorer.exe')
  await expect(sourceGuide).toContainText('xdg-open ~/.codex')
  await expect(sourceGuide.getByRole('button', { name: /复制 .* 命令/ })).toHaveCount(3)
  await expect(modal(page)).toContainText('运行上方命令打开 Codex 目录，然后选择 auth.json。')
  await modal(page)
    .getByLabel('Codex auth.json', { exact: true })
    .setInputFiles({
      name: 'auth.json',
      mimeType: 'application/json',
      buffer: Buffer.from(authJSON),
    })
  await expect(modal(page).locator('.subscription-file-picker')).toContainText('auth.json')
  await subscriptionTabs.getByRole('tab', { name: '上传文件' }).press('ArrowRight')
  await expect(subscriptionTabs.getByRole('tab', { name: '手动输入' })).toBeFocused()
  await expect(subscriptionTabs.getByRole('tab', { name: '手动输入' })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  const authContent = modal(page).getByLabel('auth.json 内容', { exact: true })
  await expect(authContent).toBeVisible()
  await expect(modal(page).getByLabel('Codex auth.json', { exact: true })).toHaveCount(0)
  await expect(sourceSummary).toHaveText('查看复制文件内容命令')
  await expect(sourceGuide.locator('.subscription-command-list')).toBeHidden()
  await expect(modal(page)).toContainText('运行上方命令获取内容，然后粘贴到输入框。')
  await expect(modal(page).getByText('auth.json 内容', { exact: true })).toHaveCount(0)
  await modal(page).screenshot({
    path: '../.cache/web-visual/provider-subscription-paste.png',
  })
  await sourceSummary.click()
  await expect(sourceGuide).toContainText('pbcopy < ~/.codex/auth.json')
  await expect(sourceGuide).toContainText('Set-Clipboard')
  await expect(sourceGuide).toContainText('cat ~/.codex/auth.json')
  await sourceGuide.getByRole('button', { name: '复制 macOS 命令', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText('已复制')
  await authContent.fill('{')
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText(
    '请输入或上传不超过 64 KiB 的有效 Codex auth.json。',
  )
  await authContent.fill(authJSON)
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  const credentialList = modal(page)
  await expect(credentialList).toContainText(
    '测试连接成功后会重新获取额度状态、额度重置时间和检查时间。',
  )
  await expect(credentialList.getByRole('columnheader')).toHaveText([
    '认证凭据',
    '认证方式',
    '运行状态',
    '操作',
  ])
  const subscriptionRow = credentialList.getByRole('row').filter({ hasText: '个人订阅' })
  await expect(subscriptionRow).toContainText('OpenAI 个人订阅 · ccount')
  const subscriptionAuthCell = subscriptionRow.getByRole('cell').nth(1)
  await expect(subscriptionAuthCell).not.toContainText('个人订阅')
  await expect(subscriptionAuthCell).toContainText('订阅套餐 · plus')
  await expect(subscriptionAuthCell.locator('.credential-plan')).toHaveCSS(
    'border-bottom-style',
    'solid',
  )
  await expect(subscriptionRow).not.toContainText('Token')
  await expect(subscriptionRow).not.toContainText('••••••••')
  await expect(subscriptionRow).toContainText('最后验证时间')
  await expect(subscriptionRow.locator('.credential-quota-reset')).toHaveText('额度重置时间 · -')
  await expect(subscriptionRow.getByRole('cell').nth(2)).toHaveText('正常')
  await expect(
    subscriptionRow.getByRole('button', {
      name: '验证 OpenAI 个人订阅 · ccount 的可用性',
      exact: true,
    }),
  ).toBeVisible()
  await expect(subscriptionRow.getByRole('button', { name: '删除', exact: true })).toBeVisible()
  expect(state.resources).toEqual([
    expect.objectContaining({ authType: 'SUBSCRIPTION', authAdapter: 'OPENAI_CODEX' }),
  ])

  await page.route(
    '**/api/v1/resources/*/credential/export',
    (route) =>
      route.fulfill({
        status: 503,
        contentType: 'text/html',
        body: '<html><body>Service Unavailable</body></html>',
      }),
    { times: 1 },
  )
  await subscriptionRow
    .getByRole('button', { name: '导出 OpenAI 个人订阅 · ccount 的 auth.json', exact: true })
    .click()
  await expect(page.getByRole('alert')).toContainText('服务暂时不可用')

  const downloadPromise = page.waitForEvent('download')
  await subscriptionRow
    .getByRole('button', { name: '导出 OpenAI 个人订阅 · ccount 的 auth.json', exact: true })
    .click()
  const exported = await downloadPromise
  expect(exported.suggestedFilename()).toBe('auth.json')
  const exportedStream = await exported.createReadStream()
  const exportedChunks: Buffer[] = []
  for await (const chunk of exportedStream) exportedChunks.push(Buffer.from(chunk))
  expect(Buffer.concat(exportedChunks).toString()).toBe(authJSON)
  await expect(page.getByRole('status')).toContainText('已导出 auth.json')

  await subscriptionRow
    .getByRole('button', { name: '验证 OpenAI 个人订阅 · ccount 的可用性', exact: true })
    .click()
  await expect(modal(page).getByRole('status')).toContainText('个人订阅验证通过')
  await expect(modal(page)).toContainText(
    '成功后会更新额度状态、额度重置时间和检查时间，不会发起模型调用。',
  )
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  expect(state.resources[0].quotaStatus).toBe('AVAILABLE')
  const automaticQuotaRefresh = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname.endsWith(
        `/resources/${state.resources[0].id}/test-connection`,
      ),
  )
  const quotaDetailRequest = page.waitForRequest((request) =>
    new URL(request.url()).pathname.endsWith(`/resources/${state.resources[0].id}/quotas`),
  )
  await providerRow
    .getByRole('button', {
      name: '管理 OpenAI 的认证凭据',
      exact: true,
    })
    .click()
  await automaticQuotaRefresh
  await quotaDetailRequest
  const refreshedSubscriptionRow = modal(page).getByRole('row').filter({ hasText: '个人订阅' })
  const formattedResetAt = await page.evaluate((value) => {
    return new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'medium',
      timeStyle: 'short',
      hour12: false,
    }).format(new Date(value))
  }, stamp)
  await expect(refreshedSubscriptionRow).toContainText('5 小时窗口 · 剩余 75%')
  await expect(refreshedSubscriptionRow).toContainText('7 天窗口 · 剩余 94%')
  await expect(refreshedSubscriptionRow).toContainText('Codex Spark · 7 天窗口 · 剩余 100%')
  await expect(refreshedSubscriptionRow).not.toContainText('额度可用')
  await expect(refreshedSubscriptionRow.locator('.credential-quota-reset')).toHaveText([
    `额度重置时间 · ${formattedResetAt}`,
    `额度重置时间 · ${formattedResetAt}`,
    `额度重置时间 · ${formattedResetAt}`,
  ])
  await expect(refreshedSubscriptionRow).toContainText('可用重置卡 · 1')
  const consumeResetCreditRequest = page.waitForRequest(
    (request) =>
      request.method() === 'POST' &&
      new URL(request.url()).pathname.endsWith(
        `/resources/${state.resources[0].id}/rate-limit-reset-credit/consume`,
      ),
  )
  await refreshedSubscriptionRow.getByRole('button', { name: '使用重置卡', exact: true }).click()
  await consumeResetCreditRequest
  await expect(page.getByRole('status')).toContainText('重置卡使用成功，额度已刷新')
  await expect(refreshedSubscriptionRow).toContainText('可用重置卡 · 0')
  await expect(
    refreshedSubscriptionRow.getByRole('button', { name: '使用重置卡', exact: true }),
  ).toHaveCount(0)
  await modal(page).screenshot({
    path: '../.cache/web-visual/provider-subscription-quota.png',
  })
  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('API_KEY')
  await modal(page).getByLabel('API Key', { exact: true }).fill('fallback-api-key')
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  const apiKeyRow = modal(page).getByRole('row').filter({ hasText: 'OpenAI API Key' })
  await expect(apiKeyRow).toBeVisible()
  await expect(apiKeyRow.getByRole('button', { name: /导出/ })).toHaveCount(0)

  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
  await modal(page).getByRole('tab', { name: '手动输入' }).click()
  await modal(page)
    .getByLabel('auth.json 内容', { exact: true })
    .fill(authJSON.replace('fixture-account', 'fixture-account-2'))
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  const secondSubscriptionRow = modal(page)
    .getByRole('row')
    .filter({ hasText: 'OpenAI 个人订阅 · ount-2' })
  await expect(secondSubscriptionRow).toBeVisible()
  expect(state.resources).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ name: 'OpenAI 个人订阅 · ccount' }),
      expect.objectContaining({ name: 'OpenAI 个人订阅 · ount-2' }),
    ]),
  )
  await secondSubscriptionRow.getByRole('button', { name: '删除', exact: true }).click()
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(secondSubscriptionRow).toHaveCount(0)

  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
  await modal(page).getByRole('tab', { name: '手动输入' }).click()
  await modal(page).getByLabel('auth.json 内容', { exact: true }).fill(authJSON)
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText(
    '该服务商下已存在此订阅账号的凭证，请更新已有凭证。',
  )
  await modal(page).getByRole('button', { name: '取消', exact: true }).click()

  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  expect(state.resources).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ authType: 'SUBSCRIPTION' }),
      expect.objectContaining({ authType: 'API_KEY' }),
    ]),
  )

  await providerRow.getByRole('button', { name: '测试 OpenAI 的连接', exact: true }).click()
  const testSelectionDialog = page.getByRole('dialog', {
    name: 'OpenAI / 选择测试凭证',
  })
  await expect(testSelectionDialog).toBeVisible()
  await expect(testSelectionDialog.getByRole('radio')).toHaveCount(2)
  await testSelectionDialog
    .getByRole('radio', { name: '使用 OpenAI API Key 测试连接', exact: true })
    .check()
  await testSelectionDialog.getByRole('button', { name: '开始测试', exact: true }).click()
  await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()

  await providerRow
    .getByRole('button', {
      name: '管理 OpenAI 的认证凭据',
      exact: true,
    })
    .click()
  await modal(page)
    .getByRole('row')
    .filter({ hasText: 'OpenAI 个人订阅 · ccount' })
    .getByRole('button', { name: '删除', exact: true })
    .click()
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect
    .poll(() => state.resources.filter((resource) => resource.authType === 'SUBSCRIPTION').length)
    .toBe(0)
  expect(state.resources.some((resource) => resource.authType === 'API_KEY')).toBe(true)
})

test('Anthropic 官方渠道直接输入 Claude 个人订阅 Token', async ({ page }) => {
  const state = await fixture(page)
  state.providers.push({
    id: '83',
    name: 'Anthropic',
    code: 'anthropic-official',
    type: 'OFFICIAL',
    status: 'ACTIVE',
    website: 'https://www.anthropic.com',
    endpoints: [{ protocolType: 'ANTHROPIC', baseUrl: 'https://api.anthropic.com' }],
    proxyEnabled: false,
    proxyUrl: null,
    proxyHeaders: [],
    modelSyncSupported: false,
    authAdapters: ['API_KEY', 'ANTHROPIC_CLAUDE_CODE'],
    createdAt: stamp,
    updatedAt: stamp,
  })
  await signIn(page)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  const providerRow = page.getByRole('row').filter({ hasText: 'Anthropic' })
  await providerRow.getByRole('button', { name: '管理 Anthropic 的认证凭据' }).click()
  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')

  const token = 'future-anthropic-token-format'
  await expect(modal(page)).toContainText('claude setup-token')
  const commandGuide = modal(page).locator('div.subscription-source-guide')
  await expect(commandGuide).toBeVisible()
  await expect(modal(page).locator('details.subscription-source-guide')).toHaveCount(0)
  await expect(commandGuide.locator('code')).toHaveText('claude setup-token')
  await expect(modal(page).getByRole('tablist')).toHaveCount(0)
  await expect(modal(page).locator('input[type="file"]')).toHaveCount(0)
  await expect(modal(page).locator('.subscription-command-list > div')).toHaveCount(1)
  await expect(modal(page).getByRole('button', { name: '复制 Claude Code 命令' })).toBeVisible()
  const tokenInput = modal(page).getByLabel('Claude Code OAuth Token', { exact: true })
  await tokenInput.fill('invalid token')
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('有效 Claude Code OAuth Token')

  await tokenInput.fill(token)
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  expect(state.resources).toEqual([
    expect.objectContaining({
      authType: 'SUBSCRIPTION',
      authAdapter: 'ANTHROPIC_CLAUDE_CODE',
      credential: token,
    }),
  ])
  await expect(modal(page).getByRole('row').filter({ hasText: 'Anthropic 个人订阅' })).toBeVisible()
})

test('状态 switch 直接生效且危险操作仍需确认', async ({ page }) => {
  await fixture(page)
  await signIn(page)
  await page.getByRole('button', { name: '界面语言', exact: true }).click()
  await page.getByRole('menuitemradio', { name: 'English', exact: true }).click()
  await page.getByRole('link', { name: 'Providers', exact: true }).click()
  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  const status = row.getByRole('switch', { name: 'Status for DeepSeek' })
  await status.click()
  await expect(status).not.toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  await row.locator('summary', { hasText: '⋮' }).click()
  await row.getByRole('button', { name: 'Delete', exact: true }).click()

  const dialog = modal(page)
  await expect(dialog).toHaveAccessibleName('Delete provider')
  await expect(dialog).toContainText(
    'Its model mappings and configured keys will stop routing immediately and cannot be restored.',
  )
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  await expect(dialog.getByRole('button', { name: 'Delete', exact: true })).toBeVisible()
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
    true,
  )
  await mkdir('../.cache/web-visual', { recursive: true })
  await dialog.screenshot({ path: '../.cache/web-visual/delete-confirm-dialog-en.png' })
})

test('创建和编辑用户时可选择分组', async ({ page }) => {
  const state = await fixture(page)
  state.groups.push(
    {
      id: '51',
      code: 'engineering',
      name: '研发组',
      remark: '研发模型权限',
      status: 'ACTIVE',
      createdAt: stamp,
    },
    {
      id: '52',
      code: 'legacy',
      name: '停用组',
      status: 'DISABLED',
      createdAt: stamp,
    },
    {
      id: '53',
      code: 'operations',
      name: '运维组',
      status: 'ACTIVE',
      createdAt: stamp,
    },
    {
      id: '54',
      code: 'quality',
      name: '质量组',
      status: 'ACTIVE',
      createdAt: stamp,
    },
  )
  state.relationships.set('groups/51/members', new Set([longID]))
  state.relationships.set('groups/52/members', new Set([longID]))

  await signIn(page)
  await page.getByRole('button', { name: '创建用户' }).click()
  const dialog = modal(page)
  const createActive = dialog.getByRole('checkbox', { name: '选择分组 研发组', exact: true })
  const createDisabled = dialog.getByRole('checkbox', { name: '选择分组 停用组', exact: true })
  await expect(createActive).toBeEnabled()
  await expect(createDisabled).toHaveCount(0)
  expect(state.groupQueries.at(-1)?.get('status')).toBe('ACTIVE')
  await expect(dialog.getByText(/已选择.*共.*个分组/)).toHaveCount(0)
  const groupOptions = dialog.locator('.member-group-option')
  await expect(groupOptions).toHaveCount(3)
  const optionTops = await groupOptions.evaluateAll((options) =>
    options.map((option) => Math.round(option.getBoundingClientRect().top)),
  )
  expect(new Set(optionTops).size).toBe(1)
  await expect(dialog.getByLabel('备注', { exact: true })).toHaveAttribute('rows', '3')
  await expect(dialog.locator('.member-group-list').getByText('启用', { exact: true })).toHaveCount(
    0,
  )
  await dialog.getByLabel('用户名', { exact: true }).fill('新用户')
  await createActive.check()
  await dialog.getByRole('button', { name: '创建', exact: true }).click()
  const created = state.members.find((member) => member.name === '新用户')
  expect(created.status).toBe('DISABLED')
  await expect(
    page
      .getByRole('row')
      .filter({ hasText: '新用户' })
      .getByRole('switch', { name: '新用户的激活状态' }),
  ).not.toBeChecked()
  expect(state.relationships.get('groups/51/members')?.has(created.id)).toBe(true)
  expect(state.relationships.get('groups/52/members')?.has(created.id)).toBe(false)

  const memberRow = page.getByRole('row').filter({ hasText: '林知远' })
  await memberRow.getByRole('button', { name: '编辑', exact: true }).click()
  const editActive = dialog.getByRole('checkbox', { name: '选择分组 研发组', exact: true })
  const editDisabled = dialog.getByRole('checkbox', { name: '选择分组 停用组', exact: true })
  await expect(dialog).toHaveAccessibleName('编辑 林知远')
  await expect(editActive).toBeChecked()
  await expect(editDisabled).toHaveCount(0)
  await dialog.getByLabel('用户名', { exact: true }).fill('林知远（编辑）')
  await dialog.getByLabel('备注', { exact: true }).fill('已调整分组')
  state.conflict(true)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  expect(state.members.find((member) => member.id === longID)?.name).toBe('林知远')
  state.conflict(false)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(page.locator('.toast-success')).toContainText('已保存')
  await expect(page.locator('.notice')).toHaveCount(0)
  expect(state.members.find((member) => member.id === longID)?.name).toBe('林知远（编辑）')
  expect(state.relationships.get('groups/51/members')?.has(longID)).toBe(true)
  expect(state.relationships.get('groups/52/members')?.has(longID)).toBe(false)
})

test('分组编辑表单与创建一致、失败恢复及停用授权限制', async ({ page }) => {
  const state = await fixture(page)
  state.groups.push({
    id: '51',
    code: 'engineering',
    name: '研发组',
    status: 'ACTIVE',
    createdAt: stamp,
  })
  state.models[1].status = 'DISABLED'
  await signIn(page)
  await page.getByRole('link', { name: '用户分组', exact: true }).click()
  await page.getByRole('button', { name: '创建分组' }).click()
  const createDialog = modal(page)
  const createFormRows = createDialog.locator('.group-form-row')
  await expect(createFormRows).toHaveCount(3)
  await expect(createFormRows.locator('.group-form-label')).toHaveText(['名称', '访问模型', '备注'])
  await expect(createFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await expect(createDialog.getByLabel('名称', { exact: true })).toHaveAttribute('required', '')
  await expect(createFormRows.nth(1).locator('.group-form-label')).toHaveClass(/required-label/)
  await expect(createFormRows.nth(1).locator('.group-model-field')).toHaveAttribute(
    'aria-required',
    'true',
  )
  expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
  await expect(createDialog.getByLabel('备注', { exact: true })).toBeVisible()
  await expect(createDialog.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await expect(createDialog.getByText('Claude Sonnet', { exact: true })).toHaveCount(0)
  const createSelectedPane = createDialog.locator('.group-model-selected')
  await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('0')
  await expect(createSelectedPane).toContainText('尚未选择模型，请从模型列表中勾选。')
  const createModelSearch = createDialog.getByRole('searchbox', { name: '搜索模型' })
  await expect(createModelSearch).toHaveAttribute('placeholder', '搜索模型名称、编码或厂商')
  await expect(createDialog.locator('.group-model-search-box > svg')).toHaveCount(0)
  await createModelSearch.fill('deepseek-v4-flash')
  await expect(createDialog.getByText('显示 1 个', { exact: true })).toBeVisible()
  await expect(createDialog.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await createModelSearch.fill('不存在的模型')
  await expect(createDialog.getByText('显示 0 个', { exact: true })).toBeVisible()
  await expect(createDialog.locator('.group-model-filter-empty')).toContainText('没有匹配的模型。')
  await createDialog.getByRole('button', { name: '清除筛选', exact: true }).click()
  await expect(createModelSearch).toHaveValue('')
  const createCatalogHead = createDialog.locator('.group-model-catalog .group-model-pane-head')
  await expect(createCatalogHead.getByText('模型列表', { exact: true })).toBeVisible()
  await expect(createCatalogHead.locator('.group-model-count-tag')).toHaveText('1')
  const createBulkActions = createCatalogHead.locator('.group-model-bulk-actions')
  await expect(createBulkActions.getByRole('button', { name: '全选', exact: true })).toBeVisible()
  await expect(createBulkActions.locator('.group-model-action-separator')).toHaveText('/')
  await expect(createBulkActions.getByRole('button', { name: '反选', exact: true })).toBeVisible()
  await expect(createCatalogHead.getByText('名称', { exact: true })).toHaveCount(0)
  await expect(createDialog.locator('.group-model-catalog .group-model-list-head')).toHaveCount(0)
  await expect(createDialog.getByText('deepseek-v4-flash', { exact: true })).toBeVisible()
  const createModelRow = createDialog
    .locator('.group-model-catalog-row')
    .filter({ hasText: 'DeepSeek V4 Flash' })
  await expect(createModelRow.locator('.modality-tag')).toHaveText(['输入：文本', '输出：文本'])
  await createDialog.getByLabel('名称', { exact: true }).fill('必填校验分组')
  await createDialog.getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('必须选择至少一个访问模型')
  await expect(createDialog.locator('.alert.error')).toHaveCount(0)
  await createDialog.getByRole('button', { name: '关闭', exact: true }).click()
  state.models[1].status = 'ACTIVE'
  await page.getByRole('button', { name: '创建分组' }).click()
  const multimodalRow = modal(page)
    .locator('.group-model-catalog-row')
    .filter({ hasText: 'Claude Sonnet' })
  await expect(multimodalRow.locator('.modality-tag')).toHaveText([
    '输入：文本 / 图片',
    '输出：文本',
  ])
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  state.models[1].status = 'DISABLED'
  await page.getByRole('button', { name: '编辑', exact: true }).click()
  expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
  const dialog = modal(page)
  const active = dialog.getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true })
  const disabled = dialog.getByRole('checkbox', { name: '授权 Claude Sonnet', exact: true })
  await expect(dialog.getByLabel('名称', { exact: true })).toHaveValue('研发组')
  await expect(dialog.getByLabel('备注', { exact: true })).toBeVisible()
  await expect(active).toBeEnabled()
  await expect(active).not.toBeChecked()
  await expect(disabled).toHaveCount(0)
  await dialog.getByLabel('名称', { exact: true }).fill('研发平台组')
  await dialog.getByLabel('备注', { exact: true }).fill('更新后的备注')
  await active.check()
  state.conflict(true)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  await expect(active).toBeChecked()
  expect(state.relationships.get('groups/51/models')?.has('71')).toBe(false)
  state.conflict(false)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  expect(state.groups[0].name).toBe('研发平台组')
  expect(state.groups[0].remark).toBe('更新后的备注')
  expect(state.relationships.get('groups/51/models')?.has('71')).toBe(true)

  await page.getByRole('button', { name: '编辑', exact: true }).click()
  const editModelSearch = dialog.getByRole('searchbox', { name: '搜索模型' })
  await expect(editModelSearch).toBeVisible()
  await expect(active).toBeVisible()
  await expect(active).toBeChecked()
  await editModelSearch.fill('没有匹配项')
  await expect(active).toHaveCount(0)
  const editSelectedPane = dialog.locator('.group-model-selected')
  await expect(editSelectedPane.locator('.group-model-pane-head > span')).toHaveText('1')
  await expect(editSelectedPane.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '清空模型搜索', exact: true }).click()
  await expect(active).toBeVisible()
  await expect(dialog.getByRole('button', { name: '移除 DeepSeek V4 Flash' })).toBeVisible()
  await mkdir('../.cache/web-visual', { recursive: true })
  await dialog.screenshot({ path: '../.cache/web-visual/group-model-checklist.png' })
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  state.relationships.get('groups/51/models')!.add('72')
  await page.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(disabled).toHaveCount(0)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  expect(state.relationships.get('groups/51/models')?.has('72')).toBe(false)
  state.groups[0].status = 'DISABLED'
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await page.getByRole('link', { name: '用户分组', exact: true }).click()
  await page.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(active).toBeChecked()
  await expect(active).toBeEnabled()
  await active.uncheck()
  await expect(active).not.toBeChecked()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  expect(state.relationships.get('groups/51/models')?.has('71')).toBe(false)
})

test('访问模型支持对当前筛选结果全选和反选', async ({ page }) => {
  const state = await fixture(page)
  state.groups.push({
    id: '51',
    code: 'engineering',
    name: '研发组',
    status: 'ACTIVE',
    createdAt: stamp,
  })
  state.relationships.set('groups/51/models', new Set(['71']))
  await signIn(page)
  await page.getByRole('link', { name: '用户分组', exact: true }).click()

  await page.getByRole('button', { name: '创建分组', exact: true }).click()
  const createDialog = modal(page)
  const createCatalog = createDialog.locator('.group-model-catalog')
  const createSelectAll = createCatalog.getByRole('button', { name: '全选', exact: true })
  const createInvertSelection = createCatalog.getByRole('button', {
    name: '反选',
    exact: true,
  })
  const createSearch = createDialog.getByRole('searchbox', { name: '搜索模型' })
  const createSelectedPane = createDialog.locator('.group-model-selected')
  await expect(createDialog.locator('.group-model-workspace')).toHaveCSS(
    'grid-template-columns',
    /\S+ \S+/,
  )
  const deepSeek = createDialog.getByRole('checkbox', {
    name: '授权 DeepSeek V4 Flash',
    exact: true,
  })
  const claude = createDialog.getByRole('checkbox', { name: '授权 Claude Sonnet', exact: true })
  await createSelectAll.click()
  await expect(deepSeek).toBeChecked()
  await expect(claude).toBeChecked()
  await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('2')
  await expect(createSelectedPane.locator('.group-model-list-head')).toHaveCount(0)
  await createInvertSelection.click()
  await expect(deepSeek).not.toBeChecked()
  await expect(claude).not.toBeChecked()

  await createSearch.fill('DeepSeek')
  await createSelectAll.click()
  await expect(deepSeek).toBeChecked()
  await createSearch.fill('Claude')
  await createSelectAll.click()
  await expect(claude).toBeChecked()
  await createSearch.fill('DeepSeek')
  await createInvertSelection.click()
  await expect(deepSeek).not.toBeChecked()
  await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('1')
  await createSearch.fill('')
  await expect(claude).toBeChecked()
  await createSelectedPane.getByRole('button', { name: '移除 Claude Sonnet' }).click()
  await expect(claude).not.toBeChecked()
  await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('0')
  await createDialog.getByRole('button', { name: '关闭', exact: true }).click()

  await page.getByRole('button', { name: '编辑', exact: true }).click()
  const editDialog = modal(page)
  const editCatalog = editDialog.locator('.group-model-catalog')
  const editSelectAll = editCatalog.getByRole('button', { name: '全选', exact: true })
  const editInvertSelection = editCatalog.getByRole('button', { name: '反选', exact: true })
  const editDeepSeek = editDialog.getByRole('checkbox', {
    name: '授权 DeepSeek V4 Flash',
    exact: true,
  })
  const editClaude = editDialog.getByRole('checkbox', {
    name: '授权 Claude Sonnet',
    exact: true,
  })
  await expect(editDeepSeek).toBeChecked()
  await expect(editClaude).not.toBeChecked()
  await editSelectAll.click()
  await expect(editDeepSeek).toBeChecked()
  await expect(editClaude).toBeChecked()
  await editInvertSelection.click()
  await expect(editDeepSeek).not.toBeChecked()
  await expect(editClaude).not.toBeChecked()
})

test('没有模型时提示先添加模型且不打开创建分组弹窗', async ({ page }) => {
  const state = await fixture(page)
  state.models.splice(0)
  await signIn(page)
  await page.getByRole('link', { name: '用户分组', exact: true }).click()

  await page.getByRole('button', { name: '创建分组' }).click()

  await expect(page.getByRole('alert')).toHaveText('请先添加模型。')
  await expect(modal(page)).toHaveCount(0)
  expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
})

test('分组列表按最新记录倒序显示并提示输入分组名称', async ({ page }) => {
  const state = await fixture(page)
  state.groups.push(
    {
      id: '51',
      code: 'earlier-group',
      name: '较早分组',
      remark: '核心服务组',
      status: 'ACTIVE',
      createdAt: '2026-09-06T14:10:00Z',
    },
    {
      id: '52',
      code: 'latest-group',
      name: '最新分组',
      status: 'ACTIVE',
      createdAt: '2026-09-06T16:13:00Z',
    },
  )
  await signIn(page)
  await page.getByRole('link', { name: '用户分组', exact: true }).click()

  await expect(page.getByRole('columnheader')).toHaveText([
    'ID',
    '名称',
    '启用状态',
    '月度额度',
    '创建时间',
    '备注',
    '操作',
  ])
  await expect(page.locator('tbody tr .person strong')).toHaveText(['最新分组', '较早分组'])
  await expect(
    page.getByRole('row').filter({ hasText: '最新分组' }).getByRole('cell').first().locator('code'),
  ).toHaveText('52')
  await expect(
    page
      .getByRole('row')
      .filter({ hasText: '最新分组' })
      .getByRole('cell')
      .first()
      .locator('.technical-value-copy'),
  ).toHaveCount(0)
  await expect(page.locator('tbody tr .avatar')).toHaveText(['最', '较'])
  await expect(
    page.getByRole('row').filter({ hasText: '最新分组' }).getByRole('cell').nth(5),
  ).toHaveText('-')
  await expect(
    page.getByRole('row').filter({ hasText: '较早分组' }).getByRole('cell').nth(5),
  ).toHaveText('核心服务组')
  await expect(page.getByRole('searchbox', { name: '分组名称' })).toHaveAttribute(
    'placeholder',
    '请输入分组名称',
  )
  await expect(page.locator('.list-search-label')).toHaveText('分组名称')

  const latestSwitch = page.getByRole('switch', { name: '最新分组的启用状态' })
  await expect(latestSwitch).toHaveText('')
  await expect(latestSwitch).toBeChecked()
  await latestSwitch.click()
  await expect(latestSwitch).not.toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  expect(state.groups.find((group) => group.id === '52')?.status).toBe('DISABLED')

  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await page.getByRole('button', { name: '创建用户' }).click()
  await expect(modal(page).getByRole('checkbox', { name: '选择分组 最新分组' })).toHaveCount(0)
  await expect(modal(page).getByRole('checkbox', { name: '选择分组 较早分组' })).toBeVisible()
})

test('没有用户分组时提示先添加分组且不打开创建用户弹窗', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page)

  await page.getByRole('button', { name: '创建用户' }).click()

  await expect(page.getByRole('alert')).toHaveText('请先添加用户分组。')
  await expect(modal(page)).toHaveCount(0)
  expect(state.groupQueries.at(-1)?.get('status')).toBe('ACTIVE')
})

test('启停状态时保留列表直到当前页重新加载完成', async ({ page }) => {
  await fixture(page)
  await signIn(page)
  await page.getByRole('link', { name: '模型', exact: true }).click()

  const modelRow = page.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
  const modelSwitch = modelRow.getByRole('switch', { name: 'DeepSeek V4 Flash的启用状态' })
  await expect(modelSwitch).toBeChecked()

  let releaseRefresh = () => {}
  const refreshGate = new Promise<void>((resolve) => {
    releaseRefresh = resolve
  })
  let delayNextRefresh = true
  await page.route('**/api/v1/models?**', async (route) => {
    if (delayNextRefresh) {
      delayNextRefresh = false
      await refreshGate
    }
    await route.fallback()
  })

  await modelSwitch.click()
  await expect(modal(page)).toHaveCount(0)
  await expect(modelRow).toBeVisible()
  await expect(page.locator('tbody tr')).toHaveCount(2)

  releaseRefresh()
  await expect(modelSwitch).not.toBeChecked()
})

test('管理员通过网页完成配置、Key 生命周期和用量查询', async ({ page }) => {
  const state = await fixture(page),
    errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await signIn(page)
  await expect(page.getByText(longID, { exact: true })).toBeVisible()
  const memberSearch = page.getByRole('searchbox', { name: '用户名' })
  await expect(memberSearch).toHaveAttribute('placeholder', '请输入用户名')
  await expect(page.locator('.list-search-label')).toHaveText('用户名')
  await memberSearch.fill('林知远')
  await expect(page.getByText('陈清和', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.getByText('陈清和', { exact: true })).toHaveCount(0)
  await memberSearch.fill('没有这个成员')
  await memberSearch.press('Enter')
  await expect(page.getByText('没有匹配的记录', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '重置', exact: true }).click()
  await expect(memberSearch).toHaveValue('')
  await expect(page.getByText('陈清和', { exact: true })).toBeVisible()
  state.groups.push({
    id: '70',
    code: 'default-group',
    name: '默认分组',
    remark: '',
    status: 'ACTIVE',
    createdAt: stamp,
    updatedAt: stamp,
  })
  await page.getByRole('button', { name: '创建用户' }).click()
  const memberFormRows = modal(page).locator('.member-form-row')
  await expect(memberFormRows).toHaveCount(3)
  await expect(memberFormRows.locator('.member-form-label')).toHaveText([
    '用户名',
    '用户分组',
    '备注',
  ])
  await expect(memberFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await mkdir('test-results/visual', { recursive: true })
  await page.screenshot({ path: 'test-results/visual/member-create.png', fullPage: true })
  await modal(page).getByLabel('用户名', { exact: true }).fill('浏览器验收成员')
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('必须选择至少一个用户分组')
  await expect(modal(page).locator('.alert.error')).toHaveCount(0)
  await modal(page).getByRole('checkbox', { name: '选择分组 默认分组' }).check()
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByText('浏览器验收成员', { exact: true })).toBeVisible()
  await page.screenshot({ path: 'test-results/visual/members.png', fullPage: true })

  await page.getByRole('link', { name: '用户分组', exact: true }).click()
  await page.getByRole('button', { name: '创建分组' }).click()
  await modal(page).getByLabel('名称', { exact: true }).fill('平台研发组')
  await modal(page).getByLabel('备注', { exact: true }).fill('平台模型权限')
  await expect(modal(page).getByLabel('编码', { exact: true })).toHaveCount(0)
  const createGrant = modal(page).getByRole('checkbox', {
    name: '授权 DeepSeek V4 Flash',
    exact: true,
  })
  await expect(createGrant).toBeEnabled()
  await createGrant.check()
  await expect(
    modal(page).locator('.group-model-selected .group-model-pane-head > span'),
  ).toHaveText('1')
  state.conflict(true)
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('操作冲突')
  state.conflict(false)
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByRole('button', { name: '用户', exact: true })).toHaveCount(0)
  const createdGroup = state.groups.find((group) => group.name === '平台研发组')
  expect(createdGroup.code).toBe(`group-${createdGroup.id}`)
  expect(createdGroup.remark).toBe('平台模型权限')
  expect(state.relationships.get(`groups/${createdGroup.id}/models`)).toEqual(new Set(['71']))
  await page
    .getByRole('row')
    .filter({ hasText: '平台研发组' })
    .getByRole('button', { name: '编辑', exact: true })
    .click()
  await expect(
    modal(page).getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true }),
  ).toBeChecked()
  await expect(modal(page).getByText('DeepSeek V4 Flash', { exact: true })).toHaveCount(2)
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

  await page.getByRole('link', { name: '模型', exact: true }).click()
  const modelSwitch = page.getByRole('switch', { name: 'DeepSeek V4 Flash的启用状态' })
  await expect(modelSwitch).toBeChecked()
  await modelSwitch.press('Space')
  await expect(modelSwitch).not.toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  await modelSwitch.click()
  await expect(modelSwitch).toBeChecked()
  await expect(page.getByRole('button', { name: '授权', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: '用户分组', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: '平台研发组' })
    .getByRole('button', { name: '编辑', exact: true })
    .click()
  const modelGrant = modal(page).getByRole('checkbox', {
    name: '授权 DeepSeek V4 Flash',
    exact: true,
  })
  await expect(modelGrant).toBeChecked()
  await modelGrant.click()
  await expect(modelGrant).not.toBeChecked()
  await modelGrant.click()
  await expect(modelGrant).toBeChecked()
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  const createdGroupRow = page.getByRole('row').filter({ hasText: '平台研发组' })
  await createdGroupRow.locator('summary').click()
  await createdGroupRow.getByRole('button', { name: '删除', exact: true }).click()
  await expect(modal(page)).toContainText('用户关联和模型授权将失效')
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: '平台研发组' })).toHaveCount(0)
  expect(state.relationships.has(`groups/${createdGroup.id}/models`)).toBe(false)

  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await expect(page.getByRole('link', { name: '服务商凭证', exact: true })).toHaveCount(0)
  const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(providerRow.getByRole('link', { name: '在新页面打开 DeepSeek 官网' })).toHaveCount(0)
  await providerRow
    .getByRole('button', {
      name: '管理 DeepSeek 的认证凭据',
      exact: true,
    })
    .click()
  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByLabel('API Key', { exact: true }).fill('fixture-upstream-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await expect(modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })).toBeVisible()
  await expect(page.locator('.toast-success')).toContainText('已保存')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  const testProviderConnection = providerRow.getByRole('button', {
    name: '测试 DeepSeek 的连接',
    exact: true,
  })
  await expect(testProviderConnection).toBeVisible()
  await expect(testProviderConnection).toHaveAttribute('title', '测试连接')
  await expect(testProviderConnection.locator('svg')).toBeVisible()
  const startAnthropicConnectionTest = async () => {
    const protocolDialog = page.getByRole('dialog', {
      name: 'DeepSeek / 选择测试协议',
    })
    await expect(protocolDialog).toBeVisible()
    await expect(protocolDialog.getByText('https://api.deepseek.com/anthropic')).toBeVisible()
    await expect(
      protocolDialog.getByText('https://api.deepseek.com', { exact: true }),
    ).toBeVisible()
    const anthropic = protocolDialog.getByRole('radio', {
      name: '使用 Anthropic 协议测试连接',
      exact: true,
    })
    await expect(anthropic).toBeChecked()
    const requestPromise = page.waitForRequest((request) => {
      const url = new URL(request.url())
      return request.method() === 'POST' && url.pathname.endsWith('/test-connection')
    })
    await protocolDialog.getByRole('button', { name: '开始测试', exact: true }).click()
    const request = await requestPromise
    expect(new URL(request.url()).searchParams.get('protocol')).toBe('ANTHROPIC')
  }
  await testProviderConnection.click()
  await startAnthropicConnectionTest()
  await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
  await expect(modal(page).getByText('Anthropic', { exact: true })).toBeVisible()
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  expect(state.modelSyncRequests).toHaveLength(0)
  await providerRow.getByRole('button', { name: '同步 DeepSeek 的官方模型', exact: true }).click()
  await expect(modal(page).getByRole('status')).toContainText('模型目录同步完成')
  await expect(modal(page).getByText('官方接口', { exact: true })).toBeVisible()
  expect(state.modelSyncRequests).toHaveLength(1)
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  const editCredential = providerRow.getByRole('button', {
    name: '管理 DeepSeek 的认证凭据',
  })
  await editCredential.click()
  let credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
  let testConnection = credentialRow.getByRole('button', {
    name: '验证 DeepSeek API Key 的可用性',
    exact: true,
  })
  await expect(testConnection).toBeVisible()
  expect(await page.content()).not.toContain('fixture-upstream-credential')
  await testConnection.click()
  await startAnthropicConnectionTest()
  await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  state.failTest()
  await editCredential.click()
  credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
  testConnection = credentialRow.getByRole('button', {
    name: '验证 DeepSeek API Key 的可用性',
    exact: true,
  })
  await testConnection.click()
  await startAnthropicConnectionTest()
  await expect(modal(page).getByRole('status')).toContainText('上游认证失败')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  await expect(editCredential).toHaveText('凭证')
  await expect(providerRow.getByRole('button', { name: '更新 API Key' })).toHaveCount(0)
  await editCredential.click()
  credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
  await expect(credentialRow.getByRole('cell').first()).toContainText('最后验证时间')
  await expect(credentialRow.getByRole('cell').first()).not.toContainText('—')
  await expect(modal(page).getByLabel('API Key', { exact: true })).toHaveCount(0)
  await expect(
    credentialRow.getByRole('button', {
      name: '验证 DeepSeek API Key 的可用性',
      exact: true,
    }),
  ).toBeVisible()
  await expect(credentialRow.getByRole('button', { name: '删除', exact: true })).toBeVisible()
  await credentialRow.getByRole('button', { name: '删除', exact: true }).click()
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(modal(page).getByText('暂无认证凭据', { exact: true })).toBeVisible()
  await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
  await modal(page).getByLabel('API Key', { exact: true }).fill('replacement-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await expect(modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })).toBeVisible()
  await expect(page.locator('.toast-success')).toContainText('已保存')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  expect(state.modelSyncRequests).toHaveLength(1)
  await expect(
    providerRow.getByRole('switch', {
      name: 'DeepSeek 服务商凭证的启用状态',
    }),
  ).toHaveCount(0)
  await expect(
    providerRow.getByRole('button', {
      name: '管理 DeepSeek 的认证凭据',
    }),
  ).toHaveText('凭证')

  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('button', { name: '密钥', exact: true })
    .click()
  await modal(page).getByLabel('Key 名称').fill('工作站')
  await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
  await expect(page.getByRole('textbox', { name: '完整 Key 仅展示这一次' })).toHaveValue(
    'vk-fixture-key-shown-once',
  )
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
  expect(await page.content()).not.toContain('vk-fixture-key-shown-once')
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('button', { name: '查看 Key 记录', exact: true })
    .click()
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(1)
  await expect(modal(page).getByRole('button', { name: '撤销', exact: true })).toHaveCount(0)
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(
    page
      .getByRole('row')
      .filter({ hasText: '林知远' })
      .getByRole('button', { name: '撤销', exact: true }),
  ).toHaveCount(0)
  await expect(page.getByRole('row').filter({ hasText: '林知远' })).not.toContainText('已撤销')
  expect(state.keys[0].status).toBe('REVOKED')
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('switch', { name: '林知远的激活状态' })
    .click()
  await expect(page.getByRole('switch', { name: '林知远的激活状态' })).not.toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  expect(state.members[0].status).toBe('DISABLED')

  const memberListRequestCount = state.memberListQueries.length
  await page.getByRole('link', { name: '用量分析' }).click()
  await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: '统计排行', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('member')
  await page.getByRole('tab', { name: '模型请求', exact: true }).click()
  await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('model')
  await page.getByRole('tab', { name: '服务商调用', exact: true }).click()
  await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('provider')
  const providerStatisticRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(providerStatisticRow).toContainText('20')
  await page.screenshot({ path: 'test-results/visual/usage-statistics.png', fullPage: true })
  await providerStatisticRow.getByRole('button', { name: '查看明细', exact: true }).click()
  await expect.poll(() => state.usageQueries.at(-1)?.get('providerId')).toBe('81')
  const dateRangeButton = page.getByRole('button', { name: '选择起止日期', exact: true })
  await expect(dateRangeButton.locator('svg')).toHaveCount(1)
  const defaultDates = (await dateRangeButton.textContent())?.match(/\d{4}\/\d{2}\/\d{2}/g) || [],
    [defaultStart = '', defaultEnd = ''] = defaultDates
  expect(defaultDates).toHaveLength(2)
  expect(
    Date.parse(defaultEnd.replaceAll('/', '-')) - Date.parse(defaultStart.replaceAll('/', '-')),
  ).toBe(6 * 86400000)
  await expect
    .poll(() => {
      const initialUsageQuery = state.usageQueries.at(-1)
      return (
        Date.parse(initialUsageQuery?.get('to') || '') -
        Date.parse(initialUsageQuery?.get('from') || '')
      )
    })
    .toBe(7 * 86400000)
  const initialUsageQuery = state.usageQueries.at(-1)
  const defaultStartValue = defaultStart.replaceAll('/', '-')
  const defaultEndValue = defaultEnd.replaceAll('/', '-')
  expect(initialUsageQuery?.get('from')).toBe(`${defaultStartValue}T00:00:00.000Z`)
  expect(initialUsageQuery?.get('to')).toBe(
    new Date(Date.parse(`${defaultEndValue}T00:00:00Z`) + 86400000).toISOString(),
  )
  await dateRangeButton.click()
  const dateRangeDialog = page.getByRole('dialog', { name: '选择起止日期' })
  await expect(dateRangeDialog.locator('.calendar-panel')).toHaveCount(2)
  await page.screenshot({ path: 'test-results/visual/usage-date-range.png', fullPage: true })
  await dateRangeDialog
    .getByRole('button', { name: `选择 ${defaultStart.replaceAll('/', '-')}` })
    .first()
    .click()
  await expect(dateRangeDialog).toContainText('请选择结束日期')
  await dateRangeDialog
    .getByRole('button', { name: `选择 ${defaultEnd.replaceAll('/', '-')}` })
    .first()
    .click()
  await expect(dateRangeDialog).toHaveCount(0)
  const memberInput = page.getByRole('combobox', { name: '用户', exact: true })
  await expect(memberInput).toHaveAttribute('placeholder', '请输入用户名')
  await expect(page.locator('.member-autocomplete > svg')).toHaveCount(0)
  expect(state.memberListQueries).toHaveLength(memberListRequestCount)
  await memberInput.fill('林知')
  await page.getByRole('option', { name: '林知远', exact: true }).click()
  await expect(memberInput).toHaveValue('林知远')
  expect(state.memberSuggestionQueries.at(-1)?.get('name')).toBe('林知')
  expect(state.memberSuggestionQueries.at(-1)?.get('limit')).toBe('8')
  await page.getByRole('combobox', { name: '模型', exact: true }).selectOption('71')
  await page.getByRole('combobox', { name: '服务商', exact: true }).selectOption('81')
  await expect(page.getByRole('combobox', { name: '服务商凭证', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect.poll(() => state.usageQueries.at(-1)?.get('memberId')).toBe(longID)
  expect(state.usageQueries.at(-1)?.get('modelId')).toBe('71')
  expect(state.usageQueries.at(-1)?.get('providerId')).toBe('81')
  expect(state.usageQueries.at(-1)?.get('resourceId')).toBeNull()
  const usageRow = page.getByRole('row').filter({ hasText: 'OpenAI' })
  const protocolBadge = usageRow.locator('.protocol-label')
  await expect(protocolBadge).toHaveText('OpenAI Responses')
  const protocolBox = await protocolBadge.boundingBox()
  const statusBox = await usageRow.locator('.status').boundingBox()
  expect(protocolBox).not.toBeNull()
  expect(statusBox).not.toBeNull()
  expect(protocolBox!.x + protocolBox!.width).toBeLessThanOrEqual(statusBox!.x)
  await expect(usageRow.locator('td').nth(5)).toHaveText('0')
  await expect(usageRow.locator('td').nth(6)).toHaveText('-')
  await expect(usageRow.locator('td').nth(7)).toHaveText('0')
  await page.getByRole('button', { name: '详情', exact: true }).click()
  await expect(modal(page).getByText('req_live_compatible')).toBeVisible()
  await expect(modal(page)).toContainText('UPSTREAM_BILLING_BLOCKED')
  await expect(modal(page)).toContainText('上游余额不足、计费异常或订阅已过期。')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
  await page.screenshot({ path: 'test-results/visual/usage.png', fullPage: true })
  expect(
    await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length })),
  ).toEqual({ local: 1, session: 0 })
  await page.getByRole('button', { name: '账号菜单' }).click()
  await page.getByRole('menuitem', { name: '注销登录' }).click()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  expect(await page.content()).not.toContain('林知远')
  expect(errors).toEqual([])
})

test('会话失效清理页面、移动端导航与刷新不恢复失效 JWT', async ({ page }) => {
  const state = await fixture(page)
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page, 'home')
  await expect(page.getByRole('button', { name: '打开导航' })).toBeVisible()
  await page.getByRole('button', { name: '打开导航' }).click()
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await expect(page.getByRole('heading', { name: '模型', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await mkdir('test-results/visual', { recursive: true })
  await page.screenshot({
    path: 'test-results/visual/mobile.png',
    fullPage: true,
    animations: 'disabled',
  })
  state.expire()
  await expect(page.getByRole('button', { name: '刷新', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '打开导航' }).click()
  await page.getByRole('link', { name: '操作日志', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('登录已失效')
  await page.reload()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
})

test('14 寸屏幕默认展开侧栏并将横向溢出限制在表格内', async ({ page }) => {
  await fixture(page)
  await page.setViewportSize({ width: 1366, height: 768 })
  await signIn(page)

  expect(
    await page.evaluate(() => {
      const sidebar = document.querySelector<HTMLElement>('.sidebar')!
      const workspace = document.querySelector<HTMLElement>('.workspace')!
      const navLabel = sidebar.querySelector<HTMLElement>('nav a span')!
      const topbar = document.querySelector<HTMLElement>('.topbar')!
      return {
        sidebarWidth: Math.round(sidebar.getBoundingClientRect().width),
        workspaceLeft: Math.round(workspace.getBoundingClientRect().left),
        navLabelDisplay: getComputedStyle(navLabel).display,
        topbarHeight: Math.round(topbar.getBoundingClientRect().height),
        documentFits: document.documentElement.scrollWidth <= innerWidth,
      }
    }),
  ).toEqual({
    sidebarWidth: 240,
    workspaceLeft: 240,
    navLabelDisplay: 'block',
    topbarHeight: 58,
    documentFits: true,
  })
  const memberTableScroll = page.locator('.members-table').locator('..')
  expect(
    await memberTableScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)
  const modelLink = page.getByRole('link', { name: '模型', exact: true })
  await expect(modelLink).toBeVisible()
  await expect(modelLink.locator('span')).toBeVisible()
  await expect(modelLink).toHaveAttribute('data-label', '模型')

  await page.getByRole('link', { name: '用量分析', exact: true }).click()
  await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()
  await expect(page.locator('.usage-context')).toHaveCount(0)
  await expect(page.locator('.usage-list-panel > .usage-filters')).toHaveCount(1)
  await expect(page.locator('.usage-list-panel > .table-scroll')).toHaveCount(1)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({
    path: '../.cache/web-visual/responsive-14-inch.png',
    fullPage: true,
    animations: 'disabled',
  })
})

test('高密度表格在常用桌面分辨率保持稳定列宽和单行技术字段', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))
  await fixture(page)
  await signIn(page, 'home')
  await page.getByRole('link', { name: '服务商', exact: true }).click()

  for (const width of [1366, 1440, 1920]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(page.getByRole('heading', { name: '服务商', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const scroll = page.locator('.providers-table').locator('..')
    expect(await scroll.evaluate((element) => element.scrollWidth > element.clientWidth + 1)).toBe(
      width < 1920,
    )
    const headers = page.locator('.providers-table th')
    expect((await headers.nth(3).boundingBox())!.width).toBeGreaterThanOrEqual(120)
    expect((await headers.nth(6).boundingBox())!.width).toBeGreaterThanOrEqual(170)
  }

  const managementPages = [
    { name: '用户管理', path: 'members' },
    { name: '用户分组', path: 'groups' },
    { name: '模型', path: 'models' },
    { name: '服务商', path: 'providers' },
  ]
  const idColumnWidths: Array<{ name: string; width: number }> = []
  for (const { name, path } of managementPages) {
    await page.getByRole('link', { name, exact: true }).click()
    await expect(page).toHaveURL(new RegExp(`#/${path}$`))
    const idHeader = page.getByRole('columnheader', { name: 'ID', exact: true }).first()
    await expect(idHeader).toBeVisible()
    idColumnWidths.push({
      name,
      width: Math.round((await idHeader.boundingBox())!.width),
    })
  }
  expect(idColumnWidths).toEqual(
    managementPages.map(({ name }) => ({
      name,
      width: 88,
    })),
  )

  const endpoint = page.locator('.providers-table .endpoint').first()
  await expect(endpoint).toHaveAttribute('title', /https:\/\//)
  await expect(endpoint.locator('.technical-value-text')).toHaveCSS('white-space', 'nowrap')
  await expect(endpoint.getByRole('button', { name: '复制', exact: true })).toBeVisible()

  await page.getByRole('link', { name: '用量分析', exact: true }).click()
  const statisticsScroll = page.locator('.statistics-table').locator('..')
  expect(
    await statisticsScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)
  expect(
    (await page.locator('.statistics-table th').last().boundingBox())!.width,
  ).toBeGreaterThanOrEqual(100)

  await page.getByRole('link', { name: '操作日志', exact: true }).click()
  const operationHeaders = page.locator('.operations-table th')
  expect((await operationHeaders.nth(4).boundingBox())!.width).toBeGreaterThanOrEqual(108)
  expect((await operationHeaders.last().boundingBox())!.width).toBeGreaterThanOrEqual(88)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(pageErrors).toEqual([])
})

test('有操作列表固定首尾列，分组模型选择器按可用宽度改为上下结构', async ({ page }) => {
  await fixture(page)
  await page.setViewportSize({ width: 800, height: 768 })
  await signIn(page, 'home')
  await page.goto('/#/usage?view=records')
  await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()

  const tableScroll = page.locator('.usage-list-panel > .table-scroll')
  await expect(tableScroll).toHaveClass(/is-overflowing/)
  await expect(tableScroll).toHaveClass(/table-scroll--actions/)
  await expect(tableScroll).toHaveClass(/is-at-start/)
  await expect(tableScroll).not.toHaveClass(/is-at-end/)

  const firstHeader = tableScroll.getByRole('columnheader').first()
  const actionHeader = tableScroll.getByRole('columnheader').last()
  const initialEdges = await Promise.all([
    tableScroll.boundingBox(),
    firstHeader.boundingBox(),
    actionHeader.boundingBox(),
  ])
  expect(initialEdges.every(Boolean)).toBe(true)
  expect(Math.abs(initialEdges[1]!.x - initialEdges[0]!.x)).toBeLessThanOrEqual(1)
  expect(
    Math.abs(
      initialEdges[2]!.x + initialEdges[2]!.width - (initialEdges[0]!.x + initialEdges[0]!.width),
    ),
  ).toBeLessThanOrEqual(1)
  await expect(actionHeader).not.toHaveCSS('box-shadow', 'none')

  await tableScroll.evaluate((element) => {
    element.scrollLeft = element.scrollWidth - element.clientWidth
  })
  await expect(tableScroll).toHaveClass(/is-at-end/)
  await expect(tableScroll).not.toHaveClass(/is-at-start/)
  await expect(firstHeader).not.toHaveCSS('box-shadow', 'none')
  await expect(actionHeader).toHaveCSS('box-shadow', 'none')

  await page.setViewportSize({ width: 480, height: 768 })
  await page.goto('/#/groups')
  await expect(page.getByRole('heading', { name: '用户分组', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '创建分组', exact: true }).click()

  const selectionWorkspace = modal(page).locator('.group-model-workspace')
  const catalogPane = selectionWorkspace.locator('.group-model-catalog')
  const selectedPane = selectionWorkspace.locator('.group-model-selected')
  await expect(selectionWorkspace).toHaveCSS('grid-template-columns', /\S+/)
  const [catalogBox, selectedBox] = await Promise.all([
    catalogPane.boundingBox(),
    selectedPane.boundingBox(),
  ])
  expect(catalogBox).not.toBeNull()
  expect(selectedBox).not.toBeNull()
  expect(selectedBox!.y).toBeGreaterThan(catalogBox!.y + catalogBox!.height - 1)
  expect(
    await selectionWorkspace.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
  ).toBe(true)
})

test('桌面侧栏可手动折叠和展开', async ({ page }) => {
  await fixture(page)
  await page.setViewportSize({ width: 1366, height: 768 })
  await signIn(page)

  const sidebar = page.locator('.sidebar')
  const workspace = page.locator('.workspace')
  const modelLabel = page.getByRole('link', { name: '模型', exact: true }).locator('span')
  const collapse = page.getByRole('button', { name: '收起导航' })
  await expect(collapse).toHaveAttribute('aria-expanded', 'true')

  await collapse.click()
  const expand = page.getByRole('button', { name: '展开导航' })
  await expect(expand).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(76)
  await expect
    .poll(() => workspace.evaluate((element) => element.getBoundingClientRect().left))
    .toBe(76)
  await expect(modelLabel).toBeHidden()

  await page.setViewportSize({ width: 700, height: 768 })
  await page.getByRole('button', { name: '打开导航' }).click()
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(240)
  await expect
    .poll(() => workspace.evaluate((element) => element.getBoundingClientRect().left))
    .toBe(0)
  await expect(modelLabel).toBeVisible()
  await expect(expand).toBeHidden()

  await page.setViewportSize({ width: 1366, height: 768 })
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(76)
  await expect(modelLabel).toBeHidden()

  await expand.click()
  await expect(page.getByRole('button', { name: '收起导航' })).toHaveAttribute(
    'aria-expanded',
    'true',
  )
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(240)
  await expect
    .poll(() => workspace.evaluate((element) => element.getBoundingClientRect().left))
    .toBe(240)
  await expect(modelLabel).toBeVisible()
})

test('用户页面使用明确术语并在紧凑侧栏展示菜单提示', async ({ page }) => {
  await fixture(page)
  await page.setViewportSize({ width: 900, height: 768 })
  await signIn(page)

  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建用户' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '用户名', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '激活状态', exact: true })).toBeVisible()
  await expect(page.locator('.workspace-square')).toHaveCount(0)
  await expect(page.locator('.workspace-label')).toHaveCount(0)
  await expect(page.locator('.nav-section')).toHaveText(['GitHub 仓库', '基础配置', '使用记录'])
  const communityRepository = page.locator('.community-source-link')
  await expect(communityRepository).toHaveText('GitHub 仓库')
  await expect(communityRepository).toHaveAttribute(
    'aria-label',
    '在新页面打开 Zentrola GitHub 仓库',
  )
  await expect(communityRepository).toHaveAttribute('href', 'https://github.com/zentrola/zentrola')
  await expect(communityRepository).toHaveAttribute('target', '_blank')
  await expect(page.getByRole('searchbox', { name: '用户名' })).toHaveAttribute(
    'placeholder',
    '请输入用户名',
  )

  const modelLink = page.getByRole('link', { name: '模型', exact: true })
  await expect(modelLink).toHaveAttribute('data-label', '模型')
  await modelLink.hover()
  await expect
    .poll(() => modelLink.evaluate((link) => getComputedStyle(link, '::after').opacity))
    .toBe('1')
})

test('成员列表按 ID 倒序前后翻页，保持长 ID 并拒绝无效用量日期范围', async ({ page }) => {
  await fixture(page)
  let pages = 0
  await page.route('**/api/v1/members?**', async (route) => {
    pages++
    const after = new URL(route.request().url()).searchParams.get('after')
    if (after) expect(after).toBe(longID)
    await route.fulfill({
      json: {
        code: 'OK',
        data: {
          items: [
            {
              id: after ? '90071992547409930' : longID,
              name: after ? '第二页成员' : '第一页成员',
              status: 'ACTIVE',
              createdAt: stamp,
            },
          ],
          nextCursor: after ? null : longID,
          total: 21,
        },
      },
    })
  })
  await signIn(page)
  await expect(page.getByLabel('每页')).toHaveValue('20')
  await expect(page.getByRole('button', { name: '首页' })).toBeDisabled()
  await page.getByRole('button', { name: '下一页' }).click()
  await expect(page.getByText('第一页成员', { exact: true })).toHaveCount(0)
  await expect(page.getByText('第二页成员', { exact: true })).toBeVisible()
  await expect(page.locator('tbody tr .person strong')).toHaveText(['第二页成员'])
  await expect(page.getByText('第 2 / 2 页', { exact: true })).toBeVisible()
  await expect(page.getByText('共 21 条', { exact: true })).toBeVisible()
  expect(pages).toBe(2)
  await page.getByRole('button', { name: '上一页' }).click()
  await expect(page.getByText('第一页成员', { exact: true })).toBeVisible()
  await expect(page.getByText('第二页成员', { exact: true })).toHaveCount(0)
  expect(pages).toBe(3)
  await page.getByRole('button', { name: '下一页' }).click()
  await page.getByRole('button', { name: '首页' }).click()
  await expect(page.getByText('第一页成员', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '首页' })).toBeDisabled()
  expect(pages).toBe(5)
  await page.getByRole('link', { name: '用量分析' }).click()
  await page.getByRole('tab', { name: '请求明细', exact: true }).click()
  await expect(page.getByRole('button', { name: '搜索', exact: true })).toBeEnabled()
  await expect(page.getByText('默认查询昨天至今天')).toHaveCount(0)
  await expect(page.locator('.model-filter-field .filter-label')).toHaveCSS('font-weight', '400')
  const dateRangeButton = page.getByRole('button', { name: '选择起止日期', exact: true })
  const [defaultStart = ''] =
    (await dateRangeButton.textContent())?.match(/\d{4}\/\d{2}\/\d{2}/g) || []
  await dateRangeButton.click()
  const dateRangeDialog = page.getByRole('dialog', { name: '选择起止日期' })
  const defaultStartButton = dateRangeDialog
    .getByRole('button', { name: `选择 ${defaultStart.replaceAll('/', '-')}` })
    .first()
  await defaultStartButton.click()
  const selectedDateBox = await defaultStartButton.boundingBox()
  expect(selectedDateBox?.width).toBe(selectedDateBox?.height)
  await defaultStartButton.click()
  await expect(dateRangeDialog.getByRole('alert')).toContainText('起止日期不能相同')
  await dateRangeButton.click()
  await dateRangeButton.click()
  for (let index = 0; index < 13; index += 1)
    await dateRangeDialog.getByRole('button', { name: '上个月', exact: true }).click()
  await dateRangeDialog
    .locator('.calendar-panel')
    .first()
    .locator('.calendar-days button:not(.muted)')
    .first()
    .click()
  for (let index = 0; index < 13; index += 1)
    await dateRangeDialog.getByRole('button', { name: '下个月', exact: true }).click()
  await dateRangeDialog
    .locator('.calendar-panel')
    .last()
    .locator('.calendar-days button:not(.muted)')
    .last()
    .click()
  await expect(dateRangeDialog.getByRole('alert')).toContainText('有效的起止日期')
  await page.getByRole('button', { name: '选择起止日期', exact: true }).click()
  await page.getByRole('button', { name: '重置', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('列表搜索覆盖全部分页并安全传递不透明游标', async ({ page }) => {
  await fixture(page)
  const opaqueCursor = 'cursor&segment=two'
  const searchRequests: URL[] = []
  await page.route('**/api/v1/members?**', async (route) => {
    const url = new URL(route.request().url())
    const limit = Number(url.searchParams.get('limit'))
    const after = url.searchParams.get('after')
    if (limit === 100) searchRequests.push(url)
    const item = {
      id: after === opaqueCursor ? '90071992547409929' : longID,
      name: after === opaqueCursor ? '跨页目标成员' : '第一页成员',
      remark: '',
      status: 'ACTIVE',
      createdAt: stamp,
    }
    await route.fulfill({
      json: {
        code: 'OK',
        data: {
          items: [item],
          nextCursor: limit === 100 && !after ? opaqueCursor : null,
          total: 2,
        },
      },
    })
  })
  await signIn(page)
  await expect(page.getByText('跨页目标成员', { exact: true })).toHaveCount(0)
  await page.getByRole('searchbox', { name: '用户名' }).fill('跨页目标成员')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.getByText('跨页目标成员', { exact: true })).toBeVisible()
  await expect(page.getByLabel('分页')).toHaveCount(0)
  expect(searchRequests).toHaveLength(2)
  expect(searchRequests[1].searchParams.get('after')).toBe(opaqueCursor)
  await page.getByRole('button', { name: '重置', exact: true }).click()
  await expect(page.getByLabel('分页')).toBeVisible()
})

test('成本管理聚焦成本概览和计费明细', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-10-07T08:00:00Z') })
  const state = await fixture(page)
  state.resources.push({
    id: '89',
    providerId: '81',
    name: 'DeepSeek 订阅',
    authType: 'SUBSCRIPTION',
    authAdapter: 'OPENAI_CODEX',
    subscriptionType: 'PERSONAL',
    effectiveAt: '2026-10-01T00:00:00Z',
    expiresAt: null,
    runtimeStatus: 'HEALTHY',
    credentialConfigured: true,
    createdAt: stamp,
    updatedAt: stamp,
  })
  state.credentialPrices.set('89', {
    modelPrices: [],
    subscriptionPrices: [
      {
        id: '930',
        providerCredentialId: '89',
        currency: 'CNY',
        periodAmount: '1000.00000000',
        billingPeriod: 'MONTH',
        effectiveAt: '2026-10-01T00:00:00Z',
        createdAt: stamp,
      },
    ],
    subscriptionPrice: {
      id: '930',
      providerCredentialId: '89',
      currency: 'CNY',
      periodAmount: '1000.00000000',
      billingPeriod: 'MONTH',
      effectiveAt: '2026-10-01T00:00:00Z',
      createdAt: stamp,
    },
  })
  await signIn(page)

  await page.getByRole('link', { name: '成本管理', exact: true }).click()
  await expect(page.getByRole('heading', { name: '成本管理', exact: true })).toBeVisible()
  await expect(page.locator('.billing-tabs').getByRole('tab')).toHaveText(['成本概览', '计费明细'])
  const overviewDateRange = page.getByRole('button', { name: '选择起止日期', exact: true })
  await expect(overviewDateRange).toContainText('2026/10/01 — 2026/10/07')
  await expect(page.getByText('当月实时口径', { exact: true })).toHaveCount(0)
  const ledger = page.locator('.cost-ledger')
  await expect(ledger.getByText('CNY', { exact: true })).toBeVisible()
  await expect(ledger.getByText('USD', { exact: true })).toBeVisible()
  await expect(ledger.getByText('¥1,000', { exact: true })).toBeVisible()
  await expect(ledger.getByText('$0.008', { exact: true })).toBeVisible()
  await expect(ledger.getByText('1 项有效订阅', { exact: true })).toBeVisible()
  const attribution = page.locator('.cost-attribution-panel')
  await expect(attribution.getByRole('heading', { name: '费用分摊', exact: true })).toBeVisible()
  await expect(attribution.getByText('预计分摊', { exact: true })).toHaveCount(0)
  await expect(attribution.getByText('林知远', { exact: true })).toHaveCount(2)
  await expect(attribution.getByText('¥1,000', { exact: true })).toBeVisible()
  await expect(attribution.getByText('$0.008', { exact: true })).toBeVisible()
  await expect(page.getByText('当月订阅尚未结算，暂未生成最终成本归属。')).toHaveCount(0)
  await overviewDateRange.click()
  const overviewDateDialog = page.getByRole('dialog', { name: '选择起止日期' })
  await overviewDateDialog.getByRole('button', { name: '上个月' }).click()
  await overviewDateDialog.getByRole('button', { name: '选择 2026-09-10' }).click()
  await overviewDateDialog.getByRole('button', { name: '选择 2026-09-20' }).click()
  await expect(overviewDateRange).toContainText('2026/09/10 — 2026/09/20')
  const overviewRequest = page.waitForRequest((request) =>
    request.url().includes('/billing/statistics?'),
  )
  await page.getByRole('button', { name: '查询', exact: true }).click()
  const overviewSearch = new URL((await overviewRequest).url()).searchParams
  expect(overviewSearch.get('from')).toBe('2026-09-10T00:00:00.000Z')
  expect(overviewSearch.get('to')).toBe('2026-09-21T00:00:00.000Z')
  await expect(page.getByText('已结算口径', { exact: true })).toBeVisible()
  await expect(ledger.getByText('¥710', { exact: true })).toBeVisible()
  await expect(ledger.getByText('$5.125', { exact: true })).toBeVisible()
  await expect(ledger.getByText('¥715.125')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '用量排行', exact: true })).toBeVisible()
  const usageRanking = page.locator('.cost-usage-ranking')
  await expect(usageRanking).toContainText('林知远')
  await expect(usageRanking.locator('.cost-rank-marker.rank-1')).toHaveText('1')
  await expect(usageRanking.getByText('最高用量', { exact: true })).toBeVisible()

  await page.getByRole('tab', { name: '计费明细', exact: true }).click()
  await expect(
    page.locator('.billing-detail-metrics').getByText('$0.008', { exact: true }),
  ).toBeVisible()
  const usageTable = page.locator('.usage-cost-table')
  await expect(usageTable.getByText('已核算', { exact: true })).toBeVisible()
  await expect(usageTable.getByText('订阅账期分摊', { exact: true })).toBeVisible()

  const costPanel = page.locator('.usage-cost-panel')
  await expect(costPanel.getByLabel('用户分组')).toBeVisible()
  await expect(costPanel.getByLabel('模型')).toBeVisible()
  await expect(costPanel.getByLabel('服务商')).toBeVisible()
  await expect(costPanel.getByLabel('核算状态')).toBeVisible()
  await expect(costPanel.getByLabel('计费方式')).toBeVisible()
  await expect(costPanel.getByLabel('主体类型')).toBeHidden()
  const costDateRange = costPanel.getByRole('button', { name: '选择起止日期', exact: true })
  await expect(costDateRange).toBeVisible()
  await costDateRange.click()
  const costDateDialog = costPanel.getByRole('dialog', { name: '选择起止日期' })
  await costDateDialog.getByRole('button', { name: '选择 2026-10-02' }).click()
  await costDateDialog.getByRole('button', { name: '选择 2026-10-05' }).click()
  await expect(costDateRange).toContainText('2026/10/02 — 2026/10/05')
  const costRequest = page.waitForRequest(
    (request) =>
      request.url().includes('/billing/usage-costs?') &&
      new URL(request.url()).searchParams.get('modelId') === '71',
  )
  await costPanel.getByRole('button', { name: /更多筛选/ }).click()
  await expect(costPanel.getByLabel('主体类型')).toBeVisible()
  await costPanel.getByLabel('主体类型').selectOption('MEMBER')
  await costPanel.getByLabel('主体 ID').fill(longID)
  await costPanel.getByLabel('模型').selectOption('71')
  await costPanel.getByLabel('服务商').selectOption('81')
  await costPanel.getByLabel('客户端协议').selectOption('OPENAI_RESPONSES')
  await costPanel.getByLabel('核算状态').selectOption('RATED')
  await costPanel.getByLabel('计费方式').selectOption('API_KEY')
  await costPanel.getByLabel('币种').selectOption('USD')
  const collapseFilters = costPanel.getByRole('button', { name: /收起筛选/ })
  await expect(collapseFilters.locator('.usage-cost-filter-count')).toHaveText('4')
  await collapseFilters.click()
  await expect(costPanel.getByLabel('主体类型')).toBeHidden()
  await expect(costPanel.getByRole('button', { name: /更多筛选/ })).toContainText('4')
  await costPanel.getByRole('button', { name: '查询', exact: true }).click()
  const searched = new URL((await costRequest).url()).searchParams
  expect(Object.fromEntries(searched)).toMatchObject({
    from: '2026-10-02T00:00:00.000Z',
    to: '2026-10-06T00:00:00.000Z',
    principalType: 'MEMBER',
    principalId: longID,
    modelId: '71',
    providerId: '81',
    clientProtocol: 'OPENAI_RESPONSES',
    ratingStatus: 'RATED',
    billingType: 'API_KEY',
    currency: 'USD',
  })

  await page.getByRole('button', { name: '查看详情' }).first().click()
  const usageDetail = page.getByRole('dialog', { name: '单次用量成本详情' })
  await expect(usageDetail.locator('tbody tr').nth(0).locator('td').first()).toHaveText('1')
  await expect(usageDetail.locator('tbody tr').nth(1).locator('td').first()).toHaveText('2')
  await expect(usageDetail.getByText('$0.0036', { exact: true })).toBeVisible()
  await expect(usageDetail.getByText('$0.0042', { exact: true })).toBeVisible()
  await expect(usageDetail.getByText('$0.0079', { exact: true })).toBeVisible()
  await usageDetail.getByRole('button', { name: '关闭' }).click()

  await page.getByRole('button', { name: '查看详情' }).nth(1).click()
  const subscriptionDetail = page.getByRole('dialog', { name: '单次用量成本详情' })
  await expect(subscriptionDetail).toContainText('个人订阅按账期分摊')
  await expect(subscriptionDetail).toContainText('¥1,000')
  await expect(subscriptionDetail).toContainText('¥700')
  await subscriptionDetail.getByRole('button', { name: '关闭' }).click()

  const downloadPromise = page.waitForEvent('download')
  await costPanel.getByRole('button', { name: '导出 CSV', exact: true }).click()
  const exportDownload = await downloadPromise
  expect(exportDownload.suggestedFilename()).toContain('zentrola-usage-costs-')
})
