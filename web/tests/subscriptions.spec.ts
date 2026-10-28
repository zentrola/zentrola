import { test, expect } from '@playwright/test'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('个人订阅', () => {
  test('凭据弹窗关闭后停止自动额度刷新，重新打开时保留上次额度直到刷新成功', async ({ page }) => {
    const state = await fixture(page)
    state.providers.push({
      id: '82',
      name: 'OpenAI',
      code: 'openai-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: 'https://openai.com',
      endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      authAdapters: ['API_KEY', 'OPENAI_CODEX'],
      createdAt: stamp,
      updatedAt: stamp,
    })
    for (const [id, name] of [
      ['89', 'OpenAI 个人订阅 A'],
      ['90', 'OpenAI 个人订阅 B'],
    ])
      state.resources.push({
        id,
        providerId: '82',
        name,
        authType: 'SUBSCRIPTION',
        authAdapter: 'OPENAI_CODEX',
        subscriptionType: 'PERSONAL',
        planCode: 'plus',
        externalAccountRef: `account-${id}`,
        priority: 100,
        effectiveAt: null,
        expiresAt: null,
        quotaStatus: 'UNKNOWN',
        quotaCheckedAt: null,
        quotaResetsAt: null,
        runtimeStatus: 'HEALTHY',
        blockedReason: null,
        blockedAt: null,
        lastErrorAt: null,
        lastHttpStatus: null,
        lastErrorCode: null,
        credentialConfigured: true,
        subscriptionPrice: null,
        createdAt: stamp,
        updatedAt: stamp,
      })
    const testRequests: string[] = []
    let holdFirst = false
    let releaseFirst = () => {}
    await page.route('**/api/v1/resources/*/test-connection', async (route) => {
      const id = new URL(route.request().url()).pathname.split('/').at(-2)!
      testRequests.push(id)
      if (holdFirst && id === '89') {
        holdFirst = false
        await new Promise<void>((resolve) => {
          releaseFirst = resolve
        })
      }
      await route.fallback()
    })
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const openCredentials = () =>
      page.getByRole('button', { name: '管理 OpenAI 的认证凭据' }).click()
    await openCredentials()
    const firstRow = modal(page).getByRole('row').filter({ hasText: 'OpenAI 个人订阅 A' })
    const secondRow = modal(page).getByRole('row').filter({ hasText: 'OpenAI 个人订阅 B' })
    await expect(firstRow).toContainText('剩余 75%')
    await expect(secondRow).toContainText('剩余 75%')
    expect(testRequests).toEqual(['89', '90'])
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()

    holdFirst = true
    await openCredentials()
    await expect.poll(() => testRequests.length).toBe(3)
    await expect(firstRow).toContainText('剩余 75%')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
    releaseFirst()
    await page.waitForTimeout(500)
    expect(testRequests).toEqual(['89', '90', '89'])
  })

  test('个人订阅额度刷新失败时展示 Codex 可执行文件配置项', async ({ page }) => {
    const state = await fixture(page)
    state.providers.push({
      id: '82',
      name: 'OpenAI',
      code: 'openai-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: 'https://openai.com',
      endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      authAdapters: ['API_KEY', 'OPENAI_CODEX'],
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.resources.push({
      id: '89',
      providerId: '82',
      name: 'OpenAI 个人订阅 · ccount',
      authType: 'SUBSCRIPTION',
      authAdapter: 'OPENAI_CODEX',
      subscriptionType: 'PERSONAL',
      planCode: 'plus',
      externalAccountRef: 'fixture-account',
      priority: 100,
      effectiveAt: null,
      expiresAt: null,
      quotaStatus: 'UNKNOWN',
      quotaCheckedAt: null,
      quotaResetsAt: null,
      runtimeStatus: 'HEALTHY',
      blockedReason: null,
      blockedAt: null,
      lastErrorAt: null,
      lastHttpStatus: null,
      lastErrorCode: null,
      credentialConfigured: true,
      subscriptionPrice: null,
      createdAt: stamp,
      updatedAt: stamp,
    })
    await page.route('**/api/v1/resources/89/test-connection', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 'OK',
          data: {
            ok: false,
            code: 'CODEX_APP_SERVER_UNAVAILABLE',
            httpStatus: null,
            latencyMs: 0,
          },
          requestId: 'req_codex_app_server_unavailable',
        }),
      }),
    )

    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page
      .getByRole('row')
      .filter({ hasText: 'OpenAI' })
      .getByRole('button', { name: '管理 OpenAI 的认证凭据', exact: true })
      .click()

    const subscriptionRow = modal(page).getByRole('row').filter({ hasText: '个人订阅' })
    await expect(subscriptionRow.locator('.credential-quota-feedback')).toHaveText(
      '额度刷新失败 · Codex App Server 无法启动，请检查 Codex 可执行文件配置（CODEX_EXECUTABLE）并重启后端。',
    )
  })

  test('多个个人订阅按顺序刷新额度', async ({ page }) => {
    const state = await fixture(page)
    state.providers.push({
      id: '82',
      name: 'OpenAI',
      code: 'openai-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: 'https://openai.com',
      endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      authAdapters: ['API_KEY', 'OPENAI_CODEX'],
      createdAt: stamp,
      updatedAt: stamp,
    })
    for (const [id, account] of [
      ['89', 'fixture-account-1'],
      ['90', 'fixture-account-2'],
    ]) {
      state.resources.push({
        id,
        providerId: '82',
        name: `OpenAI 个人订阅 · ${account.slice(-6)}`,
        authType: 'SUBSCRIPTION',
        authAdapter: 'OPENAI_CODEX',
        subscriptionType: 'PERSONAL',
        planCode: 'plus',
        externalAccountRef: account,
        priority: 100,
        effectiveAt: null,
        expiresAt: null,
        quotaStatus: 'UNKNOWN',
        quotaCheckedAt: null,
        quotaResetsAt: null,
        runtimeStatus: 'HEALTHY',
        blockedReason: null,
        blockedAt: null,
        lastErrorAt: null,
        lastHttpStatus: null,
        lastErrorCode: null,
        credentialConfigured: true,
        subscriptionPrice: null,
        createdAt: stamp,
        updatedAt: stamp,
      })
    }

    let activeRequests = 0
    let maximumActiveRequests = 0
    let requestCount = 0
    let notifySecondRequest = () => {}
    const secondRequestStarted = new Promise<void>((resolve) => {
      notifySecondRequest = resolve
    })
    await page.route('**/api/v1/resources/*/test-connection', async (route) => {
      requestCount += 1
      activeRequests += 1
      maximumActiveRequests = Math.max(maximumActiveRequests, activeRequests)
      if (requestCount === 1) {
        await Promise.race([
          secondRequestStarted,
          new Promise<void>((resolve) => setTimeout(resolve, 100)),
        ])
      } else {
        notifySecondRequest()
      }
      activeRequests -= 1
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 'OK',
          data: { ok: true, code: 'OK', httpStatus: 200, latencyMs: 1 },
          requestId: `req_subscription_${requestCount}`,
        }),
      })
    })

    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page
      .getByRole('row')
      .filter({ hasText: 'OpenAI' })
      .getByRole('button', { name: '管理 OpenAI 的认证凭据', exact: true })
      .click()

    await expect.poll(() => requestCount).toBe(2)
    await expect.poll(() => activeRequests).toBe(0)
    expect(maximumActiveRequests).toBe(1)
  })

  test('服务商支持个人订阅优先并保留 API Key 兜底', async ({ page }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    const state = await fixture(page)
    state.providers.push({
      id: '82',
      name: 'OpenAI',
      code: 'openai-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: 'https://openai.com',
      endpoints: [{ protocolType: 'OPENAI', baseUrl: 'https://api.openai.com/v1' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      authAdapters: ['API_KEY', 'OPENAI_CODEX'],
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.models.push({
      id: '75',
      code: 'gpt-fixture',
      name: 'GPT Fixture',
      status: 'ACTIVE',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '82',
      publisherProviderName: 'OpenAI',
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.providerMappings.set('82', [
      {
        id: '94',
        providerId: '82',
        modelId: '75',
        upstreamModelCode: 'gpt-fixture',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const providerRow = page.getByRole('row').filter({ hasText: 'OpenAI' })

    await providerRow
      .getByRole('button', {
        name: '管理 OpenAI 的认证凭据',
        exact: true,
      })
      .click()
    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
    const authJSON = JSON.stringify({
      auth_mode: 'chatgpt',
      tokens: {
        id_token: 'fixture-id-token',
        access_token: 'fixture-access-token',
        refresh_token: 'fixture-refresh-token',
        account_id: 'fixture-account',
      },
    })
    const subscriptionTabs = modal(page).getByRole('tablist', {
      name: '个人订阅凭据导入方式',
    })
    await expect(subscriptionTabs.getByRole('tab')).toHaveText(['上传文件', '手动输入'])
    await expect(subscriptionTabs.getByRole('tab', { name: '上传文件' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    const sourceGuide = modal(page).locator('.subscription-source-guide')
    const sourceSummary = sourceGuide.locator('summary')
    await expect(sourceSummary).toHaveText('找不到文件？查看打开目录命令')
    await expect(sourceGuide.locator('.subscription-command-list')).toBeHidden()
    await modal(page).screenshot({
      path: '../.cache/web-visual/provider-subscription-upload.png',
    })
    await sourceSummary.click()
    await expect(sourceGuide).toHaveAttribute('open', '')
    await expect(sourceGuide).toContainText('open ~/.codex')
    await expect(sourceGuide).toContainText('explorer.exe')
    await expect(sourceGuide).toContainText('xdg-open ~/.codex')
    await expect(sourceGuide.getByRole('button', { name: /复制 .* 命令/ })).toHaveCount(3)
    await expect(modal(page)).toContainText('运行上方命令打开 Codex 目录，然后选择 auth.json。')
    await modal(page)
      .getByLabel('Codex auth.json', { exact: true })
      .setInputFiles({
        name: 'auth.json',
        mimeType: 'application/json',
        buffer: Buffer.from(authJSON),
      })
    await expect(modal(page).locator('.subscription-file-picker')).toContainText('auth.json')
    await subscriptionTabs.getByRole('tab', { name: '上传文件' }).press('ArrowRight')
    await expect(subscriptionTabs.getByRole('tab', { name: '手动输入' })).toBeFocused()
    await expect(subscriptionTabs.getByRole('tab', { name: '手动输入' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    const authContent = modal(page).getByLabel('auth.json 内容', { exact: true })
    await expect(authContent).toBeVisible()
    await expect(modal(page).getByLabel('Codex auth.json', { exact: true })).toHaveCount(0)
    await expect(sourceSummary).toHaveText('查看复制文件内容命令')
    await expect(sourceGuide.locator('.subscription-command-list')).toBeHidden()
    await expect(modal(page)).toContainText('运行上方命令获取内容，然后粘贴到输入框。')
    await expect(modal(page).getByText('auth.json 内容', { exact: true })).toHaveCount(0)
    await modal(page).screenshot({
      path: '../.cache/web-visual/provider-subscription-paste.png',
    })
    await sourceSummary.click()
    await expect(sourceGuide).toContainText('pbcopy < ~/.codex/auth.json')
    await expect(sourceGuide).toContainText('Set-Clipboard')
    await expect(sourceGuide).toContainText('cat ~/.codex/auth.json')
    await sourceGuide.getByRole('button', { name: '复制 macOS 命令', exact: true }).click()
    await expect(page.locator('.toast-success')).toContainText('已复制')
    await authContent.fill('{')
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText(
      '请输入或上传不超过 64 KiB 的有效 Codex auth.json。',
    )
    await authContent.fill(authJSON)
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    const credentialList = modal(page)
    await expect(credentialList).toContainText(
      'OpenAI 个人订阅可导出为 auth.json，其连接测试会刷新额度。Claude 订阅连接测试会调用所选模型，额度来自推理响应中的统一限额，网关转发请求时也会定期同步。',
    )
    await expect(credentialList.getByRole('columnheader')).toHaveText([
      '认证凭据',
      '认证方式',
      '调用状态',
      '运行状态',
      '操作',
    ])
    const subscriptionRow = credentialList.getByRole('row').filter({ hasText: '个人订阅' })
    await expect(subscriptionRow).toContainText('OpenAI 个人订阅 · ccount')
    const subscriptionAuthCell = subscriptionRow.getByRole('cell').nth(1)
    await expect(subscriptionAuthCell).not.toContainText('个人订阅')
    await expect(subscriptionAuthCell).toContainText('订阅套餐 · plus')
    await expect(subscriptionAuthCell.locator('.credential-plan')).toHaveCSS(
      'border-bottom-style',
      'solid',
    )
    await expect(subscriptionRow).not.toContainText('Token')
    await expect(subscriptionRow).not.toContainText('••••••••')
    await expect(subscriptionRow).toContainText('最后验证时间')
    await expect(subscriptionRow.locator('.credential-quota-reset')).toHaveText('额度重置时间 · -')
    await expect(subscriptionRow.getByRole('cell').nth(3)).toHaveText('正常')
    await expect(
      subscriptionRow.getByRole('button', {
        name: '验证 OpenAI 个人订阅 · ccount 的可用性',
        exact: true,
      }),
    ).toBeVisible()
    await expect(subscriptionRow.getByRole('button', { name: '删除', exact: true })).toBeVisible()
    expect(state.resources).toEqual([
      expect.objectContaining({ authType: 'SUBSCRIPTION', authAdapter: 'OPENAI_CODEX' }),
    ])

    await page.route(
      '**/api/v1/resources/*/credential/export',
      (route) =>
        route.fulfill({
          status: 503,
          contentType: 'text/html',
          body: '<html><body>Service Unavailable</body></html>',
        }),
      { times: 1 },
    )
    await subscriptionRow
      .getByRole('button', { name: '导出 OpenAI 个人订阅 · ccount 的 auth.json', exact: true })
      .click()
    await expect(page.getByRole('alert')).toContainText('服务暂时不可用')

    const downloadPromise = page.waitForEvent('download')
    await subscriptionRow
      .getByRole('button', { name: '导出 OpenAI 个人订阅 · ccount 的 auth.json', exact: true })
      .click()
    const exported = await downloadPromise
    expect(exported.suggestedFilename()).toBe('auth.json')
    const exportedStream = await exported.createReadStream()
    const exportedChunks: Buffer[] = []
    for await (const chunk of exportedStream) exportedChunks.push(Buffer.from(chunk))
    expect(Buffer.concat(exportedChunks).toString()).toBe(authJSON)
    await expect(page.getByRole('status')).toContainText('已导出 auth.json')

    await subscriptionRow
      .getByRole('button', { name: '验证 OpenAI 个人订阅 · ccount 的可用性', exact: true })
      .click()
    await expect(modal(page).getByRole('status')).toContainText('个人订阅验证通过')
    await expect(modal(page)).toContainText(
      '成功后会更新额度状态、额度重置时间和检查时间，不会发起模型调用。',
    )
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    expect(state.resources[0].quotaStatus).toBe('AVAILABLE')
    const automaticQuotaRefresh = page.waitForRequest(
      (request) =>
        request.method() === 'POST' &&
        new URL(request.url()).pathname.endsWith(
          `/resources/${state.resources[0].id}/test-connection`,
        ),
    )
    const quotaDetailRequest = page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith(`/resources/${state.resources[0].id}/quotas`),
    )
    await providerRow
      .getByRole('button', {
        name: '管理 OpenAI 的认证凭据',
        exact: true,
      })
      .click()
    await automaticQuotaRefresh
    await quotaDetailRequest
    const refreshedSubscriptionRow = modal(page).getByRole('row').filter({ hasText: '个人订阅' })
    const formattedResetAt = await page.evaluate((value) => {
      return new Intl.DateTimeFormat('zh-CN', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }).format(new Date(value))
    }, stamp)
    await expect(refreshedSubscriptionRow).toContainText('5 小时窗口 · 剩余 75%')
    await expect(refreshedSubscriptionRow).toContainText('7 天窗口 · 剩余 94%')
    await expect(refreshedSubscriptionRow).toContainText('Codex Spark · 7 天窗口 · 剩余 100%')
    await expect(refreshedSubscriptionRow).not.toContainText('额度可用')
    await expect(refreshedSubscriptionRow.locator('.credential-quota-reset')).toHaveText([
      `额度重置时间 · ${formattedResetAt}`,
      `额度重置时间 · ${formattedResetAt}`,
      `额度重置时间 · ${formattedResetAt}`,
    ])
    await expect(refreshedSubscriptionRow).toContainText('可用重置卡 · 1')
    const consumeResetCreditRequest = page.waitForRequest(
      (request) =>
        request.method() === 'POST' &&
        new URL(request.url()).pathname.endsWith(
          `/resources/${state.resources[0].id}/rate-limit-reset-credit/consume`,
        ),
    )
    await refreshedSubscriptionRow.getByRole('button', { name: '使用重置卡', exact: true }).click()
    await consumeResetCreditRequest
    await expect(page.getByRole('status')).toContainText('重置卡使用成功，额度已刷新')
    await expect(refreshedSubscriptionRow).toContainText('可用重置卡 · 0')
    await expect(
      refreshedSubscriptionRow.getByRole('button', { name: '使用重置卡', exact: true }),
    ).toHaveCount(0)
    await modal(page).screenshot({
      path: '../.cache/web-visual/provider-subscription-quota.png',
    })
    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('API_KEY')
    await modal(page).getByLabel('API Key', { exact: true }).fill('fallback-api-key')
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    const apiKeyRow = modal(page).getByRole('row').filter({ hasText: 'OpenAI API Key' })
    await expect(apiKeyRow).toBeVisible()
    await expect(apiKeyRow.getByRole('button', { name: /导出/ })).toHaveCount(0)

    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
    await modal(page).getByRole('tab', { name: '手动输入' }).click()
    await modal(page)
      .getByLabel('auth.json 内容', { exact: true })
      .fill(authJSON.replace('fixture-account', 'fixture-account-2'))
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    const secondSubscriptionRow = modal(page)
      .getByRole('row')
      .filter({ hasText: 'OpenAI 个人订阅 · ount-2' })
    await expect(secondSubscriptionRow).toBeVisible()
    expect(state.resources).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: 'OpenAI 个人订阅 · ccount' }),
        expect.objectContaining({ name: 'OpenAI 个人订阅 · ount-2' }),
      ]),
    )
    await secondSubscriptionRow.getByRole('button', { name: '删除', exact: true }).click()
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(secondSubscriptionRow).toHaveCount(0)

    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')
    await modal(page).getByRole('tab', { name: '手动输入' }).click()
    await modal(page).getByLabel('auth.json 内容', { exact: true }).fill(authJSON)
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText(
      '该服务商下已存在此订阅账号的凭证，请更新已有凭证。',
    )
    await modal(page).getByRole('button', { name: '取消', exact: true }).click()

    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    expect(state.resources).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ authType: 'SUBSCRIPTION' }),
        expect.objectContaining({ authType: 'API_KEY' }),
      ]),
    )

    await providerRow.getByRole('button', { name: '测试 OpenAI 的连接', exact: true }).click()
    const testSelectionDialog = page.getByRole('dialog', {
      name: 'OpenAI / 选择测试凭证',
    })
    await expect(testSelectionDialog).toBeVisible()
    await expect(testSelectionDialog.getByRole('radio')).toHaveCount(2)
    await testSelectionDialog
      .getByRole('radio', { name: '使用 OpenAI API Key 测试连接', exact: true })
      .check()
    await testSelectionDialog.getByRole('button', { name: '开始测试', exact: true }).click()
    await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()

    await providerRow
      .getByRole('button', {
        name: '管理 OpenAI 的认证凭据',
        exact: true,
      })
      .click()
    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'OpenAI 个人订阅 · ccount' })
      .getByRole('button', { name: '删除', exact: true })
      .click()
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect
      .poll(() => state.resources.filter((resource) => resource.authType === 'SUBSCRIPTION').length)
      .toBe(0)
    expect(state.resources.some((resource) => resource.authType === 'API_KEY')).toBe(true)
  })

  test('Anthropic 官方渠道直接输入 Claude 个人订阅 Token', async ({ page }) => {
    const state = await fixture(page)
    state.providers.push({
      id: '83',
      name: 'Anthropic',
      code: 'anthropic-official',
      type: 'OFFICIAL',
      status: 'ACTIVE',
      website: 'https://www.anthropic.com',
      endpoints: [{ protocolType: 'ANTHROPIC', baseUrl: 'https://api.anthropic.com' }],
      proxyEnabled: false,
      proxyUrl: null,
      proxyHeaders: [],
      modelSyncSupported: false,
      authAdapters: ['API_KEY', 'ANTHROPIC_CLAUDE_CODE'],
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.models[1].publisherProviderId = '83'
    state.models[1].publisherProviderName = 'Anthropic'
    state.models.push({
      ...state.models[1],
      id: '73',
      code: 'claude-opus',
      name: 'Claude Opus',
    })
    state.providerMappings.set('83', [
      {
        id: '95',
        providerId: '83',
        modelId: '72',
        upstreamModelCode: 'claude-sonnet',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
      {
        id: '96',
        providerId: '83',
        modelId: '73',
        upstreamModelCode: 'claude-opus',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const providerRow = page.getByRole('row').filter({ hasText: 'Anthropic' })
    await providerRow.getByRole('button', { name: '管理 Anthropic 的认证凭据' }).click()
    await modal(page).getByRole('button', { name: '新增凭据', exact: true }).click()
    await modal(page).getByRole('combobox', { name: '认证方式' }).selectOption('SUBSCRIPTION')

    const token = 'future-anthropic-token-format'
    await expect(modal(page)).toContainText('claude setup-token')
    const commandGuide = modal(page).locator('div.subscription-source-guide')
    await expect(commandGuide).toBeVisible()
    await expect(modal(page).locator('details.subscription-source-guide')).toHaveCount(0)
    await expect(commandGuide.locator('code')).toHaveText('claude setup-token')
    await expect(modal(page).getByRole('tablist')).toHaveCount(0)
    await expect(modal(page).locator('input[type="file"]')).toHaveCount(0)
    await expect(modal(page).locator('.subscription-command-list > div')).toHaveCount(1)
    await expect(modal(page).getByRole('button', { name: '复制 Claude Code 命令' })).toBeVisible()
    const tokenInput = modal(page).getByLabel('Claude Code OAuth Token', { exact: true })
    await tokenInput.fill('invalid token')
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('有效 Claude Code OAuth Token')

    await tokenInput.fill(token)
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    expect(state.resources).toEqual([
      expect.objectContaining({
        authType: 'SUBSCRIPTION',
        authAdapter: 'ANTHROPIC_CLAUDE_CODE',
        credential: token,
      }),
    ])
    await expect(
      modal(page).getByRole('row').filter({ hasText: 'Anthropic 个人订阅' }),
    ).toBeVisible()
    await expect(
      modal(page).getByRole('row').filter({ hasText: 'Anthropic 个人订阅' }),
    ).toContainText('额度检查时间 · -')

    await modal(page)
      .getByRole('row')
      .filter({ hasText: 'Anthropic 个人订阅' })
      .getByRole('button', { name: /验证 Anthropic 个人订阅.*的可用性/ })
      .click()
    const selectionDialog = page.getByRole('dialog', { name: 'Anthropic / 选择测试模型' })
    await expect(selectionDialog).toBeVisible()
    await expect(
      selectionDialog.getByRole('radio', { name: /使用 Claude .* 测试连接/ }),
    ).toHaveCount(2)
    await selectionDialog.getByRole('radio', { name: '使用 Claude Opus 测试连接' }).check()
    const testRequest = page.waitForRequest(
      (request) => request.method() === 'POST' && request.url().includes('/test-connection'),
    )
    await selectionDialog.getByRole('button', { name: '开始测试' }).click()
    expect(new URL((await testRequest).url()).searchParams.get('providerModelMappingId')).toBe('96')
    await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
    await expect(modal(page)).toContainText('会根据响应中的统一限额更新额度状态')
    expect(state.resources[0].quotaCheckedAt).toBe(stamp)
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()

    // 打开凭据弹窗只读取已记录的额度，不为刷新额度再次发起模型调用。
    const automaticTests: string[] = []
    page.on('request', (request) => {
      if (request.method() === 'POST' && request.url().includes('/test-connection'))
        automaticTests.push(request.url())
    })
    const quotaDetailRequest = page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith(`/resources/${state.resources[0].id}/quotas`),
    )
    await providerRow.getByRole('button', { name: '管理 Anthropic 的认证凭据' }).click()
    await quotaDetailRequest
    const subscriptionRow = modal(page).getByRole('row').filter({ hasText: 'Anthropic 个人订阅' })
    await expect(subscriptionRow).toContainText('5 小时窗口 · 剩余 50%')
    await expect(subscriptionRow).toContainText('7 天窗口 · 剩余 44%')
    await expect(subscriptionRow).not.toContainText('five_hour')
    await expect(subscriptionRow.locator('.credential-quota-reset')).toHaveCount(2)
    expect(automaticTests).toEqual([])

    const refreshButton = subscriptionRow.getByRole('button', {
      name: '刷新 Anthropic 个人订阅 的额度',
      exact: true,
    })
    await expect(refreshButton).toHaveText('刷新额度')
    await expect(refreshButton).toHaveAttribute(
      'title',
      /发送一次最小模型请求.*消耗极少量订阅额度.*可能开启新的 5 小时窗口/,
    )
    const refreshRequest = page.waitForRequest(
      (request) =>
        request.method() === 'POST' &&
        new URL(request.url()).pathname.endsWith(
          `/resources/${state.resources[0].id}/refresh-quota`,
        ),
    )
    await refreshButton.click()
    await refreshRequest
    await expect(page.locator('.toast')).toContainText('额度已刷新')
    await expect(subscriptionRow).toContainText('5 小时窗口 · 剩余 99%')
    await expect(subscriptionRow).toContainText('7 天窗口 · 剩余 93%')
    expect(automaticTests).toEqual([])

    await page.route('**/api/v1/resources/*/refresh-quota', (route) =>
      route.fulfill({
        json: {
          code: 'OK',
          data: {
            ok: false,
            code: 'UPSTREAM_QUOTA_UNAVAILABLE',
            httpStatus: 200,
            latencyMs: 80,
            quotas: null,
          },
          requestId: 'req_quota_refresh',
        },
      }),
    )
    await refreshButton.click()
    await expect(subscriptionRow).toContainText(
      '额度刷新失败 · 上游响应未包含额度信息，请稍后重试。',
    )
  })
})
