import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { longID, stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('布局与综合流程', () => {
  test('状态 switch 直接生效且危险操作仍需确认', async ({ page }) => {
    await fixture(page)
    await signIn(page)
    await page.getByRole('button', { name: '界面语言', exact: true }).click()
    await page.getByRole('menuitemradio', { name: 'English', exact: true }).click()
    await page.getByRole('link', { name: 'Providers', exact: true }).click()
    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    const status = row.getByRole('switch', { name: 'Status for DeepSeek' })
    await status.click()
    await expect(status).not.toBeChecked()
    await expect(modal(page)).toHaveCount(0)
    await row.locator('summary', { hasText: '⋮' }).click()
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
    await expect(page.getByText(longID, { exact: true })).toBeVisible()
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
    await expect(
      modal(page).locator('.group-model-selected .group-model-pane-head > span'),
    ).toHaveText('1')
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
    await expect(modal(page).getByText('DeepSeek V4 Flash', { exact: true })).toHaveCount(2)
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
    const createdGroupRow = page.getByRole('row').filter({ hasText: '平台研发组' })
    await createdGroupRow.locator('summary').click()
    await createdGroupRow.getByRole('button', { name: '删除', exact: true }).click()
    await expect(modal(page)).toContainText('用户关联和模型授权将失效')
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: '平台研发组' })).toHaveCount(0)
    expect(state.relationships.has(`groups/${createdGroup.id}/models`)).toBe(false)

    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await expect(page.getByRole('link', { name: '服务商凭证', exact: true })).toHaveCount(0)
    const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await expect(providerRow.getByRole('link', { name: '在新页面打开 DeepSeek 官网' })).toHaveCount(
      0,
    )
    await providerRow
      .getByRole('button', {
        name: '管理 DeepSeek 的认证凭据',
        exact: true,
      })
      .click()
    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByLabel('API Key', { exact: true }).fill('fixture-upstream-credential')
    await modal(page).getByRole('button', { name: '保存' }).click()
    await expect(modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })).toBeVisible()
    await expect(page.locator('.toast-success')).toContainText('已保存')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    const testProviderConnection = providerRow.getByRole('button', {
      name: '测试 DeepSeek 的连接',
      exact: true,
    })
    await expect(testProviderConnection).toBeVisible()
    await expect(testProviderConnection).toHaveAttribute('title', '测试连接')
    await expect(testProviderConnection.locator('svg')).toBeVisible()
    const startAnthropicConnectionTest = async () => {
      const protocolDialog = page.getByRole('dialog', {
        name: 'DeepSeek / 选择测试协议',
      })
      await expect(protocolDialog).toBeVisible()
      await expect(protocolDialog.getByText('https://api.deepseek.com/anthropic')).toBeVisible()
      await expect(
        protocolDialog.getByText('https://api.deepseek.com', { exact: true }),
      ).toBeVisible()
      const anthropic = protocolDialog.getByRole('radio', {
        name: '使用 Anthropic 协议测试连接',
        exact: true,
      })
      await expect(anthropic).toBeChecked()
      const requestPromise = page.waitForRequest((request) => {
        const url = new URL(request.url())
        return request.method() === 'POST' && url.pathname.endsWith('/test-connection')
      })
      await protocolDialog.getByRole('button', { name: '开始测试', exact: true }).click()
      const request = await requestPromise
      expect(new URL(request.url()).searchParams.get('protocol')).toBe('ANTHROPIC')
    }
    await testProviderConnection.click()
    await startAnthropicConnectionTest()
    await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
    await expect(modal(page).getByText('Anthropic', { exact: true })).toBeVisible()
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    expect(state.modelSyncRequests).toHaveLength(0)
    await providerRow.getByRole('button', { name: '同步 DeepSeek 的官方模型', exact: true }).click()
    await expect(modal(page).getByRole('status')).toContainText('模型目录同步完成')
    await expect(modal(page).getByText('官方接口', { exact: true })).toBeVisible()
    expect(state.modelSyncRequests).toHaveLength(1)
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    const editCredential = providerRow.getByRole('button', {
      name: '管理 DeepSeek 的认证凭据',
    })
    await editCredential.click()
    let credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
    let testConnection = credentialRow.getByRole('button', {
      name: '验证 DeepSeek API Key 的可用性',
      exact: true,
    })
    await expect(testConnection).toBeVisible()
    expect(await page.content()).not.toContain('fixture-upstream-credential')
    await testConnection.click()
    await startAnthropicConnectionTest()
    await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    state.failTest()
    await editCredential.click()
    credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
    testConnection = credentialRow.getByRole('button', {
      name: '验证 DeepSeek API Key 的可用性',
      exact: true,
    })
    await testConnection.click()
    await startAnthropicConnectionTest()
    await expect(modal(page).getByRole('status')).toContainText('上游认证失败')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    await expect(editCredential).toHaveText('凭证')
    await expect(providerRow.getByRole('button', { name: '更新 API Key' })).toHaveCount(0)
    await editCredential.click()
    credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
    await expect(credentialRow.getByRole('cell').first()).toContainText('最后验证时间')
    await expect(credentialRow.getByRole('cell').first()).not.toContainText('—')
    await expect(modal(page).getByLabel('API Key', { exact: true })).toHaveCount(0)
    await expect(
      credentialRow.getByRole('button', {
        name: '验证 DeepSeek API Key 的可用性',
        exact: true,
      }),
    ).toBeVisible()
    await expect(credentialRow.getByRole('button', { name: '删除', exact: true })).toBeVisible()
    await credentialRow.getByRole('button', { name: '删除', exact: true }).click()
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(modal(page).getByText('暂无认证凭据', { exact: true })).toBeVisible()
    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByLabel('API Key', { exact: true }).fill('replacement-credential')
    await modal(page).getByRole('button', { name: '保存' }).click()
    await expect(modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })).toBeVisible()
    await expect(page.locator('.toast-success')).toContainText('已保存')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    expect(state.modelSyncRequests).toHaveLength(1)
    await expect(
      providerRow.getByRole('switch', {
        name: 'DeepSeek 服务商凭证的启用状态',
      }),
    ).toHaveCount(0)
    await expect(
      providerRow.getByRole('button', {
        name: '管理 DeepSeek 的认证凭据',
      }),
    ).toHaveText('凭证')

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
    await page.getByRole('link', { name: '用量分析' }).click()
    await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()
    await expect(page.getByRole('tab', { name: '统计排行', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('member')
    await page.getByRole('tab', { name: '模型请求', exact: true }).click()
    await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('model')
    await page.getByRole('tab', { name: '服务商调用', exact: true }).click()
    await expect.poll(() => state.statisticQueries.at(-1)?.get('dimension')).toBe('provider')
    const providerStatisticRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await expect(providerStatisticRow).toContainText('20')
    await page.screenshot({ path: 'test-results/visual/usage-statistics.png', fullPage: true })
    await providerStatisticRow.getByRole('button', { name: '查看明细', exact: true }).click()
    await expect.poll(() => state.usageQueries.at(-1)?.get('providerId')).toBe('81')
    const dateRangeButton = page.getByRole('button', { name: '选择起止日期', exact: true })
    await expect(dateRangeButton.locator('svg')).toHaveCount(1)
    const defaultDates = (await dateRangeButton.textContent())?.match(/\d{4}\/\d{2}\/\d{2}/g) || [],
      [defaultStart = '', defaultEnd = ''] = defaultDates
    expect(defaultDates).toHaveLength(2)
    expect(
      Date.parse(defaultEnd.replaceAll('/', '-')) - Date.parse(defaultStart.replaceAll('/', '-')),
    ).toBe(6 * 86400000)
    await expect
      .poll(() => {
        const initialUsageQuery = state.usageQueries.at(-1)
        return (
          Date.parse(initialUsageQuery?.get('to') || '') -
          Date.parse(initialUsageQuery?.get('from') || '')
        )
      })
      .toBe(7 * 86400000)
    const initialUsageQuery = state.usageQueries.at(-1)
    const defaultStartValue = defaultStart.replaceAll('/', '-')
    const defaultEndValue = defaultEnd.replaceAll('/', '-')
    expect(initialUsageQuery?.get('from')).toBe(`${defaultStartValue}T00:00:00.000Z`)
    expect(initialUsageQuery?.get('to')).toBe(
      new Date(Date.parse(`${defaultEndValue}T00:00:00Z`) + 86400000).toISOString(),
    )
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
    await expect(page.locator('.member-autocomplete > svg')).toHaveCount(0)
    expect(state.memberListQueries).toHaveLength(memberListRequestCount)
    await memberInput.fill('林知')
    await page.getByRole('option', { name: '林知远', exact: true }).click()
    await expect(memberInput).toHaveValue('林知远')
    expect(state.memberSuggestionQueries.at(-1)?.get('name')).toBe('林知')
    expect(state.memberSuggestionQueries.at(-1)?.get('limit')).toBe('8')
    await page.getByRole('combobox', { name: '模型', exact: true }).selectOption('71')
    await page.getByRole('combobox', { name: '服务商', exact: true }).selectOption('81')
    await expect(page.getByRole('combobox', { name: '服务商凭证', exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect.poll(() => state.usageQueries.at(-1)?.get('memberId')).toBe(longID)
    expect(state.usageQueries.at(-1)?.get('modelId')).toBe('71')
    expect(state.usageQueries.at(-1)?.get('providerId')).toBe('81')
    expect(state.usageQueries.at(-1)?.get('resourceId')).toBeNull()
    const usageRow = page.getByRole('row').filter({ hasText: 'OpenAI' })
    const protocolBadge = usageRow.locator('.protocol-label')
    await expect(protocolBadge).toHaveText('OpenAI Responses')
    const protocolBox = await protocolBadge.boundingBox()
    const statusBox = await usageRow.locator('.status').boundingBox()
    expect(protocolBox).not.toBeNull()
    expect(statusBox).not.toBeNull()
    expect(protocolBox!.x + protocolBox!.width).toBeLessThanOrEqual(statusBox!.x)
    await expect(usageRow.locator('td').nth(5)).toHaveText('0')
    await expect(usageRow.locator('td').nth(6)).toHaveText('-')
    await expect(usageRow.locator('td').nth(7)).toHaveText('0')
    await page.getByRole('button', { name: '详情', exact: true }).click()
    await expect(modal(page).getByText('req_live_compatible')).toBeVisible()
    await expect(modal(page)).toContainText('UPSTREAM_BILLING_BLOCKED')
    await expect(modal(page)).toContainText('上游余额不足、计费异常或订阅已过期。')
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
    await page.getByRole('button', { name: '打开导航' }).click()
    await page.getByRole('link', { name: '操作日志', exact: true }).click()
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
      sidebarWidth: 240,
      workspaceLeft: 240,
      navLabelDisplay: 'block',
      topbarHeight: 58,
      documentFits: true,
    })
    const memberTableScroll = page.locator('.members-table').locator('..')
    expect(
      await memberTableScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true)
    const modelLink = page.getByRole('link', { name: '模型', exact: true })
    await expect(modelLink).toBeVisible()
    await expect(modelLink.locator('span')).toBeVisible()
    await expect(modelLink).toHaveAttribute('data-label', '模型')

    await page.getByRole('link', { name: '用量分析', exact: true }).click()
    await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()
    await expect(page.locator('.usage-context')).toHaveCount(0)
    await expect(page.locator('.usage-list-panel > .usage-filters')).toHaveCount(1)
    await expect(page.locator('.usage-list-panel > .table-scroll')).toHaveCount(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({
      path: '../.cache/web-visual/responsive-14-inch.png',
      fullPage: true,
      animations: 'disabled',
    })
  })

  test('高密度表格在常用桌面分辨率保持稳定列宽和单行技术字段', async ({ page }) => {
    const pageErrors: string[] = []
    page.on('pageerror', (error) => pageErrors.push(error.message))
    await fixture(page)
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    for (const width of [1366, 1440, 1920]) {
      await page.setViewportSize({ width, height: 900 })
      await expect(page.getByRole('heading', { name: '服务商', exact: true })).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      )
      const scroll = page.locator('.providers-table').locator('..')
      expect(
        await scroll.evaluate((element) => element.scrollWidth > element.clientWidth + 1),
      ).toBe(width < 1920)
      const headers = page.locator('.providers-table th')
      expect((await headers.nth(3).boundingBox())!.width).toBeGreaterThanOrEqual(120)
      expect((await headers.nth(6).boundingBox())!.width).toBeGreaterThanOrEqual(170)
    }

    const managementPages = [
      { name: '用户管理', path: 'members' },
      { name: '用户分组', path: 'groups' },
      { name: '模型', path: 'models' },
      { name: '服务商', path: 'providers' },
    ]
    const idColumnWidths: Array<{ name: string; width: number }> = []
    for (const { name, path } of managementPages) {
      await page.getByRole('link', { name, exact: true }).click()
      await expect(page).toHaveURL(new RegExp(`#/${path}$`))
      const idHeader = page.getByRole('columnheader', { name: 'ID', exact: true }).first()
      await expect(idHeader).toBeVisible()
      idColumnWidths.push({
        name,
        width: Math.round((await idHeader.boundingBox())!.width),
      })
    }
    expect(idColumnWidths).toEqual(
      managementPages.map(({ name }) => ({
        name,
        width: 88,
      })),
    )

    const endpoint = page.locator('.providers-table .endpoint').first()
    await expect(endpoint).toHaveAttribute('title', /https:\/\//)
    await expect(endpoint.locator('.technical-value-text')).toHaveCSS('white-space', 'nowrap')
    await expect(endpoint.getByRole('button', { name: '复制', exact: true })).toBeVisible()

    await page.getByRole('link', { name: '用量分析', exact: true }).click()
    const statisticsScroll = page.locator('.statistics-table').locator('..')
    expect(
      await statisticsScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true)
    expect(
      (await page.locator('.statistics-table th').last().boundingBox())!.width,
    ).toBeGreaterThanOrEqual(100)

    await page.getByRole('link', { name: '操作日志', exact: true }).click()
    const operationHeaders = page.locator('.operations-table th')
    expect((await operationHeaders.nth(4).boundingBox())!.width).toBeGreaterThanOrEqual(108)
    expect((await operationHeaders.last().boundingBox())!.width).toBeGreaterThanOrEqual(88)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(pageErrors).toEqual([])
  })

  test('有操作列表固定首尾列，分组模型选择器按可用宽度改为上下结构', async ({ page }) => {
    await fixture(page)
    await page.setViewportSize({ width: 800, height: 768 })
    await signIn(page, 'home')
    await page.goto('/#/usage?view=records')
    await expect(page.getByRole('heading', { name: '用量分析', exact: true })).toBeVisible()

    const tableScroll = page.locator('.usage-list-panel > .table-scroll')
    await expect(tableScroll).toHaveClass(/is-overflowing/)
    await expect(tableScroll).toHaveClass(/table-scroll--actions/)
    await expect(tableScroll).toHaveClass(/is-at-start/)
    await expect(tableScroll).not.toHaveClass(/is-at-end/)

    const firstHeader = tableScroll.getByRole('columnheader').first()
    const actionHeader = tableScroll.getByRole('columnheader').last()
    const initialEdges = await Promise.all([
      tableScroll.boundingBox(),
      firstHeader.boundingBox(),
      actionHeader.boundingBox(),
    ])
    expect(initialEdges.every(Boolean)).toBe(true)
    expect(Math.abs(initialEdges[1]!.x - initialEdges[0]!.x)).toBeLessThanOrEqual(1)
    expect(
      Math.abs(
        initialEdges[2]!.x + initialEdges[2]!.width - (initialEdges[0]!.x + initialEdges[0]!.width),
      ),
    ).toBeLessThanOrEqual(1)
    await expect(actionHeader).not.toHaveCSS('box-shadow', 'none')

    await tableScroll.evaluate((element) => {
      element.scrollLeft = element.scrollWidth - element.clientWidth
    })
    await expect(tableScroll).toHaveClass(/is-at-end/)
    await expect(tableScroll).not.toHaveClass(/is-at-start/)
    await expect(firstHeader).not.toHaveCSS('box-shadow', 'none')
    await expect(actionHeader).toHaveCSS('box-shadow', 'none')

    await page.setViewportSize({ width: 480, height: 768 })
    await page.goto('/#/groups')
    await expect(page.getByRole('heading', { name: '用户分组', exact: true })).toBeVisible()
    await page.getByRole('button', { name: '创建分组', exact: true }).click()

    const selectionWorkspace = modal(page).locator('.group-model-workspace')
    const catalogPane = selectionWorkspace.locator('.group-model-catalog')
    const selectedPane = selectionWorkspace.locator('.group-model-selected')
    await expect(selectionWorkspace).toHaveCSS('grid-template-columns', /\S+/)
    const [catalogBox, selectedBox] = await Promise.all([
      catalogPane.boundingBox(),
      selectedPane.boundingBox(),
    ])
    expect(catalogBox).not.toBeNull()
    expect(selectedBox).not.toBeNull()
    expect(selectedBox!.y).toBeGreaterThan(catalogBox!.y + catalogBox!.height - 1)
    expect(
      await selectionWorkspace.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1,
      ),
    ).toBe(true)
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
    await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(240)
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
    await expect.poll(() => sidebar.evaluate((element) => element.clientWidth)).toBe(240)
    await expect
      .poll(() => workspace.evaluate((element) => element.getBoundingClientRect().left))
      .toBe(240)
    await expect(modelLabel).toBeVisible()
  })
})
