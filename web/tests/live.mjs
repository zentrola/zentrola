// 显式验收脚本：真实本地服务；不推理、不覆盖已有资源、不保存任何凭证或录制。
import { chromium } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import assert from 'node:assert/strict'
const webBase = process.env.ZENTROLA_WEB_URL || 'http://127.0.0.1:9528'
const apiBase = process.env.ZENTROLA_API_URL || 'http://127.0.0.1:8081'
if (
  !['127.0.0.1', 'localhost'].includes(new URL(webBase).hostname) ||
  !['127.0.0.1', 'localhost'].includes(new URL(apiBase).hostname)
)
  throw new Error('Local validation only')
const password = process.env.ZENTROLA_TEST_ADMIN_PASSWORD
const username = process.env.ZENTROLA_TEST_ADMIN_USERNAME
if (!username || !password)
  throw new Error('ZENTROLA_TEST_ADMIN_USERNAME and ZENTROLA_TEST_ADMIN_PASSWORD are required')
const browser = await chromium.launch()
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  reducedMotion: 'reduce',
})
const page = await context.newPage(),
  report = [],
  errors = []
page.on('pageerror', () => errors.push('browser_runtime_error'))
page.on('console', (msg) => {
  if (msg.type() === 'error') errors.push('browser_console_error')
})
let token = '',
  memberID = '',
  groupID = '',
  modelID = '',
  keyID = '',
  step = 'startup',
  failed = false
const stamp = Date.now(),
  memberName = `Web 验收 ${stamp}`,
  groupName = `Web 验收组 ${stamp}`
const dialog = () => page.locator('dialog').last()
const api = async (path, method = 'GET') => {
  const response = await context.request.fetch(`${apiBase}/api/v1${path}`, {
    method,
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!response.ok()) throw new Error('Validation API failed')
  return (await response.json()).data
}
const clickResponse = async (locator, path) => {
  const received = page.waitForResponse(
    (r) => new URL(r.url()).pathname === `/api/v1${path}` && r.request().method() === 'POST',
  )
  await locator.click()
  const response = await received
  if (!response.ok()) throw new Error('Validation mutation failed')
  return (await response.json()).data
}
try {
  await mkdir('../.cache/web-visual', { recursive: true })
  step = 'production_assets'
  const response = await page.goto(webBase)
  assert.equal(response.status(), 200)
  assert.ok(response.headers()['content-security-policy'].includes("script-src 'self'"))
  await page.getByRole('heading', { name: '欢迎回来' }).waitFor()
  await page.screenshot({ path: '../.cache/web-visual/login.png', fullPage: true })
  step = 'login'
  await page.getByLabel('管理员账号').fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  const login = await clickResponse(page.getByRole('button', { name: '登录控制台' }), '/auth/login')
  token = login.token
  await page.getByRole('heading', { name: '用户管理', exact: true }).waitFor()
  step = 'create_member'
  await page.getByRole('button', { name: '创建用户' }).click()
  await dialog().getByLabel('名称', { exact: true }).fill(memberName)
  memberID = (
    await clickResponse(dialog().getByRole('button', { name: '创建', exact: true }), '/members')
  ).id
  await page.getByText(memberName, { exact: true }).waitFor()
  step = 'create_group'
  await page.getByRole('link', { name: '用户分组', exact: true }).click()
  const models = await api('/models?limit=100')
  const model = models.items.find((item) => item.code === 'deepseek-v4-flash')
  modelID = model.id
  await page.getByRole('button', { name: '创建分组' }).click()
  await dialog().getByLabel('名称', { exact: true }).fill(groupName)
  await dialog()
    .getByRole('checkbox', { name: `授权 ${model.name}`, exact: true })
    .check()
  const createdGroup = await clickResponse(
    dialog().getByRole('button', { name: '创建', exact: true }),
    '/groups',
  )
  groupID = createdGroup.id
  assert.equal(createdGroup.code, `group-${groupID}`)
  step = 'assign_model'
  await page
    .getByRole('row')
    .filter({ hasText: groupName })
    .getByRole('button', { name: '编辑', exact: true })
    .click()
  assert.equal(
    await dialog()
      .getByRole('checkbox', { name: `授权 ${model.name}`, exact: true })
      .isChecked(),
    true,
  )
  await dialog().getByRole('button', { name: '关闭', exact: true }).click()
  assert.ok((await api(`/groups/${groupID}/models`)).items.some((x) => x.id === modelID))
  step = 'resource_connection'
  const resources = await api('/resources?limit=100')
  const resource = resources.items.find((item) => item.name === 'DeepSeek 开发资源')
  const providers = await api('/providers?limit=100')
  const provider = providers.items.find((item) => item.id === resource.providerId)
  await page.getByRole('link', { name: '服务商', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: provider.name })
    .getByRole('button', { name: `测试 ${provider.name} 的连接`, exact: true })
    .click()
  await dialog().getByText('连接测试通过', { exact: true }).waitFor({ timeout: 25000 })
  await dialog().getByRole('button', { name: '关闭', exact: true }).last().click()
  step = 'issue_revoke_key'
  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: memberName })
    .getByRole('button', { name: '密钥', exact: true })
    .click()
  await dialog().getByLabel('Key 名称').fill('Web 验收临时 Key')
  const until = new Date(Date.now() + 15 * 60000)
  await dialog()
    .getByLabel('到期时间（可选）')
    .fill(new Date(until.getTime() - until.getTimezoneOffset() * 60000).toISOString().slice(0, 16))
  keyID = (
    await clickResponse(
      dialog().getByRole('button', { name: '密钥', exact: true }),
      `/members/${memberID}/keys`,
    )
  ).id
  const output = page.getByRole('textbox', { name: '完整 Key 仅展示这一次' })
  await output.waitFor()
  assert.ok((await output.inputValue()).startsWith('vk-'))
  await dialog().getByRole('button', { name: '我已保存，关闭' }).click()
  assert.equal(await output.count(), 0)
  await page
    .getByRole('row')
    .filter({ hasText: memberName })
    .getByRole('button', { name: '查看 Key 记录', exact: true })
    .click()
  await dialog().getByRole('button', { name: '撤销', exact: true }).click()
  await dialog().getByRole('button', { name: '撤销', exact: true }).click()
  await page.locator('.toast-success').filter({ hasText: 'Key 已撤销' }).waitFor()
  await dialog().getByRole('button', { name: '关闭', exact: true }).click()
  assert.ok((await api(`/members/${memberID}/keys`)).items[0].revokedAt)
  step = 'usage_and_logs'
  await page.getByRole('link', { name: '用量记录' }).click()
  await page.getByRole('button', { name: '详情', exact: true }).first().waitFor()
  await page.screenshot({ path: '../.cache/web-visual/usage.png', fullPage: true })
  await page.getByRole('combobox', { name: '用户', exact: true }).selectOption(memberID)
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await page.getByText('当前时间范围内暂无调用记录。可调整筛选条件后重试。').waitFor()
  await page.getByRole('link', { name: '操作日志', exact: true }).click()
  await page.getByRole('button', { name: '详情', exact: true }).first().waitFor()
  assert.deepEqual(await page.evaluate(() => [localStorage.length, sessionStorage.length]), [1, 0])
  assert.equal(errors.length, 0)
  report.push({ check: 'real_browser_go_postgres_flow', passed: true })
} catch {
  failed = true
  report.push({ check: step, passed: false })
} finally {
  for (const [path, method] of [
    [keyID ? `/access-keys/${keyID}/revoke` : '', 'POST'],
    [groupID ? `/groups/${groupID}` : '', 'DELETE'],
  ]) {
    if (path)
      try {
        await api(path, method)
      } catch {
        failed = true
        report.push({ check: 'cleanup', passed: false })
      }
  }
  if (memberID) {
    const response = await context.request.patch(`${apiBase}/api/v1/members/${memberID}/status`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { status: 'DISABLED' },
    })
    if (!response.ok()) {
      failed = true
      report.push({ check: 'disable_test_member', passed: false })
    }
  }
  report.push({ check: 'cleanup_finished', passed: !failed })
  await browser.close()
  await writeFile(
    '../.cache/web-live-results.json',
    JSON.stringify({ at: new Date().toISOString(), report }, null, 2),
  )
  console.log(JSON.stringify(report))
  if (failed) process.exitCode = 1
}
