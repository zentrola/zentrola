import { api } from './api'
import { t } from './i18n'
import type { ModelPrice, ProviderMapping, SubscriptionPrice } from './types'

// 凭证计费编辑的表单结构、校验与保存请求。
export type PriceCurrency = '' | 'CNY' | 'USD'

export type ModelPriceForm = {
  inputPrice: string
  outputPrice: string
  cachedInputPrice: string
  // 留空表示不配置缓存写入单价，核算按普通输入单价计费。
  cacheCreationInputPrice: string
  effectiveDate: string
}

export type SubscriptionPriceForm = {
  periodAmount: string
  billingPeriod: '' | 'MONTH' | 'YEAR'
  effectiveDate: string
}

const pricePattern = /^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$/
const datePattern = /^\d{4}-\d{2}-\d{2}$/

export function isScheduledPrice(price: ModelPrice | SubscriptionPrice) {
  return price.effectiveAt > new Date().toISOString()
}

export function mappingLabel(mapping: ProviderMapping, modelNames: Map<string, string>) {
  return modelNames.get(mapping.modelId) || mapping.upstreamModelCode || mapping.modelId
}

export function groupModelPrices(prices: ModelPrice[]) {
  const grouped = new Map<string, ModelPrice[]>()
  for (const price of prices) {
    const group = grouped.get(price.providerModelId)
    if (group) group.push(price)
    else grouped.set(price.providerModelId, [price])
  }
  return grouped
}

export function fillModelPriceForm(form: ModelPriceForm, price?: ModelPrice) {
  form.inputPrice = price?.inputPrice ?? ''
  form.outputPrice = price?.outputPrice ?? ''
  form.cachedInputPrice = price?.cachedInputPrice ?? ''
  form.cacheCreationInputPrice = price?.cacheCreationInputPrice ?? ''
  form.effectiveDate = price?.effectiveAt.slice(0, 10) ?? ''
}

export function fillSubscriptionPriceForm(
  form: SubscriptionPriceForm,
  price: SubscriptionPrice | null,
) {
  form.periodAmount = price?.periodAmount ?? ''
  form.billingPeriod = price?.billingPeriod ?? ''
  form.effectiveDate = price?.effectiveAt.slice(0, 10) ?? ''
}

// 校验通过时返回空字符串，否则返回首个错误提示。
export function modelPriceFormError(form: ModelPriceForm) {
  if (
    ![form.inputPrice, form.outputPrice, form.cachedInputPrice].every((value) =>
      pricePattern.test(value),
    ) ||
    (form.cacheCreationInputPrice !== '' && !pricePattern.test(form.cacheCreationInputPrice))
  )
    return t('resources.pricing.invalidAmount')
  if (!datePattern.test(form.effectiveDate)) return t('resources.pricing.invalidDate')
  return ''
}

export function subscriptionPriceFormError(
  form: SubscriptionPriceForm,
  prices: SubscriptionPrice[],
  editingID?: string,
) {
  if (!pricePattern.test(form.periodAmount)) return t('resources.pricing.invalidAmount')
  if (!datePattern.test(form.effectiveDate)) return t('resources.pricing.invalidDate')
  if (
    prices.some(
      (price) => price.id !== editingID && price.effectiveAt.slice(0, 10) === form.effectiveDate,
    )
  )
    return t('resources.pricing.duplicateEffectiveDate')
  return ''
}

// 新增价格版本时 priceID 为空；生效日期按 UTC 自然日零点提交。
export function saveModelPrice(
  resourceID: string,
  mappingID: string,
  priceID: string | undefined,
  currency: PriceCurrency,
  form: ModelPriceForm,
) {
  const path = `/resources/${resourceID}/prices/models/${mappingID}`
  return api<ModelPrice>(priceID ? `${path}/${priceID}` : path, 'PUT', {
    currency,
    inputPrice: form.inputPrice,
    outputPrice: form.outputPrice,
    cachedInputPrice: form.cachedInputPrice,
    cacheCreationInputPrice: form.cacheCreationInputPrice || null,
    effectiveAt: `${form.effectiveDate}T00:00:00Z`,
  })
}

export function saveSubscriptionPrice(
  resourceID: string,
  priceID: string | undefined,
  currency: PriceCurrency,
  form: SubscriptionPriceForm,
) {
  const path = `/resources/${resourceID}/prices/subscription`
  return api<SubscriptionPrice>(priceID ? `${path}/${priceID}` : path, 'PUT', {
    currency,
    periodAmount: form.periodAmount,
    billingPeriod: form.billingPeriod,
    effectiveAt: `${form.effectiveDate}T00:00:00Z`,
  })
}
