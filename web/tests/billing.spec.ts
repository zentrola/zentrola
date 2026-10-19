import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { longID, stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn } from './mock/helpers'

test.describe('成本管理', () => {
  test('成本管理日期允许同一天并标注 UTC，校验文案与规则一致', async ({ page }) => {
    await page.clock.install({ time: new Date('2026-10-07T08:00:00Z') })
    await fixture(page)
    await signIn(page, 'home')
    await page.getByRole('link', { name: '成本管理', exact: true }).click()
    const field = page.locator('.date-range-field')
    await expect(field.locator('.utc-badge')).toHaveText('UTC')
    const trigger = page.getByRole('button', { name: '选择起止日期', exact: true })
    await expect(trigger).toHaveAccessibleDescription('日期按 UTC 自然日计算')
    await trigger.click()
    const dialog = page.getByRole('dialog', { name: '选择起止日期' })
    await dialog.getByRole('button', { name: '选择 2026-10-05' }).first().click()
    await dialog.getByRole('button', { name: '选择 2026-10-05' }).first().click()
    await expect(dialog).toHaveCount(0)
    await expect(trigger).toContainText('2026/10/05 — 2026/10/05')

    await trigger.click()
    for (let index = 0; index < 13; index += 1)
      await dialog.getByRole('button', { name: '上个月', exact: true }).click()
    await dialog.getByRole('button', { name: '选择 2025-09-01' }).first().click()
    for (let index = 0; index < 13; index += 1)
      await dialog.getByRole('button', { name: '下个月', exact: true }).click()
    await dialog.getByRole('button', { name: '选择 2026-10-05' }).first().click()
    await expect(dialog.getByRole('alert')).toHaveText(
      '请选择有效的起止日期，且范围不超过 366 天。',
    )
  })

  test('成本管理聚焦成本概览和计费明细', async ({ page }) => {
    await page.clock.install({ time: new Date('2026-10-07T08:00:00Z') })
    const state = await fixture(page)
    state.resources.push({
      id: '89',
      providerId: '81',
      name: 'DeepSeek 订阅',
      authType: 'SUBSCRIPTION',
      authAdapter: 'OPENAI_CODEX',
      subscriptionType: 'PERSONAL',
      effectiveAt: '2026-10-01T00:00:00Z',
      expiresAt: null,
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.credentialPrices.set('89', {
      modelPrices: [],
      subscriptionPrices: [
        {
          id: '930',
          providerCredentialId: '89',
          currency: 'CNY',
          periodAmount: '1000.00000000',
          billingPeriod: 'MONTH',
          effectiveAt: '2026-10-01T00:00:00Z',
          createdAt: stamp,
        },
      ],
      subscriptionPrice: {
        id: '930',
        providerCredentialId: '89',
        currency: 'CNY',
        periodAmount: '1000.00000000',
        billingPeriod: 'MONTH',
        effectiveAt: '2026-10-01T00:00:00Z',
        createdAt: stamp,
      },
    })
    await signIn(page)

    await page.getByRole('link', { name: '成本管理', exact: true }).click()
    await expect(page.getByRole('heading', { name: '成本管理', exact: true })).toBeVisible()
    await expect(page.locator('.billing-tabs').getByRole('tab')).toHaveText([
      '成本概览',
      '计费明细',
    ])
    const overviewDateRange = page.getByRole('button', { name: '选择起止日期', exact: true })
    await expect(overviewDateRange).toContainText('2026/10/01 — 2026/10/07')
    await expect(page.getByText('当月实时口径', { exact: true })).toHaveCount(0)
    const ledger = page.locator('.cost-ledger')
    await expect(ledger.getByText('CNY', { exact: true })).toBeVisible()
    await expect(ledger.getByText('USD', { exact: true })).toBeVisible()
    await expect(ledger.getByText('¥1,000', { exact: true })).toBeVisible()
    await expect(ledger.getByText('$0.008', { exact: true })).toBeVisible()
    await expect(ledger.getByText('1 项有效订阅', { exact: true })).toBeVisible()
    const attribution = page.locator('.cost-attribution-panel')
    await expect(attribution.getByRole('heading', { name: '费用分摊', exact: true })).toBeVisible()
    await expect(attribution.getByText('预计分摊', { exact: true })).toHaveCount(0)
    await expect(attribution.getByText('林知远', { exact: true })).toHaveCount(2)
    await expect(attribution.getByText('¥1,000', { exact: true })).toBeVisible()
    await expect(attribution.getByText('$0.008', { exact: true })).toBeVisible()
    await expect(page.getByText('当月订阅尚未结算，暂未生成最终成本归属。')).toHaveCount(0)
    await overviewDateRange.click()
    const overviewDateDialog = page.getByRole('dialog', { name: '选择起止日期' })
    await overviewDateDialog.getByRole('button', { name: '上个月' }).click()
    await overviewDateDialog.getByRole('button', { name: '选择 2026-09-10' }).click()
    await overviewDateDialog.getByRole('button', { name: '选择 2026-09-20' }).click()
    await expect(overviewDateRange).toContainText('2026/09/10 — 2026/09/20')
    const overviewRequest = page.waitForRequest((request) =>
      request.url().includes('/billing/statistics?'),
    )
    await page.getByRole('button', { name: '查询', exact: true }).click()
    const overviewSearch = new URL((await overviewRequest).url()).searchParams
    expect(overviewSearch.get('from')).toBe('2026-09-10T00:00:00.000Z')
    expect(overviewSearch.get('to')).toBe('2026-09-21T00:00:00.000Z')
    await expect(page.getByText('已结算口径', { exact: true })).toBeVisible()
    await expect(ledger.getByText('¥710', { exact: true })).toBeVisible()
    await expect(ledger.getByText('$5.125', { exact: true })).toBeVisible()
    await expect(ledger.getByText('¥715.125')).toHaveCount(0)
    await expect(page.getByRole('heading', { name: '用量排行', exact: true })).toBeVisible()
    const usageRanking = page.locator('.cost-usage-ranking')
    await expect(usageRanking).toContainText('林知远')
    await expect(usageRanking.locator('.cost-rank-marker.rank-1')).toHaveText('1')
    await expect(usageRanking.getByText('最高用量', { exact: true })).toBeVisible()

    await page.getByRole('tab', { name: '计费明细', exact: true }).click()
    await expect(page.locator('.billing-detail-metrics')).toHaveCount(0)
    const usageTable = page.locator('.usage-cost-table')
    await expect(usageTable.getByText('按 Token 调用计费', { exact: true })).toBeVisible()
    await expect(usageTable.getByText('个人订阅分摊', { exact: true })).toBeVisible()

    const costPanel = page.locator('.usage-cost-panel')
    const principalFilter = costPanel.getByRole('group', { name: '调用方' })
    const callerType = principalFilter.getByLabel('调用方类型')
    const callerName = principalFilter.getByRole('combobox', { name: '用户' })
    await expect(callerType).toBeVisible()
    await expect(callerName).toBeDisabled()
    await expect(costPanel.getByLabel('模型')).toBeVisible()
    await expect(costPanel.getByLabel('核算状态')).toHaveCount(0)
    await expect(costPanel.getByLabel('计费方式')).toBeVisible()
    await expect(costPanel.getByLabel('用户分组')).toHaveCount(0)
    await expect(costPanel.getByLabel('服务商')).toHaveCount(0)
    await expect(costPanel.getByRole('button', { name: /更多筛选/ })).toHaveCount(0)
    const principalTypeBounds = await callerType.boundingBox()
    const principalNameBounds = await callerName.boundingBox()
    expect(principalNameBounds!.x).toBeGreaterThan(principalTypeBounds!.x)
    expect(Math.abs(principalNameBounds!.y - principalTypeBounds!.y)).toBeLessThan(2)
    for (const label of ['模型', '计费方式']) {
      const field = costPanel.locator('label').filter({ hasText: label })
      const labelBounds = await field.locator('.filter-label').boundingBox()
      const controlBounds = await field.locator('select').boundingBox()
      expect(controlBounds!.x).toBeGreaterThan(labelBounds!.x)
      expect(Math.abs(controlBounds!.y - labelBounds!.y)).toBeLessThan(20)
    }
    const costDateRange = costPanel.getByRole('button', { name: '选择起止日期', exact: true })
    await expect(costDateRange).toBeVisible()
    await costDateRange.click()
    const costDateDialog = costPanel.getByRole('dialog', { name: '选择起止日期' })
    await costDateDialog.getByRole('button', { name: '选择 2026-10-02' }).click()
    await costDateDialog.getByRole('button', { name: '选择 2026-10-05' }).click()
    await expect(costDateRange).toContainText('2026/10/02 — 2026/10/05')
    const costRequest = page.waitForRequest(
      (request) =>
        request.url().includes('/billing/usage-costs?') &&
        new URL(request.url()).searchParams.get('modelId') === '71',
    )
    await callerType.selectOption('APPLICATION')
    const applicationName = principalFilter.getByRole('combobox', { name: '应用' })
    await applicationName.fill('报表')
    await principalFilter.getByRole('option', { name: '报表应用' }).click()
    await expect(applicationName).toHaveValue('报表应用')
    expect(state.applicationSuggestionQueries.at(-1)?.get('name')).toBe('报表')
    const applicationCostRequest = page.waitForRequest(
      (request) =>
        request.url().includes('/billing/usage-costs?') &&
        new URL(request.url()).searchParams.get('principalType') === 'APPLICATION',
    )
    await costPanel.getByRole('button', { name: '查询', exact: true }).click()
    expect(new URL((await applicationCostRequest).url()).searchParams.get('principalId')).toBe(
      '90071992547409944',
    )
    await callerType.selectOption('MEMBER')
    await expect(callerName).toHaveValue('')
    await callerName.fill('林知')
    await principalFilter.getByRole('option', { name: '林知远' }).click()
    await expect(callerName).toHaveValue('林知远')
    expect(state.memberSuggestionQueries.at(-1)?.get('name')).toBe('林知')
    await costPanel.getByLabel('模型').selectOption('71')
    await costPanel.getByLabel('计费方式').selectOption('API_KEY')
    const billingTypeBounds = await costPanel.getByLabel('计费方式').boundingBox()
    const searchButtonBounds = await costPanel
      .getByRole('button', { name: '查询', exact: true })
      .boundingBox()
    expect(Math.abs(searchButtonBounds!.y - billingTypeBounds!.y)).toBeLessThan(4)
    await mkdir('../.cache/web-visual', { recursive: true })
    await costPanel.screenshot({ path: '../.cache/web-visual/usage-cost-filters.png' })
    const costDesktopViewport = page.viewportSize()!
    await page.setViewportSize({ width: 390, height: 844 })
    await costPanel.screenshot({ path: '../.cache/web-visual/usage-cost-filters-mobile.png' })
    await expect
      .poll(() => costPanel.evaluate((panel) => panel.scrollWidth <= panel.clientWidth + 1))
      .toBe(true)
    const mobileTypeBounds = await callerType.boundingBox()
    const mobileNameBounds = await callerName.boundingBox()
    expect(mobileNameBounds!.x).toBeGreaterThan(mobileTypeBounds!.x)
    expect(Math.abs(mobileNameBounds!.y - mobileTypeBounds!.y)).toBeLessThan(2)
    await page.setViewportSize(costDesktopViewport)
    await costPanel.getByRole('button', { name: '查询', exact: true }).click()
    const searched = new URL((await costRequest).url()).searchParams
    expect(Object.fromEntries(searched)).toMatchObject({
      from: '2026-10-02T00:00:00.000Z',
      to: '2026-10-06T00:00:00.000Z',
      principalType: 'MEMBER',
      principalId: longID,
      modelId: '71',
      billingType: 'API_KEY',
    })
    for (const removed of [
      'groupId',
      'providerId',
      'resourceId',
      'clientProtocol',
      'status',
      'ratingStatus',
      'currency',
    ])
      expect(searched.has(removed)).toBe(false)

    await page.getByRole('button', { name: '查看详情' }).first().click()
    const usageDetail = page.getByRole('dialog', { name: '详情', exact: true })
    await expect(usageDetail.locator('.billing-detail-grid > div')).toHaveCount(12)
    await expect(
      usageDetail.locator('.billing-detail-grid > div').filter({ hasText: '缓存写入 Token' }),
    ).toContainText('-')
    const detailColumns = await usageDetail
      .locator('.billing-detail-grid > div')
      .evaluateAll((fields) => fields.map((field) => field.getBoundingClientRect().left))
    expect(Math.max(...detailColumns) - Math.min(...detailColumns)).toBeGreaterThan(350)
    await expect(usageDetail.locator('tbody tr').nth(0).locator('td').first()).toHaveText('1')
    await expect(usageDetail.locator('tbody tr').nth(1).locator('td').first()).toHaveText('2')
    await expect(usageDetail.getByText('$0.0036', { exact: true })).toBeVisible()
    await expect(usageDetail.getByText('$0.0042', { exact: true })).toBeVisible()
    await expect(usageDetail.getByText('$0.0079', { exact: true })).toBeVisible()
    await expect(
      usageDetail.getByRole('columnheader', { name: '缓存写入单价/百万 Token' }),
    ).toBeVisible()
    await expect(usageDetail.getByRole('columnheader', { name: '缓存写入成本' })).toBeVisible()
    await expect(usageDetail.getByText('$0.00000375', { exact: true })).toBeVisible()
    await expect(
      usageDetail.locator('tbody tr').nth(0).getByTitle('未配置缓存写入单价，按输入单价计费'),
    ).toHaveText('-')
    await usageDetail.getByRole('button', { name: '关闭' }).click()

    await page.getByRole('button', { name: '查看详情' }).nth(1).click()
    const subscriptionDetail = page.getByRole('dialog', { name: '详情', exact: true })
    await expect(subscriptionDetail).toContainText('个人订阅按账期分摊')
    await expect(subscriptionDetail).toContainText('¥1,000')
    await expect(subscriptionDetail).toContainText('¥700')
    // 账期左闭右开，结束端展示最后一个包含日而不是 periodEnd 当天。
    await expect(
      subscriptionDetail
        .locator('.subscription-cost-note dl > div')
        .filter({ hasText: '归属账期' }),
    ).toContainText('2026年9月1日 – 2026年9月30日')
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({ path: '../.cache/web-visual/usage-cost-detail.png', fullPage: true })
    const desktopViewport = page.viewportSize()!
    await page.setViewportSize({ width: 390, height: 844 })
    expect(
      await subscriptionDetail.evaluate((dialog) => dialog.scrollWidth <= dialog.clientWidth + 1),
    ).toBe(true)
    await page.screenshot({
      path: '../.cache/web-visual/usage-cost-detail-mobile.png',
      fullPage: true,
    })
    await page.setViewportSize(desktopViewport)
    await subscriptionDetail.getByRole('button', { name: '关闭' }).click()

    const exportRequest = page.waitForRequest((request) =>
      request.url().includes('/billing/usage-costs/export?'),
    )
    const downloadPromise = page.waitForEvent('download')
    await costPanel.getByRole('button', { name: '导出 CSV', exact: true }).click()
    const exportDownload = await downloadPromise
    expect(exportDownload.suggestedFilename()).toContain('zentrola-usage-costs-')
    const exported = new URL((await exportRequest).url()).searchParams
    expect((await exportRequest).headers()['accept-language']).toBe(
      await page.locator('html').getAttribute('lang'),
    )
    expect(Object.fromEntries(exported)).toMatchObject({
      principalType: 'MEMBER',
      principalId: longID,
      modelId: '71',
      billingType: 'API_KEY',
    })
    for (const removed of [
      'groupId',
      'providerId',
      'resourceId',
      'clientProtocol',
      'status',
      'ratingStatus',
      'currency',
    ])
      expect(exported.has(removed)).toBe(false)
  })
})
