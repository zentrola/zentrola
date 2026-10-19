import { t } from './i18n'
import { formatMoney } from './money'
import type { Resource } from './types'

// 服务商凭据的录入校验与列表展示，均为无副作用的纯函数。
export type CredentialAuthType = 'API_KEY' | 'SUBSCRIPTION'
export type CredentialInput = {
  providerId: string
  name: string
  credential: string
  authType: CredentialAuthType
  authAdapter: string
}

export function validApiKeyCredential(value: string) {
  return /^[\x21-\x7e]{1,4096}$/.test(value)
}

// Claude 个人订阅为单行 Token，其他个人订阅为 auth.json 对象；大小上限与后端校验保持一致。
export function validSubscriptionCredential(value: string, claudeSubscription: boolean) {
  const size = new TextEncoder().encode(value).length
  if (!value.trim() || size > 65536) return false
  if (claudeSubscription) {
    const token = value.trim()
    return token.length <= 4096 && /^[\x21-\x7e]+$/.test(token)
  }
  try {
    const parsed = JSON.parse(value)
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
  } catch {
    return false
  }
}

export function subscriptionPriceLabel(resource: Resource) {
  const price = resource.subscriptionPrice
  if (!price) return '-'
  return `${formatMoney(price.periodAmount, price.currency)} / ${t(`resources.pricing.periods.${price.billingPeriod}`)}`
}

// 生效日期为 UTC 自然日，直接取 ISO 日期部分展示。
export function subscriptionPriceEffectiveDate(resource: Resource) {
  return resource.subscriptionPrice?.effectiveAt.slice(0, 10) ?? '-'
}

export function exportableSubscription(resource: Resource) {
  return (
    resource.authType === 'SUBSCRIPTION' &&
    resource.authAdapter === 'OPENAI_CODEX' &&
    resource.subscriptionType === 'PERSONAL'
  )
}
