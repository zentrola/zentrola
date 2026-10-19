import { test, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { longID, stamp } from './mock/constants'
import { fixture } from './mock/fixture'
import { signIn, modal } from './mock/helpers'

test.describe('用户与应用', () => {
  test('应用管理签发仅展示一次的 App Key', async ({ page }) => {
    const state = await fixture(page)
    state.groups.push({
      id: '51',
      code: 'engineering',
      name: '研发组',
      status: 'ACTIVE',
      createdAt: stamp,
    })
    const applications: any[] = []
    await page.route('**/api/v1/applications**', async (route) => {
      const request = route.request()
      const url = new URL(request.url())
      const path = url.pathname.replace('/api/v1', '')
      const reply = (data: unknown, status = 200) =>
        route.fulfill({
          status,
          contentType: 'application/json',
          body: JSON.stringify({ code: 'OK', data }),
        })
      if (path === '/applications' && request.method() === 'GET')
        return reply({ items: applications, total: applications.length, nextCursor: null })
      if (path === '/applications' && request.method() === 'POST') {
        const input = request.postDataJSON()
        const created = {
          id: '71',
          name: input.name,
          remark: input.remark,
          status: 'DISABLED',
          createdAt: stamp,
        }
        applications.push(created)
        return reply(created, 201)
      }
      if (path === '/applications/71/keys' && request.method() === 'POST')
        return reply(
          {
            id: '81',
            key: 'ak-once-only-secret',
            maskedKey: 'ak-******',
            name: request.postDataJSON().name,
            expiresAt: null,
          },
          201,
        )
      if (path === '/applications/71/keys' && request.method() === 'GET') {
        const keys = [
          {
            id: '82',
            name: '已过期',
            maskedKey: 'ak-expired******',
            status: 'ACTIVE',
            expiresAt: '2020-01-01T00:00:00Z',
            revokedAt: null,
            createdAt: stamp,
          },
          {
            id: '81',
            name: '生产调用',
            maskedKey: 'ak-active******',
            status: 'ACTIVE',
            expiresAt: null,
            revokedAt: null,
            createdAt: stamp,
          },
          {
            id: '80',
            name: '已撤销',
            maskedKey: 'ak-revoked******',
            status: 'REVOKED',
            expiresAt: null,
            revokedAt: '2026-09-20T09:12:26Z',
            createdAt: stamp,
          },
        ]
        const items = url.searchParams.get('expiry') === 'unexpired' ? keys.slice(1, 2) : keys
        return reply({ items, total: items.length, nextCursor: null })
      }
      return reply(null, 404)
    })

    await signIn(page, 'home')
    const memberNavigation = page.getByRole('link', { name: '用户管理', exact: true })
    const applicationNavigation = page.getByRole('link', { name: '应用管理', exact: true })
    await expect(memberNavigation).toBeVisible()
    await expect(applicationNavigation).toBeVisible()
    await applicationNavigation.click()
    await expect(page).toHaveURL(/#\/applications$/)
    await expect(applicationNavigation).toHaveClass(/router-link-active/)
    await expect(memberNavigation).not.toHaveClass(/router-link-active/)
    await expect(page.getByRole('tab', { name: '应用管理' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: '应用管理', exact: true })).toBeVisible()
    await expect(page.getByRole('columnheader').first()).toHaveText('ID')
    await page.getByRole('button', { name: '创建应用' }).click()
    const dialog = modal(page)
    await dialog.getByLabel('应用名称').fill('自动化服务')
    await dialog.getByRole('checkbox', { name: '选择分组 研发组' }).check()
    await dialog.getByRole('button', { name: '创建', exact: true }).click()
    const row = page.getByRole('row').filter({ hasText: '自动化服务' })
    await expect(row).toBeVisible()
    await row.hover()
    await expect(row.getByRole('cell').first()).toHaveCSS('background-color', 'rgb(248, 250, 252)')
    await expect(row.getByRole('cell').last()).toHaveCSS('background-color', 'rgb(248, 250, 252)')
    const applicationMoreActions = row.locator('summary')
    await expect(applicationMoreActions).toHaveAttribute('aria-label', '自动化服务 的更多操作')
    await expect(applicationMoreActions).toHaveText('⋮')
    await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
    const tableScroll = page.locator('.table-scroll').first()
    const scrollHeightBeforeMenu = await tableScroll.evaluate((element) => element.scrollHeight)
    await applicationMoreActions.click()
    const quotaAction = row.getByRole('button', { name: '增加配额', exact: true })
    const deleteAction = row.getByRole('button', { name: '删除', exact: true })
    await expect(quotaAction).toBeVisible()
    await expect(deleteAction).toBeVisible()
    const menuBox = (await row.locator('.row-action-more-menu').boundingBox())!
    const quotaActionBox = (await quotaAction.boundingBox())!
    const deleteActionBox = (await deleteAction.boundingBox())!
    expect(deleteActionBox.y).toBeGreaterThan(quotaActionBox.y)
    expect(quotaActionBox.x).toBeGreaterThanOrEqual(menuBox.x)
    expect(deleteActionBox.x + deleteActionBox.width).toBeLessThanOrEqual(menuBox.x + menuBox.width)
    expect(await tableScroll.evaluate((element) => element.scrollHeight)).toBe(
      scrollHeightBeforeMenu,
    )
    await applicationMoreActions.press('Escape')
    await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
    await expect(row.getByRole('cell').first().locator('code')).toHaveText('71')
    await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({
      path: '../.cache/web-visual/application-id-first.png',
      animations: 'disabled',
    })
    await expect(row.getByRole('link', { name: '审计记录' })).toHaveCount(0)
    await row.getByRole('button', { name: 'App Key', exact: true }).click()
    await modal(page).getByLabel('Key 名称').fill('生产调用')
    await modal(page).getByRole('button', { name: '签发 App Key' }).click()
    await expect(
      modal(page).getByRole('textbox', { name: '完整 App Key 仅展示这一次' }),
    ).toHaveValue('ak-once-only-secret')
    await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
    await expect(page.getByText('ak-once-only-secret')).toHaveCount(0)
    await row.getByRole('button', { name: '查看 App Key 记录' }).click()
    const expiryFilter = modal(page).getByRole('combobox', { name: '查看范围' })
    await expect(expiryFilter).toHaveValue('unexpired')
    await expect(modal(page).locator('tbody tr td:first-child')).toHaveText(['生产调用'])
    await expiryFilter.selectOption('all')
    await expect(modal(page).locator('tbody tr td:first-child')).toHaveText([
      '已过期',
      '生产调用',
      '已撤销',
    ])
    await expect(
      modal(page).getByRole('row').filter({ hasText: '已撤销' }).locator('td').nth(2),
    ).toHaveText('2026年9月20日')
    await expect(
      modal(page).getByRole('row').filter({ hasText: '已撤销' }).locator('td').nth(3),
    ).toHaveText('-')
    await modal(page).getByRole('button', { name: '关闭' }).click()
    await memberNavigation.click()
    await expect(page).toHaveURL(/#\/members$/)
    await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
    await page.goto('/#/applications')
    await expect(applicationNavigation).toHaveClass(/router-link-active/)
    await expect(page.getByRole('row').filter({ hasText: '自动化服务' })).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.locator('.workspace')).toHaveCSS('margin-left', '0px')
    await page.getByRole('button', { name: '打开导航' }).click()
    await expect(memberNavigation).toBeVisible()
    await expect(applicationNavigation).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })

  test('配额状态超过 100 个对象时分批查询并合并展示', async ({ page }) => {
    const state = await fixture(page)
    for (let index = 0; index < 150; index++) {
      state.members.push({
        id: String(80000000000000000n + BigInt(index)),
        name: `配额成员 ${index}`,
        remark: '',
        status: 'ACTIVE',
        monthlyTokenLimit: '1000',
        quotaUsedTokens: String(index),
        createdAt: stamp,
      })
    }
    await signIn(page)
    const lastRow = page
      .getByRole('row')
      .filter({ has: page.getByText('配额成员 0', { exact: true }) })
    await expect(lastRow).toContainText('剩余 1,000')
    const firstRow = page.getByRole('row').filter({ hasText: '配额成员 149' })
    await expect(firstRow).toContainText('剩余 851')
    const batches = state.tokenQuotaQueries.map((query) => (query.get('scopeIds') || '').split(','))
    expect(batches.length).toBeGreaterThanOrEqual(2)
    expect(batches.every((ids) => ids.length <= 100)).toBe(true)
    expect(new Set(batches.slice(-2).flat()).size).toBe(150)
    await expect(page.getByRole('alert')).toHaveCount(0)
  })

  test('成员列表按需查看 Key 并处理删除和失败恢复', async ({ page }) => {
    const state = await fixture(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    state.keys.push(
      {
        id: '801',
        memberID: longID,
        name: '工作站',
        maskedKey: 'vk-work1234********1234',
        status: 'ACTIVE',
        expiresAt: null,
        revokedAt: null,
        createdAt: '2019-01-01T00:00:00Z',
      },
      {
        id: '802',
        memberID: longID,
        name: '临时测试',
        maskedKey: 'zt_vk_temp********temp',
        status: 'ACTIVE',
        expiresAt: '2020-01-01T00:00:00Z',
        revokedAt: null,
        createdAt: '2019-02-01T00:00:00Z',
      },
      {
        id: '803',
        memberID: longID,
        name: '已撤销的 Key',
        maskedKey: 'vk-revoked********1234',
        status: 'REVOKED',
        expiresAt: null,
        revokedAt: '2026-09-20T09:12:26Z',
        createdAt: '2019-03-01T00:00:00Z',
      },
    )
    await signIn(page)
    const row = page.getByRole('row').filter({ hasText: '林知远' })
    await expect(page.getByRole('columnheader').first()).toHaveText('ID')
    await expect(page.getByRole('button', { name: '管理 Key' })).toHaveCount(0)
    await expect(page.getByRole('columnheader', { name: '访问密钥', exact: true })).toHaveCount(0)
    await expect(row.getByRole('cell').first().locator('code')).toHaveText(longID)
    await expect(row.getByRole('cell').first().locator('.technical-value-copy')).toHaveCount(0)
    expect(
      await row
        .getByRole('cell')
        .first()
        .locator('code')
        .evaluate((element) => {
          return element.scrollWidth > element.clientWidth
        }),
    ).toBe(true)
    await expect(row.getByRole('cell').first().locator('.technical-value')).toHaveAttribute(
      'title',
      longID,
    )
    await expect(row).not.toContainText('vk-work1234')
    await expect(row).not.toContainText('zt_vk_temp')
    await expect(row.getByText('工作站', { exact: true })).toHaveCount(0)
    await expect(row).not.toContainText('长期有效')
    await expect(row).not.toContainText('2020年1月1日')
    await expect(row).not.toContainText('已过期')
    await expect(row.getByRole('button', { name: '撤销', exact: true })).toHaveCount(0)
    const viewKeys = row.getByRole('button', { name: '查看 Key 记录', exact: true })
    await expect(viewKeys.locator('svg')).toHaveCount(1)
    await viewKeys.click()
    await expect(modal(page)).toHaveAccessibleName('林知远的 Key 记录')
    await expect(modal(page)).toContainText('完整 Key 仅在分配成功时展示一次')
    const expiryFilter = modal(page).getByRole('combobox', { name: '查看范围' })
    await expect(expiryFilter).toHaveValue('unexpired')
    await expect(modal(page).locator('tbody tr td:first-child')).toHaveText(['工作站'])
    await expiryFilter.selectOption('all')
    await expect(modal(page).locator('tbody tr td:first-child')).toHaveText([
      '已撤销的 Key',
      '临时测试',
      '工作站',
    ])
    await expect(modal(page).getByRole('columnheader')).toHaveText([
      '显示名称',
      '访问密钥',
      '过期日期',
      '操作',
    ])
    await expect(modal(page).getByRole('columnheader', { name: '操作' })).toHaveCSS(
      'text-align',
      'center',
    )
    const actionCenters = await modal(page)
      .locator('tbody tr td:last-child > *')
      .evaluateAll((elements) =>
        elements.map((element) => {
          const bounds = element.getBoundingClientRect()
          return bounds.left + bounds.width / 2
        }),
      )
    expect(Math.max(...actionCenters) - Math.min(...actionCenters)).toBeLessThan(2)
    await expect(modal(page).locator('code')).toHaveText([
      'vk-revoked********1234',
      'zt_vk_temp********temp',
      'vk-work1234********1234',
    ])
    const maskedKey = modal(page).getByRole('row').filter({ hasText: '工作站' })
    await expect(maskedKey.getByRole('button', { name: '复制', exact: true })).toHaveCount(0)
    await expect(maskedKey.locator('td').nth(2)).toHaveText('长期有效')
    await expect(modal(page)).toContainText('2020年1月1日')
    await expect(
      modal(page).getByRole('row').filter({ hasText: '临时测试' }).locator('td').nth(2),
    ).toHaveText('2020年1月1日')
    await expect(
      modal(page).getByRole('row').filter({ hasText: '已撤销的 Key' }).locator('td').nth(2),
    ).toHaveText('2026年9月20日')
    await expect(
      modal(page)
        .getByRole('row')
        .filter({ hasText: '临时测试' })
        .getByRole('button', { name: '撤销' }),
    ).toHaveCount(0)
    await expect(
      modal(page).getByRole('row').filter({ hasText: '临时测试' }).locator('td').nth(3),
    ).toHaveText('-')
    const historyDialog = page.getByRole('dialog', { name: '林知远的 Key 记录', exact: true })
    const historyRevoke = historyDialog
      .getByRole('row')
      .filter({ hasText: '工作站' })
      .getByRole('button', { name: '撤销' })
    await historyRevoke.click()
    await expect(modal(page)).toContainText('确认撤销「工作站」')
    await modal(page).getByRole('button', { name: '取消', exact: true }).click()
    expect(state.keys[0].status).toBe('ACTIVE')
    await historyRevoke.click()
    state.conflict(true)
    await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('操作冲突')
    await expect(page.locator('.toast')).not.toContainText('req_browser_fixture')
    expect(state.keys[0].status).toBe('ACTIVE')
    state.conflict(false)
    await modal(page).getByRole('button', { name: '撤销', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(1)
    await expect(page.locator('.toast-success')).toContainText('Key 已撤销')
    await expect(historyDialog.locator('.notice')).toHaveCount(0)
    await expect(historyRevoke).toHaveCount(0)
    await expect(maskedKey.locator('td').nth(3)).toHaveText('-')
    expect(state.keys[0].status).toBe('REVOKED')
    expect(state.keys[1].status).toBe('ACTIVE')
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({ path: '../.cache/web-visual/member-key-list.png', fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await modal(page).evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
    await page.screenshot({
      path: '../.cache/web-visual/member-key-list-mobile.png',
      fullPage: true,
    })
    await page.keyboard.press('Escape')
    await expect(page.locator('dialog')).toHaveCount(0)
    await expect(viewKeys).toBeFocused()
    await page.setViewportSize({ width: 1440, height: 1000 })
    const memberTableScroll = page.locator('.table-scroll')
    expect(
      await memberTableScroll.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true)
    const disabledMemberRow = page.getByRole('row').filter({ hasText: '周予安' })
    const disabledMemberStatus = disabledMemberRow.getByRole('switch', {
      name: '周予安的激活状态',
    })
    await expect(disabledMemberStatus).toBeEnabled()
    await expect(disabledMemberStatus).not.toHaveAttribute('title')
    await expect(disabledMemberStatus).toHaveText('')
    await expect(disabledMemberRow.getByRole('button', { name: '密钥', exact: true })).toBeEnabled()
    const assignKey = row.getByRole('button', { name: '密钥', exact: true })
    await expect(assignKey.locator('svg')).toHaveCount(0)
    await assignKey.click()
    await modal(page).getByLabel('Key 名称').fill('自动化')
    await expect(modal(page).getByLabel('到期日期（可选）')).toHaveAttribute('type', 'date')
    await modal(page).getByLabel('到期日期（可选）').fill('2020-01-01')
    await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('不能早于今天')
    await expect(modal(page).locator('.alert.error')).toHaveCount(0)
    await modal(page).getByLabel('到期日期（可选）').fill('2099-12-31')
    await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
    await modal(page).getByRole('button', { name: '复制', exact: true }).click()
    await expect(page.locator('.toast-success')).toContainText('已复制')
    await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    await expect(row).not.toContainText('vk-fixture1')
    await expect(row.getByRole('cell').first().locator('code')).toHaveText(longID)
    await expect(row).not.toContainText('zt_vk_temp')
    await expect(row).not.toContainText('2020年1月1日')
    expect(state.keys).toHaveLength(4)
    await expect(row).not.toContainText('2099年12月31日')
    expect(
      await page.evaluate((value) => {
        const expiry = new Date(value)
        return [
          expiry.getFullYear(),
          expiry.getMonth() + 1,
          expiry.getDate(),
          expiry.getHours(),
          expiry.getMinutes(),
          expiry.getSeconds(),
          expiry.getMilliseconds(),
        ]
      }, state.keys.at(-1).expiresAt),
    ).toEqual([2099, 12, 31, 23, 59, 59, 999])
    await mkdir('../.cache/web-visual', { recursive: true })
    await page.screenshot({ path: '../.cache/web-visual/members-keys.png', fullPage: true })
    await row.locator('summary').click()
    await row.getByRole('button', { name: '删除', exact: true }).click()
    await modal(page).getByRole('button', { name: '取消' }).click()
    expect(state.members).toHaveLength(3)
    await row.locator('summary').click()
    await row.getByRole('button', { name: '删除', exact: true }).click()
    state.conflict(true)
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('操作冲突')
    expect(state.members).toHaveLength(3)
    state.conflict(false)
    await modal(page).getByRole('button', { name: '删除', exact: true }).click()
    await expect(row).toHaveCount(0)
    await expect(page.locator('.toast-success')).toContainText('用户已删除')
    await expect(page.locator('.notice')).toHaveCount(0)
    expect(state.members).toHaveLength(2)
  })

  test('密钥在弹窗打开期间到期后刷新有效期内列表', async ({ page }) => {
    await page.clock.install({ time: new Date('2026-10-09T08:00:00Z') })
    await fixture(page)
    let requests = 0
    await page.route(`**/api/v1/members/${longID}/keys*`, async (route) => {
      requests++
      const items =
        requests === 1
          ? [
              {
                id: '801',
                name: '即将过期',
                maskedKey: 'vk-expiring********1234',
                status: 'ACTIVE',
                expiresAt: '2026-10-09T08:00:01Z',
                revokedAt: null,
                createdAt: stamp,
              },
            ]
          : []
      await route.fulfill({
        json: { code: 'OK', data: { items, total: items.length, nextCursor: null } },
      })
    })
    await signIn(page)
    await page
      .getByRole('row')
      .filter({ hasText: '林知远' })
      .getByRole('button', { name: '查看 Key 记录' })
      .click()
    await expect(modal(page).getByText('即将过期')).toBeVisible()
    await page.clock.fastForward(2000)
    await expect(modal(page).getByText('即将过期')).toHaveCount(0)
    await expect(modal(page)).toContainText('没有有效的访问密钥。')
    expect(requests).toBe(2)
  })

  test('成员列表不预查密钥且分配密钥后可激活', async ({ page }) => {
    const state = await fixture(page)
    await signIn(page)
    const row = page.getByRole('row').filter({ hasText: '周予安' })
    const status = row.getByRole('switch', { name: '周予安的激活状态' })

    await expect(status).toBeEnabled()
    await status.click()
    await expect(page.locator('.toast')).toContainText('请先为该用户生成调用 Key，再启用用户')
    await expect(status).not.toBeChecked()
    await row.getByRole('button', { name: '密钥', exact: true }).click()
    await modal(page).getByLabel('Key 名称').fill('工作站')
    await modal(page).getByRole('button', { name: '密钥', exact: true }).click()
    await modal(page).getByRole('button', { name: '我已保存，关闭' }).click()

    await expect(status).toBeEnabled()
    await status.click()
    await expect(status).toBeChecked()
    await expect(modal(page)).toHaveCount(0)
    await expect(status).toHaveText('')
    expect(state.members.find((member) => member.name === '周予安')?.status).toBe('ACTIVE')
  })

  test('用户月度额度展示已用与剩余并支持增量调整和取消限制', async ({ page }) => {
    const state = await fixture(page)
    state.members[0].monthlyTokenLimit = '1000'
    state.members[0].quotaUsedTokens = '600'
    await signIn(page)

    const row = page.getByRole('row').filter({ hasText: '林知远' })
    const quotaCell = row.getByRole('cell').nth(3)
    await expect(quotaCell).toContainText(/600\s*\/\s*1,000/)
    await expect(quotaCell).toContainText('剩余 400')

    const moreActions = row.locator('summary')
    await expect(moreActions).toHaveAttribute('aria-label', '林知远 的更多操作')
    await expect(row.getByRole('button', { name: '增加配额', exact: true })).toBeHidden()
    await moreActions.click()
    const otherRow = page.getByRole('row').filter({ hasText: '陈清和' })
    await otherRow.locator('summary').click()
    await expect(row.locator('.row-action-more-menu')).toBeHidden()
    await expect(otherRow.locator('.row-action-more-menu')).toBeVisible()
    await expect(page.locator('.row-action-more-menu:popover-open')).toHaveCount(1)
    await moreActions.click()
    await row.getByRole('button', { name: '增加配额', exact: true }).click()
    const quotaDialog = modal(page)
    await expect(
      quotaDialog.getByRole('group', { name: '常用额度' }).getByRole('button'),
    ).toHaveText(['100 万', '1000 万', '1 亿'])
    await quotaDialog.getByRole('button', { name: '1 亿', exact: true }).click()
    await expect(quotaDialog.getByLabel('增加 Token 数')).toHaveValue('100000000')
    await expect(quotaDialog.getByText('约 1亿 Token', { exact: true })).toBeVisible()
    await quotaDialog.getByLabel('增加 Token 数').fill('10000000')
    await expect(quotaDialog.getByText('约 1000万 Token', { exact: true })).toBeVisible()
    await quotaDialog.getByLabel('增加 Token 数').fill('500')
    await expect(quotaDialog.getByLabel('调整原因（选填）')).toHaveAttribute(
      'placeholder',
      '例如：项目扩容',
    )
    await modal(page).getByRole('button', { name: '增加配额', exact: true }).click()

    await expect(page.locator('.toast-success')).toContainText('配额已增加')
    await expect(modal(page)).toHaveCount(0)
    await expect(quotaCell).toContainText(/600\s*\/\s*1,500/)
    await expect(quotaCell).toContainText('剩余 900')
    expect(state.members[0].monthlyTokenLimit).toBe('1500')

    await moreActions.click()
    await row.getByRole('button', { name: '取消额度限制', exact: true }).click()
    await expect(modal(page)).toContainText('取消后将不再按月度 Token 用量拒绝请求')
    await modal(page).getByRole('button', { name: '取消额度限制', exact: true }).click()

    await expect(page.locator('.toast-success')).toContainText('额度限制已取消')
    await expect(modal(page)).toHaveCount(0)
    await expect(quotaCell).toHaveText('-')
    expect(state.members[0].monthlyTokenLimit).toBeNull()
  })

  test('成员 Key 弹层加载失败可重试', async ({ page }) => {
    await fixture(page)
    let fail = true
    await page.route(`**/api/v1/members/${longID}/keys?*`, async (route) => {
      if (!fail) return route.fallback()
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'UNAVAILABLE', data: null }),
      })
    })
    await signIn(page)
    const row = page.getByRole('row').filter({ hasText: '林知远' })
    await expect(row.getByRole('alert')).toHaveCount(0)
    await row.getByRole('button', { name: '查看 Key 记录' }).click()
    await expect(modal(page).getByRole('alert')).toBeVisible()
    fail = false
    await modal(page).getByRole('button', { name: '重试' }).click()
    await expect(modal(page).getByRole('alert')).toHaveCount(0)
  })

  test('主列表不请求密钥，弹层按需分页并支持失败重试和切换每页条数', async ({ page }) => {
    const state = await fixture(page)
    for (let i = 0; i < 51; i++) {
      state.keys.push({
        id: (90071992547409931n + BigInt(i)).toString(),
        memberID: longID,
        name: `设备 ${i}`,
        maskedKey: `vk-${String(i).padStart(8, '0')}********${String(i).padStart(4, '0')}`,
        key: `secret-fixture-full-key-${i}`,
        status: 'ACTIVE',
        expiresAt: null,
        revokedAt: null,
        createdAt: stamp,
      })
    }
    const requests: URLSearchParams[] = []
    let failMore = true
    await page.route(`**/api/v1/members/${longID}/keys?*`, async (route) => {
      const query = new URL(route.request().url()).searchParams
      requests.push(query)
      if (query.has('after') && failMore)
        return route.fulfill({ status: 503, json: { code: 'SERVICE_UNAVAILABLE' } })
      return route.fallback()
    })
    await signIn(page)
    const row = page.getByRole('row').filter({ hasText: '林知远' })
    expect(requests).toHaveLength(0)
    await row.getByRole('button', { name: '查看 Key 记录' }).click()
    const dialog = modal(page)
    await expect(dialog.locator('tbody tr')).toHaveCount(20)
    await expect(dialog.locator('tbody tr').first()).toContainText('设备 50')
    await expect(dialog.getByText('每页', { exact: true })).toHaveCount(0)
    await expect(dialog.locator('.pagination-controls')).toHaveCSS('justify-content', 'flex-end')
    await expect(dialog.getByText('每页显示', { exact: true })).toBeVisible()
    await expect(dialog.getByText('条', { exact: true })).toBeVisible()
    await expect(dialog.getByLabel('每页')).toHaveValue('20')
    await expect(dialog.getByLabel('每页').locator('option')).toHaveText(['20', '50', '100'])
    expect(requests.at(-1)!.get('limit')).toBe('20')
    await dialog.getByRole('button', { name: '下一页' }).click()
    await expect(dialog.getByRole('alert')).toContainText('服务暂时不可用')
    await expect(dialog.locator('tbody tr')).toHaveCount(20)
    expect(requests.at(-1)!.get('after')).toBe('90071992547409962')
    failMore = false
    await dialog.getByRole('button', { name: '重试' }).click()
    await expect(dialog.locator('tbody tr')).toHaveCount(20)
    await expect(dialog.locator('tbody tr').first()).toContainText('设备 30')
    await expect(dialog.locator('tbody tr').last()).toContainText('设备 11')
    await expect(dialog.getByText('第 2 / 3 页', { exact: true })).toBeVisible()
    await expect(dialog.getByText('共 51 条', { exact: true })).toBeVisible()
    await dialog.getByLabel('每页').selectOption('100')
    await expect(dialog.locator('tbody tr')).toHaveCount(51)
    await expect(dialog.getByText('第 1 / 1 页', { exact: true })).toBeVisible()
    await expect(dialog.getByRole('button', { name: '下一页' })).toBeDisabled()
    expect(requests.at(-1)!.get('limit')).toBe('100')
    expect(await page.content()).not.toContain('secret-fixture-full-key')
    await dialog.getByRole('button', { name: '关闭', exact: true }).click()
    await expect(row.getByRole('button', { name: '查看 Key 记录' })).toBeEnabled()
  })

  test('创建和编辑用户时可选择分组', async ({ page }) => {
    const state = await fixture(page)
    state.groups.push(
      {
        id: '51',
        code: 'engineering',
        name: '研发组',
        remark: '研发模型权限',
        status: 'ACTIVE',
        createdAt: stamp,
      },
      {
        id: '52',
        code: 'legacy',
        name: '停用组',
        status: 'DISABLED',
        createdAt: stamp,
      },
      {
        id: '53',
        code: 'operations',
        name: '运维组',
        status: 'ACTIVE',
        createdAt: stamp,
      },
      {
        id: '54',
        code: 'quality',
        name: '质量组',
        status: 'ACTIVE',
        createdAt: stamp,
      },
    )
    state.relationships.set('groups/51/members', new Set([longID]))
    state.relationships.set('groups/52/members', new Set([longID]))

    await signIn(page)
    await page.getByRole('button', { name: '创建用户' }).click()
    const dialog = modal(page)
    const createActive = dialog.getByRole('checkbox', { name: '选择分组 研发组', exact: true })
    const createDisabled = dialog.getByRole('checkbox', { name: '选择分组 停用组', exact: true })
    await expect(createActive).toBeEnabled()
    await expect(createDisabled).toHaveCount(0)
    expect(state.groupQueries.at(-1)?.get('status')).toBe('ACTIVE')
    await expect(dialog.getByText(/已选择.*共.*个分组/)).toHaveCount(0)
    const groupOptions = dialog.locator('.member-group-option')
    await expect(groupOptions).toHaveCount(3)
    const optionTops = await groupOptions.evaluateAll((options) =>
      options.map((option) => Math.round(option.getBoundingClientRect().top)),
    )
    expect(new Set(optionTops).size).toBe(1)
    await expect(dialog.getByLabel('备注', { exact: true })).toHaveAttribute('rows', '3')
    await expect(
      dialog.locator('.member-group-list').getByText('启用', { exact: true }),
    ).toHaveCount(0)
    await dialog.getByLabel('用户名', { exact: true }).fill('新用户')
    await createActive.check()
    await dialog.getByRole('button', { name: '创建', exact: true }).click()
    const created = state.members.find((member) => member.name === '新用户')
    expect(created.status).toBe('DISABLED')
    await expect(
      page
        .getByRole('row')
        .filter({ hasText: '新用户' })
        .getByRole('switch', { name: '新用户的激活状态' }),
    ).not.toBeChecked()
    expect(state.relationships.get('groups/51/members')?.has(created.id)).toBe(true)
    expect(state.relationships.get('groups/52/members')?.has(created.id)).toBe(false)

    const memberRow = page.getByRole('row').filter({ hasText: '林知远' })
    await memberRow.getByRole('button', { name: '编辑', exact: true }).click()
    const editActive = dialog.getByRole('checkbox', { name: '选择分组 研发组', exact: true })
    const editDisabled = dialog.getByRole('checkbox', { name: '选择分组 停用组', exact: true })
    await expect(dialog).toHaveAccessibleName('编辑 林知远')
    await expect(editActive).toBeChecked()
    await expect(editDisabled).toHaveCount(0)
    await dialog.getByLabel('用户名', { exact: true }).fill('林知远（编辑）')
    await dialog.getByLabel('备注', { exact: true }).fill('已调整分组')
    state.conflict(true)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('.toast')).toContainText('操作冲突')
    expect(state.members.find((member) => member.id === longID)?.name).toBe('林知远')
    state.conflict(false)
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(page.locator('dialog')).toHaveCount(0)
    await expect(page.locator('.toast-success')).toContainText('已保存')
    await expect(page.locator('.notice')).toHaveCount(0)
    expect(state.members.find((member) => member.id === longID)?.name).toBe('林知远（编辑）')
    expect(state.relationships.get('groups/51/members')?.has(longID)).toBe(true)
    expect(state.relationships.get('groups/52/members')?.has(longID)).toBe(false)
  })

  test('没有用户分组时提示先添加分组且不打开创建用户弹窗', async ({ page }) => {
    const state = await fixture(page)
    await signIn(page)

    await page.getByRole('button', { name: '创建用户' }).click()

    await expect(page.getByRole('alert')).toHaveText('请先添加用户分组。')
    await expect(modal(page)).toHaveCount(0)
    expect(state.groupQueries.at(-1)?.get('status')).toBe('ACTIVE')
  })

  test('用户页面使用明确术语并在紧凑侧栏展示菜单提示', async ({ page }) => {
    await fixture(page)
    await page.setViewportSize({ width: 900, height: 768 })
    await signIn(page)

    await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '创建用户' })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '用户名', exact: true })).toBeVisible()
    await expect(page.getByRole('columnheader', { name: '激活状态', exact: true })).toBeVisible()
    await expect(page.locator('.workspace-square')).toHaveCount(0)
    await expect(page.locator('.workspace-label')).toHaveCount(0)
    await expect(page.locator('.nav-section')).toHaveText(['GitHub 仓库', '基础配置', '使用记录'])
    const communityRepository = page.locator('.community-source-link')
    await expect(communityRepository).toHaveText('GitHub 仓库')
    await expect(communityRepository).toHaveAttribute(
      'aria-label',
      '在新页面打开 Zentrola GitHub 仓库',
    )
    await expect(communityRepository).toHaveAttribute(
      'href',
      'https://github.com/zentrola/zentrola',
    )
    await expect(communityRepository).toHaveAttribute('target', '_blank')
    await expect(page.getByRole('searchbox', { name: '用户名' })).toHaveAttribute(
      'placeholder',
      '请输入用户名',
    )

    const modelLink = page.getByRole('link', { name: '模型', exact: true })
    await expect(modelLink).toHaveAttribute('data-label', '模型')
    await modelLink.hover()
    await expect
      .poll(() => modelLink.evaluate((link) => getComputedStyle(link, '::after').opacity))
      .toBe('1')
  })

  test('成员列表按 ID 倒序前后翻页，保持长 ID 并拒绝无效用量日期范围', async ({ page }) => {
    await fixture(page)
    let pages = 0
    await page.route('**/api/v1/members?**', async (route) => {
      pages++
      const after = new URL(route.request().url()).searchParams.get('after')
      if (after) expect(after).toBe(longID)
      await route.fulfill({
        json: {
          code: 'OK',
          data: {
            items: [
              {
                id: after ? '90071992547409930' : longID,
                name: after ? '第二页成员' : '第一页成员',
                status: 'ACTIVE',
                createdAt: stamp,
              },
            ],
            nextCursor: after ? null : longID,
            total: 21,
          },
        },
      })
    })
    await signIn(page)
    await expect(page.getByLabel('每页')).toHaveValue('20')
    await expect(page.getByRole('button', { name: '首页' })).toBeDisabled()
    await page.getByRole('button', { name: '下一页' }).click()
    await expect(page.getByText('第一页成员', { exact: true })).toHaveCount(0)
    await expect(page.getByText('第二页成员', { exact: true })).toBeVisible()
    await expect(page.locator('tbody tr .person strong')).toHaveText(['第二页成员'])
    await expect(page.getByText('第 2 / 2 页', { exact: true })).toBeVisible()
    await expect(page.getByText('共 21 条', { exact: true })).toBeVisible()
    expect(pages).toBe(2)
    await page.getByRole('button', { name: '上一页' }).click()
    await expect(page.getByText('第一页成员', { exact: true })).toBeVisible()
    await expect(page.getByText('第二页成员', { exact: true })).toHaveCount(0)
    expect(pages).toBe(3)
    await page.getByRole('button', { name: '下一页' }).click()
    await page.getByRole('button', { name: '首页' }).click()
    await expect(page.getByText('第一页成员', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '首页' })).toBeDisabled()
    expect(pages).toBe(5)
    await page.getByRole('link', { name: '用量分析' }).click()
    await page.getByRole('tab', { name: '请求明细', exact: true }).click()
    await expect(page.getByRole('button', { name: '搜索', exact: true })).toBeEnabled()
    await expect(page.getByText('默认查询昨天至今天')).toHaveCount(0)
    await expect(page.locator('.model-filter-field .filter-label')).toHaveCSS('font-weight', '400')
    const dateRangeButton = page.getByRole('button', { name: '选择起止日期', exact: true })
    const [defaultStart = ''] =
      (await dateRangeButton.textContent())?.match(/\d{4}\/\d{2}\/\d{2}/g) || []
    await dateRangeButton.click()
    const dateRangeDialog = page.getByRole('dialog', { name: '选择起止日期' })
    const defaultStartButton = dateRangeDialog
      .getByRole('button', { name: `选择 ${defaultStart.replaceAll('/', '-')}` })
      .first()
    await defaultStartButton.click()
    const selectedDateBox = await defaultStartButton.boundingBox()
    expect(selectedDateBox?.width).toBe(selectedDateBox?.height)
    await defaultStartButton.click()
    await expect(dateRangeDialog.getByRole('alert')).toContainText('起止日期不能相同')
    await dateRangeButton.click()
    await dateRangeButton.click()
    for (let index = 0; index < 13; index += 1)
      await dateRangeDialog.getByRole('button', { name: '上个月', exact: true }).click()
    await dateRangeDialog
      .locator('.calendar-panel')
      .first()
      .locator('.calendar-days button:not(.muted)')
      .first()
      .click()
    for (let index = 0; index < 13; index += 1)
      await dateRangeDialog.getByRole('button', { name: '下个月', exact: true }).click()
    await dateRangeDialog
      .locator('.calendar-panel')
      .last()
      .locator('.calendar-days button:not(.muted)')
      .last()
      .click()
    await expect(dateRangeDialog.getByRole('alert')).toContainText('有效的起止日期')
    await page.getByRole('button', { name: '选择起止日期', exact: true }).click()
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(page.getByRole('alert')).toHaveCount(0)
  })

  test('列表搜索覆盖全部分页并安全传递不透明游标', async ({ page }) => {
    await fixture(page)
    const opaqueCursor = 'cursor&segment=two'
    const searchRequests: URL[] = []
    await page.route('**/api/v1/members?**', async (route) => {
      const url = new URL(route.request().url())
      const limit = Number(url.searchParams.get('limit'))
      const after = url.searchParams.get('after')
      if (limit === 100) searchRequests.push(url)
      const item = {
        id: after === opaqueCursor ? '90071992547409929' : longID,
        name: after === opaqueCursor ? '跨页目标成员' : '第一页成员',
        remark: '',
        status: 'ACTIVE',
        createdAt: stamp,
      }
      await route.fulfill({
        json: {
          code: 'OK',
          data: {
            items: [item],
            nextCursor: limit === 100 && !after ? opaqueCursor : null,
            total: 2,
          },
        },
      })
    })
    await signIn(page)
    await expect(page.getByText('跨页目标成员', { exact: true })).toHaveCount(0)
    await page.getByRole('searchbox', { name: '用户名' }).fill('跨页目标成员')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect(page.getByText('跨页目标成员', { exact: true })).toBeVisible()
    await expect(page.getByLabel('分页')).toHaveCount(0)
    expect(searchRequests).toHaveLength(2)
    expect(searchRequests[1].searchParams.get('after')).toBe(opaqueCursor)
    await page.getByRole('button', { name: '重置', exact: true }).click()
    await expect(page.getByLabel('分页')).toBeVisible()
  })

  test('离开页面时中止进行中的全量搜索，不再请求后续分页', async ({ page }) => {
    await fixture(page)
    const searchCursors: Array<string | null> = []
    let releaseFirstPage = () => {}
    await page.route('**/api/v1/members?**', async (route) => {
      const url = new URL(route.request().url())
      if (url.searchParams.get('limit') !== '100') return route.fallback()
      const after = url.searchParams.get('after')
      searchCursors.push(after)
      if (!after)
        await new Promise<void>((resolve) => {
          releaseFirstPage = resolve
        })
      await route
        .fulfill({
          json: {
            code: 'OK',
            data: {
              items: [
                {
                  id: after ? '90071992547409929' : longID,
                  name: after ? '第二页成员' : '第一页成员',
                  remark: '',
                  status: 'ACTIVE',
                  createdAt: stamp,
                },
              ],
              nextCursor: after ? null : 'next-page',
              total: 2,
            },
          },
        })
        .catch(() => {})
    })
    await signIn(page)
    await page.getByRole('searchbox', { name: '用户名' }).fill('第二页成员')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    await expect.poll(() => searchCursors.length).toBe(1)
    await page.getByRole('link', { name: '用户分组', exact: true }).click()
    await expect(page.getByRole('heading', { name: '用户分组', exact: true })).toBeVisible()
    releaseFirstPage()
    await page.waitForTimeout(500)
    expect(searchCursors).toEqual([null])
  })
})
