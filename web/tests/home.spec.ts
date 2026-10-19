import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('仪表盘', () => {
  test('仪表盘初始化向导依次高亮配置入口', async ({ page }) => {
    await fixture(page)
    await signIn(page, 'home')

    await page.getByRole('button', { name: '初始化向导', exact: true }).click()
    const guide = page.locator('.initialization-tour')
    await expect(guide).toBeVisible()
    await expect(guide.getByRole('heading', { name: '添加服务商' })).toBeVisible()
    await expect(guide).toHaveAttribute('data-target', 'provider')
    await expect(page.locator('#nav-providers')).toHaveClass(/initialization-tour-target/)
    await expect(page.locator('#nav-models')).not.toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'model')
    await expect(guide.getByRole('heading', { name: '添加模型' })).toBeVisible()
    await expect(page.locator('#nav-models')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'group')
    await expect(guide.getByRole('heading', { name: '创建用户分组' })).toBeVisible()
    await expect(page.locator('#nav-groups')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'member')
    await expect(guide.getByRole('heading', { name: '添加用户' })).toBeVisible()
    await expect(page.locator('#nav-members')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'usage')
    await expect(guide.getByRole('heading', { name: '查看用量分析' })).toBeVisible()
    await expect(guide.getByText('第 5 步，共 5 步', { exact: true })).toBeVisible()
    await expect(page.locator('#nav-usage')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '完成', exact: true }).click()
    await expect(guide).toHaveCount(0)
    await expect(page.locator('.initialization-tour-target')).toHaveCount(0)
  })

  test('首页展示本月指标、应用接入、配置脚本和分项排行榜', async ({ page }) => {
    const state = await fixture(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    state.resources.push({
      id: '88',
      name: 'DeepSeek Key',
      providerId: '81',
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      blockedReason: null,
      blockedAt: null,
      lastErrorAt: null,
      lastHttpStatus: null,
      lastErrorCode: null,
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    const dashboardRequest = page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith('/usage/dashboard'),
    )
    await signIn(page, 'home')

    const dashboardURL = new URL((await dashboardRequest).url())
    const expectedRange = await page.evaluate(() => {
      const now = new Date()
      const year = now.getUTCFullYear()
      const month = now.getUTCMonth()
      return {
        from: new Date(Date.UTC(year, month, 1)).toISOString(),
        to: new Date(Date.UTC(year, month + 1, 1)).toISOString(),
      }
    })
    expect(dashboardURL.searchParams.get('from')).toBe(expectedRange.from)
    expect(dashboardURL.searchParams.get('to')).toBe(expectedRange.to)

    await expect(page.locator('.token-total dd')).toHaveText('12.8K')
    await expect(page.locator('.token-total dd')).toHaveAttribute('title', '12,840')
    await expect(page.getByRole('heading', { name: '本月概览', exact: true })).toBeVisible()
    const expectedMonthLabel = await page.evaluate(() =>
      new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: 'long',
        timeZone: 'UTC',
      }).format(new Date()),
    )
    await expect(page.locator('.dashboard-section-head .dashboard-period')).toHaveText(
      expectedMonthLabel,
    )
    await expect(page.locator('.dashboard-period-group .utc-badge')).toHaveAttribute(
      'title',
      '日期按 UTC 自然日计算',
    )
    await expect(page.getByText('Token 消耗', { exact: true })).toBeVisible()
    await expect(
      page.locator('.provider-total').getByRole('img', { name: '已启用服务商均可用' }),
    ).toHaveClass(/is-healthy/)
    expect(
      await page.locator('.provider-metric-label').evaluate((element) => {
        const label = element.querySelector('dt')!.getBoundingClientRect()
        const badge = element.querySelector('.provider-health-badge')!.getBoundingClientRect()
        return { above: badge.top < label.top, right: badge.left > label.right }
      }),
    ).toEqual({ above: true, right: true })
    await expect(page.getByText('最新统计', { exact: true })).toHaveCount(0)
    await expect(page.locator('.provider-health-alert')).toHaveCount(0)
    await expect(
      page.getByText('激活用户数').locator('..').getByText('2', { exact: true }),
    ).toBeVisible()
    await expect(
      page.getByText('支持模型').locator('..').getByText('2', { exact: true }),
    ).toBeVisible()
    await expect(page.locator('.provider-total').getByText('1', { exact: true })).toBeVisible()
    await expect(page.getByText('掌握今日模型服务状态、Token 消耗与团队使用趋势。')).toHaveCount(0)
    await expect(page.getByText('当前已启用')).toHaveCount(0)
    await expect(page.getByText('输入 + 输出')).toHaveCount(0)
    expect(
      await page
        .locator('.metric-strip > div')
        .evaluateAll((elements) =>
          elements.every((element) => getComputedStyle(element).textAlign === 'center'),
        ),
    ).toBe(true)
    expect(
      await page
        .locator('.metric-strip')
        .evaluate(
          (element) =>
            getComputedStyle(element).gridTemplateColumns.split(' ').filter(Boolean).length,
        ),
    ).toBe(4)
    expect(
      await page
        .locator('.ranking-grid')
        .evaluate(
          (element) =>
            getComputedStyle(element).gridTemplateColumns.split(' ').filter(Boolean).length,
        ),
    ).toBe(3)
    await expect(page.getByText('用户 Token 消耗排行')).toBeVisible()
    await expect(page.getByText('模型请求排行')).toBeVisible()
    await expect(page.getByText('服务商调用排行')).toBeVisible()
    const activeModels = page.getByRole('region', { name: '当前模型服务商' })
    await expect(activeModels.getByText('3 个模型', { exact: true })).toBeVisible()
    await expect(activeModels.locator('.active-model-row')).toHaveCount(3)
    await expect(activeModels.getByText('Claude Sonnet 5', { exact: true })).toBeVisible()
    await expect(activeModels.getByText('claude-sonnet-5', { exact: true })).toBeVisible()
    await expect(activeModels.getByText('CC Vibe', { exact: true })).toBeVisible()
    await expect(activeModels.getByText('LingkeAI', { exact: true })).toHaveCount(0)
    await expect(activeModels.locator('.active-model-identity').first()).toHaveCSS(
      'display',
      'flex',
    )
    await expect(activeModels.locator('.active-model-code')).toHaveCount(3)
    await expect(activeModels.locator('.active-provider-name')).toHaveCount(3)
    await expect(activeModels.locator('.active-provider-tag')).toHaveCount(0)
    await expect(page.locator('.access-panel').getByText('OpenAI', { exact: true })).toBeVisible()
    await expect(
      page.locator('.access-panel').getByText('Anthropic', { exact: true }),
    ).toBeVisible()
    await expect(page.getByText('适用于 Codex 等 OpenAI 兼容客户端')).toBeVisible()
    await expect(page.getByText('适用于 Claude Code 等 Anthropic 兼容客户端')).toBeVisible()
    await expect(page.getByText('林知远', { exact: true })).toBeVisible()
    await expect(
      page.locator('.model-ranking').getByText('DeepSeek V4 Flash', { exact: true }),
    ).toBeVisible()
    await expect(page.locator('.token-ranking [title="8,200 Token"]')).toHaveText('8.2K Token')
    const firstModelUsage = page.locator('.model-ranking .rank-label span').first()
    await expect(firstModelUsage).toHaveText('18 次 / 9.1K Token')
    await expect(firstModelUsage).toHaveAttribute('title', '18 次 / 9,100 Token')
    await expect(page.locator('.model-ranking')).not.toContainText('deepseek-v4-flash')
    const firstProviderUsage = page.locator('.provider-ranking .rank-label span').first()
    await expect(firstProviderUsage).toHaveText('20 次 / 9.2K Token')
    await expect(firstProviderUsage).toHaveAttribute('title', '20 次 / 9,200 Token')
    for (const [name, dimension] of [
      ['用户 Token 消耗排行', 'member'],
      ['模型请求排行', 'model'],
      ['服务商调用排行', 'provider'],
    ])
      await expect(page.getByRole('link', { name, exact: true })).toHaveAttribute(
        'href',
        new RegExp(`dimension=${dimension}`),
      )
    await expect(page.getByText(/\/v1$/, { exact: true })).toBeVisible()
    await expect(page.getByText(/\/anthropic$/, { exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({ path: '../.cache/web-visual/home.png', fullPage: true })

    const accessPanel = page.getByRole('region', { name: '应用接入' })
    const openaiRow = accessPanel.locator('.access-addresses > div').filter({ hasText: 'OpenAI' })
    const openaiUrl = await openaiRow.locator('code').innerText()
    await openaiRow.getByRole('button', { name: '复制 OpenAI 接入地址' }).click()
    await expect(page.locator('.toast-success')).toContainText('已复制')
    await expect(accessPanel.getByRole('button', { name: '接入指南' })).toHaveCount(1)
    await accessPanel.getByRole('button', { name: '接入指南' }).click()
    await expect(modal(page).getByRole('heading', { name: '应用接入指南' })).toBeVisible()
    await expect(modal(page).getByRole('tab', { name: '使用脚本' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(modal(page).locator('.setup-step-number')).toHaveText(['1', '2'])
    await expect(modal(page).getByRole('heading', { name: '选择协议并复制配置脚本' })).toBeVisible()
    await expect(modal(page).getByRole('tab', { name: 'OpenAI' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(modal(page).locator('.setup-script').first()).toContainText(
      `export OPENAI_BASE_URL='${openaiUrl}'`,
    )
    await expect(modal(page).locator('.setup-script').first()).toContainText(
      'export OPENAI_API_KEY="$ZENTROLA_KEY"',
    )
    await expect(modal(page)).toContainText('输入过程不会回显')
    await expect(modal(page).locator('.setup-script')).toHaveCount(2)
    await modal(page).getByRole('button', { name: '复制 macOS / Linux 脚本' }).click()
    await expect(page.locator('.toast-success')).toContainText('已复制')
    await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
      `$env:OPENAI_BASE_URL = '${openaiUrl}'`,
    )
    await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
      '$env:OPENAI_API_KEY = $zentrolaKey',
    )
    await modal(page).getByRole('tab', { name: 'CC Switch' }).click()
    await expect(modal(page).getByRole('tab', { name: 'CC Switch' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(modal(page).locator('.setup-script').first()).toBeHidden()
    await expect(modal(page).getByRole('link', { name: '访问官网' })).toHaveAttribute(
      'href',
      'https://ccswitch.io/',
    )
    await expect(modal(page).getByText(/\.msi 安装包或 Portable \.zip/)).toBeVisible()
    await page.screenshot({ path: '../.cache/web-visual/home-setup-cc-switch.png', fullPage: true })
    await modal(page).getByRole('tab', { name: '使用脚本' }).click()
    await page.screenshot({ path: '../.cache/web-visual/home-setup.png', fullPage: true })
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()

    const anthropicRow = accessPanel
      .locator('.access-addresses > div')
      .filter({ hasText: 'Anthropic' })
    const anthropicUrl = await anthropicRow.locator('code').innerText()
    await accessPanel.getByRole('button', { name: '接入指南' }).click()
    await modal(page).getByRole('tab', { name: 'Anthropic' }).click()
    await expect(modal(page).getByRole('tab', { name: 'Anthropic' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(modal(page).locator('.setup-script').first()).toContainText(
      `export ANTHROPIC_BASE_URL='${anthropicUrl}'`,
    )
    await expect(modal(page).locator('.setup-script').first()).toContainText(
      'export ANTHROPIC_API_KEY="$ZENTROLA_KEY"',
    )
    await expect(modal(page).locator('.setup-script').nth(1)).toContainText(
      `$env:ANTHROPIC_BASE_URL = '${anthropicUrl}'`,
    )
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(modal(page)).toBeVisible()
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      .toBe(true)
    await page.screenshot({ path: '../.cache/web-visual/home-setup-mobile.png', fullPage: true })
  })

  test('没有当前模型服务商时隐藏整个区块', async ({ page }) => {
    const state = await fixture(page)
    state.activeModelRows.splice(0)
    const activeModelsResponse = page.waitForResponse((response) =>
      new URL(response.url()).pathname.endsWith('/gateway/active-models'),
    )
    await signIn(page, 'home')
    await activeModelsResponse

    await expect(page.getByRole('region', { name: '当前模型服务商' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: '用户 Token 消耗排行' })).toBeVisible()
  })

  test('当前模型服务商加载失败时提示并允许重试', async ({ page }) => {
    await fixture(page)
    let attempts = 0
    await page.route('**/api/v1/gateway/active-models', async (route) => {
      attempts++
      await route.fulfill({
        status: attempts === 1 ? 503 : 200,
        json: {
          code: attempts === 1 ? 'UNAVAILABLE' : 'OK',
          data:
            attempts === 1
              ? null
              : [{ modelCode: 'test-model', modelName: '测试模型', providerName: '测试服务商' }],
        },
      })
    })
    await signIn(page, 'home')
    const alert = page.getByRole('alert').filter({ hasText: '当前模型服务商' })
    await expect(alert).toBeVisible()
    await expect(page.getByRole('region', { name: '当前模型服务商' })).toHaveCount(0)
    await alert.getByRole('button', { name: '重试' }).click()
    await expect(page.getByRole('region', { name: '当前模型服务商' })).toContainText('测试模型')
    await expect(alert).toHaveCount(0)
    expect(attempts).toBe(2)
  })

  test('当前模型服务商在大量记录时限制卡片高度并内部滚动', async ({ page }) => {
    const state = await fixture(page)
    state.activeModelRows.splice(
      0,
      state.activeModelRows.length,
      ...Array.from({ length: 100 }, (_, index) => ({
        modelName: `Model ${index + 1}`,
        modelCode: `model-${String(index + 1).padStart(3, '0')}`,
        providerName: `Provider ${index + 1}`,
      })),
    )
    await signIn(page, 'home')

    const activeModels = page.getByRole('region', { name: '当前模型服务商' })
    await expect(activeModels.getByText('100 个模型', { exact: true })).toBeVisible()
    await expect(activeModels.locator('.active-model-row')).toHaveCount(100)
    const dimensions = await activeModels
      .locator('.active-models-table-wrap')
      .evaluate((element) => {
        const style = getComputedStyle(element)
        return {
          clientHeight: element.clientHeight,
          scrollHeight: element.scrollHeight,
          maxHeight: style.maxHeight,
          overflowY: style.overflowY,
          headerPosition: getComputedStyle(element.querySelector('th')!).position,
        }
      })
    expect(dimensions.maxHeight).toBe('280px')
    expect(dimensions.overflowY).toBe('auto')
    expect(dimensions.headerPosition).toBe('sticky')
    expect(dimensions.clientHeight).toBeLessThanOrEqual(280)
    expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight)
    await page.screenshot({
      path: '../.cache/web-visual/home-active-models-100.png',
      fullPage: true,
    })
  })

  test('首页在启用服务商不可用时提醒管理员', async ({ page }) => {
    const state = await fixture(page)
    state.resources.push({
      id: '88',
      name: 'DeepSeek Key',
      providerId: '81',
      status: 'ACTIVE',
      runtimeStatus: 'BLOCKED',
      blockedReason: 'BILLING',
      blockedAt: stamp,
      lastErrorAt: stamp,
      lastHttpStatus: 402,
      lastErrorCode: 'UPSTREAM_BILLING_BLOCKED',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })

    await signIn(page, 'home')

    await expect(
      page.locator('.provider-total').getByRole('img', { name: '已启用服务商均不可用' }),
    ).toHaveClass(/is-unavailable/)
    const alert = page.locator('.provider-health-alert')
    await expect(alert).toContainText('DeepSeek 当前不可用')
    await expect(alert).toContainText('余额或计费异常 · HTTP 402')
    await expect(alert.getByRole('link', { name: '查看服务商' })).toHaveAttribute(
      'href',
      '#/providers?runtimeStatus=ABNORMAL',
    )
  })

  test('首页用黄色角标表示部分启用服务商不可用', async ({ page }) => {
    const state = await fixture(page)
    state.providers.push({
      id: '82',
      name: 'Anthropic',
      code: 'anthropic-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: null,
      endpoints: [{ protocolType: 'ANTHROPIC', baseUrl: 'https://api.anthropic.com' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.resources.push(
      {
        id: '88',
        name: 'DeepSeek Key',
        providerId: '81',
        status: 'ACTIVE',
        runtimeStatus: 'HEALTHY',
        blockedReason: null,
        blockedAt: null,
        lastErrorAt: null,
        lastHttpStatus: null,
        lastErrorCode: null,
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      },
      {
        id: '89',
        name: 'Anthropic Key',
        providerId: '82',
        status: 'ACTIVE',
        runtimeStatus: 'BLOCKED',
        blockedReason: 'AUTHENTICATION',
        blockedAt: stamp,
        lastErrorAt: stamp,
        lastHttpStatus: 401,
        lastErrorCode: 'UPSTREAM_AUTH_FAILED',
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      },
    )

    await signIn(page, 'home')

    await expect(
      page.locator('.provider-total').getByRole('img', {
        name: '1 / 2 个已启用服务商可用',
      }),
    ).toHaveClass(/is-degraded/)
    await expect(page.locator('.provider-health-alert')).toContainText('Anthropic 当前不可用')
    await page.getByRole('link', { name: '查看服务商' }).click()
    const runtimeFilter = page.getByRole('combobox', { name: '运行状态' })
    const deepSeekName = page.locator('tbody .person strong', { hasText: /^DeepSeek$/ })
    const anthropicName = page.locator('tbody .person strong', { hasText: /^Anthropic$/ })
    await expect(runtimeFilter).toHaveValue('ABNORMAL')
    await expect(anthropicName).toBeVisible()
    await expect(deepSeekName).toHaveCount(0)

    await runtimeFilter.selectOption('HEALTHY')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(deepSeekName).toBeVisible()
    await expect(anthropicName).toHaveCount(0)
  })

  test('首页与服务商页使用同一异常规则统计缺少模型和不可用的服务商', async ({ page }) => {
    const state = await fixture(page)
    const provider = (id: string, name: string, code: string) => ({
      id,
      name,
      code,
      type: 'THIRD_PARTY',
      status: 'ACTIVE',
      website: null,
      endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.example.com/v1' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      createdAt: stamp,
      updatedAt: stamp,
    })
    const resource = (id: string, providerId: string, runtimeStatus: string) => ({
      id,
      name: `凭证 ${id}`,
      providerId,
      status: 'ACTIVE',
      runtimeStatus,
      blockedReason: runtimeStatus === 'BLOCKED' ? 'AUTHENTICATION' : null,
      blockedAt: null,
      lastErrorAt: runtimeStatus === 'BLOCKED' ? stamp : null,
      lastHttpStatus: runtimeStatus === 'BLOCKED' ? 401 : null,
      lastErrorCode: null,
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.providers.push(provider('82', '缺模型服务商', 'missing-model'))
    state.providers.push(provider('83', '鉴权失败服务商', 'blocked-provider'))
    state.providerMappings.set('83', [
      {
        id: '95',
        providerId: '83',
        modelId: '71',
        upstreamModelCode: 'deepseek-v4-flash',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    state.resources.push(
      resource('88', '81', 'HEALTHY'),
      resource('89', '82', 'HEALTHY'),
      resource('90', '83', 'BLOCKED'),
    )

    await signIn(page, 'home')
    const alert = page.locator('.provider-health-alert')
    await expect(alert).toContainText('2 个已启用服务商不可用')
    await expect(alert).toContainText('涉及：缺模型服务商和鉴权失败服务商')
    await expect(
      page.locator('.provider-total').getByRole('img', { name: '1 / 3 个已启用服务商可用' }),
    ).toBeVisible()
    await alert.getByRole('link', { name: '查看服务商' }).click()
    const names = page.locator('tbody .person strong')
    await expect(names).toHaveText(['缺模型服务商', '鉴权失败服务商'])
    await expect(
      page.getByRole('button', { name: '为 缺模型服务商 配置模型映射后即可启用' }),
    ).toBeVisible()
    await expect(
      page.getByRole('button', {
        name: '查看 鉴权失败服务商 的凭证，当前运行状态：不可用：凭证认证失败 · HTTP 401',
      }),
    ).toBeVisible()
  })
})
