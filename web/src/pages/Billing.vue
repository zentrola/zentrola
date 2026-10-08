<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { all, api, errorText } from '../api'
import { count } from '../composables'
import { t } from '../i18n'
import type {
  BillingCurrency,
  BillingStatistics,
  BillingType,
  Page,
  Resource,
  UsageCostSummary,
  UsageStatistic,
} from '../types'
import DateRangePicker from '../components/DateRangePicker.vue'
import PageHeader from '../components/PageHeader.vue'
import TableScroll from '../components/TableScroll.vue'
import UsageCosts from '../components/UsageCosts.vue'

type BillingView = 'usageCosts' | 'overview'
type StatisticDimension = 'member' | 'application' | 'model' | 'provider'

const statisticDimensions: StatisticDimension[] = ['member', 'application', 'model', 'provider']
const activeView = ref<BillingView>('overview')
const from = ref('')
const to = ref('')
const statistics = ref<BillingStatistics | null>(null)
const currentUsageCosts = ref<UsageCostSummary | null>(null)
const currentResources = ref<Resource[]>([])
const usageRanking = ref<UsageStatistic[]>([])
const statisticDimension = ref<StatisticDimension>('member')
const statisticsLoading = ref(false)
const statisticsError = ref('')
const appliedCurrentMonthBasis = ref(false)
let statisticsRevision = 0

const currentMonthRangeSelected = computed(() => {
  const now = new Date()
  const monthStart = dateValue(new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1)))
  const nextMonth = dateValue(new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1)))
  const today = dateValue(now)
  return from.value === monthStart && to.value >= today && to.value < nextMonth
})
const activeSubscriptionResources = computed(() => {
  const now = new Date().toISOString()
  return currentResources.value.filter(
    (resource) =>
      resource.authType === 'SUBSCRIPTION' &&
      resource.subscriptionPrice &&
      (!resource.effectiveAt || resource.effectiveAt <= now) &&
      (!resource.expiresAt || resource.expiresAt > now),
  )
})

function dateValue(value: Date) {
  return value.toISOString().slice(0, 10)
}
function resetDates() {
  const now = new Date()
  const currentMonth = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1))
  from.value = dateValue(currentMonth)
  to.value = dateValue(now)
}
function rangeParams() {
  const start = Date.parse(`${from.value}T00:00:00Z`)
  const endDate = new Date(`${to.value}T00:00:00Z`)
  endDate.setUTCDate(endDate.getUTCDate() + 1)
  const end = endDate.getTime()
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    end <= start ||
    end - start > 366 * 86400000
  )
    return null
  return new URLSearchParams({
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
  })
}
async function loadStatistics() {
  const params = rangeParams()
  if (!params) {
    statisticsError.value = t('billing.invalidRange')
    return
  }
  const billingParams = new URLSearchParams(params)
  billingParams.set('limit', '100')
  const rankingParams = new URLSearchParams(params)
  rankingParams.delete('billingType')
  rankingParams.set('dimension', statisticDimension.value)
  rankingParams.set('limit', '10')
  const revision = ++statisticsRevision
  statisticsLoading.value = true
  statisticsError.value = ''
  try {
    const useCurrentBasis = currentMonthRangeSelected.value
    const [result, ranking, usageCosts, resources] = await Promise.all([
      api<BillingStatistics>(
        useCurrentBasis
          ? `/billing/current-attribution?${billingParams}`
          : `/billing/statistics?${billingParams}`,
      ),
      api<Page<UsageStatistic>>(`/usage/statistics?${rankingParams}`),
      useCurrentBasis
        ? api<UsageCostSummary>(`/billing/usage-costs/summary?${params}`)
        : Promise.resolve(null),
      useCurrentBasis ? all<Resource>('/resources') : Promise.resolve([]),
    ])
    if (revision === statisticsRevision) {
      statistics.value = result
      usageRanking.value = ranking.items
      currentUsageCosts.value = usageCosts
      currentResources.value = resources
      appliedCurrentMonthBasis.value = useCurrentBasis
    }
  } catch (error) {
    if (revision === statisticsRevision) statisticsError.value = errorText(error)
  } finally {
    if (revision === statisticsRevision) statisticsLoading.value = false
  }
}
function search() {
  if (activeView.value === 'usageCosts') return
  void loadStatistics()
}
function selectView(view: BillingView) {
  if (view === activeView.value) return
  activeView.value = view
}
function billingLabel(value: BillingType) {
  return t(value === 'SUBSCRIPTION' ? 'billing.subscription' : 'billing.apiKey')
}
function principalLabel(value: string) {
  return t(`billing.principalTypes.${value}`)
}
function amount(value: string, valueCurrency: BillingCurrency) {
  const match = value.match(/^(-?)(\d+)(?:\.(\d+))?$/)
  if (!match) return '-'
  const fraction = (match[3] || '').replace(/0+$/, '')
  const integer = match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  const symbol = valueCurrency === 'CNY' ? '¥' : '$'
  return `${match[1]}${symbol}${integer}${fraction ? `.${fraction}` : ''}`
}
function totalFor(valueCurrency: BillingCurrency, valueType: BillingType) {
  return statistics.value?.totals.find(
    (item) => item.currency === valueCurrency && item.billingType === valueType,
  )
}
function addAmounts(values: string[]) {
  const scale = 100000000n
  let total = 0n
  for (const value of values) {
    const match = value.match(/^(\d+)(?:\.(\d+))?$/)
    if (!match) continue
    const fraction = (match[2] || '').padEnd(8, '0').slice(0, 8)
    total += BigInt(match[1]) * scale + BigInt(fraction || '0')
  }
  const integer = total / scale
  const fraction = (total % scale).toString().padStart(8, '0').replace(/0+$/, '')
  return `${integer}${fraction ? `.${fraction}` : ''}`
}
function currentSubscriptionAmount(valueCurrency: BillingCurrency) {
  return addAmounts(
    activeSubscriptionResources.value
      .filter((resource) => resource.subscriptionPrice?.currency === valueCurrency)
      .map((resource) => resource.subscriptionPrice?.periodAmount || '0'),
  )
}
function currentSubscriptionCount(valueCurrency: BillingCurrency) {
  return activeSubscriptionResources.value.filter(
    (resource) => resource.subscriptionPrice?.currency === valueCurrency,
  ).length
}
function currentAPIKeyAmount(valueCurrency: BillingCurrency) {
  return (
    currentUsageCosts.value?.totals.find((item) => item.currency === valueCurrency)?.amount || '0'
  )
}
function overviewAmount(valueCurrency: BillingCurrency, valueType: BillingType) {
  if (!appliedCurrentMonthBasis.value) return totalFor(valueCurrency, valueType)?.totalAmount || '0'
  return valueType === 'SUBSCRIPTION'
    ? currentSubscriptionAmount(valueCurrency)
    : currentAPIKeyAmount(valueCurrency)
}
function overviewMeta(valueCurrency: BillingCurrency, valueType: BillingType) {
  if (!appliedCurrentMonthBasis.value)
    return `${count(totalFor(valueCurrency, valueType)?.totalTokens ?? 0)} Token`
  if (valueType === 'SUBSCRIPTION')
    return t('billing.activeSubscriptions', { count: currentSubscriptionCount(valueCurrency) })
  return t('billing.currentApiKeyCost')
}
function hasOverviewCosts() {
  return (['CNY', 'USD'] as BillingCurrency[]).some((valueCurrency) =>
    (['SUBSCRIPTION', 'API_KEY'] as BillingType[]).some(
      (valueType) => !/^0(?:\.0+)?$/.test(overviewAmount(valueCurrency, valueType)),
    ),
  )
}
function hasCurrentSubscriptionCosts() {
  return (['CNY', 'USD'] as BillingCurrency[]).some(
    (valueCurrency) => !/^0(?:\.0+)?$/.test(currentSubscriptionAmount(valueCurrency)),
  )
}
function selectStatisticDimension(value: StatisticDimension) {
  if (statisticsLoading.value || statisticDimension.value === value) return
  statisticDimension.value = value
  void loadStatistics()
}
function statisticShare(value: UsageStatistic) {
  if (!value.overallTokens) return '0%'
  return `${((value.tokens / value.overallTokens) * 100).toFixed(1).replace('.0', '')}%`
}
function statisticCount(value: UsageStatistic) {
  return t(statisticDimension.value === 'provider' ? 'billing.callCount' : 'billing.requestCount', {
    count: count(value.count),
  })
}
onMounted(() => {
  resetDates()
  search()
})
onBeforeUnmount(() => {
  statisticsRevision++
})
</script>

<template>
  <PageHeader name="billing" />
  <div class="view-tabs billing-tabs" role="tablist" :aria-label="t('nav.billing')">
    <button
      v-for="view in ['overview', 'usageCosts'] as BillingView[]"
      :key="view"
      type="button"
      role="tab"
      :aria-selected="activeView === view"
      :class="{ selected: activeView === view }"
      @click="selectView(view)"
    >
      {{ t(`billing.${view}`) }}
    </button>
  </div>

  <section v-if="activeView !== 'usageCosts'" class="panel billing-filter-panel">
    <form class="usage-filters billing-filters" @submit.prevent="search">
      <DateRangePicker v-model:from="from" v-model:to="to" />
      <button class="button primary" :disabled="statisticsLoading">
        {{ t('billing.search') }}
      </button>
    </form>
  </section>

  <UsageCosts v-if="activeView === 'usageCosts'" />

  <template v-else-if="activeView === 'overview'">
    <div v-if="statisticsError" class="alert error billing-alert" role="alert">
      {{ statisticsError }}
      <button type="button" class="text-button" @click="loadStatistics">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-else-if="statisticsLoading && !statistics" class="empty-state">
      {{ t('common.loading') }}
    </p>
    <template v-else-if="statistics">
      <section class="panel cost-ledger cost-overview-panel" aria-live="polite">
        <header class="cost-overview-heading">
          <div>
            <h2>{{ t('billing.costBreakdown') }}</h2>
            <p>
              {{
                t(
                  appliedCurrentMonthBasis
                    ? 'billing.currentOverviewDescription'
                    : 'billing.overviewDescription',
                )
              }}
            </p>
          </div>
          <span v-if="!appliedCurrentMonthBasis" class="settled-badge">
            {{ t('billing.settledBasis') }}
          </span>
        </header>
        <div v-if="!hasOverviewCosts()" class="cost-empty-notice">
          <strong>
            {{ t(appliedCurrentMonthBasis ? 'billing.noCurrentCosts' : 'billing.noSettledCosts') }}
          </strong>
          <span>
            {{
              t(
                appliedCurrentMonthBasis
                  ? 'billing.noCurrentCostsHint'
                  : 'billing.noSettledCostsHint',
              )
            }}
          </span>
        </div>
        <div
          v-for="valueCurrency in ['CNY', 'USD'] as BillingCurrency[]"
          :key="valueCurrency"
          class="cost-ledger-row"
        >
          <strong class="cost-currency">{{ valueCurrency }}</strong>
          <div
            v-for="valueType in ['SUBSCRIPTION', 'API_KEY'] as BillingType[]"
            :key="valueType"
            class="cost-ledger-entry"
          >
            <span>{{ billingLabel(valueType) }}</span>
            <strong>{{ amount(overviewAmount(valueCurrency, valueType), valueCurrency) }}</strong>
            <small>{{ overviewMeta(valueCurrency, valueType) }}</small>
          </div>
        </div>
      </section>
      <section class="panel billing-table-panel cost-attribution-panel">
        <header class="cost-section-heading">
          <div>
            <h2>{{ t('billing.costAttribution') }}</h2>
            <p>
              {{
                t(
                  appliedCurrentMonthBasis
                    ? 'billing.currentCostAttributionDescription'
                    : 'billing.costAttributionDescription',
                )
              }}
            </p>
          </div>
        </header>
        <TableScroll>
          <table class="billing-table">
            <thead>
              <tr>
                <th>{{ t('billing.principal') }}</th>
                <th>{{ t('billing.principalType') }}</th>
                <th>{{ t('billing.billingType') }}</th>
                <th>{{ t('billing.currency') }}</th>
                <th class="numeric">{{ t('billing.tokens') }}</th>
                <th class="numeric">{{ t('billing.amount') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in statistics.items"
                :key="`${item.principalId}-${item.billingType}-${item.currency}`"
              >
                <td>
                  <strong>{{ item.principalName }}</strong
                  ><small class="subline">ID {{ item.principalId }}</small>
                </td>
                <td>{{ principalLabel(item.principalType) }}</td>
                <td>{{ billingLabel(item.billingType) }}</td>
                <td>{{ item.currency }}</td>
                <td class="numeric">{{ count(item.tokens) }}</td>
                <td class="numeric amount-cell" :class="{ negative: item.amount.startsWith('-') }">
                  {{ amount(item.amount, item.currency) }}
                </td>
              </tr>
              <tr v-if="!statistics.items.length">
                <td colspan="6" class="empty-cell">
                  {{
                    t(
                      appliedCurrentMonthBasis
                        ? hasCurrentSubscriptionCosts()
                          ? 'billing.emptyCurrentAttribution'
                          : 'billing.emptyCurrentAttributionNoCosts'
                        : 'billing.emptyOverview',
                    )
                  }}
                </td>
              </tr>
            </tbody>
          </table>
        </TableScroll>
      </section>
      <section class="panel usage-ranking-panel">
        <header class="cost-section-heading usage-ranking-heading">
          <div>
            <h2>{{ t('billing.usageRanking') }}</h2>
            <p>{{ t('billing.usageRankingDescription') }}</p>
          </div>
          <div class="dimension-tabs" role="tablist" :aria-label="t('billing.usageRanking')">
            <button
              v-for="value in statisticDimensions"
              :key="value"
              type="button"
              role="tab"
              :disabled="statisticsLoading"
              :aria-selected="statisticDimension === value"
              :class="{ selected: statisticDimension === value }"
              @click="selectStatisticDimension(value)"
            >
              {{ t(`usage.${value}`) }}
            </button>
          </div>
        </header>
        <ol v-if="usageRanking.length" class="cost-usage-ranking">
          <li v-for="(row, index) in usageRanking" :key="row.entityId">
            <div class="cost-rank-marker" :class="`rank-${Math.min(index + 1, 4)}`">
              <span class="cost-rank-number">{{ index + 1 }}</span>
            </div>
            <div class="cost-rank-identity">
              <strong>{{ row.name }}</strong>
              <small class="cost-rank-meta">
                <span>{{ row.code || '-' }}</span>
                <span v-if="index === 0" class="cost-rank-highlight">
                  {{ t('billing.highestUsage') }}
                </span>
              </small>
            </div>
            <div class="cost-rank-value">
              <strong>{{ count(row.tokens) }}</strong>
              <small>{{ statisticShare(row) }} · {{ statisticCount(row) }}</small>
            </div>
          </li>
        </ol>
        <p v-else class="empty-state cost-ranking-empty">{{ t('usage.statisticsEmpty') }}</p>
      </section>
    </template>
  </template>
</template>
