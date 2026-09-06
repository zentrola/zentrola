// 仅针对显式提供的本地空库实例；不读取或修改已有管理员。
import { chromium } from '@playwright/test'
import { randomBytes } from 'node:crypto'
import { mkdir, writeFile } from 'node:fs/promises'
import assert from 'node:assert/strict'
const base = process.env.ZENTROLA_SETUP_WEB_URL
if (!base || !['127.0.0.1', 'localhost'].includes(new URL(base).hostname))
  throw new Error('Explicit local setup test URL required')
const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
const page = await context.newPage(),
  errors = []
page.on('pageerror', () => errors.push('runtime_error'))
page.on('console', (msg) => {
  if (msg.type() === 'error') errors.push('console_error')
})
const password = randomBytes(24).toString('base64'),
  username = 'setup-test-owner'
let step = 'initial_status',
  passed = false
try {
  const initial = await context.request.get(`${base}/api/v1/auth/setup`)
  assert.equal(initial.status(), 200)
  assert.equal((await initial.json()).data.required, true)
  assert.equal((await context.request.get(`${base}/health/ready`)).status(), 503)
  await page.goto(base)
  await page.getByRole('heading', { name: '初始化管理员' }).waitFor()
  await mkdir('../.cache/web-visual', { recursive: true })
  await page.screenshot({ path: '../.cache/web-visual/setup.png', fullPage: true })
  step = 'create_first_admin'
  await page.getByLabel('管理员账号').fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByLabel('确认密码').fill(password)
  await page.getByRole('button', { name: '创建管理员', exact: true }).click()
  await page.getByText('管理员已创建，请使用刚设置的账号和密码登录。').waitFor()
  assert.equal(await page.getByLabel('密码', { exact: true }).inputValue(), '')
  step = 'login'
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '登录控制台' }).click()
  await page.getByRole('heading', { name: '成员', exact: true }).waitFor()
  assert.equal((await context.request.get(`${base}/health/ready`)).status(), 200)
  step = 'closed_setup'
  const status = await context.request.get(`${base}/api/v1/auth/setup`)
  assert.equal((await status.json()).data.required, false)
  const repeated = await context.request.post(`${base}/api/v1/auth/setup`, {
    data: { username, password },
  })
  assert.equal(repeated.status(), 409)
  assert.equal((await repeated.json()).code, 'ALREADY_INITIALIZED')
  await page.reload()
  await page.getByRole('heading', { name: '欢迎回来' }).waitFor()
  assert.equal(await page.getByRole('button', { name: '创建管理员', exact: true }).count(), 0)
  assert.equal(errors.length, 0)
  passed = true
} catch {
  process.exitCode = 1
} finally {
  await browser.close()
  const result = { passed, step: passed ? 'complete' : step }
  await writeFile('../.cache/setup-live-results.json', JSON.stringify(result))
  console.log(JSON.stringify(result))
}
