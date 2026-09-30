import { test, expect } from '@playwright/test'

test('空系统创建首位管理员，校验确认密码后切换为登录', async ({ page }) => {
  let initialized = false,
    setupCalls = 0
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname,
      method = route.request().method()
    const reply = (data: unknown, status = 200, code = 'OK') =>
      route.fulfill({ status, json: { code, data } })
    if (path.endsWith('/auth/setup')) {
      if (method === 'GET') return reply({ required: !initialized })
      setupCalls++
      expect(route.request().postDataJSON()).toEqual({
        username: 'owner',
        password: 'owner-password-123',
      })
      initialized = true
      return reply({ initialized: true }, 201)
    }
    if (path.endsWith('/auth/login'))
      return reply({
        token: 'setup-test-token',
        expiresAt: new Date(Date.now() + 3600000).toISOString(),
      })
    if (path.endsWith('/me')) return reply({ id: '1', username: 'owner', displayName: 'owner' })
    return reply({ items: [], nextCursor: null, total: 0 })
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '初始化管理员' })).toBeVisible()
  await page.getByLabel('管理员账号').fill('owner')
  await page.getByLabel('密码', { exact: true }).fill('owner-password-123')
  await page.getByLabel('确认密码').fill('different-password')
  await page.getByRole('button', { name: '创建管理员', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('两次输入的密码不一致')
  await expect(page.getByLabel('确认密码')).toBeFocused()
  expect(setupCalls).toBe(0)
  await page.getByLabel('确认密码').fill('owner-password-123')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page.getByRole('button', { name: '创建管理员', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByRole('status')).toContainText('管理员已创建')
  await expect(page.getByLabel('密码', { exact: true })).toHaveValue('')
  expect(setupCalls).toBe(1)
  await page.getByLabel('密码', { exact: true }).fill('owner-password-123')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await expect(page.getByRole('heading', { name: '仪表盘', exact: true })).toBeVisible()
  expect(await page.evaluate(() => [localStorage.length, sessionStorage.length])).toEqual([1, 0])
  await page.reload()
  await expect(page.getByRole('heading', { name: '仪表盘', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '创建管理员', exact: true })).toHaveCount(0)
})

test('初始化按字段校验，错误修正后消失，并限制密码字符数和字符集', async ({ page }) => {
  let setupCalls = 0
  await page.route('**/api/v1/auth/setup', async (route) => {
    if (route.request().method() === 'GET')
      return route.fulfill({ json: { code: 'OK', data: { required: true } } })
    setupCalls++
    expect(route.request().postDataJSON()).toEqual({
      username: '中'.repeat(21) + 'a',
      password: 'A1!'.repeat(10),
    })
    return route.fulfill({ status: 201, json: { code: 'OK', data: { initialized: true } } })
  })
  await page.goto('/')
  const username = page.getByLabel('管理员账号')
  const password = page.getByLabel('密码', { exact: true })
  const confirm = page.getByLabel('确认密码')
  const submit = page.getByRole('button', { name: '创建管理员', exact: true })
  await expect(submit).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByText('中文字符通常占 3 bytes。', { exact: false })).toHaveCount(0)
  await submit.click()
  await expect(username).toBeFocused()
  await expect(page.getByRole('alert')).toHaveCount(3)
  expect(setupCalls).toBe(0)

  await username.fill(' owner')
  await expect(username).toHaveAttribute('aria-invalid', 'true')
  await expect(username).toHaveAccessibleDescription('账号首尾不能有空白字符。')
  await username.fill('owner\u0085')
  await expect(username).toHaveAccessibleDescription('账号不能包含控制字符。')
  await username.fill('中'.repeat(22))
  await expect(username).toHaveAccessibleDescription(/账号过长/)
  await username.fill('中'.repeat(21) + 'a')
  await expect(username).toHaveAttribute('aria-invalid', 'false')

  await password.fill('abc12')
  await expect(password).toHaveAccessibleDescription(/密码过短/)
  await password.fill('abc123')
  await expect(password).toHaveAttribute('aria-invalid', 'false')
  await confirm.fill('abc123')
  await expect(confirm).toHaveAttribute('aria-invalid', 'false')
  await password.fill('a'.repeat(31))
  await expect(password).toHaveAccessibleDescription(/密码过长/)
  await expect(confirm).toHaveAccessibleDescription('两次输入的密码不一致。')
  await submit.click()
  await expect(password).toBeFocused()
  expect(setupCalls).toBe(0)
  await password.fill('密码123456')
  await expect(password).toHaveAccessibleDescription(/只能使用数字、英文字母和特殊符号/)
  await password.fill('A1!'.repeat(10))
  await confirm.fill('A1!'.repeat(10))
  await expect(page.getByRole('alert')).toHaveCount(0)
  await submit.click()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(setupCalls).toBe(1)
})

test('首次输入不提前报错，失焦后就地提示并随输入更新', async ({ page }) => {
  await page.route('**/api/v1/auth/setup', (route) =>
    route.fulfill({ json: { code: 'OK', data: { required: true } } }),
  )
  await page.goto('/')
  const username = page.getByLabel('管理员账号')
  await username.fill(' owner')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await username.press('Tab')
  await expect(username).toHaveAccessibleDescription('账号首尾不能有空白字符。')
  await username.fill('owner')
  await expect(username).toHaveAttribute('aria-invalid', 'false')
  await expect(page.locator('#username-error')).toHaveCount(0)
})

test('初始化状态失败不开放注册，其他页面抢先完成后关闭初始化入口', async ({ page }) => {
  let healthy = false,
    completedElsewhere = false
  await page.route('**/api/v1/auth/setup', async (route) => {
    if (!healthy) return route.fulfill({ status: 503, json: { code: 'SERVICE_UNAVAILABLE' } })
    if (route.request().method() === 'POST') {
      completedElsewhere = true
      return route.fulfill({ status: 409, json: { code: 'ALREADY_INITIALIZED' } })
    }
    return route.fulfill({ json: { code: 'OK', data: { required: !completedElsewhere } } })
  })
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('服务暂时不可用')
  await expect(page.getByRole('button', { name: '创建管理员', exact: true })).toHaveCount(0)
  healthy = true
  await page.getByRole('button', { name: '重试' }).click()
  await expect(page.getByRole('heading', { name: '初始化管理员' })).toBeVisible()
  await page.getByLabel('管理员账号').fill('owner')
  await page.getByLabel('密码', { exact: true }).fill('owner-password-123')
  await page.getByLabel('确认密码').fill('owner-password-123')
  await page.getByRole('button', { name: '创建管理员', exact: true }).click()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('系统已完成管理员初始化')
})

test('非 JSON 服务端错误保留 HTTP 错误语义', async ({ page }) => {
  await page.route('**/api/v1/auth/setup', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'text/html',
      body: '<html><body>Service Unavailable</body></html>',
    }),
  )
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('服务暂时不可用')
  await expect(page.getByRole('alert')).not.toContainText('无法连接服务')
})
