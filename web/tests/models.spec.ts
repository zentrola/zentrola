import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal, expectOnlyModalBodyScrolls } from './mock/helpers'

test.describe('模型', () => {
  test('模型新增编辑、模态校验、冲突恢复和窄屏表单', async ({ page }) => {
    const state = await fixture(page)
    await signIn(page)
    await page.getByRole('link', { name: '模型', exact: true }).click()
    await expect.poll(() => state.providerQueries.at(-1)?.get('type')).toBe('OFFICIAL')
    await expect(page.getByRole('columnheader')).toHaveText([
      'ID',
      '模型名称',
      '模型厂商',
      '启用状态',
      '输入类型',
      '输出类型',
      '备注',
      '操作',
    ])
    const existingModelRow = page.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
    await expect(existingModelRow.locator('.person strong')).toHaveText('DeepSeek V4 Flash')
    await expect(existingModelRow.locator('.person small')).toHaveText('deepseek-v4-flash')
    await expect(existingModelRow.getByRole('cell')).toHaveCount(8)
    await expect(existingModelRow.getByRole('cell').first().locator('code')).toHaveText('71')
    await expect(
      existingModelRow.getByRole('cell').first().locator('.technical-value-copy'),
    ).toHaveCount(0)
    await expect(existingModelRow.getByRole('cell').nth(2)).toHaveText('DeepSeek')
    const publisherFilter = page.getByRole('combobox', { name: '模型厂商', exact: true })
    await expect(publisherFilter.locator('..')).toHaveCSS('flex-direction', 'row')
    await expect(publisherFilter.locator('option')).toHaveText(['全部', 'DeepSeek'])
    await publisherFilter.selectOption('81')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.locator('tbody tr')).toHaveCount(1)
    await expect(existingModelRow).toBeVisible()
    await expect(page.getByRole('row').filter({ hasText: 'Claude Sonnet' })).toHaveCount(0)
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(publisherFilter).toHaveValue('')
    await expect(page.locator('tbody tr')).toHaveCount(2)
    await page.getByRole('button', { name: '添加模型' }).click()
    const dialog = modal(page)
    const modelFormRows = dialog.locator('.model-form-row')
    await expect(modelFormRows).toHaveCount(6)
    await expect(modelFormRows.locator('.model-form-label')).toHaveText([
      '模型编码',
      '模型名称',
      '模型厂商',
      '输入类型',
      '输出类型',
      '备注',
    ])
    await expect(modelFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
    await dialog.getByLabel('模型名称', { exact: true }).fill('测试官方模型')
    await dialog.getByLabel('模型编码', { exact: true }).fill('official-test-v1')
    await dialog.getByLabel('模型厂商', { exact: true }).selectOption('81')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('至少选择一项')
    await expect(dialog.locator('.alert.error')).toHaveCount(0)
    await dialog
      .getByRole('group', { name: '输入类型' })
      .getByLabel('文本', { exact: true })
      .check()
    await dialog
      .getByRole('group', { name: '输入类型' })
      .getByLabel('图片', { exact: true })
      .check()
    await dialog
      .getByRole('group', { name: '输出类型' })
      .getByLabel('文本', { exact: true })
      .check()
    await dialog.getByLabel('备注', { exact: true }).fill('图文理解用途')
    state.conflict(true)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('操作冲突')
    await expect(dialog.getByLabel('模型编码', { exact: true })).toHaveValue('official-test-v1')
    state.conflict(false)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    const row = page.getByRole('row').filter({ hasText: 'official-test-v1' })
    await expect(row).not.toContainText('多模态')
    await expect(row.getByText('图片', { exact: true })).toBeVisible()
    await expect(row.getByText('文本', { exact: true })).toHaveCount(2)
    await expect(row).toContainText('图文理解用途')
    const created = state.models.find((model) => model.code === 'official-test-v1')
    expect(created.inputModalities).toEqual(['TEXT', 'IMAGE'])
    expect(created.outputModalities).toEqual(['TEXT'])
    expect(created.status).toBe('DISABLED')
    expect(created.publisherProviderId).toBe('81')
    expect(created.publisherProviderName).toBe('DeepSeek')
    await expect(row.getByRole('cell').nth(2)).toHaveText('DeepSeek')
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(dialog.getByLabel('发布方编码', { exact: true })).toHaveCount(0)
    await expect(dialog.getByLabel('模型厂商', { exact: true })).toHaveValue('81')
    await mkdir('../.cache/web-visual', { recursive: true })
    await dialog.screenshot({ path: '../.cache/web-visual/model-edit-desktop.png' })
    await expect(
      dialog.getByRole('group', { name: '输入类型' }).getByLabel('图片', { exact: true }),
    ).toBeChecked()
    await dialog.getByLabel('模型编码', { exact: true }).fill('official-test-v2')
    await expect(dialog.getByRole('status')).toContainText('客户端需要使用新编码')
    await dialog
      .getByRole('group', { name: '输出类型' })
      .getByLabel('音频', { exact: true })
      .check()
    await page.setViewportSize({ width: 390, height: 480 })
    await expectOnlyModalBodyScrolls(dialog)
    await expect(
      dialog.locator('.modal-footer').getByRole('button', { name: '保存' }),
    ).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeVisible()
    expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({ path: '../.cache/web-visual/model-edit-mobile.png' })
    await dialog.getByLabel('模型厂商', { exact: true }).selectOption('')
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    expect(created.code).toBe('official-test-v2')
    expect(created.status).toBe('DISABLED')
    expect(created.outputModalities).toEqual(['TEXT', 'AUDIO'])
    expect(created.publisherProviderId).toBeNull()
    expect(created.publisherProviderName).toBeNull()
    await page.setViewportSize({ width: 1440, height: 1000 })
    const modelSearch = page.getByRole('searchbox', { name: '模型名称' })
    await expect(modelSearch).toHaveAttribute('placeholder', '请输入模型名称')
    await expect(page.locator('.list-search-label')).toHaveText('模型名称')
    const modelQueryCount = state.modelQueries.length
    await modelSearch.fill('图文理解用途')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    expect(state.modelQueries).toHaveLength(modelQueryCount + 1)
    await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(1)
    await page.screenshot({ path: '../.cache/web-visual/model-catalog-desktop.png' })
    const updatedRow = page.getByRole('row').filter({ hasText: 'official-test-v2' })
    await expect(updatedRow.getByRole('cell').nth(2)).toHaveText('-')
    await updatedRow.locator('summary').click()
    await updatedRow.getByRole('button', { name: '删除', exact: true }).click()
    const deleteDialog = modal(page)
    await expect(deleteDialog).toContainText('相关服务商映射和分组授权将同时失效')
    await deleteDialog.getByRole('button', { name: '删除', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: 'official-test-v2' })).toHaveCount(0)
    expect(state.models.some((model) => model.id === created.id)).toBe(false)
  })

  test('模型页面配置服务商优先级，编辑服务商不重置配置', async ({ page }) => {
    const state = await fixture(page)
    state.providerMappings.get('81')![0].priority = 25
    await signIn(page)
    await page.getByRole('link', { name: '服务商', exact: true }).click()
    const edit = page.getByRole('row').filter({ hasText: 'DeepSeek' }).getByRole('button', {
      name: '编辑',
    })
    await edit.click()
    const dialog = modal(page)
    await expect(dialog.getByText('路由优先级')).toHaveCount(0)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    expect(state.providerInputs.at(-1)?.mappings[0].priority).toBeUndefined()
    expect(state.providerMappings.get('81')![0].priority).toBe(25)
    state.resources.push({ providerId: '81' })

    await page.getByRole('link', { name: '模型', exact: true }).click()
    const providerTrigger = page
      .getByRole('row')
      .filter({ hasText: 'DeepSeek V4 Flash' })
      .getByRole('button', { name: 'DeepSeek V4 Flash 的服务商优先级' })
    await expect(providerTrigger).toHaveText('服务提供商')
    await providerTrigger.click()
    const priorityDialog = page.getByRole('dialog', { name: 'DeepSeek V4 Flash 的服务商优先级' })
    await expect(priorityDialog.getByRole('columnheader')).toHaveText(['服务商', '状态', '优先级'])
    await expect(
      priorityDialog.getByRole('row').filter({ hasText: 'DeepSeek' }).getByRole('cell').nth(1),
    ).toHaveText('正常')
    const priority = priorityDialog.getByRole('textbox', { name: 'DeepSeek 的优先级' })
    await expect(priority).toHaveValue('25')
    await priority.fill('0')
    await priorityDialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(priorityDialog.getByRole('alert')).toContainText('优先级必须是 1 到 100 的整数')
    await priority.fill('5')
    state.conflict(true)
    await priorityDialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(priorityDialog.getByRole('alert')).toBeVisible()
    state.conflict(false)
    await priorityDialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(priorityDialog).toHaveCount(0)
    expect(state.providerMappings.get('81')![0].priority).toBe(5)
    await expect(providerTrigger).toHaveText('服务提供商')
  })

  test('模型编码旁显示服务商入口，点击后才读取映射', async ({ page }) => {
    const state = await fixture(page)
    let priorityReads = 0
    await page.route('**/api/v1/models/*/provider-priorities', async (route) => {
      if (route.request().method() === 'GET') priorityReads++
      await route.fallback()
    })
    state.providers.push({
      ...state.providers[0],
      id: '82',
      name: '备用服务商',
      status: 'DISABLED',
    })
    state.providerMappings.set('82', [
      {
        id: '94',
        providerId: '82',
        modelId: '71',
        upstreamModelCode: 'deepseek-v4-flash',
        priority: 10,
        createdAt: stamp,
        updatedAt: stamp,
      },
    ])
    await signIn(page)
    await page.getByRole('link', { name: '模型', exact: true }).click()
    const row = page.getByRole('row').filter({ hasText: 'DeepSeek V4 Flash' })
    const trigger = row.getByRole('button', { name: 'DeepSeek V4 Flash 的服务商优先级' })
    await expect(trigger).toHaveText('服务提供商')
    expect(priorityReads).toBe(0)
    await trigger.click()
    const dialog = page.getByRole('dialog', { name: 'DeepSeek V4 Flash 的服务商优先级' })
    await expect(dialog.getByRole('row').filter({ hasText: '备用服务商' })).toContainText(
      '服务商禁用',
    )
    await expect(dialog.getByRole('textbox', { name: '备用服务商 的优先级' })).toHaveValue('10')
    expect(priorityReads).toBe(1)
    await dialog.getByRole('button', { name: '关闭' }).click()
    await expect(
      page
        .getByRole('row')
        .filter({ hasText: 'Claude Sonnet' })
        .getByRole('button', { name: 'Claude Sonnet 的服务商优先级' }),
    ).toHaveText('服务提供商')
  })

  test('切换模型时忽略前一个优先级弹层的延迟响应', async ({ page }) => {
    await fixture(page)
    await page.route('**/api/v1/models/71/provider-priorities', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 400))
      await route.fallback()
    })
    await signIn(page)
    await page.getByRole('link', { name: '模型', exact: true }).click()
    await page
      .getByRole('row')
      .filter({ hasText: 'DeepSeek V4 Flash' })
      .getByRole('button', { name: 'DeepSeek V4 Flash 的服务商优先级' })
      .click()
    await page
      .getByRole('dialog', { name: 'DeepSeek V4 Flash 的服务商优先级' })
      .getByRole('button', { name: '关闭' })
      .click()
    await page
      .getByRole('row')
      .filter({ hasText: 'Claude Sonnet' })
      .getByRole('button', { name: 'Claude Sonnet 的服务商优先级' })
      .click()
    const dialog = page.getByRole('dialog', { name: 'Claude Sonnet 的服务商优先级' })
    await expect(dialog.getByText('当前模型暂无服务商提供服务。')).toBeVisible()
    await page.waitForTimeout(500)
    await expect(dialog.getByText('当前模型暂无服务商提供服务。')).toBeVisible()
    await expect(dialog.getByRole('textbox')).toHaveCount(0)
  })
})
