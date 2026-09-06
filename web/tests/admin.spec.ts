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
      createdAt: stamp,
      updatedAt: stamp,
    },
  ]
  const providers = [
    {
      id: '81',
      name: 'DeepSeek Official',
      code: 'deepseek-official',
      status: 'ACTIVE',
      baseUrl: 'https://api.deepseek.com/anthropic',
      openaiBaseUrl: 'https://api.deepseek.com',
    },
  ]
  const groups: any[] = [],
    resources: any[] = [],
    keys: any[] = [],
    usageQueries: URLSearchParams[] = []
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
    if (path === '/me')
      return reply({ id: '1', organizationId: '11', username: 'admin', displayName: '组织管理员' })
    if (path === '/auth/logout') return reply({ clearToken: true })
    const pageReply = (items: any[]) => reply({ items, nextCursor: null })
    if (segments.length === 1 && method === 'GET') {
      if (path === '/members') return pageReply(members)
      if (path === '/models') return pageReply(models)
      if (path === '/providers') return pageReply(providers)
      if (path === '/groups') return pageReply(groups)
      if (path === '/resources') return pageReply(resources.map(({ credential: _, ...row }) => row))
      if (path === '/operation-logs') return pageReply([])
      if (path === '/usage') {
        usageQueries.push(url.searchParams)
        return pageReply([
          {
            id: '99',
            requestId: 'req_live_compatible',
            principalId: longID,
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
            usageId: '101',
            attemptNo: 1,
            attemptStatus: 'SUCCESS',
            errorType: null,
          },
        ])
      }
    }
    if (path === '/usage/writer') return reply({ pending: 0, failed: 0 })
    if (method === 'POST' && segments.length === 1) {
      const row = {
        ...body,
        id: next(),
        status: path === '/resources' || path === '/models' ? 'DISABLED' : 'ACTIVE',
        createdAt: stamp,
        updatedAt: stamp,
        credentialConfigured: path === '/resources',
      }
      if (conflict) return reply(null, 409, 'CONFLICT')
      const target =
        path === '/members'
          ? members
          : path === '/groups'
            ? groups
            : path === '/models'
              ? models
              : resources
      target.push(row)
      const { credential: _, ...safe } = row
      return reply(safe, 201)
    }
    if (segments[0] === 'models' && segments.length === 2 && method === 'PUT') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const row = models.find((model) => model.id === segments[1])
      Object.assign(row, body)
      return reply(row)
    }
    if (segments[0] === 'members' && segments.length === 2 && method === 'DELETE') {
      if (conflict) return reply(null, 409, 'CONFLICT')
      const index = members.findIndex((member) => member.id === segments[1])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      members.splice(index, 1)
      return reply({ deleted: true })
    }
    if (segments[2] === 'status') {
      const list =
        segments[0] === 'members' ? members : segments[0] === 'models' ? models : resources
      const row = list.find((x) => x.id === segments[1])
      row.status = body.status
      return reply(row)
    }
    if (segments[0] === 'members' && segments[2] === 'keys') {
      if (method === 'GET')
        return pageReply(
          keys
            .filter((k) => k.memberID === segments[1])
            .map(({ key: _, memberID: __, ...safe }) => safe),
        )
      expect(segments[1]).toBe(longID)
      const row = {
        ...body,
        id: next(),
        memberID: segments[1],
        key: 'fixture-key-shown-once',
        prefix: 'fixture-key',
        status: 'ACTIVE',
        createdAt: stamp,
        revokedAt: null,
      }
      keys.push(row)
      return reply(row, 201)
    }
    if (segments[0] === 'access-keys') {
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
    return reply(null, 404, 'NOT_FOUND')
  })
  return {
    members,
    models,
    groups,
    resources,
    keys,
    relationships,
    usageQueries,
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
async function signIn(page: Page) {
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('fixture-password')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await expect(page.getByRole('heading', { name: '成员', exact: true })).toBeVisible()
}
const modal = (page: Page) => page.locator('dialog').last()
test('成员列表直接展示多个 Key、分配校验以及删除确认和失败恢复', async ({ page }) => {
  const state = await fixture(page)
  state.keys.push(
    {
      id: '801',
      memberID: longID,
      name: '工作站',
      prefix: 'zt_vk_work',
      status: 'ACTIVE',
      expiresAt: null,
      revokedAt: null,
      createdAt: stamp,
    },
    {
      id: '802',
      memberID: longID,
      name: '临时测试',
      prefix: 'zt_vk_temp',
      status: 'ACTIVE',
      expiresAt: '2020-01-01T00:00:00Z',
      revokedAt: null,
      createdAt: stamp,
    },
  )
  await signIn(page)
  const row = page.getByRole('row').filter({ hasText: '林知远' })
  await expect(page.getByRole('button', { name: '管理 Key' })).toHaveCount(0)
  await expect(row).toContainText('zt_vk_work…')
  await expect(row).toContainText('长期有效')
  await expect(row).toContainText('2020年1月1日')
  await expect(row).toContainText('已过期')
  await expect(
    page.getByRole('row').filter({ hasText: '周予安' }).getByRole('button', { name: '分配 Key' }),
  ).toBeDisabled()
  await row.getByRole('button', { name: '分配 Key' }).click()
  await modal(page).getByLabel('Key 名称').fill('自动化')
  await modal(page).getByLabel('到期时间（可选）').fill('2020-01-01T12:00')
  await modal(page).getByRole('button', { name: '分配 Key' }).click()
  await expect(modal(page).getByRole('alert')).toContainText('必须晚于当前时间')
  await modal(page).getByLabel('到期时间（可选）').fill('2099-12-31T12:00')
  await modal(page).getByRole('button', { name: '分配 Key' }).click()
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  await expect(row).toContainText('自动化')
  await expect(row).toContainText('2099年12月31日')
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/members-keys.png', fullPage: true })
  await row.getByRole('button', { name: '删除', exact: true }).click()
  await modal(page).getByRole('button', { name: '取消' }).click()
  expect(state.members).toHaveLength(3)
  await row.getByRole('button', { name: '删除', exact: true }).click()
  state.conflict(true)
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(modal(page).getByRole('alert')).toContainText('操作冲突')
  expect(state.members).toHaveLength(3)
  state.conflict(false)
  await modal(page).getByRole('button', { name: '删除', exact: true }).click()
  await expect(row).toHaveCount(0)
  await expect(page.getByRole('status')).toContainText('成员已删除')
  expect(state.members).toHaveLength(2)
})

test('成员 Key 加载失败可重试', async ({ page }) => {
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
  await expect(row.getByRole('alert')).toBeVisible()
  fail = false
  await row.getByRole('button', { name: '重试' }).click()
  await expect(row.getByRole('alert')).toHaveCount(0)
  await expect(row).toContainText('还没有访问密钥')
})

test('模型新增编辑、模态校验、冲突恢复和窄屏表单', async ({ page }) => {
  const state = await fixture(page)
  await signIn(page)
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await page.getByRole('button', { name: '添加模型' }).click()
  const dialog = modal(page)
  await dialog.getByLabel('官方模型名称', { exact: true }).fill('测试官方模型')
  await dialog.getByLabel('官方模型编码', { exact: true }).fill('official-test-v1')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('至少选择一项')
  await dialog.getByRole('group', { name: '输入类型' }).getByLabel('文本', { exact: true }).check()
  await dialog.getByRole('group', { name: '输入类型' }).getByLabel('图片', { exact: true }).check()
  await dialog.getByRole('group', { name: '输出类型' }).getByLabel('文本', { exact: true }).check()
  await dialog.getByLabel('备注', { exact: true }).fill('图文理解用途')
  state.conflict(true)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('操作冲突')
  await expect(dialog.getByLabel('官方模型编码', { exact: true })).toHaveValue('official-test-v1')
  state.conflict(false)
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('dialog')).toHaveCount(0)
  const row = page.getByRole('row').filter({ hasText: 'official-test-v1' })
  await expect(row).toContainText('多模态')
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
  await page.getByRole('searchbox', { name: '搜索已加载记录' }).fill('图文理解用途')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(1)
  await page.screenshot({ path: '../.cache/web-visual/model-catalog-desktop.png' })
})
test('分组勾选模型、搜索、失败恢复及停用授权限制', async ({ page }) => {
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
  await page.getByRole('link', { name: '分组', exact: true }).click()
  await page.getByRole('button', { name: '模型授权', exact: true }).click()
  const dialog = modal(page)
  const active = dialog.getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true })
  const disabled = dialog.getByRole('checkbox', { name: '授权 Claude Sonnet', exact: true })
  await expect(active).toBeEnabled()
  await expect(active).not.toBeChecked()
  await expect(disabled).toBeDisabled()
  state.conflict(true)
  await active.click()
  await expect(dialog.getByRole('alert')).toContainText('操作冲突')
  await expect(active).not.toBeChecked()
  expect(state.relationships.get('groups/51/models')?.has('71')).toBe(false)
  state.conflict(false)
  await active.click()
  await expect(active).toBeChecked()
  await expect(dialog.getByRole('alert')).toHaveCount(0)
  await dialog.getByRole('searchbox').fill('claude-sonnet')
  await dialog.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(active).toHaveCount(0)
  await expect(disabled).toBeVisible()
  expect(state.relationships.get('groups/51/models')?.has('71')).toBe(true)
  await dialog.getByRole('button', { name: '重置', exact: true }).click()
  await expect(active).toBeChecked()
  await mkdir('../.cache/web-visual', { recursive: true })
  await dialog.screenshot({ path: '../.cache/web-visual/group-model-checklist.png' })
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  state.relationships.get('groups/51/models')!.add('72')
  await page.getByRole('button', { name: '模型授权', exact: true }).click()
  await expect(disabled).toBeChecked()
  await expect(disabled).toBeEnabled()
  await disabled.click()
  await expect(disabled).not.toBeChecked()
  await expect(disabled).toBeDisabled()
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  state.groups[0].status = 'DISABLED'
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await page.getByRole('link', { name: '分组', exact: true }).click()
  await page.getByRole('button', { name: '模型授权', exact: true }).click()
  await expect(active).toBeChecked()
  await expect(active).toBeEnabled()
  await active.click()
  await expect(active).not.toBeChecked()
  await expect(active).toBeDisabled()
})

test('管理员通过网页完成配置、Key 生命周期和用量查询', async ({ page }) => {
  const state = await fixture(page),
    errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await signIn(page)
  await expect(page.getByText(longID, { exact: true })).toBeVisible()
  const memberSearch = page.getByRole('searchbox', { name: '搜索已加载记录' })
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
  await page.getByRole('button', { name: '创建成员' }).click()
  await modal(page).getByLabel('名称', { exact: true }).fill('浏览器验收成员')
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByText('浏览器验收成员', { exact: true })).toBeVisible()
  await mkdir('test-results/visual', { recursive: true })
  await page.screenshot({ path: 'test-results/visual/members.png', fullPage: true })

  await page.getByRole('link', { name: '分组', exact: true }).click()
  await page.getByRole('button', { name: '创建分组' }).click()
  await modal(page).getByLabel('名称', { exact: true }).fill('平台研发组')
  await modal(page).getByLabel('编码', { exact: true }).fill('platform')
  state.conflict(true)
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await expect(modal(page).getByRole('alert')).toContainText('操作冲突')
  state.conflict(false)
  await modal(page).getByRole('button', { name: '创建', exact: true }).click()
  await page.getByRole('button', { name: '成员分配', exact: true }).click()
  await modal(page).getByLabel('添加成员').selectOption(longID)
  await modal(page).getByRole('button', { name: '添加', exact: true }).click()
  await expect(modal(page).getByText('林知远', { exact: true })).toBeVisible()
  await modal(page).getByRole('searchbox').fill('不存在的分配成员')
  await modal(page).getByRole('button', { name: '搜索', exact: true }).click()
  await expect(modal(page).getByText('没有匹配的记录', { exact: true })).toBeVisible()
  await modal(page).getByRole('button', { name: '重置', exact: true }).click()
  await expect(modal(page).getByText('林知远', { exact: true })).toBeVisible()
  await modal(page).getByRole('button', { name: '模型授权', exact: true }).click()
  await modal(page).getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true }).click()
  await expect(
    modal(page).getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true }),
  ).toBeChecked()
  await expect(modal(page).getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
  await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

  await page.getByRole('link', { name: '模型', exact: true }).click()
  const modelSwitch = page.getByRole('switch', { name: 'DeepSeek V4 Flash的启用状态' })
  await expect(modelSwitch).toBeChecked()
  await modelSwitch.press('Space')
  await modal(page).getByRole('button', { name: '取消', exact: true }).click()
  await expect(modelSwitch).toBeChecked()
  await modelSwitch.click()
  await modal(page).getByRole('button', { name: '确认', exact: true }).click()
  await expect(modelSwitch).not.toBeChecked()
  await modelSwitch.click()
  await modal(page).getByRole('button', { name: '确认', exact: true }).click()
  await expect(modelSwitch).toBeChecked()
  await expect(page.getByRole('button', { name: '授权', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: '分组', exact: true }).click()
  await page.getByRole('button', { name: '模型授权', exact: true }).click()
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

  await page.getByRole('link', { name: '资源', exact: true }).click()
  await page.getByRole('button', { name: '添加资源' }).click()
  await modal(page).getByLabel('名称', { exact: true }).fill('开发环境资源')
  await modal(page).getByLabel('上游服务').selectOption('81')
  await modal(page).getByLabel('上游 API Key', { exact: true }).fill('fixture-upstream-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await expect(page.getByText('开发环境资源', { exact: true })).toBeVisible()
  expect(await page.content()).not.toContain('fixture-upstream-credential')
  await page.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(modal(page).getByRole('status')).toContainText('连接测试通过')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  state.failTest()
  await page.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(modal(page).getByRole('status')).toContainText('上游认证失败')
  await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
  await page.getByRole('button', { name: '覆盖凭证' }).click()
  await modal(page).getByLabel('上游 API Key', { exact: true }).fill('replacement-credential')
  await modal(page).getByRole('button', { name: '保存' }).click()
  await page.getByRole('switch', { name: '开发环境资源的启用状态' }).click()
  await modal(page).getByRole('button', { name: '确认', exact: true }).click()
  await expect(page.getByRole('switch', { name: '开发环境资源的启用状态' })).toBeChecked()

  await page.getByRole('link', { name: '成员', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('button', { name: '分配 Key' })
    .click()
  await modal(page).getByLabel('Key 名称').fill('工作站')
  await modal(page).getByRole('button', { name: '分配 Key' }).click()
  await expect(page.getByRole('textbox', { name: '完整 Key 仅展示这一次' })).toHaveValue(
    'fixture-key-shown-once',
  )
  await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
  expect(await page.content()).not.toContain('fixture-key-shown-once')
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('button', { name: '撤销', exact: true })
    .click()
  await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
  await expect(
    page.getByRole('row').filter({ hasText: '林知远' }).getByText('已撤销', { exact: true }),
  ).toBeVisible()
  await page
    .getByRole('row')
    .filter({ hasText: '林知远' })
    .getByRole('switch', { name: '林知远的启用状态' })
    .click()
  await modal(page).getByRole('button', { name: '确认', exact: true }).click()
  await expect(page.getByRole('switch', { name: '林知远的启用状态' })).not.toBeChecked()
  expect(state.members[0].status).toBe('DISABLED')

  await page.getByRole('link', { name: '用量记录' }).click()
  await page.getByRole('combobox', { name: '成员', exact: true }).selectOption(longID)
  await page.getByRole('combobox', { name: '模型', exact: true }).selectOption('71')
  await page
    .getByRole('combobox', { name: '资源', exact: true })
    .selectOption(state.resources[0].id)
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect.poll(() => state.usageQueries.at(-1)?.get('memberId')).toBe(longID)
  expect(state.usageQueries.at(-1)?.get('modelId')).toBe('71')
  expect(state.usageQueries.at(-1)?.get('resourceId')).toBe(state.resources[0].id)
  const usageRow = page.getByRole('row').filter({ hasText: 'OpenAI' })
  await expect(usageRow.locator('td').nth(5)).toHaveText('0')
  await expect(usageRow.locator('td').nth(6)).toHaveText('—')
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
  await signIn(page)
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

test('列表分页保持长 ID，并拒绝无效用量时间范围', async ({ page }) => {
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
              id: after ? '90071992547409932' : longID,
              name: after ? '第二页成员' : '第一页成员',
              status: 'ACTIVE',
              createdAt: stamp,
            },
          ],
          nextCursor: after ? null : longID,
        },
      },
    })
  })
  await signIn(page)
  await page.getByRole('button', { name: '加载更多' }).click()
  await expect(page.getByText('第一页成员', { exact: true })).toBeVisible()
  await expect(page.getByText('第二页成员', { exact: true })).toBeVisible()
  expect(pages).toBe(2)
  await page.getByRole('link', { name: '用量记录' }).click()
  await expect(page.getByRole('button', { name: '搜索', exact: true })).toBeEnabled()
  await page.getByLabel('结束时间', { exact: true }).fill('2020-01-01T00:00')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('有效的起止时间')
  await page.getByRole('button', { name: '重置', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
})
