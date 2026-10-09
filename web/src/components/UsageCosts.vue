<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { all, api, download, errorText } from '../api'
import { count, date, dateOnly, useCollection } from '../composables'
import { t } from '../i18n'
import { showErrorToast } from '../toast'
import type { BillingCurrency, Model, UsageCost, UsageCostDetail, UsageCostSummary } from '../types'
import DateRangePicker from './DateRangePicker.vue'
import ListFooter from './ListFooter.vue'
import Modal from './Modal.vue'
import Status from './Status.vue'
import TableScroll from './TableScroll.vue'
import TechnicalValue from './TechnicalValue.vue'

const from = ref('')
const to = ref('')
const principalType = ref('')
const principalId = ref('')
const modelId = ref('')
const ratingStatus = ref('')
const billingType = ref('')
const query = ref('')
const summary = ref<UsageCostSummary | null>(null)
const summaryLoading = ref(false)
const summaryError = ref('')
const exporting = ref(false)
const models = ref<Model[]>([])
const lookupError = ref('')
const selected = ref<UsageCostDetail | null>(null)
const detailOpen = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
let summaryRevision = 0
let detailRevision = 0

const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<UsageCost>(() => `/billing/usage-costs?${query.value}`)

const totalTokens = computed(
  () => (summary.value?.inputTokens || 0) + (summary.value?.outputTokens || 0),
)

function dateValue(value: Date) {
  return value.toISOString().slice(0, 10)
}
function resetDates() {
  const now = new Date()
  from.value = dateValue(new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1)))
  to.value = dateValue(now)
}
function params() {
  const start = Date.parse(`${from.value}T00:00:00Z`)
  const endDate = new Date(`${to.value}T00:00:00Z`)
  endDate.setUTCDate(endDate.getUTCDate() + 1)
  const end = endDate.getTime()
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    end <= start ||
    end - start > 366 * 86400000
  ) {
    showErrorToast(t('billing.invalidRange'))
    return null
  }
  const result = new URLSearchParams({
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
  })
  for (const [key, value] of [
    ['principalType', principalType.value],
    ['principalId', principalId.value.trim()],
    ['modelId', modelId.value],
    ['ratingStatus', ratingStatus.value],
    ['billingType', billingType.value],
  ])
    if (value) result.set(key, value)
  return result
}
async function search() {
  const result = params()
  if (!result) return
  result.set('sort', 'startedAt')
  result.set('order', 'desc')
  query.value = result.toString()
  const revision = ++summaryRevision
  summaryLoading.value = true
  summaryError.value = ''
  const summaryParams = new URLSearchParams(result)
  summaryParams.delete('sort')
  summaryParams.delete('order')
  const summaryPromise = api<UsageCostSummary>(`/billing/usage-costs/summary?${summaryParams}`)
    .then((value) => {
      if (revision === summaryRevision) summary.value = value
    })
    .catch((reason) => {
      if (revision === summaryRevision) summaryError.value = errorText(reason)
    })
    .finally(() => {
      if (revision === summaryRevision) summaryLoading.value = false
    })
  await Promise.all([load(), summaryPromise])
}
function reset() {
  principalType.value = ''
  principalId.value = ''
  modelId.value = ''
  ratingStatus.value = ''
  billingType.value = ''
  resetDates()
  void search()
}
async function loadLookups() {
  lookupError.value = ''
  try {
    models.value = await all<Model>('/models')
  } catch (reason) {
    lookupError.value = errorText(reason)
  }
}
function amount(value: string | null, valueCurrency: BillingCurrency | null) {
  if (!value || !valueCurrency) return '-'
  const match = value.match(/^(\d+)(?:\.(\d+))?$/)
  if (!match) return '-'
  const fraction = (match[2] || '').replace(/0+$/, '')
  const integer = match[1].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return `${valueCurrency === 'CNY' ? '¥' : '$'}${integer}${fraction ? `.${fraction}` : ''}`
}
function costTotal(valueCurrency: BillingCurrency) {
  return summary.value?.totals.find((value) => value.currency === valueCurrency)?.amount || '0'
}
function ratingLabel(value: string) {
  return t(`billing.ratingStatuses.${value}`)
}
function protocolLabel(value: string) {
  return t(`billing.protocols.${value}`)
}
async function openDetail(id: string) {
  const revision = ++detailRevision
  detailOpen.value = true
  detailLoading.value = true
  detailError.value = ''
  selected.value = null
  try {
    const result = await api<UsageCostDetail>(`/billing/usage-costs/${id}`)
    if (revision === detailRevision) selected.value = result
  } catch (reason) {
    if (revision === detailRevision) detailError.value = errorText(reason)
  } finally {
    if (revision === detailRevision) detailLoading.value = false
  }
}
function closeDetail() {
  detailRevision++
  detailOpen.value = false
  selected.value = null
}
async function exportCSV() {
  const result = params()
  if (!result || exporting.value) return
  result.set('sort', 'startedAt')
  result.set('order', 'desc')
  exporting.value = true
  try {
    const blob = await download(`/billing/usage-costs/export?${result}`)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `zentrola-usage-costs-${from.value}-${to.value}.csv`
    anchor.click()
    URL.revokeObjectURL(url)
  } catch (reason) {
    showErrorToast(errorText(reason))
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  resetDates()
  void loadLookups()
  void search()
})
</script>

<template>
  <section class="panel billing-filter-panel usage-cost-panel">
    <form class="usage-filters usage-cost-filters" @submit.prevent="search">
      <DateRangePicker v-model:from="from" v-model:to="to" />
      <fieldset class="usage-cost-principal-filter">
        <legend class="filter-label">{{ t('billing.principal') }}</legend>
        <div class="usage-cost-principal-controls">
          <select v-model="principalType" :aria-label="t('billing.principalType')">
            <option value="">{{ t('common.all') }}</option>
            <option value="MEMBER">{{ t('billing.principalTypes.MEMBER') }}</option>
            <option value="APPLICATION">{{ t('billing.principalTypes.APPLICATION') }}</option>
          </select>
          <input
            v-model="principalId"
            inputmode="numeric"
            :aria-label="t('billing.principalId')"
            :placeholder="t('billing.optionalId')"
          />
        </div>
      </fieldset>
      <label>
        <span class="filter-label">{{ t('billing.model') }}</span>
        <select v-model="modelId">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="model in models" :key="model.id" :value="model.id">
            {{ model.name }}
          </option>
        </select>
      </label>
      <label>
        <span class="filter-label">{{ t('billing.ratingStatus') }}</span>
        <select v-model="ratingStatus">
          <option value="">{{ t('common.all') }}</option>
          <option
            v-for="value in [
              'RATED',
              'SUBSCRIPTION_SHARED',
              'INCOMPLETE_TOKENS',
              'MISSING_PRICE',
              'PENDING_RATING',
              'NOT_BILLABLE',
            ]"
            :key="value"
            :value="value"
          >
            {{ ratingLabel(value) }}
          </option>
        </select>
      </label>
      <label>
        <span class="filter-label">{{ t('billing.billingType') }}</span>
        <select v-model="billingType">
          <option value="">{{ t('common.all') }}</option>
          <option value="API_KEY">{{ t('billing.apiKey') }}</option>
          <option value="SUBSCRIPTION">{{ t('billing.subscription') }}</option>
        </select>
      </label>
      <div class="usage-cost-actions">
        <button class="button primary" :disabled="loading || summaryLoading">
          {{ t('billing.search') }}
        </button>
        <button type="button" class="button secondary" :disabled="loading" @click="reset">
          {{ t('common.reset') }}
        </button>
        <button type="button" class="button secondary" :disabled="exporting" @click="exportCSV">
          {{ t(exporting ? 'billing.exporting' : 'billing.exportCsv') }}
        </button>
      </div>
    </form>
    <p v-if="lookupError" class="alert error">{{ lookupError }}</p>
  </section>

  <div v-if="summaryError" class="alert error billing-alert" role="alert">{{ summaryError }}</div>
  <section v-else class="billing-detail-metrics" aria-live="polite">
    <article class="panel">
      <span>{{ t('billing.tokens') }}</span
      ><strong>{{ count(totalTokens) }}</strong
      ><small>{{ count(summary?.cachedInputTokens ?? 0) }} {{ t('billing.cachedTokens') }}</small>
    </article>
    <article
      v-for="valueCurrency in ['CNY', 'USD'] as BillingCurrency[]"
      :key="valueCurrency"
      class="panel"
    >
      <span>{{ valueCurrency }}</span
      ><strong>{{ amount(costTotal(valueCurrency), valueCurrency) }}</strong
      ><small>{{ t('billing.accruedCost') }}</small>
    </article>
  </section>
  <p v-if="summary?.unrated" class="billing-rating-notice" role="status">
    {{ t('billing.pendingRatingNotice', { count: count(summary.unrated) }) }}
  </p>

  <section class="panel billing-table-panel">
    <div v-if="error" class="alert error" role="alert">
      {{ error }}
      <button type="button" class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </div>
    <p v-if="loading" class="empty-state">{{ t('common.loading') }}</p>
    <TableScroll v-else>
      <table class="billing-table usage-cost-table">
        <thead>
          <tr>
            <th>{{ t('billing.callTime') }}</th>
            <th>{{ t('billing.principal') }}</th>
            <th>{{ t('billing.model') }}</th>
            <th>{{ t('billing.resource') }}</th>
            <th>{{ t('billing.protocol') }}</th>
            <th>{{ t('billing.ratingStatus') }}</th>
            <th class="numeric">{{ t('billing.tokens') }}</th>
            <th class="numeric">{{ t('billing.amount') }}</th>
            <th>{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in items" :key="item.id">
            <td>
              {{ date(item.startedAt) }}<small class="subline">#{{ item.attemptNo }}</small>
            </td>
            <td>
              <strong>{{ item.principalName }}</strong
              ><small class="subline"
                >{{ t(`billing.principalTypes.${item.principalType}`) }} ·
                {{ item.principalId }}</small
              >
            </td>
            <td>
              <strong>{{ item.modelName }}</strong
              ><small class="subline">{{ item.providerName }}</small>
            </td>
            <td>
              <strong>{{ item.resourceName }}</strong
              ><small class="subline">{{ item.resourceId }}</small>
            </td>
            <td>{{ protocolLabel(item.clientProtocol) }}</td>
            <td>
              <span class="issue-badge" :class="item.ratingStatus.toLowerCase()">{{
                ratingLabel(item.ratingStatus)
              }}</span>
            </td>
            <td class="numeric">{{ count((item.inputTokens ?? 0) + (item.outputTokens ?? 0)) }}</td>
            <td class="numeric amount-cell">{{ amount(item.totalCost, item.currency) }}</td>
            <td>
              <button type="button" class="text-button" @click="openDetail(item.id)">
                {{ t('billing.details') }}
              </button>
            </td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="9" class="empty-cell">{{ t('billing.emptyUsageCosts') }}</td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <ListFooter
      :cursor="cursor"
      :page="page"
      :page-size="pageSize"
      :total="total"
      :loading="loading"
      @first="load()"
      @previous="previous"
      @more="load(true)"
      @page-size="setPageSize"
    />
  </section>

  <Modal
    v-if="detailOpen"
    :title="t('billing.usageCostDetails')"
    wide
    :busy="detailLoading"
    @close="closeDetail"
  >
    <p v-if="detailLoading" class="empty-state">{{ t('common.loading') }}</p>
    <div v-else-if="detailError" class="alert error">{{ detailError }}</div>
    <template v-else-if="selected">
      <dl class="detail-grid billing-detail-grid">
        <div>
          <dt>Usage ID</dt>
          <dd>{{ selected.id }}</dd>
        </div>
        <div class="billing-request-id">
          <dt>Request ID</dt>
          <dd><TechnicalValue :value="selected.requestId" /></dd>
        </div>
        <div>
          <dt>{{ t('billing.principal') }}</dt>
          <dd>{{ selected.principalName }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.model') }}</dt>
          <dd>{{ selected.modelName }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.resource') }}</dt>
          <dd>{{ selected.resourceName }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.protocol') }}</dt>
          <dd>{{ protocolLabel(selected.clientProtocol) }}</dd>
        </div>
        <div>
          <dt>{{ t('common.status') }}</dt>
          <dd><Status :value="selected.status" /></dd>
        </div>
        <div>
          <dt>{{ t('billing.ratingStatus') }}</dt>
          <dd>{{ ratingLabel(selected.ratingStatus) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.inputTokens') }}</dt>
          <dd>{{ count(selected.inputTokens) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.cachedTokens') }}</dt>
          <dd>{{ count(selected.cachedInputTokens) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.outputTokens') }}</dt>
          <dd>{{ count(selected.outputTokens) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.amount') }}</dt>
          <dd>{{ amount(selected.totalCost, selected.currency) }}</dd>
        </div>
      </dl>
      <section v-if="selected.subscription" class="subscription-cost-note">
        <h3>{{ t('billing.subscriptionAllocation') }}</h3>
        <p>{{ t('billing.subscriptionNoUsageCost') }}</p>
        <dl class="detail-grid">
          <div>
            <dt>{{ t('billing.priceVersion') }}</dt>
            <dd>{{ selected.subscription.priceId }}</dd>
          </div>
          <div>
            <dt>{{ t('billing.periodAmount') }}</dt>
            <dd>
              {{ amount(selected.subscription.periodAmount, selected.subscription.currency) }}
            </dd>
          </div>
          <div>
            <dt>{{ t('billing.period') }}</dt>
            <dd>
              {{
                selected.subscription.periodStart && selected.subscription.periodEnd
                  ? `${dateOnly(selected.subscription.periodStart)} – ${dateOnly(selected.subscription.periodEnd)}`
                  : '-'
              }}
            </dd>
          </div>
          <div>
            <dt>{{ t('billing.allocatedCost') }}</dt>
            <dd>
              {{ amount(selected.subscription.principalAmount, selected.subscription.currency) }}
            </dd>
          </div>
        </dl>
      </section>
      <TableScroll v-if="selected.ratings.length">
        <table class="billing-table">
          <thead>
            <tr>
              <th>{{ t('billing.revision') }}</th>
              <th>{{ t('billing.priceVersion') }}</th>
              <th>{{ t('billing.priceEffectiveAt') }}</th>
              <th class="numeric">{{ t('billing.inputPrice') }}</th>
              <th class="numeric">{{ t('billing.cachedInputPrice') }}</th>
              <th class="numeric">{{ t('billing.outputPrice') }}</th>
              <th class="numeric">{{ t('billing.inputCost') }}</th>
              <th class="numeric">{{ t('billing.cachedInputCost') }}</th>
              <th class="numeric">{{ t('billing.outputCost') }}</th>
              <th class="numeric">{{ t('billing.amount') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rating in selected.ratings" :key="rating.id">
              <td>{{ rating.revision }}</td>
              <td>{{ rating.modelPriceId }}</td>
              <td>{{ date(rating.priceEffectiveAt) }}</td>
              <td class="numeric">{{ amount(rating.inputPrice, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.cachedInputPrice, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.outputPrice, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.inputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.cachedInputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.outputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.totalCost, rating.currency) }}</td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </template>
  </Modal>
</template>
