import { validText } from './composables'
import { t } from './i18n'
import type { Model, ModelProviderMapping, Modality } from './types'

// 模型编辑表单与服务商优先级的校验和展示规则，均为无副作用的纯函数。
export const modalities: Modality[] = ['TEXT', 'IMAGE', 'AUDIO', 'VIDEO']

export type ModelForm = {
  name: string
  code: string
  publisherProviderId: string
  inputModalities: Modality[]
  outputModalities: Modality[]
  remark: string
}

export function fillModelForm(form: ModelForm, model: Model | null) {
  Object.assign(form, {
    name: model?.name ?? '',
    code: model?.code ?? '',
    publisherProviderId: model?.publisherProviderId ?? '',
    inputModalities: [...(model?.inputModalities ?? [])],
    outputModalities: [...(model?.outputModalities ?? [])],
    remark: model?.remark ?? '',
  })
}

// 校验通过时返回空字符串，否则返回首个错误提示。
export function modelFormError(form: ModelForm) {
  if (
    !validText(form.name, 128) ||
    !validText(form.code, 128) ||
    (form.remark !== '' && !validText(form.remark, 2000))
  )
    return t('common.byteLimit')
  if (!form.inputModalities.length || !form.outputModalities.length)
    return t('models.modalityRequired')
  return ''
}

// 优先级为 1 到 100 的整数。
export function validPriority(value: string) {
  return /^(?:[1-9]|[1-9]\d|100)$/.test(value)
}

export function providerRoutingAvailable(row: ModelProviderMapping) {
  return row.providerStatus === 'ACTIVE' && row.endpointConfigured && row.resourceAvailable
}

export function providerRoutingStatus(row: ModelProviderMapping) {
  if (row.providerStatus !== 'ACTIVE') return t('models.providerDisabled')
  if (!row.endpointConfigured) return t('models.providerNoEndpoint')
  if (!row.resourceAvailable) return t('models.providerNoResource')
  return t('models.providerAvailable')
}
