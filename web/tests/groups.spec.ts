import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('用户分组', () => {
  test('分组编辑表单与创建一致、失败恢复及停用授权限制', async ({ page }) => {
    const state = await fixture(page)
    state.groups.push({
      id: '51',
      code: 'engineering',
      name: '研发组',
      status: 'ACTIVE',
      createdAt: stamp,
    })
    state.models[1].status = 'DISABLED'
    await signIn(page)
    await page.getByRole('link', { name: '用户分组', exact: true }).click()
    await page.getByRole('button', { name: '创建分组' }).click()
    const createDialog = modal(page)
    const createFormRows = createDialog.locator('.group-form-row')
    await expect(createFormRows).toHaveCount(3)
    await expect(createFormRows.locator('.group-form-label')).toHaveText([
      '名称',
      '访问模型',
      '备注',
    ])
    await expect(createFormRows.first()).toHaveCSS('grid-template-columns', /\S+ \S+/)
    await expect(createDialog.getByLabel('名称', { exact: true })).toHaveAttribute('required', '')
    await expect(createFormRows.nth(1).locator('.group-form-label')).toHaveClass(/required-label/)
    await expect(createFormRows.nth(1).locator('.group-model-field')).toHaveAttribute(
      'aria-required',
      'true',
    )
    expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
    await expect(createDialog.getByLabel('备注', { exact: true })).toBeVisible()
    await expect(createDialog.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
    await expect(createDialog.getByText('Claude Sonnet', { exact: true })).toHaveCount(0)
    const createSelectedPane = createDialog.locator('.group-model-selected')
    await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('0')
    await expect(createSelectedPane).toContainText('尚未选择模型，请从模型列表中勾选。')
    const createModelSearch = createDialog.getByRole('searchbox', { name: '搜索模型' })
    await expect(createModelSearch).toHaveAttribute('placeholder', '搜索模型名称、编码或厂商')
    await expect(createDialog.locator('.group-model-search-box > svg')).toHaveCount(0)
    await createModelSearch.fill('deepseek-v4-flash')
    await expect(createDialog.getByText('显示 1 个', { exact: true })).toBeVisible()
    await expect(createDialog.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
    await createModelSearch.fill('不存在的模型')
    await expect(createDialog.getByText('显示 0 个', { exact: true })).toBeVisible()
    await expect(createDialog.locator('.group-model-filter-empty')).toContainText(
      '没有匹配的模型。',
    )
    await createDialog.getByRole('button', { name: '清除筛选', exact: true }).click()
    await expect(createModelSearch).toHaveValue('')
    const createCatalogHead = createDialog.locator('.group-model-catalog .group-model-pane-head')
    await expect(createCatalogHead.getByText('模型列表', { exact: true })).toBeVisible()
    await expect(createCatalogHead.locator('.group-model-count-tag')).toHaveText('1')
    const createBulkActions = createCatalogHead.locator('.group-model-bulk-actions')
    await expect(createBulkActions.getByRole('button', { name: '全选', exact: true })).toBeVisible()
    await expect(createBulkActions.locator('.group-model-action-separator')).toHaveText('/')
    await expect(createBulkActions.getByRole('button', { name: '反选', exact: true })).toBeVisible()
    await expect(createCatalogHead.getByText('名称', { exact: true })).toHaveCount(0)
    await expect(createDialog.locator('.group-model-catalog .group-model-list-head')).toHaveCount(0)
    await expect(createDialog.getByText('deepseek-v4-flash', { exact: true })).toBeVisible()
    const createModelRow = createDialog
      .locator('.group-model-catalog-row')
      .filter({ hasText: 'DeepSeek V4 Flash' })
    await expect(createModelRow.locator('.modality-tag')).toHaveText(['输入：文本', '输出：文本'])
    await createDialog.getByLabel('名称', { exact: true }).fill('必填校验分组')
    await createDialog.getByRole('button', { name: '创建', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('必须选择至少一个访问模型')
    await expect(createDialog.locator('.alert.error')).toHaveCount(0)
    await createDialog.getByRole('button', { name: '关闭', exact: true }).click()
    state.models[1].status = 'ACTIVE'
    await page.getByRole('button', { name: '创建分组' }).click()
    const multimodalRow = modal(page)
      .locator('.group-model-catalog-row')
      .filter({ hasText: 'Claude Sonnet' })
    await expect(multimodalRow.locator('.modality-tag')).toHaveText([
      '输入：文本 / 图片',
      '输出：文本',
    ])
    await modal(page).getByRole('button', { name: '关闭', exact: true }).click()
    state.models[1].status = 'DISABLED'
    await page.getByRole('button', { name: '编辑', exact: true }).click()
    expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
    const dialog = modal(page)
    const active = dialog.getByRole('checkbox', { name: '授权 DeepSeek V4 Flash', exact: true })
    const disabled = dialog.getByRole('checkbox', { name: '授权 Claude Sonnet', exact: true })
    await expect(dialog.getByLabel('名称', { exact: true })).toHaveValue('研发组')
    await expect(dialog.getByLabel('备注', { exact: true })).toBeVisible()
    await expect(active).toBeEnabled()
    await expect(active).not.toBeChecked()
    await expect(disabled).toHaveCount(0)
    await dialog.getByLabel('名称', { exact: true }).fill('研发平台组')
    await dialog.getByLabel('备注', { exact: true }).fill('更新后的备注')
    await active.check()
    state.conflict(true)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('操作冲突')
    await expect(active).toBeChecked()
    expect(state.relationships.get('groups/51/models')?.has('71')).toBe(false)
    state.conflict(false)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    expect(state.groups[0].name).toBe('研发平台组')
    expect(state.groups[0].remark).toBe('更新后的备注')
    expect(state.relationships.get('groups/51/models')?.has('71')).toBe(true)

    await page.getByRole('button', { name: '编辑', exact: true }).click()
    const editModelSearch = dialog.getByRole('searchbox', { name: '搜索模型' })
    await expect(editModelSearch).toBeVisible()
    await expect(active).toBeVisible()
    await expect(active).toBeChecked()
    await editModelSearch.fill('没有匹配项')
    await expect(active).toHaveCount(0)
    const editSelectedPane = dialog.locator('.group-model-selected')
    await expect(editSelectedPane.locator('.group-model-pane-head > span')).toHaveText('1')
    await expect(editSelectedPane.getByText('DeepSeek V4 Flash', { exact: true })).toBeVisible()
    await dialog.getByRole('button', { name: '清空模型搜索', exact: true }).click()
    await expect(active).toBeVisible()
    await expect(dialog.getByRole('button', { name: '移除 DeepSeek V4 Flash' })).toBeVisible()
    await mkdir('../.cache/web-visual', { recursive: true })
    await dialog.screenshot({ path: '../.cache/web-visual/group-model-checklist.png' })
    await dialog.getByRole('button', { name: '关闭', exact: true }).click()
    state.relationships.get('groups/51/models')!.add('72')
    await page.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(disabled).toHaveCount(0)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    expect(state.relationships.get('groups/51/models')?.has('72')).toBe(false)
    state.groups[0].status = 'DISABLED'
    await page.getByRole('link', { name: '模型', exact: true }).click()
    await page.getByRole('link', { name: '用户分组', exact: true }).click()
    await page.getByRole('button', { name: '编辑', exact: true }).click()
    await expect(active).toBeChecked()
    await expect(active).toBeEnabled()
    await active.uncheck()
    await expect(active).not.toBeChecked()
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    expect(state.relationships.get('groups/51/models')?.has('71')).toBe(false)
  })

  test('访问模型支持对当前筛选结果全选和反选', async ({ page }) => {
    const state = await fixture(page)
    state.groups.push({
      id: '51',
      code: 'engineering',
      name: '研发组',
      status: 'ACTIVE',
      createdAt: stamp,
    })
    state.relationships.set('groups/51/models', new Set(['71']))
    await signIn(page)
    await page.getByRole('link', { name: '用户分组', exact: true }).click()

    await page.getByRole('button', { name: '创建分组', exact: true }).click()
    const createDialog = modal(page)
    const createCatalog = createDialog.locator('.group-model-catalog')
    const createSelectAll = createCatalog.getByRole('button', { name: '全选', exact: true })
    const createInvertSelection = createCatalog.getByRole('button', {
      name: '反选',
      exact: true,
    })
    const createSearch = createDialog.getByRole('searchbox', { name: '搜索模型' })
    const createSelectedPane = createDialog.locator('.group-model-selected')
    await expect(createDialog.locator('.group-model-workspace')).toHaveCSS(
      'grid-template-columns',
      /\S+ \S+/,
    )
    const deepSeek = createDialog.getByRole('checkbox', {
      name: '授权 DeepSeek V4 Flash',
      exact: true,
    })
    const claude = createDialog.getByRole('checkbox', { name: '授权 Claude Sonnet', exact: true })
    await createSelectAll.click()
    await expect(deepSeek).toBeChecked()
    await expect(claude).toBeChecked()
    await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('2')
    await expect(createSelectedPane.locator('.group-model-list-head')).toHaveCount(0)
    await createInvertSelection.click()
    await expect(deepSeek).not.toBeChecked()
    await expect(claude).not.toBeChecked()

    await createSearch.fill('DeepSeek')
    await createSelectAll.click()
    await expect(deepSeek).toBeChecked()
    await createSearch.fill('Claude')
    await createSelectAll.click()
    await expect(claude).toBeChecked()
    await createSearch.fill('DeepSeek')
    await createInvertSelection.click()
    await expect(deepSeek).not.toBeChecked()
    await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('1')
    await createSearch.fill('')
    await expect(claude).toBeChecked()
    await createSelectedPane.getByRole('button', { name: '移除 Claude Sonnet' }).click()
    await expect(claude).not.toBeChecked()
    await expect(createSelectedPane.locator('.group-model-pane-head > span')).toHaveText('0')
    await createDialog.getByRole('button', { name: '关闭', exact: true }).click()

    await page.getByRole('button', { name: '编辑', exact: true }).click()
    const editDialog = modal(page)
    const editCatalog = editDialog.locator('.group-model-catalog')
    const editSelectAll = editCatalog.getByRole('button', { name: '全选', exact: true })
    const editInvertSelection = editCatalog.getByRole('button', { name: '反选', exact: true })
    const editDeepSeek = editDialog.getByRole('checkbox', {
      name: '授权 DeepSeek V4 Flash',
      exact: true,
    })
    const editClaude = editDialog.getByRole('checkbox', {
      name: '授权 Claude Sonnet',
      exact: true,
    })
    await expect(editDeepSeek).toBeChecked()
    await expect(editClaude).not.toBeChecked()
    await editSelectAll.click()
    await expect(editDeepSeek).toBeChecked()
    await expect(editClaude).toBeChecked()
    await editInvertSelection.click()
    await expect(editDeepSeek).not.toBeChecked()
    await expect(editClaude).not.toBeChecked()
  })

  test('没有模型时提示先添加模型且不打开创建分组弹窗', async ({ page }) => {
    const state = await fixture(page)
    state.models.splice(0)
    await signIn(page)
    await page.getByRole('link', { name: '用户分组', exact: true }).click()

    await page.getByRole('button', { name: '创建分组' }).click()

    await expect(page.getByRole('alert')).toHaveText('请先添加模型。')
    await expect(modal(page)).toHaveCount(0)
    expect(state.modelQueries.at(-1)?.get('status')).toBe('ACTIVE')
  })

  test('分组列表按最新记录倒序显示并提示输入分组名称', async ({ page }) => {
    const state = await fixture(page)
    state.groups.push(
      {
        id: '51',
        code: 'earlier-group',
        name: '较早分组',
        remark: '核心服务组',
        status: 'ACTIVE',
        createdAt: '2026-09-06T14:10:00Z',
      },
      {
        id: '52',
        code: 'latest-group',
        name: '最新分组',
        status: 'ACTIVE',
        createdAt: '2026-09-06T16:13:00Z',
      },
    )
    await signIn(page)
    await page.getByRole('link', { name: '用户分组', exact: true }).click()

    await expect(page.getByRole('columnheader')).toHaveText([
      'ID',
      '名称',
      '启用状态',
      '月度额度',
      '创建时间',
      '备注',
      '操作',
    ])
    await expect(page.locator('tbody tr .person strong')).toHaveText(['最新分组', '较早分组'])
    await expect(
      page
        .getByRole('row')
        .filter({ hasText: '最新分组' })
        .getByRole('cell')
        .first()
        .locator('code'),
    ).toHaveText('52')
    await expect(
      page
        .getByRole('row')
        .filter({ hasText: '最新分组' })
        .getByRole('cell')
        .first()
        .locator('.technical-value-copy'),
    ).toHaveCount(0)
    await expect(page.locator('tbody tr .avatar')).toHaveText(['最', '较'])
    await expect(
      page.getByRole('row').filter({ hasText: '最新分组' }).getByRole('cell').nth(5),
    ).toHaveText('-')
    await expect(
      page.getByRole('row').filter({ hasText: '较早分组' }).getByRole('cell').nth(5),
    ).toHaveText('核心服务组')
    await expect(page.getByRole('searchbox', { name: '分组名称' })).toHaveAttribute(
      'placeholder',
      '请输入分组名称',
    )
    await expect(page.locator('.list-search-label')).toHaveText('分组名称')

    const latestSwitch = page.getByRole('switch', { name: '最新分组的启用状态' })
    await expect(latestSwitch).toHaveText('')
    await expect(latestSwitch).toBeChecked()
    await latestSwitch.click()
    await expect(latestSwitch).not.toBeChecked()
    await expect(modal(page)).toHaveCount(0)
    expect(state.groups.find((group) => group.id === '52')?.status).toBe('DISABLED')

    await page.getByRole('link', { name: '用户管理', exact: true }).click()
    await page.getByRole('button', { name: '创建用户' }).click()
    await expect(modal(page).getByRole('checkbox', { name: '选择分组 最新分组' })).toHaveCount(0)
    await expect(modal(page).getByRole('checkbox', { name: '选择分组 较早分组' })).toBeVisible()
  })
})
