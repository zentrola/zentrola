import { computed, type Ref } from 'vue'
import { codeText, t } from './i18n'
import type { Provider, Resource } from './types'

export type ProviderRuntimeStatus =
  | 'UNCONFIGURED'
  | 'MISSING_MODEL'
  | 'HEALTHY'
  | 'PENDING_ENABLE'
  | 'DEGRADED'
  | 'BLOCKED'
  | 'UNAVAILABLE'

export type ProviderRuntime = {
  status: ProviderRuntimeStatus
  reason: string
  errorCode: string
}

// 已启用服务商出现这些运行状态时视为异常；首页提醒与服务商页筛选共用同一规则。
const abnormalStatuses: ReadonlySet<ProviderRuntimeStatus> = new Set([
  'UNCONFIGURED',
  'MISSING_MODEL',
  'BLOCKED',
  'UNAVAILABLE',
])

const emptyResources: Resource[] = []

export function groupResourcesByProvider(resources: Resource[]) {
  const grouped = new Map<string, Resource[]>()
  for (const resource of resources) {
    const group = grouped.get(resource.providerId)
    if (group) group.push(resource)
    else grouped.set(resource.providerId, [resource])
  }
  return grouped
}

// 选择服务商的首选凭据：可用的订阅优先，其次 API Key，同级按优先级数值从小到大。
function resourcePreference(resource: Resource) {
  if (
    resource.authType === 'SUBSCRIPTION' &&
    (resource.quotaStatus === 'AVAILABLE' || resource.quotaStatus === 'NEAR_LIMIT')
  )
    return 0
  return resource.authType === 'API_KEY' || !resource.authType ? 1 : 2
}

export function preferredResource(candidates: Resource[]) {
  let selected: Resource | undefined
  for (const candidate of candidates) {
    if (!candidate.isEnabled) continue
    if (
      !selected ||
      resourcePreference(candidate) < resourcePreference(selected) ||
      (resourcePreference(candidate) === resourcePreference(selected) &&
        candidate.priority < selected.priority)
    )
      selected = candidate
  }
  return selected
}

export function blockedResourceReason(resource: Resource) {
  const reason = codeText(
    'resources.blockReasons',
    resource.blockedReason || 'UNKNOWN_PERMANENT',
    t('resources.blockReasons.UNKNOWN_PERMANENT'),
  )
  return resource.lastHttpStatus ? `${reason} · HTTP ${resource.lastHttpStatus}` : reason
}

export function calculateProviderRuntime(
  provider: Provider,
  configured: Resource[],
): ProviderRuntime {
  if (!configured.length) {
    return {
      status: 'UNCONFIGURED',
      reason: t('providers.runtimeReasons.UNCONFIGURED'),
      errorCode: '',
    }
  }

  if (provider.modelCount === 0) {
    return {
      status: 'MISSING_MODEL',
      reason: t('providers.runtimeReasons.MISSING_MODEL'),
      errorCode: '',
    }
  }

  const enabled = configured.filter((resource) => resource.isEnabled)
  if (!enabled.length) {
    return {
      status: 'UNAVAILABLE',
      reason: t('providers.runtimeReasons.CREDENTIAL_DISABLED'),
      errorCode: '',
    }
  }

  let healthy = false
  let failed: Resource | undefined
  for (const resource of enabled) {
    if (resource.runtimeStatus !== 'BLOCKED') {
      healthy = true
      continue
    }
    if (!failed || (resource.lastErrorAt || '') > (failed.lastErrorAt || '')) failed = resource
  }

  if (healthy && !failed) {
    return {
      status: provider.status === 'ACTIVE' ? 'HEALTHY' : 'PENDING_ENABLE',
      reason: '',
      errorCode: '',
    }
  }

  return {
    status: healthy ? 'DEGRADED' : 'BLOCKED',
    reason: failed ? blockedResourceReason(failed) : t('providers.runtimeReasons.UNKNOWN'),
    errorCode: failed?.lastErrorCode || '',
  }
}

export function providerRuntimeAbnormal(provider: Provider, runtime: ProviderRuntime) {
  return provider.status === 'ACTIVE' && abnormalStatuses.has(runtime.status)
}

export function providerRuntimeLabel(runtime: ProviderRuntime) {
  const status = codeText('state', runtime.status)
  const hint = [runtime.reason, runtime.errorCode].filter(Boolean).join(' · ')
  return hint ? t('providers.runtimeLabel', { status, hint }) : status
}

// providers 传入实际展示的行集合，运行状态只按这些行计算一次。
export function useProviderRuntime(providers: Ref<Provider[]>, resources: Ref<Resource[]>) {
  const resourcesByProvider = computed(() => groupResourcesByProvider(resources.value))
  function resourcesFor(provider: Provider) {
    return resourcesByProvider.value.get(provider.id) ?? emptyResources
  }
  const runtimeByID = computed(
    () =>
      new Map(
        providers.value.map((provider) => [
          provider.id,
          calculateProviderRuntime(provider, resourcesFor(provider)),
        ]),
      ),
  )
  function runtimeFor(provider: Provider) {
    return (
      runtimeByID.value.get(provider.id) ??
      calculateProviderRuntime(provider, resourcesFor(provider))
    )
  }
  function abnormal(provider: Provider) {
    return providerRuntimeAbnormal(provider, runtimeFor(provider))
  }
  const preferredByProvider = computed(() => {
    const preferred = new Map<string, Resource>()
    for (const [providerID, candidates] of resourcesByProvider.value) {
      const selected = preferredResource(candidates)
      if (selected) preferred.set(providerID, selected)
    }
    return preferred
  })
  function resourceFor(provider: Provider) {
    return preferredByProvider.value.get(provider.id)
  }
  return { resourcesByProvider, resourcesFor, resourceFor, runtimeByID, runtimeFor, abnormal }
}
