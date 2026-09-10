import { test, expect, type Page } from '@playwright/test'
import { mkdir } from 'node:fs/promises'

const stamp = '2026-09-06T07:30:00Z'
const longID = '90071992547409931'
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
  const operations: any[] = [
    {
      id: '203',
      operatorName: 'admin',
      type: 'RESOURCE_CONNECTION_TEST',
      targetType: 'RESOURCE',
      targetId: '88',
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
    memberListQueries: URLSearchParams[] = [],
    memberSuggestionQueries: URLSearchParams[] = [],
    usageQueries: URLSearchParams[] = [],
    modelSyncRequests: string[] = []
  const providerInputs: any[] = []
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
    if (path === '/members/suggestions' && method === 'GET') {
      memberSuggestionQueries.push(new URLSearchParams(url.searchParams))
      const name = url.searchParams.get('name') || ''
      return pageReply(members.filter((member) => member.name.includes(name)))
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
      if (path === '/providers') return pageReply(providers)
      if (path === '/groups') {
        groupQueries.push(new URLSearchParams(url.searchParams))
        const status = url.searchParams.get('status')
        const filtered = status ? groups.filter((group) => group.status === status) : groups
        return pageReply([...filtered].sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1)))
      }
      if (path === '/resources') return pageReply(resources.map(({ credential: _, ...row }) => row))
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
            clientProtocol: 'OPENAI',
            requestAt: stamp,
            completedAt: stamp,
            status: 'SUCCESS',
            inputTokens: 0,
            outputTokens: null,
            cachedInputTokens: 0,
            latencyMs: 218,
            attemptNo: 1,
            errorType: null,
          },
        ])
      }
    }
    if (path === '/usage/writer') return reply({ pending: 0, failed: 0 })
    if (segments[0] === 'providers' && segments.length === 2 && method === 'GET') {
      const row = providers.find((provider) => provider.id === segments[1])
      if (!row) return reply(null, 404, 'NOT_FOUND')
      return reply({ ...row, mappings: providerMappings.get(row.id) || [] })
    }
    if (path === '/providers/initialize' && method === 'POST') {
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
          'Alibaba Cloud',
          'https://dashscope.aliyuncs.com/compatible-mode/v1',
          'https://qwen.ai',
        ],
      ]
      let created = 0,
        updated = 0
      for (const [code, nameZH, nameEN, baseUrl, website] of templates) {
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
          modelSyncSupported: code === 'deepseek-official',
          createdAt: stamp,
          updatedAt: stamp,
        })
        created++
      }
      return reply({
        total: templates.length,
        created,
        updated,
        existing: templates.length - created,
      })
    }
    if (method === 'POST' && segments.length === 1) {
      const id = next()
      const { modelIds = [], groupIds = [], mappings = [], ...fields } = body
      const row = {
        ...fields,
        id,
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
      Object.assign(row, body)
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
        const matching = keys.filter((key) => key.memberID === segments[1])
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
    if (segments[2] === 'credential') {
      resources.find((r) => r.id === segments[1]).credential = body.credential
      return reply({ updated: true })
    }
    if (segments[2] === 'test-connection')
      return reply({
        ok: !failedTest,
        code: failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
        httpStatus: failedTest ? 401 : 200,
        latencyMs: 140,
      })
    if (segments[2] === 'sync-models') {
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
      })
    }
    return reply(null, 404, 'NOT_FOUND')
  })
  return {
    members,
    models,
    providers,
    providerMappings,
    providerInputs,
    groups,
    resources,
    keys,
    relationships,
    groupQueries,
    modelQueries,
    memberListQueries,
    memberSuggestionQueries,
    usageQueries,
    modelSyncRequests,
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
  await expect(page.getByRole('heading', { name: '控制面板', exact: true })).toBeVisible()
  if (destination === 'home') return
  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
}
const modal = (page: Page) => page.locator('dialog').last()
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
    return {
      from: new Date(now.getFullYear(), now.getMonth(), 1).toISOString(),
      to: new Date(now.getFullYear(), now.getMonth() + 1, 1).toISOString(),
    }
  })
  expect(dashboardURL.searchParams.get('from')).toBe(expectedRange.from)
  expect(dashboardURL.searchParams.get('to')).toBe(expectedRange.to)

  await expect(page.locator('.token-total dd')).toHaveText('12.8K')
  await expect(page.locator('.token-total dd')).toHaveAttribute('title', '12,840')
  await expect(page.getByRole('heading', { name: '本月概览', exact: true })).toBeVisible()
  const expectedMonthLabel = await page.evaluate(() =>
    new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long' }).format(new Date()),
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
  await expect(page.getByText('OpenAI', { exact: true })).toBeVisible()
  await expect(page.locator('.access-panel').getByText('Anthropic', { exact: true })).toBeVisible()
  await expect(page.getByText('适用于 Codex 等 OpenAI 兼容客户端')).toBeVisible()
  await expect(page.getByText('适用于 Claude Code 等 Anthropic 兼容客户端')).toBeVisible()
  await expect(page.getByText('林知远', { exact: true })).toBeVisible()
  await expect(page.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await expect(
    page.locator('.provider-ranking').getByText('20 次调用', { exact: true }),
  ).toBeVisible()
  await expect(page.locator('.token-ranking [title="8,200 Token"]')).toHaveText('8.2K Token')
  await expect(page.locator('.model-ranking small').first()).toContainText('9.1K Token')
  await expect(page.locator('.model-ranking small').first()).toHaveAttribute('title', '9,100 Token')
  await expect(page.locator('.provider-ranking small').first()).toHaveText('9.2K Token')
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
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

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
  await page.screenshot({ path: '../.cache/web-visual/home-setup-mobile.png', fullPage: true })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
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
  await expect(operationRow.locator('.operation-target-id')).toHaveText('81')
  await operationRow.getByRole('button', { name: '详情', exact: true }).click()

  const dialog = modal(page)
  await expect(dialog.getByRole('heading', { name: '日志详情', exact: true })).toBeVisible()
  const before = dialog.locator('section[aria-label="修改前"]')
  const after = dialog.locator('section[aria-label="修改后"]')
  await expect(before.locator('code')).toContainText('"status": "ACTIVE"')
  await expect(after.locator('code')).toContainText('"status": "DISABLED"')
  await expect(before.locator('.json-line.changed')).toContainText('"status": "ACTIVE"')
  await expect(after.locator('.json-line.changed')).toContainText('"status": "DISABLED"')
  const copyBefore = before.getByRole('button', { name: '复制修改前', exact: true })
  await expect(copyBefore).toBeVisible()
  await expect(after.getByRole('button', { name: '复制修改后', exact: true })).toBeVisible()
  await copyBefore.click()
  await expect(page.locator('.toast-success')).toContainText('已复制')
  await expect(dialog.getByText('req_provider_status', { exact: true })).toBeVisible()
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
  await expect(modal(page).getByRole('button', { name: '复制日志内容', exact: true })).toBeVisible()
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
  )
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '林知远' })
  await expect(page.getByRole('button', { name: '管理 Key' })).toHaveCount(0)
  await expect(page.getByRole('columnheader', { name: '访问密钥', exact: true })).toHaveCount(0)
  await expect(row.locator('code')).toHaveCount(0)
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
  await expect(modal(page).getByRole('columnheader')).toHaveText([
    '显示名称',
    '访问密钥',
    '过期日期',
    '操作',
  ])
  await expect(modal(page).locator('tbody tr td:first-child')).toHaveText(['临时测试', '工作站'])
  await expect(modal(page).locator('code')).toHaveText([
    'zt_vk_temp********temp',
    'vk-work1234********1234',
  ])
  await expect(modal(page)).toContainText('长期有效')
  await expect(modal(page)).toContainText('2020年1月1日')
  await expect(modal(page).locator('tbody tr').first().locator('td').nth(2)).toHaveText(
    '2020年1月1日',
  )
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
  expect(state.keys[0].status).toBe('ACTIVE')
  state.conflict(false)
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(1)
  await expect(page.locator('.toast-success')).toContainText('Key 已撤销')
  await expect(historyDialog.locator('.notice')).toHaveCount(0)
  await expect(historyRevoke).toHaveCount(0)
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
  await expect(disabledMemberStatus).toContainText('未激活')
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
  await expect(row.locator('code')).toHaveCount(0)
  await expect(row).not.toContainText('zt_vk_temp')
  await expect(row).not.toContainText('2020年1月1日')
  expect(state.keys).toHaveLength(3)
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
  await row.getByRole('button', { name: '删除', exact: true }).click()
  await modal(page).getByRole('button', { name: '取消' }).click()
  expect(state.members).toHaveLength(3)
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
  await row.getByRole('button', { name: '密钥', exact: true }).click()
  await modal(page).getByLabel('Key 名称').fill('工作站')
  await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()

  await expect(status).toBeEnabled()
  await status.click()
  await expect(status).toBeChecked()
  await expect(modal(page)).toHaveCount(0)
  await expect(status).toContainText('已激活')
  expect(state.members.find((member) => member.name === '周予安')?.status).toBe('ACTIVE')
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
  await expect(page.getByRole('columnheader')).toHaveText([
    '官方模型名称',
    '模型厂商',
    '状态',
    '输入类型',
    '输出类型',
    '备注',
    '操作',
  ])
  const existingModelRow = page.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
  await expect(existingModelRow.locator('.person strong')).toHaveText('DeepSeek V4 Flash')
  await expect(existingModelRow.locator('.person small')).toHaveText('deepseek-v4-flash')
  await expect(existingModelRow.getByRole('cell')).toHaveCount(7)
  await expect(existingModelRow.getByRole('cell').nth(1)).toHaveText('DeepSeek')
  await page.getByRole('button', { name: '添加模型' }).click()
  const dialog = modal(page)
  await dialog.getByLabel('官方模型名称', { exact: true }).fill('测试官方模型')
  await dialog.getByLabel('官方模型编码', { exact: true }).fill('official-test-v1')
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
  await expect(dialog.getByLabel('官方模型编码', { exact: true })).toHaveValue('official-test-v1')
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
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(dialog.getByLabel('发布方编码', { exact: true })).toHaveCount(0)
  await mkdir('../.cache/web-visual', { recursive: true })
  await dialog.screenshot({ path: '../.cache/web-visual/model-edit-desktop.png' })
  await expect(
    dialog.getByRole('group', { name: '输入类型' }).getByLabel('图片', { exact: true }),
  ).toBeChecked()
  await dialog.getByLabel('官方模型编码', { exact: true }).fill('official-test-v2')
  await expect(dialog.getByRole('status')).toContainText('客户端需要使用新编码')
  await dialog.getByRole('group', { name: '输出类型' }).getByLabel('音频', { exact: true }).check()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeVisible()
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/model-edit-mobile.png' })
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  expect(created.code).toBe('official-test-v2')
  expect(created.status).toBe('DISABLED')
  expect(created.outputModalities).toEqual(['TEXT', 'AUDIO'])
  await page.setViewportSize({ width: 1440, height: 1000 })
  const modelSearch = page.getByRole('searchbox', { name: '模型名称' })
  await expect(modelSearch).toHaveAttribute('placeholder', '请输入模型名称')
  await expect(page.locator('.list-search-label')).toHaveText('模型名称')
  await modelSearch.fill('图文理解用途')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(1)
  await page.screenshot({ path: '../.cache/web-visual/model-catalog-desktop.png' })
})

test('服务商新增编辑、启停和窄屏导航折叠', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page)

  await page.setViewportSize({ width: 1200, height: 900 })
  const providerLink = page.getByRole('link', { name: '服务商', exact: true })
  await expect(providerLink.locator('span')).toBeVisible()
  await providerLink.click()
  await expect(page.getByRole('heading', { name: '服务商', exact: true })).toBeVisible()

  const providerSearch = page.getByRole('search')
  await expect(providerSearch.getByRole('searchbox', { name: '服务商名称' })).toHaveAttribute(
    'placeholder',
    '请输入服务商名称或接口地址',
  )
  await expect(
    page.locator('.page-heading').getByRole('button', { name: '添加服务商', exact: true }),
  ).toHaveCount(0)
  await expect(
    providerSearch.getByRole('button', { name: '添加服务商', exact: true }),
  ).toBeVisible()
  await expect(providerSearch.getByRole('button', { name: '初始化', exact: true })).toBeVisible()
  expect(
    (await providerSearch.locator('button').allTextContents()).slice(-2).map((text) => text.trim()),
  ).toEqual(['添加服务商', '初始化'])
  await providerSearch.getByRole('button', { name: '初始化', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText(
    '已补充 6 个官方服务商，并同步 1 个预置名称或官网，共 7 个',
  )
  await expect(page.locator('.provider-initialize-notice')).toHaveCount(0)
  await expect(page.getByRole('row').filter({ hasText: '月之暗面' })).toBeVisible()
  expect(state.providers.find((provider) => provider.code === 'kimi-official')?.website).toBe(
    'https://www.moonshot.cn',
  )
  await providerSearch.getByRole('button', { name: '初始化', exact: true }).click()
  await expect(page.locator('.toast-success')).toContainText(
    '官方服务商的当前语言名称和官网已是最新，共 7 个',
  )
  expect(state.providers).toHaveLength(7)

  const deepSeekRow = page.getByRole('row').filter({ hasText: '深度求索' })
  await expect(page.getByRole('columnheader', { name: '密钥', exact: true })).toBeVisible()
  const deepSeekCredential = deepSeekRow.getByRole('button', {
    name: '深度求索 密钥未配置，点击添加',
    exact: true,
  })
  await expect(deepSeekCredential).toBeVisible()
  await expect(deepSeekCredential).toHaveClass(/is-missing/)
  await expect(deepSeekCredential.locator('svg')).toHaveCount(1)
  const deepSeekProxy = deepSeekRow.getByRole('img', {
    name: '深度求索 未启用代理访问',
    exact: true,
  })
  await expect(deepSeekProxy).toBeVisible()
  await expect(deepSeekProxy.locator('svg')).toHaveCount(1)
  await expect(deepSeekRow.locator('td').last().getByRole('button')).toHaveText(['删除', '编辑'])
  await deepSeekRow.getByRole('button', { name: '编辑', exact: true }).click()
  await expect(modal(page).getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(
    modal(page).getByRole('checkbox', { name: '启用 DeepSeek V4 Flash 映射' }),
  ).toBeChecked()
  await expect(modal(page).getByLabel('DeepSeek V4 Flash 的服务商模型编码')).toHaveValue('')
  await expect(modal(page).getByText('Claude Sonnet', { exact: true })).toBeVisible()
  await expect(
    modal(page).getByRole('checkbox', { name: '启用 Claude Sonnet 映射' }),
  ).not.toBeChecked()
  await expect(modal(page).getByLabel('Claude Sonnet 的服务商模型编码')).toBeDisabled()
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
  await modal(page).getByLabel('网站').fill('https://www.deepseek.com/')
  await modal(page).getByRole('button', { name: '保存', exact: true }).click()
  expect(state.providers[0].website).toBe('https://www.deepseek.com')
  expect(state.providerMappings.get('81')).toHaveLength(1)
  await expect(page.getByRole('columnheader', { name: '网站', exact: true })).toHaveCount(0)
  const websiteLink = deepSeekRow.getByRole('link', {
    name: '在新页面打开 深度求索 官网',
    exact: true,
  })
  await expect(websiteLink).toHaveAttribute('href', 'https://www.deepseek.com')
  await expect(websiteLink).toHaveAttribute('target', '_blank')
  await expect(websiteLink.locator('svg')).toHaveCount(1)

  await page.getByRole('button', { name: '添加服务商' }).click()
  const dialog = modal(page)
  await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await expect(dialog.getByRole('button', { name: '添加映射', exact: true })).toHaveCount(0)
  const mappingCheckbox = dialog.getByRole('checkbox', {
    name: '启用 DeepSeek V4 Flash 映射',
  })
  const providerModelCode = dialog.getByLabel('DeepSeek V4 Flash 的服务商模型编码')
  await expect(mappingCheckbox).not.toBeChecked()
  await expect(providerModelCode).toBeDisabled()
  await mappingCheckbox.check()
  await expect(providerModelCode).toBeEnabled()
  await expect(providerModelCode).toHaveValue('')
  await expect(providerModelCode).toHaveAttribute('placeholder', '留空则使用 deepseek-v4-flash')
  await providerModelCode.fill('deepseek-v4-flash')
  await providerModelCode.blur()
  await expect(providerModelCode).toHaveValue('')
  await mappingCheckbox.uncheck()
  await expect(providerModelCode).toBeDisabled()
  await dialog.getByLabel('服务商名称').fill('阿里云百炼')
  await dialog.getByLabel('网站').fill('https://www.aliyun.com')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('至少填写一种协议接口地址')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  await dialog
    .getByLabel('OpenAI 协议地址')
    .fill('http://dashscope.aliyuncs.com/compatible-mode/v1')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('有效的网站或 HTTPS 接口地址')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  await dialog
    .getByLabel('OpenAI 协议地址')
    .fill('https://dashscope.aliyuncs.com/compatible-mode/v1')
  await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
  await dialog.getByRole('switch', { name: '启用代理', exact: true }).check()
  await dialog
    .getByLabel('代理服务器地址', { exact: true })
    .fill('http://proxy-user:proxy-password@proxy.example.com:8080')
  await dialog.getByRole('button', { name: '添加 Header', exact: true }).click()
  await dialog.getByLabel('KEY', { exact: true }).fill('X-Proxy-Token')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveAttribute('type', 'text')
  await dialog.getByLabel('VALUE', { exact: true }).fill('proxy-header-secret')
  await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveValue('proxy-header-secret')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('请至少启用一个系统模型')
  await expect(dialog.locator('.alert.error')).toHaveCount(0)
  await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await mappingCheckbox.check()
  await providerModelCode.fill('aliyun-deepseek-v4-flash')
  await dialog.screenshot({ path: '../.cache/web-visual/provider-model-mappings.png' })
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)

  const created = state.providers.find((provider) => provider.name === '阿里云百炼')
  expect(created.status).toBe('DISABLED')
  expect(state.providerInputs.at(-1)).toEqual(
    expect.objectContaining({
      endpoints: [
        {
          protocolType: 'OPENAI',
          baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
        },
      ],
      proxyEnabled: true,
      proxyUrl: 'http://proxy-user:proxy-password@proxy.example.com:8080',
      proxyHeaders: [{ key: 'X-Proxy-Token', value: 'proxy-header-secret' }],
    }),
  )
  expect(created.proxyUrl).not.toContain('proxy-password')
  expect(state.providerMappings.get(created.id)).toEqual([
    expect.objectContaining({
      modelId: '71',
      upstreamModelCode: 'aliyun-deepseek-v4-flash',
    }),
  ])
  const row = page.getByRole('row').filter({ hasText: '阿里云百炼' })
  await expect(row).toContainText('dashscope.aliyuncs.com/compatible-mode/v1')
  await expect(
    row.getByRole('img', { name: '阿里云百炼 已启用代理访问', exact: true }),
  ).toBeVisible()
  const createdStatus = row.getByRole('switch', { name: '阿里云百炼的启用状态' })
  await expect(createdStatus).toBeDisabled()
  await expect(createdStatus).toHaveAttribute('title', '请先配置服务商密钥，再启用服务商。')
  const missingCredential = row.getByRole('button', {
    name: '阿里云百炼 密钥未配置，点击添加',
    exact: true,
  })
  await expect(missingCredential).toHaveClass(/is-missing/)
  await missingCredential.click()
  await dialog.getByLabel('API Key', { exact: true }).fill('aliyun-fixture-credential')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(page.locator('.toast-success')).toContainText('已保存')
  await expect(
    row.getByRole('button', { name: '同步 阿里云百炼 的官方模型', exact: true }),
  ).toHaveCount(0)
  expect(state.modelSyncRequests).toHaveLength(0)
  await expect(
    row.getByRole('button', { name: '阿里云百炼 密钥已配置，点击更新', exact: true }),
  ).toHaveClass(/is-configured/)
  await expect(createdStatus).toBeEnabled()
  await expect(createdStatus).not.toHaveAttribute('title')
  const testCredential = row.getByRole('button', {
    name: '测试 阿里云百炼 的连接',
    exact: true,
  })
  await expect(testCredential).toBeVisible()
  await testCredential.click()
  await expect(modal(page).getByRole('status')).toContainText('连接测试通过')
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
  await expect(renamedStatus).toBeChecked()
  expect(created.status).toBe('ACTIVE')
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
  await page
    .getByRole('row')
    .filter({ hasText: '阿里云模型服务' })
    .getByRole('button', { name: '删除', exact: true })
    .click()
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
  await expect(row.getByText('已阻断', { exact: true })).toBeVisible()
  await expect(row).toContainText('余额或计费异常 · HTTP 402')
  await expect(row.locator('.provider-runtime .subline')).toHaveAttribute(
    'title',
    'UPSTREAM_BILLING_BLOCKED',
  )
})

test('状态 switch 直接生效且危险操作仍需确认', async ({ page }) => {
  await fixture(page)
  await signIn(page)
  await page.getByRole('button', { name: 'EN', exact: true }).click()
  await page.getByRole('link', { name: 'Providers', exact: true }).click()
  const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  const status = row.getByRole('switch', { name: 'Status for DeepSeek' })
  await status.click()
  await expect(status).not.toBeChecked()
  await expect(modal(page)).toHaveCount(0)
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
  await expect(dialog.locator('.member-group-list').getByText('启用', { exact: true })).toHaveCount(
    0,
  )
  await dialog.getByLabel('用户名', { exact: true }).fill('新用户')
  await createActive.check()
  await dialog.getByRole('button', { name: '创建', exact: true }).click()
  const created = state.members.find((member) => member.name === '新用户')
  expect(created.status).toBe('DISABLED')
  await expect(
    page.getByRole('row').filter({ hasText: '新用户' }).getByText('未激活', { exact: true }),
  ).toBeVisible()
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
  await expect(createFormRows.locator('.group-form-label')).toHaveText([
    '名称',
    '备注',
    '允许访问的模型',
  ])
  await expect(createFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
  await expect(createDialog.getByLabel('名称', { exact: true })).toHaveAttribute('required', '')
  await expect(createFormRows.nth(2).locator('.group-form-label')).toHaveClass(/required-label/)
  await expect(createFormRows.nth(2).locator('.group-model-field')).toHaveAttribute(
    'aria-required',
    'true',
  )
  expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
  await expect(createDialog.getByLabel('备注', { exact: true })).toBeVisible()
  await expect(createDialog.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await expect(createDialog.getByText('Claude Sonnet', { exact: true })).toHaveCount(0)
  await expect(createDialog.getByText('已选择 0 / 共 1 个模型')).toBeVisible()
  await expect(createDialog.getByRole('columnheader')).toHaveText([
    '请选择',
    '名称',
    '输入类型',
    '输出类型',
  ])
  await expect(createDialog.getByText('deepseek-v4-flash', { exact: true })).toHaveCount(0)
  const createModelRow = createDialog.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
  await expect(createModelRow.getByRole('cell').nth(2)).toHaveText('文本')
  await expect(createModelRow.getByRole('cell').nth(3)).toHaveText('文本')
  await createDialog.getByLabel('名称', { exact: true }).fill('必填校验分组')
  await createDialog.getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.locator('.toast')).toContainText('必须选择至少一个访问模型')
  await expect(createDialog.locator('.alert.error')).toHaveCount(0)
  await createDialog.getByRole('button', { name: '关闭', exact: true }).click()
  state.models[1].status = 'ACTIVE'
  await page.getByRole('button', { name: '创建分组' }).click()
  const multimodalRow = modal(page).getByRole('row').filter({ hasText: 'Claude Sonnet' })
  await expect(multimodalRow.getByRole('cell').nth(2)).toHaveText('文本图片')
  await expect(multimodalRow.getByRole('cell').nth(3)).toHaveText('文本')
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
  await expect(dialog.getByRole('searchbox')).toHaveCount(0)
  await expect(active).toBeVisible()
  await expect(active).toBeChecked()
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
    '名称',
    '状态',
    '创建时间',
    '备注',
    '操作',
  ])
  await expect(page.locator('tbody tr .person strong')).toHaveText(['最新分组', '较早分组'])
  await expect(page.locator('tbody tr .avatar')).toHaveText(['最', '较'])
  await expect(
    page.getByRole('row').filter({ hasText: '最新分组' }).getByRole('cell').nth(3),
  ).toHaveText('-')
  await expect(
    page.getByRole('row').filter({ hasText: '较早分组' }).getByRole('cell').nth(3),
  ).toHaveText('核心服务组')
  await expect(page.getByRole('searchbox', { name: '分组名称' })).toHaveAttribute(
    'placeholder',
    '请输入分组名称',
  )
  await expect(page.locator('.list-search-label')).toHaveText('分组名称')

  const latestSwitch = page.getByRole('switch', { name: '最新分组的启用状态' })
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
  await expect(page.getByText(longID, { exact: true })).toHaveCount(0)
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
  await expect(modal(page).getByText('已选择 1 / 共 2 个模型')).toBeVisible()
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
  await expect(modal(page).getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
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
  await page
    .getByRole('row')
    .filter({ hasText: '平台研发组' })
    .getByRole('button', { name: '删除', exact: true })
    .click()
  await expect(modal(page)).toContainText('用户关联和模型授权将失效')
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: '平台研发组' })).toHaveCount(0)
  expect(state.relationships.has(`groups/${createdGroup.id}/models`)).toBe(false)

  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await expect(page.getByRole('link', { name: '服务商凭证', exact: true })).toHaveCount(0)
  const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
  await expect(providerRow.getByRole('link', { name: '在新页面打开 DeepSeek 官网' })).toHaveCount(0)
  await providerRow
    .getByRole('button', { name: 'DeepSeek 密钥未配置，点击添加', exact: true })
    .click()
  await modal(page).getByLabel('API Key', { exact: true }).fill('fixture-upstream-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(page.locator('.toast-success')).toContainText('已保存')
  expect(state.modelSyncRequests).toHaveLength(0)
  await providerRow.getByRole('button', { name: '同步 DeepSeek 的官方模型', exact: true }).click()
  await expect(modal(page).getByRole('status')).toContainText('官方模型目录同步完成')
  expect(state.modelSyncRequests).toHaveLength(1)
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  const testConnection = providerRow.getByRole('button', {
    name: '测试 DeepSeek 的连接',
    exact: true,
  })
  await expect(testConnection).toBeVisible()
  expect(await page.content()).not.toContain('fixture-upstream-credential')
  await testConnection.click()
  await expect(modal(page).getByRole('status')).toContainText('连接测试通过')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  state.failTest()
  await testConnection.click()
  await expect(modal(page).getByRole('status')).toContainText('上游认证失败')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  const editCredential = providerRow.getByRole('button', {
    name: 'DeepSeek 密钥已配置，点击更新',
  })
  await expect(editCredential.locator('svg')).toHaveCount(1)
  await expect(providerRow.getByRole('button', { name: '更新 API Key' })).toHaveCount(0)
  await editCredential.click()
  await modal(page).getByLabel('API Key', { exact: true }).fill('replacement-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(page.locator('.toast-success')).toContainText('已保存')
  expect(state.modelSyncRequests).toHaveLength(1)
  await expect(
    providerRow.getByRole('switch', {
      name: 'DeepSeek 服务商凭证的启用状态',
    }),
  ).toHaveCount(0)
  await expect(editCredential).toHaveClass(/is-configured/)

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
  await page.getByRole('link', { name: '用量记录' }).click()
  const dateRangeButton = page.getByRole('button', { name: '选择起止日期', exact: true })
  await expect(dateRangeButton.locator('svg')).toHaveCount(1)
  const defaultDates = (await dateRangeButton.textContent())?.match(/\d{4}\/\d{2}\/\d{2}/g) || [],
    [defaultStart = '', defaultEnd = ''] = defaultDates
  expect(defaultDates).toHaveLength(2)
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
  expect(state.memberListQueries).toHaveLength(memberListRequestCount)
  await memberInput.fill('林知')
  await page.getByRole('option', { name: '林知远', exact: true }).click()
  await expect(memberInput).toHaveValue('林知远')
  expect(state.memberSuggestionQueries.at(-1)?.get('name')).toBe('林知')
  expect(state.memberSuggestionQueries.at(-1)?.get('limit')).toBe('8')
  await page.getByRole('combobox', { name: '模型', exact: true }).selectOption('71')
  await expect(page.getByRole('combobox', { name: '服务商凭证', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect.poll(() => state.usageQueries.at(-1)?.get('memberId')).toBe(longID)
  expect(state.usageQueries.at(-1)?.get('modelId')).toBe('71')
  expect(state.usageQueries.at(-1)?.get('resourceId')).toBeNull()
  const usageRow = page.getByRole('row').filter({ hasText: 'OpenAI' })
  await expect(usageRow.locator('td').nth(5)).toHaveText('0')
  await expect(usageRow.locator('td').nth(6)).toHaveText('-')
  await expect(usageRow.locator('td').nth(7)).toHaveText('0')
  await page.getByRole('button', { name: '详情', exact: true }).click()
  await expect(modal(page).getByText('req_live_compatible')).toBeVisible()
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
  await page.getByRole('button', { name: '搜索', exact: true }).click()
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
    sidebarWidth: 230,
    workspaceLeft: 230,
    navLabelDisplay: 'block',
    topbarHeight: 58,
    documentFits: true,
  })
  const modelLink = page.getByRole('link', { name: '模型', exact: true })
  await expect(modelLink).toBeVisible()
  await expect(modelLink.locator('span')).toBeVisible()
  await expect(modelLink).toHaveAttribute('data-label', '模型')

  await page.getByRole('link', { name: '用量记录', exact: true }).click()
  await expect(page.getByRole('heading', { name: '用量记录', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({
    path: '../.cache/web-visual/responsive-14-inch.png',
    fullPage: true,
    animations: 'disabled',
  })
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
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(230)
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
  await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(230)
  await expect
    .poll(() => workspace.evaluate((element) => element.getBoundingClientRect().left))
    .toBe(230)
  await expect(modelLabel).toBeVisible()
})

test('用户页面使用明确术语并在紧凑侧栏展示菜单提示', async ({ page }) => {
  await fixture(page)
  await page.setViewportSize({ width: 900, height: 768 })
  await signIn(page)

  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建用户' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: '用户名', exact: true })).toBeVisible()
  await expect(page.locator('.workspace-square')).toHaveCount(0)
  await expect(page.locator('.workspace-label')).toHaveCount(0)
  await expect(page.locator('.nav-section')).toHaveText(['社区版', '基础配置', '使用记录'])
  const communityRepository = page.locator('.community-source-link')
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
  await page.getByRole('link', { name: '用量记录' }).click()
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
