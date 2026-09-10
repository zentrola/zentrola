import { test, expect, type Page } from '@playwright/test'

async function fixture(page: Page) {
  const state = { calls: 0, status: 200, code: 'OK', body: null as unknown }
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const reply = (data: unknown) => route.fulfill({ json: { code: 'OK', data } })
    if (path.endsWith('/auth/setup')) return reply({ required: false })
    if (path.endsWith('/auth/login'))
      return reply({ token: 'test-token', expiresAt: new Date(Date.now() + 3600000).toISOString() })
    if (path.endsWith('/me')) return reply({ id: '1', username: 'admin', displayName: '管理员' })
    if (path.endsWith('/auth/password')) {
      state.calls++
      state.body = route.request().postDataJSON()
      expect(route.request().headers().authorization).toBe('Bearer test-token')
      expect(route.request().method()).toBe('POST')
      return route.fulfill({
        status: state.status,
        json: { code: state.code, data: { clearToken: true } },
      })
    }
    return reply({ items: [], nextCursor: null, total: 0 })
  })
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('original-password')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await page.getByRole('button', { name: '账号菜单' }).click()
  await page.getByRole('menuitem', { name: '修改密码' }).click()
  return state
}

test('修改成功后清除会话并提示使用新密码登录', async ({ page }) => {
  const state = await fixture(page)
  await page.getByLabel('当前密码', { exact: true }).fill('original-password')
  await page.getByLabel('新密码', { exact: true }).fill('123456')
  await page.getByLabel('确认新密码').fill('123456')
  await page.getByRole('button', { name: '确认修改', exact: true }).click()
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
  await expect(page.getByRole('status')).toContainText('登录密码已修改')
  expect(state.body).toEqual({
    currentPassword: 'original-password',
    newPassword: '123456',
  })
  expect(state.calls).toBe(1)
  expect(await page.evaluate(() => localStorage.getItem('zentrola.admin.session'))).toBeNull()
  await expect(page.getByLabel('密码', { exact: true })).toHaveValue('')
})

test('校验必填、字符与字节长度、相同密码与两次输入一致', async ({ page }) => {
  const state = await fixture(page)
  const submit = page.getByRole('button', { name: '确认修改', exact: true })
  await submit.click()
  await expect(page.getByLabel('当前密码', { exact: true })).toBeFocused()
  await page.getByLabel('当前密码', { exact: true }).fill('original-password')
  for (const [value, message] of [
    ['short', '密码过短'],
    ['密'.repeat(25), '密码过长'],
    ['original-password', '新密码不能与当前密码相同'],
  ]) {
    await page.getByLabel('新密码', { exact: true }).fill(value!)
    await submit.click()
    await expect(page.locator('#new-password-error')).toContainText(message!)
  }
  await page.getByLabel('新密码', { exact: true }).fill('new-password-2026')
  await page.getByLabel('确认新密码').fill('different-password')
  await submit.click()
  await expect(page.locator('#confirm-new-password-error')).toContainText('两次输入的密码不一致')
  expect(state.calls).toBe(0)
})

test('当前密码错误保留对话框和登录状态，允许重试', async ({ page }) => {
  const state = await fixture(page)
  state.status = 403
  state.code = 'CURRENT_PASSWORD_INCORRECT'
  await page.getByLabel('当前密码', { exact: true }).fill('wrong-password')
  await page.getByLabel('新密码', { exact: true }).fill('new-password-2026')
  await page.getByLabel('确认新密码').fill('new-password-2026')
  await page.getByRole('button', { name: '确认修改', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.locator('.toast')).toContainText('当前密码不正确')
  await expect(page.getByRole('dialog').locator('.alert.error')).toHaveCount(0)
  await expect(page.getByLabel('当前密码', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('当前密码', { exact: true })).toBeFocused()
  expect(await page.evaluate(() => localStorage.getItem('zentrola.admin.session'))).not.toBeNull()
  state.status = 200
  state.code = 'OK'
  await page.getByLabel('当前密码', { exact: true }).fill('original-password')
  await page.getByRole('button', { name: '确认修改', exact: true }).click()
  await expect(page.getByRole('button', { name: '登录控制台' })).toBeVisible()
})

test('移动端可打开，取消后清空输入并恢复焦点', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const state = await fixture(page)
  await page.getByLabel('当前密码', { exact: true }).fill('original-password')
  await page.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.getByRole('button', { name: '账号菜单' })).toBeFocused()
  await page.getByRole('button', { name: '账号菜单' }).click()
  await page.getByRole('menuitem', { name: '修改密码' }).click()
  await expect(page.getByLabel('当前密码', { exact: true })).toHaveValue('')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(state.calls).toBe(0)
})
