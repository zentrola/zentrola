<script setup lang="ts">
import { computed } from 'vue'
import { compactCount, count, listFormat } from '../../composables'
import { activeLocale, t } from '../../i18n'
import { useProviderRuntime } from '../../providerRuntime'
import type { Dashboard, Provider, Resource } from '../../types'
import Icon from '../Icon.vue'

const props = defineProps<{
  summary: Dashboard | null
  month: Date
  providers: Provider[]
  resources: Resource[]
  healthLoading: boolean
  healthError: string
}>()

const monthLabel = computed(() =>
  new Intl.DateTimeFormat(activeLocale.value, {
    year: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  }).format(props.month),
)
const enabledHealthProviders = computed(() =>
  props.providers.filter((provider) => provider.status === 'ACTIVE'),
)
const healthResources = computed(() => props.resources)
// 与服务商页“运行状态：异常”筛选使用同一判定规则，保证提醒数量与跳转后的列表一致。
const { runtimeFor, abnormal } = useProviderRuntime(enabledHealthProviders, healthResources)
const providerHealthIssues = computed(() =>
  enabledHealthProviders.value
    .filter(abnormal)
    .map((provider) => ({ provider, reason: runtimeFor(provider).reason })),
)
const availableProviderCount = computed(
  () => enabledHealthProviders.value.length - providerHealthIssues.value.length,
)
const providerHealthState = computed(() => {
  if (props.healthLoading) return 'loading'
  if (props.healthError) return 'unknown'
  if (!enabledHealthProviders.value.length) return 'empty'
  if (!providerHealthIssues.value.length) return 'healthy'
  return availableProviderCount.value > 0 ? 'degraded' : 'unavailable'
})
const providerHealthSummary = computed(() => {
  switch (providerHealthState.value) {
    case 'loading':
      return t('home.providerHealthChecking')
    case 'unknown':
      return t('home.providerHealthUnknown')
    case 'empty':
      return t('home.providerHealthEmpty')
    case 'healthy':
      return t('home.providerHealthHealthy')
    case 'unavailable':
      return t('home.providerHealthUnavailable')
    default:
      return t('home.providerHealthPartial', {
        available: count(availableProviderCount.value),
        total: count(enabledHealthProviders.value.length),
      })
  }
})
const providerHealthAlertTitle = computed(() =>
  providerHealthIssues.value.length === 1
    ? t('home.providerHealthSingleIssue', {
        name: providerHealthIssues.value[0]?.provider.name || '',
      })
    : t('home.providerHealthMultipleIssues', {
        count: count(providerHealthIssues.value.length),
      }),
)
const providerHealthAlertDetail = computed(() =>
  providerHealthIssues.value.length === 1
    ? providerHealthIssues.value[0]?.reason || ''
    : t('home.providerHealthAffected', {
        names: listFormat(providerHealthIssues.value.map((issue) => issue.provider.name)),
      }),
)
</script>

<template>
  <section class="dashboard-metrics" :aria-label="t('home.monthOverview')">
    <div class="dashboard-section-head">
      <div>
        <h2>{{ t('home.monthOverview') }}</h2>
      </div>
      <span class="dashboard-period-group">
        <span class="dashboard-period">{{ monthLabel }}</span
        ><span class="utc-badge" :title="t('common.utcDayHint')">{{ t('common.utc') }}</span>
      </span>
    </div>
    <dl class="metric-strip">
      <div>
        <dt>{{ t('home.activeMembers') }}</dt>
        <dd>{{ summary ? count(summary.activeMemberCount) : t('common.none') }}</dd>
      </div>
      <div>
        <dt>{{ t('home.supportedModels') }}</dt>
        <dd>{{ summary ? count(summary.modelCount) : t('common.none') }}</dd>
      </div>
      <div class="provider-total">
        <div class="provider-metric-label">
          <dt>{{ t('home.providers') }}</dt>
          <span
            class="provider-health-badge"
            :class="`is-${providerHealthState}`"
            role="img"
            :aria-label="providerHealthSummary"
            :title="providerHealthSummary"
          >
            <i></i>
          </span>
        </div>
        <dd>{{ summary ? count(summary.providerCount) : t('common.none') }}</dd>
      </div>
      <div class="token-total">
        <dt>{{ t('home.monthTokens') }}</dt>
        <dd
          :title="summary ? count(summary.totalTokens) : undefined"
          :aria-label="summary ? count(summary.totalTokens) : undefined"
        >
          {{ summary ? compactCount(summary.totalTokens) : t('common.none') }}
        </dd>
      </div>
    </dl>
    <div
      v-if="providerHealthIssues.length && !healthLoading && !healthError"
      class="provider-health-alert"
      :class="{ critical: availableProviderCount === 0 }"
      role="alert"
    >
      <Icon name="alert" :size="17" />
      <div>
        <strong>{{ providerHealthAlertTitle }}</strong>
        <span>{{ providerHealthAlertDetail }}</span>
      </div>
      <RouterLink :to="{ path: '/providers', query: { runtimeStatus: 'ABNORMAL' } }">
        {{ t('home.providerHealthViewProviders') }}
      </RouterLink>
    </div>
  </section>
</template>
