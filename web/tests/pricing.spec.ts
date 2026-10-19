import { test, expect } from '@playwright/test'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('凭证计费', () => {
  test('凭证内可按生效时间保留 API Key 单价历史并修改订阅费用', async ({ page }) => {
    const state = await fixture(page)
    state.resources.push(
      {
        id: '88',
        providerId: '81',
        name: 'DeepSeek API Key',
        authType: 'API_KEY',
        authAdapter: 'API_KEY',
        runtimeStatus: 'HEALTHY',
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      },
      {
        id: '89',
        providerId: '81',
        name: 'DeepSeek 订阅',
        authType: 'SUBSCRIPTION',
        authAdapter: 'OPENAI_CODEX',
        runtimeStatus: 'HEALTHY',
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      },
    )
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'DeepSeek API Key' })
      .getByRole('button', { name: '计费设置' })
      .click()
    await expect(modal(page).getByRole('heading', { name: '凭证计费设置' })).toBeVisible()
    await modal(page).getByRole('button', { name: '配置价格' }).click()
    let priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
    await expect(priceDialog).toBeVisible()
    await priceDialog.getByRole('button', { name: '取消' }).click()
    await expect(priceDialog).toBeHidden()
    await expect(modal(page).getByRole('heading', { name: '凭证计费设置' })).toBeVisible()
    await modal(page).getByRole('button', { name: '配置价格' }).click()
    priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
    await expect(priceDialog.getByLabel('币种')).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '普通输入' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '输出' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '缓存输入' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '缓存写入（可选）' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '缓存写入（可选）' })).toHaveAttribute(
      'placeholder',
      '留空按普通输入单价',
    )
    await expect(priceDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
    await priceDialog.getByLabel('币种').selectOption('CNY')
    await priceDialog.getByRole('textbox', { name: '普通输入' }).fill('1.25')
    await priceDialog.getByRole('textbox', { name: '输出' }).fill('2.5')
    await priceDialog.getByRole('textbox', { name: '缓存输入' }).fill('0.5')
    await priceDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
    await priceDialog.getByRole('button', { name: '保存' }).click()
    await expect(priceDialog).toBeHidden()
    expect(state.credentialPrices.get('88')?.modelPrices[0].cacheCreationInputPrice).toBeNull()
    await expect(modal(page)).toContainText('¥ 1.25 / 2.5 / 0.5 / -')
    await modal(page).getByRole('button', { name: '新增价格' }).click()
    priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
    await expect(priceDialog.getByLabel('币种')).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '普通输入' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '输出' })).toHaveValue('')
    await expect(priceDialog.getByRole('textbox', { name: '缓存输入' })).toHaveValue('')
    await expect(priceDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
    await priceDialog.getByLabel('币种').selectOption('CNY')
    await priceDialog.getByRole('textbox', { name: '普通输入' }).fill('1.25')
    await priceDialog.getByRole('textbox', { name: '输出' }).fill('3')
    await priceDialog.getByRole('textbox', { name: '缓存输入' }).fill('0.5')
    await priceDialog.getByRole('textbox', { name: '缓存写入（可选）' }).fill('1.5.0')
    await priceDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
    await priceDialog.getByRole('button', { name: '保存' }).click()
    await expect(priceDialog.getByRole('alert')).toHaveText(
      '请输入非负金额，最多 12 位整数和 8 位小数。',
    )
    await priceDialog.getByRole('textbox', { name: '缓存写入（可选）' }).fill('1.5625')
    await priceDialog.getByRole('button', { name: '保存' }).click()
    await expect(priceDialog).toBeHidden()
    expect(state.credentialPrices.get('88')?.modelPrices[0].outputPrice).toBe('3')
    expect(state.credentialPrices.get('88')?.modelPrices[0].cacheCreationInputPrice).toBe('1.5625')
    expect(state.credentialPrices.get('88')?.modelPrices).toHaveLength(2)
    await expect(modal(page).getByLabel('价格历史').locator('.price-history-item')).toHaveCount(2)
    await expect(modal(page).getByText('未配置', { exact: true })).toHaveCount(0)
    await expect(modal(page).getByLabel('价格历史')).not.toContainText('08:00')
    await expect(modal(page).getByText('¥ 1.25 / 2.5 / 0.5 / -', { exact: true })).toBeVisible()
    await expect(modal(page).getByText('¥ 1.25 / 3 / 0.5 / 1.5625').first()).toBeVisible()
    const olderPrice = modal(page).getByLabel('价格历史').locator('.price-history-item').nth(1)
    await olderPrice.getByRole('button', { name: '修改' }).click()
    priceDialog = page.getByRole('dialog', { name: '修改 DeepSeek V4 Flash 的价格' })
    await expect(priceDialog).toBeVisible()
    await expect(priceDialog.getByRole('textbox', { name: '缓存写入（可选）' })).toHaveValue('')
    await priceDialog.getByRole('textbox', { name: '输出' }).fill('2.75')
    await priceDialog.getByRole('button', { name: '保存' }).click()
    await expect(modal(page).getByText('¥ 1.25 / 2.75 / 0.5 / -', { exact: true })).toBeVisible()
    await modal(page)
      .getByLabel('价格历史')
      .locator('.price-history-item')
      .nth(1)
      .getByRole('button', { name: '删除' })
      .click()
    await page
      .getByRole('dialog', { name: '删除价格' })
      .getByRole('button', { name: '删除' })
      .click()
    await expect(modal(page).getByLabel('价格历史').locator('.price-history-item')).toHaveCount(1)

    await modal(page).getByRole('button', { name: '返回凭证' }).click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'DeepSeek 订阅' })
      .getByRole('button', { name: '计费设置' })
      .click()
    await expect(modal(page).getByText('价格列表', { exact: true })).toBeVisible()
    await expect(modal(page).locator('.price-subscription-empty')).toHaveText('-')
    await modal(page).getByRole('button', { name: '配置订阅费用' }).click()
    let subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
    await expect(subscriptionDialog).toBeVisible()
    await subscriptionDialog.getByRole('button', { name: '取消' }).click()
    await expect(subscriptionDialog).toBeHidden()
    await modal(page).getByRole('button', { name: '配置订阅费用' }).click()
    subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
    await expect(subscriptionDialog.getByLabel('计费周期起始日（UTC）')).toHaveCount(0)
    await expect(subscriptionDialog.getByLabel('币种')).toHaveValue('')
    await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('')
    await expect(subscriptionDialog.getByLabel('计费周期')).toHaveValue('')
    await expect(subscriptionDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
    await subscriptionDialog.getByLabel('币种').selectOption('CNY')
    await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('100')
    await subscriptionDialog.getByLabel('计费周期').selectOption('MONTH')
    await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
    await subscriptionDialog.getByRole('button', { name: '保存' }).click()
    await expect(subscriptionDialog).toBeHidden()
    expect(state.credentialPrices.get('89')?.subscriptionPrice.periodAmount).toBe('100')
    await modal(page).getByRole('button', { name: '新增价格' }).click()
    subscriptionDialog = page.getByRole('dialog', { name: '新增「DeepSeek 订阅」的订阅费用' })
    await expect(subscriptionDialog.getByLabel('币种')).toHaveValue('')
    await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('')
    await expect(subscriptionDialog.getByLabel('计费周期')).toHaveValue('')
    await expect(subscriptionDialog.getByLabel('生效日期（UTC）')).toHaveValue('')
    await subscriptionDialog.getByLabel('币种').selectOption('CNY')
    await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('100')
    await subscriptionDialog.getByLabel('计费周期').selectOption('MONTH')
    await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
    await subscriptionDialog.getByRole('button', { name: '保存' }).click()
    await expect(subscriptionDialog.getByRole('alert')).toHaveText(
      '该生效日期已存在，请选择其他日期。',
    )
    expect(state.credentialPrices.get('89')?.subscriptionPrices).toHaveLength(1)
    await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('120')
    await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
    await subscriptionDialog.getByRole('button', { name: '保存' }).click()
    expect(state.credentialPrices.get('89')?.subscriptionPrices).toHaveLength(2)
    const subscriptionHistory = modal(page).getByLabel('价格历史')
    await expect(subscriptionHistory.locator('.price-history-item')).toHaveCount(2)
    const olderSubscriptionPrice = subscriptionHistory.locator('.price-history-item').nth(1)
    await olderSubscriptionPrice.getByRole('button', { name: '修改' }).click()
    subscriptionDialog = page.getByRole('dialog', { name: '修改「DeepSeek 订阅」的订阅费用' })
    await expect(subscriptionDialog.getByRole('textbox', { name: '周期费用' })).toHaveValue('100')
    await subscriptionDialog.getByRole('textbox', { name: '周期费用' }).fill('110')
    await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-10-02')
    await subscriptionDialog.getByRole('button', { name: '保存' }).click()
    await expect(subscriptionDialog.getByRole('alert')).toHaveText(
      '该生效日期已存在，请选择其他日期。',
    )
    await subscriptionDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
    await subscriptionDialog.getByRole('button', { name: '保存' }).click()
    expect(state.credentialPrices.get('89')?.subscriptionPrice.periodAmount).toBe('120')
    await subscriptionHistory
      .locator('.price-history-item')
      .nth(1)
      .getByRole('button', { name: '删除' })
      .click()
    await page
      .getByRole('dialog', { name: '删除订阅费用' })
      .getByRole('button', { name: '删除' })
      .click()
    await expect(subscriptionHistory.locator('.price-history-item')).toHaveCount(1)
    await modal(page).getByRole('button', { name: '返回凭证' }).click()
    const subscriptionRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek 订阅' })
    await expect(subscriptionRow).toContainText('当前订阅费用 · ¥120 / 月')
    await expect(subscriptionRow).toContainText('生效日期（UTC） · 2026-10-02')
  })

  test('价格保存中外层计费弹窗不可关闭', async ({ page }) => {
    const state = await fixture(page)
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    let releaseSave = () => {}
    await page.route('**/api/v1/resources/88/prices/models/**', async (route) => {
      if (route.request().method() !== 'PUT') return route.fallback()
      await new Promise<void>((resolve) => {
        releaseSave = resolve
      })
      await route.fallback()
    })
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'DeepSeek API Key' })
      .getByRole('button', { name: '计费设置' })
      .click()
    const pricingDialog = page.getByRole('dialog', { name: '凭证计费设置' })
    await pricingDialog.getByRole('button', { name: '配置价格' }).click()
    const priceDialog = page.getByRole('dialog', { name: '新增 DeepSeek V4 Flash 的价格' })
    await priceDialog.getByLabel('币种').selectOption('USD')
    await priceDialog.getByRole('textbox', { name: '普通输入' }).fill('1')
    await priceDialog.getByRole('textbox', { name: '输出' }).fill('2')
    await priceDialog.getByRole('textbox', { name: '缓存输入' }).fill('0.5')
    await priceDialog.getByLabel('生效日期（UTC）').fill('2026-09-20')
    await priceDialog.getByRole('button', { name: '保存' }).click()
    await expect(
      pricingDialog.locator('.modal-head').getByRole('button', { name: '关闭' }),
    ).toBeDisabled()
    await page.keyboard.press('Escape')
    await expect(pricingDialog).toBeVisible()
    releaseSave()
    await expect(priceDialog).toBeHidden()
    await expect(
      pricingDialog.locator('.modal-head').getByRole('button', { name: '关闭' }),
    ).toBeEnabled()
  })

  test.describe('西半球时区', () => {
    test.use({ timezoneId: 'America/Los_Angeles' })

    test('价格生效日期按 UTC 自然日展示，不受浏览器时区影响', async ({ page }) => {
      const state = await fixture(page)
      state.resources.push({
        id: '88',
        providerId: '81',
        name: 'DeepSeek API Key',
        authType: 'API_KEY',
        authAdapter: 'API_KEY',
        runtimeStatus: 'HEALTHY',
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      })
      state.credentialPrices.set('88', {
        modelPrices: [
          {
            id: '901',
            providerCredentialId: '88',
            providerModelId: '92',
            currency: 'USD',
            inputPrice: '1',
            outputPrice: '2',
            cachedInputPrice: '0.5',
            effectiveAt: '2026-09-20T00:00:00Z',
            createdAt: stamp,
          },
        ],
        subscriptionPrices: [],
        subscriptionPrice: null,
      })
      await signIn(page)
      await page.getByRole('link', { name: '服务商', exact: true }).click()
      await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
      await modal(page)
        .getByRole('row')
        .filter({ hasText: 'DeepSeek API Key' })
        .getByRole('button', { name: '计费设置' })
        .click()
      await modal(page).getByRole('button', { name: '展开 DeepSeek V4 Flash 的价格' }).click()
      const history = modal(page).getByLabel('价格历史')
      await expect(history.locator('time')).toHaveText('2026年9月20日')
      await history.getByRole('button', { name: '修改' }).click()
      await expect(
        page
          .getByRole('dialog', { name: '修改 DeepSeek V4 Flash 的价格' })
          .getByLabel('生效日期（UTC）'),
      ).toHaveValue('2026-09-20')
    })
  })

  test('价格接口未更新时不误报模型映射缺失', async ({ page }) => {
    const state = await fixture(page)
    state.setPriceEndpointUnavailable(true)
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'DeepSeek API Key' })
      .getByRole('button', { name: '计费设置' })
      .click()

    await expect(modal(page)).toContainText('凭证仍然存在，但价格接口不可用')
    await expect(modal(page)).toContainText('DeepSeek V4 Flash')
    await expect(modal(page)).not.toContainText('该服务商尚未配置模型映射')
    await expect(modal(page).getByRole('button', { name: '配置价格' })).toBeDisabled()
  })

  test('价格历史较多时默认折叠并可独立展开', async ({ page }) => {
    const state = await fixture(page)
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.credentialPrices.set('88', {
      modelPrices: Array.from({ length: 6 }, (_, index) => ({
        id: String(900 + index),
        providerCredentialId: '88',
        providerModelId: '92',
        currency: 'CNY',
        inputPrice: '5',
        outputPrice: '4',
        cachedInputPrice: String(index + 1),
        effectiveAt: `2026-09-${String(index + 1).padStart(2, '0')}T00:00:00Z`,
        createdAt: stamp,
      })),
      subscriptionPrices: [],
      subscriptionPrice: null,
    })
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'DeepSeek API Key' })
      .getByRole('button', { name: '计费设置' })
      .click()

    await expect(modal(page).getByLabel('价格历史')).toHaveCount(0)
    await modal(page).getByRole('button', { name: '展开 DeepSeek V4 Flash 的价格' }).click()
    const history = modal(page).getByLabel('价格历史')
    await expect(history.locator('.price-history-item')).toHaveCount(3)
    await modal(page).getByRole('button', { name: '查看全部 6 条价格' }).click()
    await expect(history.locator('.price-history-item')).toHaveCount(6)
    await expect(history).toHaveCSS('max-height', '320px')
    await modal(page).getByRole('button', { name: '收起价格历史' }).click()
    await expect(history.locator('.price-history-item')).toHaveCount(3)
  })
})
