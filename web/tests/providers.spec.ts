import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal, expectOnlyModalBodyScrolls } from './mock/helpers'

test.describe('服务商', () => {
  test('凭据列表只接受最新请求结果，晚到的旧响应不会恢复已删除凭据', async ({ page }) => {
    const state = await fixture(page)
    for (const [id, name] of [
      ['88', 'DeepSeek 主 Key'],
      ['89', 'DeepSeek 待删除 Key'],
    ])
      state.resources.push({
        id,
        providerId: '81',
        name,
        authType: 'API_KEY',
        authAdapter: 'API_KEY',
        status: 'ACTIVE',
        runtimeStatus: 'HEALTHY',
        priority: 100,
        credentialConfigured: true,
        createdAt: stamp,
        updatedAt: stamp,
      })
    let delayNext = false
    let releaseSlow = () => {}
    await page.route('**/api/v1/resources?**', async (route) => {
      if (!delayNext) return route.fallback()
      delayNext = false
      const stale = state.resources.map(({ credential: _, quotaDetails: __, ...row }) => row)
      await new Promise<void>((resolve) => {
        releaseSlow = resolve
      })
      await route
        .fulfill({
          json: { code: 'OK', data: { items: stale, nextCursor: null, total: stale.length } },
        })
        .catch(() => {})
    })
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await expect(page.locator('tbody .person strong', { hasText: /^DeepSeek$/ })).toBeVisible()
    delayNext = true
    await page.getByRole('button', { name: '管理 DeepSeek 的认证凭据' }).click()
    const dialog = modal(page)
    const removedRow = dialog.getByRole('row').filter({ hasText: 'DeepSeek 待删除 Key' })
    await removedRow.getByRole('button', { name: '删除', exact: true }).click()
    await page
      .getByRole('dialog', { name: '删除认证凭据' })
      .getByRole('button', { name: '删除', exact: true })
      .click()
    await expect(removedRow).toHaveCount(0)
    await expect(dialog.getByRole('row').filter({ hasText: 'DeepSeek 主 Key' })).toBeVisible()
    releaseSlow()
    await page.waitForTimeout(300)
    await expect(removedRow).toHaveCount(0)
  })

  test('服务商同步入口只由后端能力参数控制', async ({ page }) => {
    await fixture(page)
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await expect(page.getByRole('columnheader').first()).toHaveText('ID')
    await expect(row.getByRole('cell').first().locator('code')).toHaveText('81')
    await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
    const syncModelsButton = row.getByRole('button', {
      name: '同步 DeepSeek 的官方模型',
      exact: true,
    })
    await expect(syncModelsButton).toBeVisible()
    await expect(row.locator('.provider-name-actions').getByRole('button')).toHaveCount(1)
    await expect(syncModelsButton.locator('svg')).toBeVisible()
    await expect(syncModelsButton.locator('path')).toHaveAttribute('d', /\S+/)
    await syncModelsButton.click()
    const dialog = page.getByRole('dialog', { name: 'DeepSeek / 模型同步结果' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('status')).toContainText('模型目录同步完成')
    await expect(dialog.getByText('官方接口', { exact: true })).toBeVisible()
  })

  test('服务商协议地址可显式选择私有网络', async ({ page }) => {
    const state = await fixture(page)
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '编辑服务商' })
    await dialog.getByLabel('OpenAI 网络位置', { exact: true }).selectOption('PRIVATE')
    await dialog.getByLabel('OpenAI 协议地址', { exact: true }).fill('http://192.168.1.20:8080/v1')
    await expect(dialog.getByText(/私有网络地址可以访问 Zentrola 所在网络/)).toBeVisible()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()

    expect(state.providerInputs.at(-1)?.endpoints).toContainEqual({
      protocolType: 'OPENAI',
      baseUrl: 'http://192.168.1.20:8080/v1',
      networkScope: 'PRIVATE',
    })
    await expect(row.getByText('私有网络', { exact: true })).toBeVisible()
  })

  test('服务商代理支持 SOCKS5 并忽略 HTTP Header', async ({ page }) => {
    const state = await fixture(page)
    state.providers[0].proxyEnabled = true
    state.providers[0].proxyUrl = 'http://proxy.example.com:8080'
    state.providers[0].proxyHeaders = [{ key: 'X-Proxy-Token', configured: true }]
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '编辑服务商' })
    await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
    await dialog.getByRole('switch', { name: '启用代理', exact: true }).check()
    await expect(dialog.getByLabel('KEY', { exact: true })).toHaveValue('X-Proxy-Token')
    const proxyURL = dialog.getByLabel('代理服务器地址', { exact: true })
    await proxyURL.fill('ftp://proxy.example.com:21')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText(
      '请输入有效的 HTTP、HTTPS 或 SOCKS5 代理地址',
    )

    await dialog.getByRole('switch', { name: '修改代理凭据', exact: true }).check()
    await proxyURL.fill('socks5://proxy-user:proxy-password@proxy.example.com:1080')
    await expect(
      dialog.getByText('SOCKS5 使用地址中的用户名和密码认证，不支持自定义 HTTP Header。', {
        exact: true,
      }),
    ).toBeVisible()
    await expect(dialog.getByRole('button', { name: '添加 Header', exact: true })).toHaveCount(0)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()

    expect(state.providerInputs.at(-1)).toEqual(
      expect.objectContaining({
        proxyEnabled: true,
        proxyUrl: 'socks5://proxy-user:proxy-password@proxy.example.com:1080',
        proxyHeaders: [],
      }),
    )
    expect(state.providers[0].proxyUrl).not.toContain('proxy-password')
  })

  test('编辑服务商代理时默认保留凭据并可显式修改', async ({ page }) => {
    const state = await fixture(page)
    state.providers[0].proxyEnabled = true
    state.providers[0].proxyUrl = 'http://******:******@geo.miyaip.app:8001'
    state.providers[0].proxyHeaders = []
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    let dialog = page.getByRole('dialog', { name: '编辑服务商' })
    await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
    const credentialSwitch = dialog.getByRole('switch', {
      name: '修改代理凭据',
      exact: true,
    })
    const proxyProtocol = dialog.getByLabel('代理协议', { exact: true })
    const proxyURLInput = dialog.getByLabel('代理服务器地址', { exact: true })
    await expect(credentialSwitch).not.toBeChecked()
    await expect(proxyProtocol).toHaveValue('http')
    await proxyURLInput.fill('https://******:******@geo.miyaip.app:8002')
    await expect(proxyProtocol).toHaveValue('https')
    await expect(proxyURLInput).toHaveValue('******:******@geo.miyaip.app:8002')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    expect(state.providerInputs.at(-1)).toEqual(
      expect.objectContaining({
        proxyUrl: 'https://******:******@geo.miyaip.app:8002',
        updateProxyCredentials: false,
      }),
    )

    await row.getByRole('button', { name: '编辑', exact: true }).click()
    dialog = page.getByRole('dialog', { name: '编辑服务商' })
    await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
    const proxyURL = dialog.getByLabel('代理服务器地址', { exact: true })
    await expect(dialog.getByLabel('代理协议', { exact: true })).toHaveValue('https')
    await dialog.getByRole('switch', { name: '修改代理凭据', exact: true }).check()
    await expect(proxyURL).toHaveValue('geo.miyaip.app:8002')
    await proxyURL.fill('socks5://new-user:new-password@geo.miyaip.app:8002')
    await expect(proxyURL).toHaveValue('new-user:new-password@geo.miyaip.app:8002')
    await expect(dialog.getByLabel('代理协议', { exact: true })).toHaveValue('socks5')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    expect(state.providerInputs.at(-1)).toEqual(
      expect.objectContaining({
        proxyUrl: 'socks5://new-user:new-password@geo.miyaip.app:8002',
        updateProxyCredentials: true,
      }),
    )
  })

  test('连接测试允许选择模型并显示实际测试模型', async ({ page }) => {
    const state = await fixture(page)
    state.providers[0].name = 'Google'
    state.providers[0].code = 'google-gemini-official'
    state.providers[0].endpoints = [
      {
        protocolType: 'OPENAI',
        baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai',
      },
    ]
    state.models.splice(
      0,
      state.models.length,
      {
        id: '73',
        code: 'gemini-3.6-flash',
        name: 'Gemini 3.6 Flash',
        status: 'ACTIVE',
        inputModalities: ['TEXT'],
        outputModalities: ['TEXT'],
        remark: '',
        publisherProviderId: '81',
        publisherProviderName: 'Google',
        createdAt: stamp,
        updatedAt: stamp,
      },
      {
        id: '74',
        code: 'gemini-3.6-pro',
        name: 'Gemini 3.6 Pro',
        status: 'ACTIVE',
        inputModalities: ['TEXT'],
        outputModalities: ['TEXT'],
        remark: '',
        publisherProviderId: '81',
        publisherProviderName: 'Google',
        createdAt: stamp,
        updatedAt: stamp,
      },
    )
    state.providerMappings.set('81', [
      {
        id: '92',
        providerId: '81',
        modelId: '73',
        upstreamModelCode: 'gemini-3.6-flash',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
      {
        id: '93',
        providerId: '81',
        modelId: '74',
        upstreamModelCode: 'gemini-3.6-pro',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'Google API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })

    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '测试 Google 的连接', exact: true }).click()

    const selection = page.getByRole('dialog', { name: 'Google / 选择测试模型' })
    await expect(selection).toBeVisible()
    await page.setViewportSize({ width: 390, height: 280 })
    await expectOnlyModalBodyScrolls(selection)
    await expect(
      selection.getByRole('radio', { name: '使用 Gemini 3.6 Flash 测试连接' }),
    ).toBeChecked()
    await selection.getByRole('radio', { name: '使用 Gemini 3.6 Pro 测试连接' }).check()
    const requestPromise = page.waitForRequest((request) => {
      const url = new URL(request.url())
      return request.method() === 'POST' && url.pathname.endsWith('/test-connection')
    })
    await selection.getByRole('button', { name: '开始测试', exact: true }).click()
    const request = await requestPromise
    expect(new URL(request.url()).searchParams.get('providerModelMappingId')).toBe('93')

    const result = page.getByRole('dialog', { name: 'Google / 连接测试结果' })
    await expect(result.getByRole('status')).toContainText('模型调用验证通过')
    await expect(result.locator('dt').filter({ hasText: /^协议$/ })).toBeVisible()
    await expect(result.getByText('gemini-3.6-pro', { exact: true })).toBeVisible()
  })

  test('服务商没有可用模型映射时隐藏连接测试入口', async ({ page }) => {
    const state = await fixture(page)
    state.providerMappings.set('81', [])
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })

    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await expect(
      providerRow.getByRole('button', { name: '测试 DeepSeek 的连接', exact: true }),
    ).toHaveCount(0)
    const credentialButton = providerRow.getByRole('button', {
      name: '管理 DeepSeek 的认证凭据',
      exact: true,
    })
    await expect(credentialButton).toBeVisible()
    await credentialButton.click()
    await expect(
      modal(page).getByRole('button', { name: '验证 DeepSeek API Key 的可用性', exact: true }),
    ).toHaveCount(0)
  })

  test('服务商启用开关分别提示缺少模型映射和认证凭据', async ({ page }) => {
    const state = await fixture(page)
    const disabledProvider = (id: string, name: string) => ({
      ...structuredClone(state.providers[0]),
      id,
      name,
      code: `provider-${id}`,
      status: 'DISABLED',
    })
    state.providers.push(
      disabledProvider('82', '模型凭据均未配置'),
      disabledProvider('83', '仅配置凭据'),
      disabledProvider('84', '仅配置模型'),
      disabledProvider('85', '配置齐全'),
    )
    state.providers.find((provider) => provider.id === '85')!.type = 'THIRD_PARTY'
    state.providerMappings.set('82', [])
    state.providerMappings.set('83', [])
    state.providerMappings.set('84', [
      {
        id: '94',
        providerId: '84',
        modelId: '71',
        upstreamModelCode: 'deepseek-v4-flash',
        priority: 100,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    state.resources.push({
      id: '89',
      name: '仅凭据 API Key',
      providerId: '83',
      authType: 'API_KEY',
      priority: 100,
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      quotaStatus: 'UNKNOWN',
      blockedReason: null,
      blockedAt: null,
      lastErrorAt: null,
      lastHttpStatus: null,
      lastErrorCode: null,
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.providerMappings.set(
      '85',
      state.providerMappings.get('84')!.map((mapping) => ({
        ...mapping,
        id: '95',
        providerId: '85',
      })),
    )
    state.resources.push({ ...state.resources.at(-1)!, id: '90', providerId: '85' })

    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    const switchFor = (name: string) =>
      page
        .getByRole('row')
        .filter({ hasText: name })
        .getByRole('switch', { name: `${name}的启用状态` })
    await expect(switchFor('模型凭据均未配置')).toBeDisabled()
    await expect(switchFor('模型凭据均未配置')).toHaveAttribute('title', '请先配置模型和凭证。')
    await expect(switchFor('仅配置凭据')).toBeDisabled()
    await expect(switchFor('仅配置凭据')).toHaveAttribute('title', '请先配置模型。')
    await expect(switchFor('仅配置模型')).toBeDisabled()
    await expect(switchFor('仅配置模型')).toHaveAttribute('title', '请先配置凭证。')
    const configuredRow = page.getByRole('row').filter({ hasText: '配置齐全' })
    await expect(configuredRow.locator('.provider-runtime-state')).toHaveText('待启用')
    await expect(switchFor('配置齐全')).toBeEnabled()
    await switchFor('配置齐全').click()
    const activation = page.getByRole('dialog', { name: '配置齐全 / 选择启用时验证的连接' })
    await expect(activation).toBeVisible()
    await page.setViewportSize({ width: 390, height: 420 })
    await expectOnlyModalBodyScrolls(activation)
    await expect(
      activation.getByRole('radio', { name: '使用 Anthropic 协议测试连接' }),
    ).toBeChecked()
    await expect(
      activation.getByRole('radio', { name: '使用 DeepSeek V4 Flash 测试连接' }),
    ).toBeChecked()
    await expect(switchFor('配置齐全')).not.toBeChecked()
    await activation.getByRole('button', { name: '测试并启用' }).click()
    await expect(page.getByRole('dialog', { name: '配置齐全 / 启用结果' })).toContainText(
      '连接测试通过，服务商已启用',
    )
    expect(state.providerActivationInputs.at(-1)).toEqual({
      resourceId: '90',
      protocol: 'ANTHROPIC',
      providerModelMappingId: '95',
    })
    await expect(configuredRow.locator('.provider-runtime-state')).toHaveText('正常')
    await page
      .getByRole('dialog', { name: '配置齐全 / 启用结果' })
      .getByRole('button', { name: '关闭', exact: true })
      .last()
      .click()
    const credentialOnlyRow = page.getByRole('row').filter({ hasText: '仅配置凭据' })
    await expect(credentialOnlyRow.locator('.provider-runtime-state')).toHaveText('未配置模型')
    await expect(credentialOnlyRow.locator('.provider-runtime-state')).toHaveAttribute(
      'title',
      '为 仅配置凭据 配置模型映射后即可启用',
    )
    await mkdir('../.cache/web-visual', { recursive: true })
    await credentialOnlyRow.screenshot({ path: '../.cache/web-visual/provider-missing-model.png' })
    await credentialOnlyRow.locator('.provider-runtime-state').click()
    await expect(modal(page).getByRole('tab', { name: '模型配置' })).toHaveAttribute(
      'aria-selected',
      'true',
    )
  })

  test('启用服务商使用弹窗选定的协议，连接失败时保持停用', async ({ page }) => {
    const state = await fixture(page)
    state.providers[0].status = 'DISABLED'
    state.models[1].publisherProviderId = '81'
    state.providerMappings.get('81')!.push({
      id: '93',
      providerId: '81',
      modelId: '72',
      upstreamModelCode: 'alternate-model',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    })
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
    state.failTest()
    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const status = page.getByRole('switch', { name: 'DeepSeek的启用状态' })
    await status.click()
    const selection = page.getByRole('dialog', { name: 'DeepSeek / 选择启用时验证的连接' })
    await selection.getByRole('radio', { name: '使用 OpenAI 协议测试连接' }).check()
    await selection.getByRole('radio', { name: '使用 Claude Sonnet 测试连接' }).check()
    await selection.getByRole('button', { name: '测试并启用' }).click()
    await expect(page.getByRole('dialog', { name: 'DeepSeek / 启用结果' })).toContainText(
      '上游认证失败',
    )
    expect(state.providerActivationInputs).toEqual([
      { resourceId: '88', protocol: 'OPENAI', providerModelMappingId: '93' },
    ])
    await expect(status).not.toBeChecked()
    expect(state.providers[0].status).toBe('DISABLED')
  })

  test('服务商操作引导依次高亮配置入口', async ({ page }) => {
    const state = await fixture(page)
    state.providers[0].website = 'https://www.deepseek.com'
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
      authType: 'API_KEY',
      authAdapter: 'API_KEY',
      status: 'ACTIVE',
      runtimeStatus: 'HEALTHY',
      credentialConfigured: true,
      createdAt: stamp,
      updatedAt: stamp,
    })

    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '操作引导', exact: true }).click()

    const guide = page.locator('.initialization-tour')
    await expect(guide).toHaveAttribute('data-target', 'create')
    await expect(guide.getByRole('heading', { name: '添加服务商' })).toBeVisible()
    await expect(page.locator('#provider-guide-create')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'website')
    await expect(guide.getByRole('heading', { name: '前往官网获取凭证' })).toBeVisible()
    const websiteLink = page.locator('#provider-guide-website')
    await expect(websiteLink).toHaveClass(/initialization-tour-target/)
    await expect(websiteLink).toHaveAttribute('href', 'https://www.deepseek.com')
    await websiteLink.evaluate((element) => {
      element.addEventListener(
        'click',
        (event) => {
          event.preventDefault()
          element.setAttribute('data-guide-clicked', 'true')
        },
        { once: true },
      )
    })
    await page.locator('.initialization-highlight').click()
    await expect(websiteLink).toHaveAttribute('data-guide-clicked', 'true')

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'credential')
    await expect(guide.getByRole('heading', { name: '添加凭证' })).toBeVisible()
    await expect(page.locator('#provider-guide-credential')).toHaveClass(
      /initialization-tour-target/,
    )

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'sync')
    await expect(guide.getByRole('heading', { name: '同步模型' })).toBeVisible()
    await expect(page.locator('#provider-guide-sync')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'test')
    await expect(guide.getByRole('heading', { name: '测试连接' })).toBeVisible()
    await expect(page.locator('#provider-guide-test')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '下一步', exact: true }).click()
    await expect(guide).toHaveAttribute('data-target', 'edit')
    await expect(guide.getByRole('heading', { name: '编辑服务商配置' })).toBeVisible()
    await expect(guide.getByText('第 6 步，共 6 步', { exact: true })).toBeVisible()
    await expect(page.locator('#provider-guide-edit')).toHaveClass(/initialization-tour-target/)

    await guide.getByRole('button', { name: '完成', exact: true }).click()
    await expect(guide).toHaveCount(0)
  })

  test('没有服务商时操作引导只显示添加服务商', async ({ page }) => {
    const state = await fixture(page)
    state.providers.splice(0)

    await signIn(page, 'home')
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page.getByRole('button', { name: '操作引导', exact: true }).click()

    const guide = page.locator('.initialization-tour')
    await expect(guide).toHaveAttribute('data-target', 'create')
    await expect(guide.getByText('第 1 步，共 1 步', { exact: true })).toBeVisible()
    await expect(guide.getByRole('button', { name: '完成', exact: true })).toBeVisible()
    await expect(page.locator('#provider-guide-create')).toHaveClass(/initialization-tour-target/)
  })

  test('服务商编辑模型列表在大量模型时可独立滚动', async ({ page }) => {
    const state = await fixture(page)
    state.models.push(
      ...Array.from({ length: 30 }, (_, index) => ({
        id: String(1000 + index),
        code: `deepseek-history-${index}`,
        name: `DeepSeek History ${index}`,
        status: 'ACTIVE',
        inputModalities: ['TEXT'],
        outputModalities: ['TEXT'],
        remark: '',
        publisherProviderId: '81',
        publisherProviderName: 'DeepSeek',
        createdAt: stamp,
        updatedAt: stamp,
      })),
    )
    await signIn(page)
    await page.setViewportSize({ width: 1200, height: 900 })
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const providerRow = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await providerRow.getByRole('button', { name: '编辑', exact: true }).click()

    const mappingList = modal(page).locator('.mapping-catalog-list')
    await expect(mappingList).toBeVisible()
    const dimensions = await mappingList.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }))
    expect(dimensions.clientHeight).toBeLessThanOrEqual(640)
    expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight)
    await mappingList.hover()
    await page.mouse.wheel(0, 600)
    await expect.poll(() => mappingList.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
    await mappingList.evaluate((element) => {
      element.scrollTop = element.scrollHeight
    })
    await expect
      .poll(() =>
        mappingList.evaluate(
          (element) => element.scrollTop + element.clientHeight >= element.scrollHeight - 1,
        ),
      )
      .toBe(true)
    const listBox = (await mappingList.boundingBox())!
    const lastRowBox = (await mappingList.locator('.mapping-catalog-row').last().boundingBox())!
    expect(lastRowBox.y + lastRowBox.height).toBeLessThanOrEqual(listBox.y + listBox.height + 1)
    const bottomGap = await mappingList.evaluate((element) => {
      const editor = element.closest<HTMLElement>('.mapping-editor')!
      return editor.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom
    })
    expect(bottomGap).toBeGreaterThanOrEqual(16)

    await page.setViewportSize({ width: 1200, height: 600 })
    const desktopWorkspace = modal(page).locator('.mapping-workspace')
    const desktopBottomGap = await desktopWorkspace.evaluate((element) => {
      const editor = element.closest<HTMLElement>('.mapping-editor')!
      return editor.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom
    })
    expect(desktopBottomGap).toBeGreaterThanOrEqual(16)

    await page.setViewportSize({ width: 700, height: 800 })
    const mappingWorkspace = modal(page).locator('.mapping-workspace')
    await expect(mappingWorkspace).toHaveCSS('grid-template-columns', /^\S+$/)
    await expect(modal(page).locator('.mapping-editor')).toHaveCSS('overflow-y', 'auto')
    const catalogBox = (await modal(page).locator('.mapping-catalog-pane').boundingBox())!
    const selectedBox = (await modal(page).locator('.mapping-selected-pane').boundingBox())!
    expect(selectedBox.y).toBeGreaterThan(catalogBox.y + catalogBox.height)
  })

  test('官方服务商编辑时不展示或提交停用模型映射', async ({ page }) => {
    const state = await fixture(page)
    state.models.push({
      id: '73',
      code: 'deepseek-disabled',
      name: 'DeepSeek Disabled',
      status: 'DISABLED',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'DeepSeek',
      createdAt: stamp,
      updatedAt: stamp,
    })
    state.providerMappings.get('81')!.push({
      id: '93',
      providerId: '81',
      modelId: '73',
      upstreamModelCode: 'deepseek-disabled',
      priority: 100,
      createdAt: stamp,
      updatedAt: stamp,
    })

    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    await page
      .getByRole('row')
      .filter({ hasText: 'DeepSeek' })
      .getByRole('button', { name: '编辑' })
      .click()
    const dialog = modal(page)
    await expect(dialog.locator('.mapping-count-tag')).toHaveText('1')
    await expect(
      dialog.getByRole('checkbox', { name: '启用 DeepSeek V4 Flash 映射' }),
    ).toBeChecked()
    await expect(dialog.getByRole('checkbox', { name: '启用 DeepSeek Disabled 映射' })).toHaveCount(
      0,
    )
    await expect(dialog.getByText('DeepSeek Disabled', { exact: true })).toHaveCount(0)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    expect(state.providerInputs.at(-1)?.mappings).toEqual([
      { modelId: '71', upstreamModelCode: 'deepseek-v4-flash' },
    ])
  })

  test('服务商新增编辑、启停和窄屏导航折叠', async ({ page }) => {
    const state = await fixture(page)
    state.models.push({
      id: '73',
      code: 'deepseek-v3-legacy',
      name: 'DeepSeek V3 Legacy',
      status: 'DISABLED',
      inputModalities: ['TEXT'],
      outputModalities: ['TEXT'],
      remark: '',
      publisherProviderId: '81',
      publisherProviderName: 'DeepSeek',
      createdAt: stamp,
      updatedAt: stamp,
    })
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await signIn(page)

    await page.setViewportSize({ width: 1200, height: 900 })
    const providerLink = page.getByRole('link', { name: '服务商', exact: true })
    await expect(providerLink.locator('span')).toBeVisible()
    await providerLink.click()
    await expect(page.getByRole('heading', { name: '服务商', exact: true })).toBeVisible()

    const providerSearch = page.getByRole('search')
    const providerSearchInput = providerSearch.getByRole('searchbox', { name: '服务商名称' })
    await expect(providerSearchInput).toHaveAttribute('placeholder', '请输入服务商名称')
    await expect
      .poll(() => providerSearchInput.evaluate((input) => getComputedStyle(input).paddingLeft))
      .toBe('12px')
    await expect(providerSearch.locator('.search-box > svg')).toHaveCount(0)
    await expect(
      providerSearch.getByRole('button', { name: '搜索', exact: true }).locator('svg'),
    ).toHaveCount(0)
    await expect(
      page.locator('.page-heading').getByRole('button', { name: '添加服务商', exact: true }),
    ).toHaveCount(0)
    await expect(
      providerSearch.getByRole('button', { name: '创建服务商', exact: true }),
    ).toBeVisible()
    await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
    const createMenu = providerSearch.getByRole('menu', { name: '创建服务商' })
    await expect(createMenu.getByRole('menuitem', { name: /三方服务商/ })).toBeVisible()
    await expect(createMenu.getByRole('menuitem', { name: /预设厂商/ })).toBeVisible()
    await createMenu.getByRole('menuitem', { name: /预设厂商/ }).click()
    const initializeDialog = modal(page)
    await expect(initializeDialog.getByRole('heading', { name: '选择模型厂商' })).toBeVisible()
    await expect(initializeDialog.getByText('已选择 8 / 8', { exact: true })).toBeVisible()
    await expect(
      initializeDialog.getByRole('checkbox', { name: '选择 xAI', exact: true }),
    ).toBeVisible()
    await expect(
      initializeDialog.getByRole('checkbox', { name: '选择 MiniMax', exact: true }),
    ).toHaveCount(0)
    await expect(
      initializeDialog.getByRole('checkbox', { name: '选择 字节跳动', exact: true }),
    ).toHaveCount(0)
    await initializeDialog.getByRole('button', { name: '清空', exact: true }).click()
    await expect(
      initializeDialog.getByRole('button', { name: '初始化所选厂商', exact: true }),
    ).toBeDisabled()
    await initializeDialog.getByRole('checkbox', { name: '选择 深度求索', exact: true }).check()
    await initializeDialog.getByRole('checkbox', { name: '选择 月之暗面', exact: true }).check()
    await initializeDialog.getByRole('checkbox', { name: '选择 通义千问', exact: true }).check()
    await expect(initializeDialog.getByText('已选择 3 / 8', { exact: true })).toBeVisible()
    await initializeDialog.getByRole('button', { name: '初始化所选厂商', exact: true }).click()
    await expect(page.locator('.toast-success')).toContainText(
      '已补充 2 个官方服务商，并更新 1 个名称或官网，共 3 个',
    )
    await expect(page.locator('.provider-initialize-notice')).toHaveCount(0)
    const kimiRow = page.getByRole('row').filter({ hasText: '月之暗面' })
    await expect(kimiRow).toBeVisible()
    await expect(
      kimiRow.getByRole('button', { name: '同步 月之暗面 的官方模型', exact: true }),
    ).toHaveCount(0)
    const qwenRow = page.getByRole('row').filter({ hasText: '通义千问' })
    await expect(
      qwenRow.getByRole('button', { name: '同步 通义千问 的官方模型', exact: true }),
    ).toBeVisible()
    expect(state.providers.find((provider) => provider.code === 'kimi-official')?.website).toBe(
      'https://www.moonshot.cn',
    )
    await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
    await providerSearch
      .getByRole('menu', { name: '创建服务商' })
      .getByRole('menuitem', { name: /预设厂商/ })
      .click()
    const repeatInitializeDialog = modal(page)
    await repeatInitializeDialog.getByRole('button', { name: '清空', exact: true }).click()
    await repeatInitializeDialog
      .getByRole('checkbox', { name: '选择 深度求索', exact: true })
      .check()
    await repeatInitializeDialog
      .getByRole('checkbox', { name: '选择 月之暗面', exact: true })
      .check()
    await repeatInitializeDialog
      .getByRole('checkbox', { name: '选择 通义千问', exact: true })
      .check()
    await repeatInitializeDialog
      .getByRole('button', { name: '初始化所选厂商', exact: true })
      .click()
    await expect(page.locator('.toast-success')).toContainText(
      '官方服务商的当前语言名称和官网已是最新，共 3 个',
    )
    expect(state.providers).toHaveLength(3)
    expect(state.providerInitializeRequests).toEqual([
      {
        locale: 'zh-CN',
        providerCodes: ['deepseek-official', 'kimi-official', 'qwen-official'],
      },
      {
        locale: 'zh-CN',
        providerCodes: ['deepseek-official', 'kimi-official', 'qwen-official'],
      },
    ])

    const deepSeekRow = page.getByRole('row').filter({ hasText: '深度求索' })
    await expect(page.getByRole('columnheader', { name: '启用状态', exact: true })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '认证凭据', exact: true })).toHaveCount(0)
    const deepSeekCredential = deepSeekRow.getByRole('button', {
      name: '管理 深度求索 的认证凭据',
      exact: true,
    })
    await expect(deepSeekCredential).toBeVisible()
    await expect(deepSeekCredential).toHaveText('凭证')
    const deepSeekProxy = deepSeekRow.getByRole('img', {
      name: '深度求索 未启用代理访问',
      exact: true,
    })
    await expect(deepSeekProxy).toBeVisible()
    await expect(deepSeekProxy.locator('svg')).toHaveCount(1)
    await expect(
      deepSeekRow.locator('td').last().getByRole('button', { name: '编辑' }),
    ).toBeVisible()
    const moreActions = deepSeekRow.locator('td').last().locator('summary')
    await expect(moreActions).toHaveAttribute('aria-label', '深度求索 的更多操作')
    await moreActions.click()
    await expect(deepSeekRow.locator('.row-action-more-menu')).toBeVisible()
    const moreMenuBox = (await deepSeekRow.locator('.row-action-more-menu').boundingBox())!
    expect(moreMenuBox.x).toBeGreaterThanOrEqual(8)
    expect(moreMenuBox.y).toBeGreaterThanOrEqual(8)
    await moreActions.click()
    expect(state.modelQueries).toHaveLength(0)
    await deepSeekRow.getByRole('button', { name: '编辑', exact: true }).click()
    expect(state.modelQueries).toHaveLength(0)
    await expect(modal(page).getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(
      modal(page).getByRole('checkbox', { name: '启用 DeepSeek V4 Flash 映射' }),
    ).toBeChecked()
    const existingProviderModelCode = modal(page).getByLabel('DeepSeek V4 Flash 的服务商模型编码')
    await expect(existingProviderModelCode).toHaveValue('deepseek-v4-flash')
    await expect(modal(page).getByText('Claude Sonnet', { exact: true })).toHaveCount(0)
    await expect(modal(page).getByText('DeepSeek V3 Legacy', { exact: true })).toHaveCount(0)
    await expect(
      modal(page).getByRole('checkbox', { name: '启用 DeepSeek V3 Legacy 映射' }),
    ).toHaveCount(0)
    await expect(modal(page).getByLabel('DeepSeek V3 Legacy 的服务商模型编码')).toHaveCount(0)
    await expect(modal(page).getByText('模型列表', { exact: true })).toBeVisible()
    await expect(modal(page).getByText('已启用模型映射', { exact: true })).toHaveCount(0)
    await expect(modal(page).getByText('模型名称', { exact: true })).toHaveCount(1)
    await expect(modal(page).getByText('服务商编码', { exact: true })).toBeVisible()
    await expect(modal(page).locator('.mapping-control > span')).toHaveCount(0)
    const editDialog = modal(page)
    const dialogBody = editDialog.locator('.modal-body')
    const dialogHeader = editDialog.locator('.modal-head')
    const dialogFooter = editDialog.locator('.modal-footer')
    const configTabs = editDialog.locator('.config-tabs')
    await expect(dialogFooter).toBeVisible()
    expect(await configTabs.evaluate((element) => getComputedStyle(element).overflowY)).toBe(
      'hidden',
    )
    await page.setViewportSize({ width: 1200, height: 600 })
    expect(
      await dialogBody.evaluate((element) => element.scrollHeight > element.clientHeight),
    ).toBe(true)
    const headerTop = (await dialogHeader.boundingBox())!.y
    const footerTop = (await dialogFooter.boundingBox())!.y
    await dialogBody.evaluate((element) => element.scrollTo(0, element.scrollHeight))
    expect((await dialogHeader.boundingBox())!.y).toBe(headerTop)
    expect((await dialogFooter.boundingBox())!.y).toBe(footerTop)
    await page.setViewportSize({ width: 1200, height: 900 })
    await modal(page).getByLabel('官方网站').fill('https://www.deepseek.com/')
    await existingProviderModelCode.fill('')
    await modal(page).getByRole('button', { name: '保存', exact: true }).click()
    expect(state.providers[0].website).toBe('https://www.deepseek.com')
    expect(state.providerMappings.get('81')).toHaveLength(1)
    expect(state.providerMappings.get('81')?.[0].upstreamModelCode).toBe('')
    await expect(page.getByRole('columnheader', { name: '网站', exact: true })).toHaveCount(0)
    const websiteLink = deepSeekRow.getByRole('link', {
      name: '在新页面打开 深度求索 官网',
      exact: true,
    })
    await expect(websiteLink).toHaveAttribute('href', 'https://www.deepseek.com')
    await expect(websiteLink).toHaveAttribute('target', '_blank')
    await expect(websiteLink.locator('svg')).toHaveCount(1)
    await deepSeekRow.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(modal(page).getByLabel('DeepSeek V4 Flash 的服务商模型编码')).toHaveValue('')
    await modal(page).getByRole('button', { name: '取消', exact: true }).click()

    await providerSearch.getByRole('button', { name: '创建服务商', exact: true }).click()
    await providerSearch
      .getByRole('menu', { name: '创建服务商' })
      .getByRole('menuitem', { name: /三方服务商/ })
      .click()
    expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
    const dialog = modal(page)
    const providerFieldRows = dialog.locator('.connection-fields .provider-field-row')
    await expect(providerFieldRows).toHaveCount(4)
    await expect(providerFieldRows.locator('.provider-field-label')).toHaveText([
      '服务商名称',
      '官方网站',
      'OpenAI 协议地址',
      'Anthropic 协议地址',
    ])
    await expect(dialog.getByText('基础配置', { exact: true })).toHaveCount(0)
    await expect(
      dialog.getByText(
        '请至少填写一个协议地址：OpenAI 或 Anthropic。公网服务必须使用 HTTPS，私有网络服务可以使用 HTTP 或 HTTPS。',
        { exact: true },
      ),
    ).toBeVisible()
    await expect(dialog.locator('.connection-fields')).toHaveCSS('grid-template-columns', /^\S+$/)
    await expect(providerFieldRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
    await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await expect(dialog.getByRole('button', { name: '添加映射', exact: true })).toHaveCount(0)
    await expect(dialog.getByText('模型映射', { exact: true })).toHaveCount(0)
    const mappingCatalogHead = dialog.locator('.mapping-catalog-pane .mapping-pane-head')
    const selectAllMappings = mappingCatalogHead.getByRole('button', {
      name: '全选',
      exact: true,
    })
    const invertMappings = mappingCatalogHead.getByRole('button', {
      name: '反选',
      exact: true,
    })
    await expect(mappingCatalogHead.getByText('模型列表', { exact: true })).toBeVisible()
    await expect(mappingCatalogHead.locator('.mapping-count-tag')).toHaveText(/\d+/)
    await expect(mappingCatalogHead.locator('.mapping-action-separator')).toHaveText('/')
    const mappingCheckbox = dialog.getByRole('checkbox', {
      name: '启用 DeepSeek V4 Flash 映射',
    })
    const claudeMappingCheckbox = dialog.getByRole('checkbox', {
      name: '启用 Claude Sonnet 映射',
    })
    await expect(dialog.getByText('尚未启用模型，请从左侧模型列表中勾选。')).toBeVisible()
    const mappingSearch = dialog.getByRole('searchbox', { name: '搜索模型映射' })
    await expect(mappingSearch).toHaveAttribute(
      'placeholder',
      '搜索系统模型名称、编码或服务商模型编码',
    )
    await expect(dialog.locator('.mapping-search-box > svg')).toHaveCount(0)
    await mappingSearch.fill('Claude')
    await expect(mappingCheckbox).toHaveCount(0)
    await expect(claudeMappingCheckbox).toBeVisible()
    await expect(dialog.getByText('显示 1 个', { exact: true })).toBeVisible()
    await mappingSearch.fill('不存在的模型')
    await expect(dialog.locator('.mapping-filter-empty')).toContainText('没有匹配的模型。')
    await dialog.getByRole('button', { name: '清除筛选', exact: true }).click()
    await expect(mappingSearch).toHaveValue('')
    await mappingSearch.fill('DeepSeek V4 Flash')
    await selectAllMappings.click()
    await expect(mappingCheckbox).toBeChecked()
    await expect(claudeMappingCheckbox).toHaveCount(0)
    await dialog.getByRole('button', { name: '清空模型映射搜索', exact: true }).click()
    await expect(claudeMappingCheckbox).toBeVisible()
    await expect(claudeMappingCheckbox).not.toBeChecked()
    await expect(mappingCheckbox).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(mappingCheckbox).toHaveCSS('border-top-style', 'none')
    const mappingList = dialog.locator('.mapping-catalog-list')
    await expect(mappingList).toHaveCSS('overflow-y', 'scroll')
    expect(
      await mappingList.evaluate(
        (element) => getComputedStyle(element, '::-webkit-scrollbar').width,
      ),
    ).toBe('12px')
    await expect(dialog.locator('.modal-body')).toHaveCSS('overflow-y', 'hidden')
    await expect(dialog.locator('.mapping-catalog-row').first()).toHaveCSS('min-height', '52px')
    const providerModelCode = dialog.getByLabel('DeepSeek V4 Flash 的服务商模型编码')
    await selectAllMappings.click()
    await expect(mappingCheckbox).toBeChecked()
    await expect(claudeMappingCheckbox).toBeChecked()
    await invertMappings.click()
    await expect(mappingCheckbox).not.toBeChecked()
    await expect(claudeMappingCheckbox).not.toBeChecked()
    await expect(providerModelCode).toHaveCount(0)
    await mappingCheckbox.check()
    await expect(mappingCheckbox).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(mappingCheckbox).toHaveCSS('border-top-style', 'none')
    await expect(selectAllMappings).toBeEnabled()
    await expect(invertMappings).toBeEnabled()
    await expect(mappingCheckbox).toBeVisible()
    await expect(claudeMappingCheckbox).toBeVisible()
    await expect(providerModelCode).toBeEnabled()
    await expect(providerModelCode).toHaveValue('')
    await expect(providerModelCode).toHaveAttribute('placeholder', '默认为 deepseek-v4-flash')
    await mappingSearch.fill('Claude')
    await expect(mappingCheckbox).toHaveCount(0)
    await expect(providerModelCode).toBeVisible()
    await dialog.getByRole('button', { name: '清空模型映射搜索', exact: true }).click()
    await providerModelCode.fill('deepseek-v4-flash')
    await providerModelCode.blur()
    await expect(providerModelCode).toHaveValue('deepseek-v4-flash')
    await dialog.getByRole('button', { name: '移除 DeepSeek V4 Flash 映射' }).click()
    await expect(mappingCheckbox).not.toBeChecked()
    await expect(providerModelCode).toHaveCount(0)
    await dialog.getByLabel('服务商名称').fill('自定义百炼')
    await dialog.getByLabel('官方网站').fill('https://www.aliyun.com')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('至少填写一种协议接口地址')
    await expect(dialog.locator('.alert.error')).toHaveCount(0)
    await dialog
      .getByLabel('OpenAI 协议地址')
      .fill('http://dashscope.aliyuncs.com/compatible-mode/v1')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('公网接口必须使用 HTTPS')
    await expect(dialog.locator('.alert.error')).toHaveCount(0)
    const openAINetworkScope = dialog.getByLabel('OpenAI 网络位置', { exact: true })
    await openAINetworkScope.selectOption('PRIVATE')
    await expect(dialog.getByText(/私有网络地址可以访问 Zentrola 所在网络/)).toBeVisible()
    await expect(dialog.getByLabel('OpenAI 协议地址')).toHaveAttribute(
      'placeholder',
      'http://192.168.1.20:8080/v1',
    )
    await openAINetworkScope.selectOption('PUBLIC')
    await dialog
      .getByLabel('OpenAI 协议地址')
      .fill('https://dashscope.aliyuncs.com/compatible-mode/v1')
    await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
    const proxySwitch = dialog.getByRole('switch', { name: '启用代理', exact: true })
    await expect(dialog.locator('.proxy-switch-row')).toHaveCSS('grid-template-columns', /\S+ \S+/)
    await expect(dialog.locator('.proxy-switch-row .provider-field-label')).toHaveCSS(
      'text-align',
      'left',
    )
    await expect(
      dialog.getByText('仅该服务商的连接测试和模型调用使用此代理，默认直连。', {
        exact: true,
      }),
    ).toHaveCount(0)
    await expect(proxySwitch).toHaveAttribute(
      'title',
      '仅该服务商的连接测试和模型调用使用此代理，默认直连。',
    )
    const proxySwitchBoxBeforeEnable = await proxySwitch.boundingBox()
    const providerNameLabelBox = await dialog
        .locator('label[for="provider-name-input"]')
        .boundingBox(),
      providerNameInputBox = await dialog.getByLabel('服务商名称', { exact: true }).boundingBox()
    expect(proxySwitchBoxBeforeEnable).not.toBeNull()
    expect(providerNameLabelBox).not.toBeNull()
    expect(providerNameInputBox).not.toBeNull()
    expect(Math.abs(proxySwitchBoxBeforeEnable!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
    await proxySwitch.check()
    await expect(dialog.locator('.proxy-url-row')).toHaveCSS('grid-template-columns', /\S+ \S+/)
    await expect(dialog.locator('.proxy-url-row .provider-field-label')).toHaveCSS(
      'text-align',
      'left',
    )
    await expect(
      dialog.getByText(
        '支持 http:// 或 https://，可直接包含用户名和密码；完整地址将加密保存，之后只显示脱敏值。',
        { exact: true },
      ),
    ).toHaveCount(0)
    const proxyUrlCredentialHint = dialog.getByRole('img', {
      name: '用户名和密码将加密保存',
      exact: true,
    })
    await expect(proxyUrlCredentialHint).toHaveAttribute('tabindex', '0')
    await expect(proxyUrlCredentialHint).toHaveAttribute('title', '用户名和密码将加密保存')
    const proxyURLLabelBox = await dialog
        .locator('.proxy-url-row .provider-field-label')
        .boundingBox(),
      proxyURLControlBox = await dialog.locator('.proxy-url-control').boundingBox(),
      proxyProtocolBox = await dialog.getByLabel('代理协议', { exact: true }).boundingBox(),
      proxyURLInputBox = await dialog.getByLabel('代理服务器地址', { exact: true }).boundingBox()
    expect(proxyURLLabelBox).not.toBeNull()
    expect(proxyURLControlBox).not.toBeNull()
    expect(proxyProtocolBox).not.toBeNull()
    expect(proxyURLInputBox).not.toBeNull()
    expect(Math.abs(proxyURLLabelBox!.width - providerNameLabelBox!.width)).toBeLessThanOrEqual(1)
    expect(Math.abs(proxyURLControlBox!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(proxyProtocolBox!.x - providerNameInputBox!.x)).toBeLessThanOrEqual(1)
    expect(proxyURLInputBox!.x).toBeGreaterThan(proxyProtocolBox!.x + proxyProtocolBox!.width)
    const proxySwitchBox = await proxySwitch.boundingBox()
    expect(proxySwitchBox).not.toBeNull()
    expect(Math.abs(proxySwitchBox!.x - proxySwitchBoxBeforeEnable!.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(proxySwitchBox!.x - proxyURLControlBox!.x)).toBeLessThanOrEqual(1)
    await dialog
      .getByLabel('代理服务器地址', { exact: true })
      .fill('http://proxy-user:proxy-password@proxy.example.com:8080')
    const proxyHeaderHint = dialog.getByText('Header Value 加密保存，且只发送给代理服务器。', {
      exact: true,
    })
    const addProxyHeaderButton = dialog.getByRole('button', { name: '添加 Header', exact: true })
    await expect(dialog.locator('.proxy-headers-head > div')).toHaveCSS('display', 'flex')
    const proxyHeaderHintBox = await proxyHeaderHint.boundingBox()
    const addProxyHeaderButtonBox = await addProxyHeaderButton.boundingBox()
    expect(proxyHeaderHintBox).not.toBeNull()
    expect(addProxyHeaderButtonBox).not.toBeNull()
    expect(
      Math.abs(
        proxyHeaderHintBox!.y +
          proxyHeaderHintBox!.height / 2 -
          (addProxyHeaderButtonBox!.y + addProxyHeaderButtonBox!.height / 2),
      ),
    ).toBeLessThanOrEqual(3)
    await addProxyHeaderButton.click()
    await expect(dialog.locator('.proxy-header-field')).toHaveCount(2)
    await expect(dialog.locator('.proxy-header-field').first()).toHaveCSS(
      'grid-template-columns',
      /\S+ \S+/,
    )
    const proxyHeaderBox = await dialog.locator('.proxy-header-field').first().boundingBox()
    expect(proxyHeaderBox).not.toBeNull()
    expect(Math.abs(proxyHeaderBox!.x - proxyURLControlBox!.x)).toBeLessThanOrEqual(1)
    await dialog.getByLabel('KEY', { exact: true }).fill('X-Proxy-Token')
    await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveAttribute('type', 'text')
    await dialog.getByLabel('VALUE', { exact: true }).fill('proxy-header-secret')
    await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveValue('proxy-header-secret')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)

    const created = state.providers.find((provider) => provider.name === '自定义百炼')
    expect(created.status).toBe('DISABLED')
    expect(state.providerMappings.get(created.id)).toEqual([])

    const row = page.getByRole('row').filter({ hasText: '自定义百炼' })
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(dialog.getByRole('tab', { name: '模型配置', exact: true })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    await mappingCheckbox.check()
    await providerModelCode.fill('aliyun-deepseek-v4-flash')
    await dialog.screenshot({ path: '../.cache/web-visual/provider-model-mappings.png' })
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)

    expect(state.providerInputs.at(-2)).toEqual(
      expect.objectContaining({
        endpoints: [
          {
            protocolType: 'OPENAI',
            baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
            networkScope: 'PUBLIC',
          },
        ],
        proxyEnabled: true,
        proxyUrl: 'http://proxy-user:proxy-password@proxy.example.com:8080',
        proxyHeaders: [{ key: 'X-Proxy-Token', value: 'proxy-header-secret' }],
      }),
    )
    expect(state.providerInputs.at(-1)?.mappings).toEqual([
      {
        modelId: '71',
        upstreamModelCode: 'aliyun-deepseek-v4-flash',
      },
    ])
    expect(created.proxyUrl).not.toContain('proxy-password')
    expect(state.providerMappings.get(created.id)).toEqual([
      expect.objectContaining({
        modelId: '71',
        upstreamModelCode: 'aliyun-deepseek-v4-flash',
      }),
    ])
    const providerNameCellBox = await row.getByRole('cell').nth(1).boundingBox()
    expect(providerNameCellBox).not.toBeNull()
    expect(providerNameCellBox!.width).toBeGreaterThanOrEqual(245)
    const endpoint = row.locator('.endpoint')
    await expect(endpoint).toHaveText('https://dashscope.aliyuncs.com/compatible-mode/v1')
    await expect(endpoint).toHaveAttribute(
      'title',
      'https://dashscope.aliyuncs.com/compatible-mode/v1',
    )
    await expect(endpoint.locator('.technical-value-text')).toHaveCSS('text-overflow', 'ellipsis')
    await expect(endpoint.locator('.technical-value-text')).toHaveCSS('white-space', 'nowrap')
    await endpoint.getByRole('button', { name: '复制', exact: true }).click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe('https://dashscope.aliyuncs.com/compatible-mode/v1')
    await expect(row.getByText('OpenAI', { exact: true })).toBeVisible()
    await expect(row.locator('.endpoint-protocol')).toHaveText(['OpenAI'])
    await expect(row.getByText('Anthropic', { exact: true })).toHaveCount(0)
    const proxyCellBox = await row.getByRole('cell').nth(4).boundingBox()
    const endpointCellBox = await row.getByRole('cell').nth(5).boundingBox()
    expect(proxyCellBox).not.toBeNull()
    expect(endpointCellBox).not.toBeNull()
    expect(proxyCellBox!.width).toBeLessThan(80)
    expect(endpointCellBox!.width).toBeGreaterThan(proxyCellBox!.width * 3)
    const enabledStatusBox = await row.getByRole('switch').boundingBox()
    const runtimeStatusBox = await row.locator('.provider-runtime-state').boundingBox()
    expect(enabledStatusBox).not.toBeNull()
    expect(runtimeStatusBox).not.toBeNull()
    expect(
      runtimeStatusBox!.x - (enabledStatusBox!.x + enabledStatusBox!.width),
    ).toBeGreaterThanOrEqual(16)
    await expect(row.locator('.provider-runtime .subline')).toHaveCount(0)
    await expect(row.locator('.provider-runtime-state')).toHaveAttribute(
      'title',
      '查看 自定义百炼 的凭证，当前运行状态：未配置：尚未配置服务商凭证',
    )
    await expect(
      row.getByRole('img', { name: '自定义百炼 已启用代理访问', exact: true }),
    ).toBeVisible()
    const createdStatus = row.getByRole('switch', { name: '自定义百炼的启用状态' })
    await expect(createdStatus).toHaveText('')
    await expect(createdStatus).toBeDisabled()
    await expect(createdStatus).toHaveAttribute('title', '请先配置凭证。')
    const missingCredential = row.getByRole('button', {
      name: '管理 自定义百炼 的认证凭据',
      exact: true,
    })
    await expect(missingCredential).toHaveText('凭证')
    await missingCredential.click()
    await expect(dialog.getByText('暂无认证凭据', { exact: true })).toBeVisible()
    await dialog.getByRole('button', { name: '新增凭据', exact: true }).click()
    await expect(dialog).not.toHaveClass(/medium/)
    const credentialFormRows = dialog.locator('.credential-form-row')
    await expect(credentialFormRows).toHaveCount(2)
    await expect(credentialFormRows.locator('.credential-form-label')).toHaveText([
      '认证方式',
      'API Key',
    ])
    await expect(credentialFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
    const apiKeyInput = dialog.getByLabel('API Key', { exact: true })
    await expect(apiKeyInput).toHaveAttribute('type', 'text')
    await apiKeyInput.fill('aliyun-fixture-credential')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(dialog.getByRole('row').filter({ hasText: '自定义百炼 API Key' })).toBeVisible()
    await expect(page.locator('.toast-success')).toContainText('已保存')
    await dialog.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    await expect(
      row.getByRole('button', { name: '同步 自定义百炼 的官方模型', exact: true }),
    ).toHaveCount(0)
    expect(state.modelSyncRequests).toHaveLength(0)
    await expect(
      row.getByRole('button', {
        name: '管理 自定义百炼 的认证凭据',
        exact: true,
      }),
    ).toHaveText('凭证')
    await expect(createdStatus).toBeEnabled()
    await expect(createdStatus).not.toHaveAttribute('title')
    await expect(
      row.getByRole('button', { name: '验证 自定义百炼 的模型调用', exact: true }),
    ).toHaveCount(0)
    await row
      .getByRole('button', {
        name: '管理 自定义百炼 的认证凭据',
        exact: true,
      })
      .click()
    const createdCredentialRow = modal(page)
      .getByRole('row')
      .filter({ hasText: '自定义百炼 API Key' })
    const testCredential = createdCredentialRow.getByRole('button', {
      name: '验证 自定义百炼 API Key 的可用性',
      exact: true,
    })
    await expect(testCredential).toBeVisible()
    await testCredential.click()
    await expect(modal(page).getByRole('status')).toContainText('模型调用验证通过')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    expect(state.resources.find((resource) => resource.providerId === created.id)?.status).toBe(
      'ACTIVE',
    )
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    await dialog.getByRole('tab', { name: '代理配置', exact: true }).click()
    await expect(dialog.getByLabel('代理服务器地址', { exact: true })).not.toHaveValue(
      /proxy-password/,
    )
    await expect(dialog.getByLabel('KEY', { exact: true })).toHaveValue('X-Proxy-Token')
    await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveValue('')
    await expect(dialog.getByLabel('VALUE', { exact: true })).toHaveAttribute(
      'placeholder',
      '已配置；留空保持不变',
    )
    await dialog.screenshot({ path: '../.cache/web-visual/provider-proxy-config.png' })
    await dialog.getByLabel('服务商名称').fill('阿里云模型服务')
    state.conflict(true)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    const providerErrorToast = page.locator('.toast')
    await expect(providerErrorToast).toContainText('操作冲突')
    await expect(providerErrorToast).toBeVisible()
    await expect(dialog.locator('.alert.error')).toHaveCount(0)
    expect(
      await providerErrorToast.evaluate((element) =>
        element.closest('[popover]')?.matches(':popover-open'),
      ),
    ).toBe(true)
    state.conflict(false)
    await providerErrorToast.getByRole('button', { name: '关闭' }).click()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: '阿里云模型服务' })).toBeVisible()

    const renamedStatus = page.getByRole('switch', { name: '阿里云模型服务的启用状态' })
    await renamedStatus.click()
    await page
      .getByRole('dialog', { name: '阿里云模型服务 / 选择启用时验证的连接' })
      .getByRole('button', { name: '测试并启用' })
      .click()
    await expect(renamedStatus).toBeChecked()
    expect(created.status).toBe('ACTIVE')
    await modal(page).getByRole('button', { name: '关闭', exact: true }).last().click()
    await expect(modal(page)).toHaveCount(0)
    await mkdir('../.cache/web-visual', { recursive: true })
    await renamedStatus.click()
    await expect(renamedStatus).not.toBeChecked()
    expect(created.status).toBe('DISABLED')
    await expect(modal(page)).toHaveCount(0)
    await page.screenshot({
      path: '../.cache/web-visual/provider-catalog-desktop.png',
      fullPage: true,
      animations: 'disabled',
    })

    await page.setViewportSize({ width: 900, height: 900 })
    await expect(providerLink.locator('span')).toBeHidden()
    await expect
      .poll(() => page.locator('.sidebar').evaluate((element) => element.clientWidth))
      .toBe(76)

    await page.setViewportSize({ width: 1200, height: 900 })
    const renamedRow = page.getByRole('row').filter({ hasText: '阿里云模型服务' })
    await renamedRow.locator('summary').click()
    await renamedRow.getByRole('button', { name: '删除', exact: true }).click()
    await expect(modal(page)).toContainText('确认删除服务商「阿里云模型服务」')
    await modal(page).screenshot({ path: '../.cache/web-visual/delete-confirm-dialog.png' })
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: '阿里云模型服务' })).toHaveCount(0)
    expect(state.providerMappings.has(created.id)).toBe(false)
    expect(state.resources.some((resource) => resource.providerId === created.id)).toBe(false)
  })

  test('服务商列表展示凭证聚合运行状态和错误原因', async ({ page }) => {
    const state = await fixture(page)
    state.resources.push({
      id: '88',
      providerId: '81',
      name: 'DeepSeek API Key',
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
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()

    await expect(page.getByRole('columnheader', { name: '运行状态', exact: true })).toBeVisible()
    const row = page.getByRole('row').filter({ hasText: 'DeepSeek' })
    await expect(row.getByText('不可用', { exact: true })).toBeVisible()
    await expect(row).not.toContainText('余额或计费异常 · HTTP 402')
    await expect(row.locator('.provider-runtime-state')).toHaveAttribute(
      'title',
      '查看 DeepSeek 的凭证，当前运行状态：不可用：余额或计费异常 · HTTP 402 · UPSTREAM_BILLING_BLOCKED',
    )
    await row.getByRole('button', { name: /查看 DeepSeek 的凭证/ }).click()
    await expect(modal(page).getByRole('heading', { name: '凭证配置' })).toBeVisible()
    await expect(modal(page).locator('.credential-title-line')).toContainText('DeepSeek')
    await expect(modal(page).locator('.credential-title-line')).toContainText('启用')
    await expect(modal(page).locator('.credential-overview dt')).toHaveText(['Base URL'])
    await expect(modal(page).locator('.credential-overview-endpoints dd > span')).toHaveCount(2)
    await expect(modal(page).locator('.credential-overview')).toContainText(
      'https://api.deepseek.com',
    )
    await expect(
      modal(page).getByRole('button', { name: '验证并恢复 DeepSeek API Key', exact: true }),
    ).toBeVisible()
    await expect(modal(page).getByRole('button', { name: '保存', exact: true })).toHaveCount(0)
    await expect(modal(page).locator('.modal-footer')).toHaveCount(0)
    await modal(page).screenshot({ path: '../.cache/web-visual/provider-credential-config.png' })
    const credentialRow = modal(page).getByRole('row').filter({ hasText: 'DeepSeek API Key' })
    await expect(credentialRow).not.toContainText('••••••••')
    await expect(credentialRow).toContainText('最后验证时间')
    await expect(modal(page).getByRole('columnheader')).toHaveText([
      '认证凭据',
      '认证方式',
      '运行状态',
      '操作',
    ])
    const runtimeCell = credentialRow.getByRole('cell').nth(2)
    await expect(runtimeCell).toContainText('不可用')
    await expect(runtimeCell).toContainText('余额或计费异常 · HTTP 402')
    await expect(runtimeCell).toContainText('UPSTREAM_BILLING_BLOCKED')
    const credentialList = modal(page).locator('.credential-list')
    expect(
      await credentialList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true)

    await page.setViewportSize({ width: 560, height: 800 })
    await expect(credentialRow).toHaveCSS('display', 'grid')
    expect(
      await credentialList.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true)
    await modal(page).screenshot({
      path: '../.cache/web-visual/provider-credential-config-mobile.png',
    })
  })
})
