import { test, expect } from '@playwright/test'

test('首次按浏览器语言选择英文，手动切换后记住偏好并覆盖自动判断', async ({ browser }) => {
  const context = await browser.newContext({
    baseURL: test.info().project.use.baseURL as string,
    locale: 'en-US',
  })
  const page = await context.newPage()
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const reply = (data: unknown) => route.fulfill({ json: { code: 'OK', data } })
    if (path.endsWith('/auth/setup')) return reply({ required: false })
    if (path.endsWith('/auth/login'))
      return reply({
        token: 'language-test-token',
        expiresAt: new Date(Date.now() + 3600000).toISOString(),
      })
    if (path.endsWith('/me'))
      return reply({ id: '1', organizationId: '2', username: 'admin', displayName: 'Admin' })
    return reply({ items: [], nextCursor: null })
  })

  await page.goto('/')
  await expect(
    page.getByRole('heading', {
      name: 'Open-source AI Coding Control Plane for engineering teams.',
    }),
  ).toBeVisible()
  await expect(page.getByText('Supports OpenAI and Anthropic protocols')).toBeVisible()
  await expect(
    page.getByText('Keep your existing clients and workflows—no change to how your team works.'),
  ).toBeVisible()
  await expect(page.getByText('Principals')).toHaveCount(0)
  await expect(page.locator('html')).toHaveAttribute('lang', 'en-US')
  await expect(page).toHaveTitle('Zentrola · Admin Console')

  await page.getByRole('group', { name: 'Language' }).getByRole('button', { name: '中' }).click()
  await expect(page.getByRole('heading', { name: '企业 AI Coding 能力治理平台' })).toBeVisible()
  await expect(page.getByText('同时支持 OpenAI / Anthropic 双协议')).toBeVisible()
  await expect(page.getByText('继续使用现有客户端与工作流，无需改变使用习惯。')).toBeVisible()
  await expect(page.getByText('成员身份')).toHaveCount(0)
  expect(await page.evaluate(() => localStorage.getItem('zentrola.ui.locale'))).toBe('zh-CN')
  await page.reload()
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()

  await page.getByRole('group', { name: '界面语言' }).getByRole('button', { name: 'EN' }).click()
  await page.getByLabel('Administrator username').fill('admin')
  await page.getByLabel('Password', { exact: true }).fill('language-test-password')
  await page.getByRole('button', { name: 'Sign in to console' }).click()
  await expect(page.getByRole('heading', { name: 'Users', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Operation logs', exact: true })).toBeVisible()

  await context.close()
})
