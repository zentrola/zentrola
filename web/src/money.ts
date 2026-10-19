import type { BillingCurrency } from './types'

const moneyScale = 100000000n
const moneyPattern = /^(-?)(\d+)(?:\.(\d+))?$/

export function currencySymbol(currency: BillingCurrency | string) {
  return currency === 'CNY' ? '¥' : '$'
}

// 后端金额统一为十进制字符串；这里只做千分位和末尾零整理，不经过浮点数，避免精度丢失。
export function formatMoney(
  value: string | null | undefined,
  currency: BillingCurrency | string | null | undefined,
) {
  if (!value || !currency) return '-'
  const match = value.match(moneyPattern)
  if (!match) return '-'
  const fraction = (match[3] || '').replace(/0+$/, '')
  const integer = match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return `${match[1]}${currencySymbol(currency)}${integer}${fraction ? `.${fraction}` : ''}`
}

// 以 8 位小数精度累加非负金额字符串，结果仍为十进制字符串。
export function addMoney(values: string[]) {
  let total = 0n
  for (const value of values) {
    const match = value.match(/^(\d+)(?:\.(\d+))?$/)
    if (!match) continue
    const fraction = (match[2] || '').padEnd(8, '0').slice(0, 8)
    total += BigInt(match[1]) * moneyScale + BigInt(fraction || '0')
  }
  const integer = total / moneyScale
  const fraction = (total % moneyScale).toString().padStart(8, '0').replace(/0+$/, '')
  return `${integer}${fraction ? `.${fraction}` : ''}`
}
