import { test, expect, type BrowserContext, type Page } from '@playwright/test'

const sessionKey = 'zentrola.admin.session'
async function fixture(context: BrowserContext) {
  const state = { meStatus: 200, meCalls: 0, loginCalls: 0, expiresAt: '' }
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const reply = (data: unknown, status = 200, code = 'OK') =>
      route.fulfill({ status, json: { code, data } })
    if (path.endsWith('/auth/setup')) return reply({ required: false })
    if (path.endsWith('/auth/login')) {
      state.loginCalls++
      state.expiresAt = new Date(Date.now() + 8 * 3600000).toISOString()
      return reply({ token: 'session-test-token', expiresAt: state.expiresAt })
    }
    expect(route.request().headers().authorization).toBe('Bearer session-test-token')
    if (path.endsWith('/me')) {
      state.meCalls++
      if (state.meStatus !== 200)
        return reply(
          null,
          state.meStatus,
          state.meStatus === 401 ? 'UNAUTHENTICATED' : 'SERVICE_UNAVAILABLE',
        )
      return reply({ id: '1', organizationId: '2', username: 'admin', displayName: '管理员' })
    }
    return reply({ items: [], nextCursor: null })
  })
  return state
}
async function signIn(page: Page) {
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await expect(page.getByRole('heading', { name: '用户', exact: true })).toBeVisible()
}
const stored = (page: Page) => page.evaluate((key) => localStorage.getItem(key), sessionKey)

test('刷新及关闭后重新打开恢复身份和路由，不重新登录或延长到期时间', async ({ page, context }) => {
  const state = await fixture(context)
  await signIn(page)
  const saved = await stored(page)
  expect(JSON.parse(saved!)).toEqual({ token: 'session-test-token', expiresAt: state.expiresAt })
  await page.getByRole('link', { name: '模型', exact: true }).click()
  await page.reload()
  await expect(page.getByRole('heading', { name: '模型', exact: true })).toBeVisible()
  expect(state.meCalls).toBe(2)
  expect(await stored(page)).toBe(saved)
  await page.close()
  const reopened = await context.newPage()
  await reopened.goto('/')
  await expect(reopened.getByRole('heading', { name: '用户', exact: true })).toBeVisible()
  expect(state.loginCalls).toBe(1)
  expect(state.meCalls).toBe(3)
  await reopened.getByRole('button', { name: '账号菜单' }).click()
  await reopened.getByRole('menuitem', { name: '注销登录' }).click()
  await expect(reopened.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  expect(await stored(reopened)).toBeNull()
  await reopened.reload()
  await expect(reopened.getByRole('button', { name: '登录控制台' })).toBeVisible()
  expect(state.meCalls).toBe(3)
})

test('恢复时 401 清除凭证，之后刷新不再尝试恢复', async ({ page, context }) => {
  const state = await fixture(context)
  await signIn(page)
  state.meStatus = 401
  await page.reload()
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('登录已失效')
  expect(await stored(page)).toBeNull()
  await page.reload()
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
  expect(state.meCalls).toBe(2)
})

test('恢复时服务暂时不可用保留凭证，重试成功后进入控制台', async ({ page, context }) => {
  const state = await fixture(context)
  await signIn(page)
  const saved = await stored(page)
  state.meStatus = 503
  await page.reload()
  await expect(page.getByRole('alert')).toContainText('服务暂时不可用')
  await expect(page.getByRole('button', { name: '登录控制台' })).toHaveCount(0)
  expect(await stored(page)).toBe(saved)
  state.meStatus = 200
  await page.getByRole('button', { name: '重试' }).click()
  await expect(page.getByRole('heading', { name: '用户', exact: true })).toBeVisible()
  expect(state.loginCalls).toBe(1)
})

for (const value of [
  '{bad-json',
  'null',
  '{"token":12,"expiresAt":"invalid"}',
  JSON.stringify({ token: 'session-test-token', expiresAt: '2020-01-01T00:00:00Z' }),
]) {
  test(`无效或过期存储不恢复身份：${value}`, async ({ page, context }) => {
    const state = await fixture(context)
    await page.goto('/')
    await page.evaluate(({ key, value }) => localStorage.setItem(key, value), {
      key: sessionKey,
      value,
    })
    await page.reload()
    await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
    expect(await stored(page)).toBeNull()
    expect(state.meCalls).toBe(0)
  })
}

test('恢复后的会话按原到期时间退出并清除存储', async ({ page, context }) => {
  await fixture(context)
  await page.clock.install()
  await signIn(page)
  await page.reload()
  await expect(page.getByRole('heading', { name: '用户', exact: true })).toBeVisible()
  await page.clock.fastForward(8 * 3600000 + 1000)
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('登录已失效')
  expect(await stored(page)).toBeNull()
})

test('退出登录同步其他标签页', async ({ page, context }) => {
  await fixture(context)
  await signIn(page)
  const other = await context.newPage()
  await other.goto('/')
  await expect(other.getByRole('heading', { name: '用户', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '账号菜单' }).click()
  await page.getByRole('menuitem', { name: '注销登录' }).click()
  await expect(other.getByRole('button', { name: '登录控制台' })).toBeVisible()
  await other.reload()
  await expect(other.getByRole('button', { name: '登录控制台' })).toBeVisible()
})

test('浏览器禁用存储时仍能登录和退出', async ({ page, context }) => {
  await fixture(context)
  await page.addInitScript(() => {
    for (const method of ['getItem', 'setItem', 'removeItem'])
      Object.defineProperty(Storage.prototype, method, {
        value() {
          throw new Error('Storage blocked')
        },
      })
  })
  await signIn(page)
  await page.getByRole('button', { name: '账号菜单' }).click()
  await page.getByRole('menuitem', { name: '注销登录' }).click()
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
})
