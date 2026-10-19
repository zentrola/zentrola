import { onUnmounted, reactive } from 'vue'
import { api, errorText } from './api'
import { codeText, i18n, t } from './i18n'
import { showErrorToast, showSuccessToast } from './toast'
import type {
  ConnectionResult,
  Provider,
  QuotaRefreshResult,
  RateLimitResetCredits,
  ResetCreditConsumeResult,
  Resource,
  ResourceQuota,
} from './types'

const defaultQuotaWindows = ['codex.primary', 'codex.secondary', 'five_hour', 'seven_day']

export function displayableQuota(quota: ResourceQuota) {
  return (
    quota.remainingValue !== null ||
    quota.usedPercent !== null ||
    quota.limitValue !== null ||
    quota.usedValue !== null
  )
}

export function quotaWindowLabel(quota: ResourceQuota) {
  const seconds = quota.windowDurationSeconds
  let window = ''
  if (seconds && seconds % 86400 === 0)
    window = t('resources.quotaWindowDays', { value: seconds / 86400 })
  else if (seconds && seconds % 3600 === 0)
    window = t('resources.quotaWindowHours', { value: seconds / 3600 })
  else if (seconds && seconds % 60 === 0)
    window = t('resources.quotaWindowMinutes', { value: seconds / 60 })
  const name = quota.name?.trim() || (defaultQuotaWindows.includes(quota.code) ? '' : quota.code)
  return [name, window].filter(Boolean).join(' · ') || quota.code
}

export function quotaRemainingLabel(quota: ResourceQuota) {
  if (quota.remainingValue !== null) {
    return t('resources.quotaRemainingValue', {
      value: quota.remainingValue,
      unit: quota.unit ? ` ${quota.unit}` : '',
    })
  }
  if (quota.usedPercent !== null) {
    const value = new Intl.NumberFormat(i18n.global.locale.value, {
      maximumFractionDigits: 1,
    }).format(Math.max(0, 100 - quota.usedPercent))
    return t('resources.quotaRemainingPercent', { value })
  }
  if (quota.limitValue !== null && quota.usedValue !== null) {
    return t('resources.quotaUsageValue', {
      used: quota.usedValue,
      limit: quota.limitValue,
      unit: quota.unit ? ` ${quota.unit}` : '',
    })
  }
  return t('resources.quotaAmountUnknown')
}

export function errorCodeText(code: string) {
  return codeText('errors', code, t('errors.UNKNOWN'))
}

// 连接测试结果转文案：失败时按错误码展示，Codex App Server 不可用时提示可执行文件配置项。
export function connectionResultMessage(
  result: ConnectionResult,
  resource?: Resource,
  activation = false,
) {
  if (result.ok) {
    if (activation) return t('providers.activationPassed')
    return t(
      resource?.authType === 'SUBSCRIPTION' && resource.authAdapter !== 'ANTHROPIC_CLAUDE_CODE'
        ? 'resources.subscriptionTestPassed'
        : 'resources.testPassed',
    )
  }
  if (result.code === 'CODEX_APP_SERVER_UNAVAILABLE') {
    return t('errors.CODEX_APP_SERVER_UNAVAILABLE', { executable: 'CODEX_EXECUTABLE' })
  }
  return errorCodeText(result.code)
}

// 凭据弹窗中的订阅额度、额度重置卡与刷新状态。
export function useCredentialQuotas(options: {
  loadResources: () => Promise<void>
  resourcesFor: (provider: Provider) => Resource[]
}) {
  const quotas = reactive<Record<string, ResourceQuota[]>>({})
  const resetCredits = reactive<Record<string, RateLimitResetCredits>>({})
  const refreshing = reactive<Record<string, boolean>>({})
  const refreshErrors = reactive<Record<string, string>>({})
  const consuming = reactive<Record<string, boolean>>({})
  const idempotencyKeys: Record<string, string> = {}
  // 凭据弹窗会话号：关闭或切换服务商后递增，进行中的自动额度刷新据此停止后续请求。
  let session = 0

  function displayableQuotas(resourceID: string) {
    return (quotas[resourceID] || []).filter(displayableQuota)
  }

  async function refreshSubscriptionQuota(resource: Resource) {
    if (refreshing[resource.id]) return
    refreshing[resource.id] = true
    delete refreshErrors[resource.id]
    try {
      const result = await api<ConnectionResult>(
        `/resources/${resource.id}/test-connection`,
        'POST',
      )
      if (!result.ok) {
        refreshErrors[resource.id] = connectionResultMessage(result, resource)
        return
      }
      const latest = await api<ResourceQuota[]>(`/resources/${resource.id}/quotas`)
      // 请求成功后再整体替换，刷新期间继续展示上一次的额度，避免闪空。
      if (result.resetCredits) resetCredits[resource.id] = result.resetCredits
      else delete resetCredits[resource.id]
      quotas[resource.id] = latest
    } catch (error) {
      refreshErrors[resource.id] = errorText(error)
    } finally {
      delete refreshing[resource.id]
    }
  }

  // 手动刷新 Claude 订阅额度：后端发送一次最小推理请求，只从响应头读取统一限额。
  async function refreshClaudeQuota(resource: Resource) {
    if (refreshing[resource.id]) return
    refreshing[resource.id] = true
    delete refreshErrors[resource.id]
    try {
      const result = await api<QuotaRefreshResult>(
        `/resources/${resource.id}/refresh-quota`,
        'POST',
      )
      if (result.quotas) quotas[resource.id] = result.quotas
      if (result.ok) {
        await options.loadResources()
        showSuccessToast(t('resources.quotaRefreshed'))
      } else {
        refreshErrors[resource.id] = errorCodeText(result.code)
      }
    } catch (error) {
      refreshErrors[resource.id] = errorText(error)
    } finally {
      delete refreshing[resource.id]
    }
  }

  async function consumeResetCredit(resource: Resource) {
    if (consuming[resource.id]) return
    consuming[resource.id] = true
    try {
      const idempotencyKey =
        idempotencyKeys[resource.id] || (idempotencyKeys[resource.id] = crypto.randomUUID())
      const creditID = resetCredits[resource.id]?.credits.find(
        (credit) => credit.status === 'available',
      )?.id
      const result = await api<ResetCreditConsumeResult>(
        `/resources/${resource.id}/rate-limit-reset-credit/consume`,
        'POST',
        {
          idempotencyKey,
          ...(creditID ? { creditId: creditID } : {}),
        },
      )
      if (result.resetCredits) resetCredits[resource.id] = result.resetCredits
      quotas[resource.id] = await api<ResourceQuota[]>(`/resources/${resource.id}/quotas`)
      await options.loadResources()
      if (result.outcome === 'reset' || result.outcome === 'alreadyRedeemed') {
        showSuccessToast(t('resources.resetCreditConsumed'))
      } else {
        showErrorToast(t(`resources.resetCreditOutcomes.${result.outcome}`))
      }
      delete idempotencyKeys[resource.id]
    } catch (error) {
      showErrorToast(errorText(error))
    } finally {
      delete consuming[resource.id]
    }
  }

  // Claude 订阅的额度来自连接测试和网关转发时的推理响应头；打开凭据弹窗只读取已记录的额度，
  // 不为刷新额度额外发起会产生费用的模型调用。
  async function loadObservedQuota(resource: Resource) {
    try {
      quotas[resource.id] = await api<ResourceQuota[]>(`/resources/${resource.id}/quotas`)
    } catch (error) {
      refreshErrors[resource.id] = errorText(error)
    }
  }

  async function refreshProviderQuotas(provider: Provider, current: number) {
    await options.loadResources()
    if (current !== session) return
    const subscriptions = options
      .resourcesFor(provider)
      .filter((resource) => resource.authType === 'SUBSCRIPTION')
    await Promise.all(
      subscriptions
        .filter((resource) => resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE')
        .map((resource) => {
          delete refreshErrors[resource.id]
          return loadObservedQuota(resource)
        }),
    )
    const codexSubscriptions = subscriptions.filter(
      (resource) => resource.authAdapter === 'OPENAI_CODEX',
    )
    // Codex App Server 的账号额度读取不适合在同一服务商下并发执行；串行刷新可避免
    // 多个个人订阅同时启动探测进程时，后续账号被上游或本地进程链路拒绝。
    for (const subscription of codexSubscriptions) {
      if (current !== session) return
      await refreshSubscriptionQuota(subscription)
    }
    if (codexSubscriptions.length && current === session) await options.loadResources()
  }

  // 打开凭据弹窗时开启新会话并自动刷新额度；关闭时结束会话，停止后续自动请求。
  function openSession(provider: Provider) {
    void refreshProviderQuotas(provider, ++session)
  }
  function closeSession() {
    session++
  }
  onUnmounted(closeSession)

  return {
    quotas,
    resetCredits,
    refreshing,
    refreshErrors,
    consuming,
    displayableQuotas,
    refreshClaudeQuota,
    consumeResetCredit,
    openSession,
    closeSession,
  }
}

export type CredentialQuotas = ReturnType<typeof useCredentialQuotas>
