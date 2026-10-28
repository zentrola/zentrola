import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('操作日志', () => {
  test('操作日志详情展示原始 JSON、差异高亮和追踪信息', async ({ page }) => {
    await fixture(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await signIn(page, 'home')
    await page.getByRole('link', { name: '操作日志', exact: true }).click()
    await expect(
      page.getByRole('searchbox', { name: '请输入操作人、对象或请求 ID' }),
    ).toHaveAttribute('placeholder', '请输入操作人、对象或请求 ID')

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
    await page.screenshot({
      path: '../.cache/web-visual/operation-diff-mobile.png',
      fullPage: true,
    })
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
    await page.screenshot({
      path: '../.cache/web-visual/operation-log-content.png',
      fullPage: true,
    })
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

    const loginRow = page.getByRole('row').filter({ hasText: '登录失败' })
    await loginRow.getByRole('button', { name: '详情', exact: true }).click()
    await expect(modal(page).locator('.json-snapshot')).toHaveCount(0)
    await expect(modal(page)).not.toContainText('failedLoginCount')
    await expect(modal(page)).not.toContainText('lockedUntil')
    await expect(modal(page)).toContainText('账号或密码错误')
    await expect(modal(page).getByText('req_login_failed', { exact: true })).toBeVisible()
    await page.screenshot({
      path: '../.cache/web-visual/operation-login-failed.png',
      fullPage: true,
    })
  })

  test('操作日志按关键词、类型和结果在服务端筛选并保持游标分页', async ({ page }) => {
    const state = await fixture(page)
    for (let index = 0; index < 25; index++) {
      state.operations.push({
        id: String(100 + index),
        operatorName: 'auditor',
        type: index % 2 ? 'MEMBER_UPDATE' : 'MODEL_UPDATE',
        targetType: index % 2 ? 'PRINCIPAL' : 'MODEL',
        targetId: String(500 + index),
        targetName: `批量对象 ${index}`,
        requestId: `req_bulk_${index}`,
        result: index % 5 ? 'SUCCESS' : 'FAILED',
        errorCode: index % 5 ? null : 'CONFLICT',
        before: null,
        after: { index },
        createdAt: stamp,
      })
    }
    await signIn(page, 'home')
    await page.getByRole('link', { name: '操作日志', exact: true }).click()
    await expect(page.getByText('共 28 条')).toBeVisible()
    const searchbox = page.getByRole('searchbox', { name: '请输入操作人、对象或请求 ID' })
    await expect(searchbox).toHaveAttribute('maxlength', '100')

    await searchbox.fill('  REQ_PROVIDER  ')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.locator('tbody tr')).toHaveCount(1)
    await expect(page.locator('tbody tr').first()).toContainText('服务商状态变更')
    expect(state.operationQueries.at(-1)!.get('keyword')).toBe('REQ_PROVIDER')

    await searchbox.fill('')
    await page.getByRole('combobox', { name: '操作类型' }).selectOption('MEMBER_UPDATE')
    await page.getByRole('combobox', { name: '结果' }).selectOption('SUCCESS')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.getByText('共 10 条')).toBeVisible()
    await expect(page.locator('tbody tr')).toHaveCount(10)
    const filteredQuery = state.operationQueries.at(-1)!
    expect(filteredQuery.get('type')).toBe('MEMBER_UPDATE')
    expect(filteredQuery.get('result')).toBe('SUCCESS')
    expect(filteredQuery.has('keyword')).toBe(false)

    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(page.getByText('共 28 条')).toBeVisible()
    await expect(page.locator('tbody tr')).toHaveCount(20)
    await page.getByRole('combobox', { name: '结果' }).selectOption('FAILED')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.getByText('共 6 条')).toBeVisible()
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(page.getByRole('combobox', { name: '结果' })).toHaveValue('')
    await page.getByRole('button', { name: '下一页', exact: true }).click()
    await expect(page.locator('tbody tr')).toHaveCount(8)
    const nextPageQuery = state.operationQueries.at(-1)!
    expect(nextPageQuery.get('after')).toBe('108')
    expect(nextPageQuery.get('limit')).toBe('20')
    expect(state.operationQueries.every((query) => query.get('limit') === '20')).toBe(true)

    await page.getByRole('combobox', { name: '操作类型' }).selectOption('LOGIN_LOCKED')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.getByText('没有匹配的记录')).toBeVisible()
  })

  test('操作日志类型筛选包含删除凭证且历史类型仍可展示', async ({ page }) => {
    const state = await fixture(page)
    const credentialOperation = (id: string, type: string, requestId: string) => ({
      id,
      operatorName: 'auditor',
      type,
      targetType: 'RESOURCE',
      targetId: '601',
      targetName: '测试凭证',
      requestId,
      result: 'SUCCESS',
      errorCode: null,
      before: { status: 'ACTIVE' },
      after: null,
      createdAt: stamp,
    })
    state.operations.push(
      credentialOperation('200', 'RESOURCE_DELETE', 'req_resource_delete'),
      credentialOperation('201', 'RESOURCE_STATUS_CHANGE', 'req_resource_status'),
    )
    await signIn(page, 'home')
    await page.getByRole('link', { name: '操作日志', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: '服务商凭证状态变更' })).toHaveCount(1)
    await expect(page.getByRole('row').filter({ hasText: '删除服务商凭证' })).toHaveCount(1)

    const typeSelect = page.getByRole('combobox', { name: '操作类型' })
    await expect(typeSelect.locator('option[value="RESOURCE_DELETE"]')).toHaveText('删除服务商凭证')
    await expect(typeSelect.locator('option[value="RESOURCE_STATUS_CHANGE"]')).toHaveText(
      '服务商凭证状态变更',
    )
    await typeSelect.selectOption('RESOURCE_DELETE')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.locator('tbody tr')).toHaveCount(1)
    await expect(page.locator('tbody tr').first()).toContainText('删除服务商凭证')
    expect(state.operationQueries.at(-1)!.get('type')).toBe('RESOURCE_DELETE')
  })

  test('操作日志首屏加载中切换为查看全部时以最新筛选结果为准', async ({ page }) => {
    await fixture(page)
    let releaseApplicationOperations = () => {}
    const applicationOperationsGate = new Promise<void>((resolve) => {
      releaseApplicationOperations = resolve
    })
    await page.route('**/api/v1/applications/71/operations?**', async (route) => {
      await applicationOperationsGate
      await route
        .fulfill({
          json: {
            code: 'OK',
            data: {
              items: [
                {
                  id: '301',
                  operatorName: 'admin',
                  type: 'APPLICATION_UPDATE',
                  targetType: 'APPLICATION',
                  targetId: '71',
                  targetName: '迟到的应用记录',
                  requestId: 'req_late_application',
                  result: 'SUCCESS',
                  errorCode: null,
                  before: null,
                  after: null,
                  createdAt: stamp,
                },
              ],
              nextCursor: null,
              total: 1,
            },
          },
        })
        .catch(() => {})
    })
    await signIn(page, 'home')
    await page.goto('/#/operations?applicationId=71')
    await expect(page.getByText('正在查看该应用的操作记录。')).toBeVisible()
    await page.getByRole('link', { name: '查看全部', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: '服务商状态变更' })).toBeVisible()
    releaseApplicationOperations()
    await page.waitForTimeout(300)
    await expect(page.getByText('迟到的应用记录')).toHaveCount(0)
    await expect(page.getByRole('row').filter({ hasText: '服务商状态变更' })).toBeVisible()
  })
})
