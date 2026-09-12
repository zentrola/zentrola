import { test, expect } from '@playwright/test'

test('后端锁定提示、账号切换、刷新重查和倒计时到期重试', async ({ page }) => {
  await page.clock.install()
  let calls = 0
  let remaining = 90
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/setup'))
      return route.fulfill({ json: { code: 'OK', data: { required: false } } })
    if (path.endsWith('/auth/login')) {
      calls++
      if (remaining > 0)
        return route.fulfill({
          status: 429,
          json: { code: 'ACCOUNT_LOCKED', data: { retryAfterSeconds: remaining } },
        })
      return route.fulfill({
        json: {
          code: 'OK',
          data: { token: 'test-token', expiresAt: new Date(Date.now() + 3600000).toISOString() },
        },
      })
    }
    if (path.endsWith('/me'))
      return route.fulfill({
        json: {
          code: 'OK',
          data: { id: '1', username: 'owner', displayName: 'owner' },
        },
      })
    return route.fulfill({ json: { code: 'OK', data: { items: [], nextCursor: null, total: 0 } } })
  })
  await page.goto('/')
  const username = page.getByLabel('管理员账号')
  const password = page.getByLabel('密码', { exact: true })
  const submit = page.locator('button.login-submit')
  await username.fill('owner')
  await password.fill('wrong-password')
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('账号已临时锁定')
  await expect(page.getByRole('alert')).toContainText('1 分 30 秒')
  await expect(submit).toBeDisabled()
  await password.fill('correct-password')
  await password.press('Enter')
  expect(calls).toBe(1)

  await username.fill('another-account')
  await expect(submit).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await username.fill('owner')
  await expect(submit).toBeDisabled()
  await page.clock.fastForward(31000)
  await expect(page.getByRole('alert')).toContainText('0 分 59 秒')

  remaining = 59
  await page.reload()
  await username.fill('owner')
  await password.fill('correct-password')
  await submit.click()
  await expect(submit).toBeDisabled()
  await expect(page.getByRole('alert')).toContainText('0 分 59 秒')
  expect(calls).toBe(2)

  remaining = 0
  await page.clock.fastForward(59000)
  await expect(submit).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await password.fill('correct-password')
  await submit.click()
  await expect(page.getByRole('heading', { name: '仪表盘', exact: true })).toBeVisible()
  expect(calls).toBe(3)
})

test('普通认证失败和缺少倒计时的锁定响应仍可重试', async ({ page }) => {
  let locked = false
  await page.route('**/api/v1/auth/setup', (route) =>
    route.fulfill({ json: { code: 'OK', data: { required: false } } }),
  )
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({
      status: locked ? 429 : 401,
      json: { code: locked ? 'ACCOUNT_LOCKED' : 'UNAUTHENTICATED' },
    }),
  )
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('owner')
  const password = page.getByLabel('密码', { exact: true })
  const submit = page.getByRole('button', { name: '登录控制台' })
  await password.fill('wrong-password')
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('账号或密码错误')
  await expect(submit).toBeEnabled()
  locked = true
  await password.fill('wrong-password')
  await submit.click()
  await expect(page.getByRole('alert')).toContainText('账号已临时锁定，请稍后重试')
  await expect(submit).toBeEnabled()
})
