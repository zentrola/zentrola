import { expect, type Locator, type Page } from '@playwright/test'
import { stamp } from './constants'

export async function expectOnlyModalBodyScrolls(dialog: Locator) {
  const layout = await dialog.evaluate((element) => {
    const body = element.querySelector('.modal-body') as HTMLElement
    const head = element.querySelector('.modal-head') as HTMLElement
    const footer = element.querySelector('.modal-footer') as HTMLElement
    const before = {
      headTop: head.getBoundingClientRect().top,
      footerTop: footer.getBoundingClientRect().top,
      dialogTop: element.getBoundingClientRect().top,
    }
    body.scrollTop = body.scrollHeight
    return {
      scrollable: body.scrollHeight > body.clientHeight,
      scrollTop: body.scrollTop,
      bodyOverflow: getComputedStyle(body).overflowY,
      dialogOverflow: getComputedStyle(element).overflowY,
      headTopUnchanged: head.getBoundingClientRect().top === before.headTop,
      footerTopUnchanged: footer.getBoundingClientRect().top === before.footerTop,
      dialogTopUnchanged: element.getBoundingClientRect().top === before.dialogTop,
    }
  })
  expect(layout).toEqual({
    scrollable: true,
    scrollTop: expect.any(Number),
    bodyOverflow: 'auto',
    dialogOverflow: 'hidden',
    headTopUnchanged: true,
    footerTopUnchanged: true,
    dialogTopUnchanged: true,
  })
  expect(layout.scrollTop).toBeGreaterThan(0)
}
// Claude 订阅额度来自推理响应头中的统一限额，连接测试或手动刷新额度后写入。
export function recordClaudeQuota(resource: any, fiveHour: number, sevenDay: number) {
  resource.quotaStatus = 'AVAILABLE'
  resource.quotaCheckedAt = stamp
  resource.quotaResetsAt = stamp
  resource.quotaDetails = [
    {
      code: 'subscription',
      name: 'Subscription',
      status: 'AVAILABLE',
      unit: null,
      limitValue: null,
      usedValue: null,
      remainingValue: null,
      usedPercent: null,
      windowDurationSeconds: null,
      resetsAt: null,
      reachedType: null,
      observedAt: stamp,
    },
    {
      code: 'five_hour',
      name: null,
      status: 'AVAILABLE',
      unit: 'PERCENT',
      limitValue: null,
      usedValue: null,
      remainingValue: null,
      usedPercent: fiveHour,
      windowDurationSeconds: 18000,
      resetsAt: stamp,
      reachedType: null,
      observedAt: stamp,
    },
    {
      code: 'seven_day',
      name: null,
      status: 'AVAILABLE',
      unit: 'PERCENT',
      limitValue: null,
      usedValue: null,
      remainingValue: null,
      usedPercent: sevenDay,
      windowDurationSeconds: 604800,
      resetsAt: stamp,
      reachedType: null,
      observedAt: stamp,
    },
  ]
}

export async function signIn(page: Page, destination: 'home' | 'members' = 'members') {
  await page.goto('/')
  await page.getByLabel('管理员账号').fill('admin')
  await page.getByLabel('密码', { exact: true }).fill('fixture-password')
  await page.getByRole('button', { name: '登录控制台' }).click()
  await expect(page.getByRole('heading', { name: '仪表盘', exact: true })).toBeVisible()
  if (destination === 'home') return
  await page.getByRole('link', { name: '用户管理', exact: true }).click()
  await expect(page.getByRole('heading', { name: '用户管理', exact: true })).toBeVisible()
}
export const modal = (page: Page) => page.locator('dialog').last()
