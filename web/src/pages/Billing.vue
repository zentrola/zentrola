<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorText } from '../api'
import { count, date, dateOnly, useCollection } from '../composables'
import { t } from '../i18n'
import type {
  BillingCurrency,
  BillingDocument,
  BillingDocumentDetail,
  BillingIssueReason,
  BillingStatistics,
  BillingType,
  UnratedUsage,
} from '../types'
import ListFooter from '../components/ListFooter.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import TableScroll from '../components/TableScroll.vue'

type BillingView = 'overview' | 'documents' | 'exceptions'

const activeView = ref<BillingView>('overview')
const from = ref('')
const to = ref('')
const billingType = ref('')
const documentType = ref('')
const currency = ref('')
const reason = ref('')
const documentQuery = ref('')
const exceptionQuery = ref('')
const statistics = ref<BillingStatistics | null>(null)
const statisticsLoading = ref(false)
const statisticsError = ref('')
const detailOpen = ref(false)
const detail = ref<BillingDocumentDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
let statisticsRevision = 0
let detailRevision = 0

const {
  items: documents,
  cursor: documentCursor,
  page: documentPage,
  pageSize: documentPageSize,
  total: documentTotal,
  loading: documentsLoading,
  error: documentsError,
  load: loadDocuments,
  previous: previousDocuments,
  retry: retryDocuments,
  setPageSize: setDocumentPageSize,
} = useCollection<BillingDocument>(() => `/billing/documents?${documentQuery.value}`)

const {
  items: exceptions,
  cursor: exceptionCursor,
  page: exceptionPage,
  pageSize: exceptionPageSize,
  total: exceptionTotal,
  loading: exceptionsLoading,
  error: exceptionsError,
  load: loadExceptions,
  previous: previousExceptions,
  retry: retryExceptions,
  setPageSize: setExceptionPageSize,
} = useCollection<UnratedUsage>(() => `/billing/unrated-usage?${exceptionQuery.value}`)

const loading = computed(() => {
  if (activeView.value === 'overview') return statisticsLoading.value
  if (activeView.value === 'documents') return documentsLoading.value
  return exceptionsLoading.value
})

function dateValue(value: Date) {
  return value.toISOString().slice(0, 10)
}
function resetDates() {
  const now = new Date()
  const first = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1))
  from.value = dateValue(first)
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
function queryParams(view: BillingView) {
  const params = rangeParams()
  if (!params) return null
  if (view !== 'exceptions' && billingType.value) params.set('billingType', billingType.value)
  if (view === 'documents') {
    if (documentType.value) params.set('documentType', documentType.value)
    if (currency.value) params.set('currency', currency.value)
  }
  if (view === 'exceptions' && reason.value) params.set('reason', reason.value)
  return params
}
async function loadStatistics() {
  const params = queryParams('overview')
  if (!params) {
    statisticsError.value = t('billing.invalidRange')
    return
  }
  params.set('limit', '100')
  const revision = ++statisticsRevision
  statisticsLoading.value = true
  statisticsError.value = ''
  try {
    const result = await api<BillingStatistics>(`/billing/statistics?${params}`)
    if (revision === statisticsRevision) statistics.value = result
  } catch (error) {
    if (revision === statisticsRevision) statisticsError.value = errorText(error)
  } finally {
    if (revision === statisticsRevision) statisticsLoading.value = false
  }
}
function search() {
  const params = queryParams(activeView.value)
  if (!params) {
    if (activeView.value === 'overview') statisticsError.value = t('billing.invalidRange')
    return
  }
  if (activeView.value === 'overview') {
    void loadStatistics()
  } else if (activeView.value === 'documents') {
    documentQuery.value = params.toString()
    void loadDocuments()
  } else {
    exceptionQuery.value = params.toString()
    void loadExceptions()
  }
}
function selectView(view: BillingView) {
  if (loading.value || view === activeView.value) return
  activeView.value = view
  search()
}
function billingLabel(value: BillingType) {
  return t(value === 'SUBSCRIPTION' ? 'billing.subscription' : 'billing.apiKey')
}
function documentLabel(value: BillingDocument['documentType']) {
  return t(value === 'CHARGE' ? 'billing.charge' : 'billing.adjustment')
}
function principalLabel(value: string) {
  return t(`billing.principalTypes.${value}`)
}
function reasonLabel(value: BillingIssueReason) {
  return t(`billing.reasons.${value}`)
}
function amount(value: string, valueCurrency: BillingCurrency, forceSign = false) {
  const match = value.match(/^(-?)(\d+)(?:\.(\d+))?$/)
  if (!match) return '-'
  const fraction = (match[3] || '').replace(/0+$/, '')
  const integer = match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  const sign = match[1] || (forceSign && value !== '0' ? '+' : '')
  const symbol = valueCurrency === 'CNY' ? '¥' : '$'
  return `${sign}${symbol}${integer}${fraction ? `.${fraction}` : ''}`
}
function ratio(value: string | null) {
  if (value === null) return '-'
  return `${(Number(value) * 100).toFixed(2).replace(/\.00$/, '')}%`
}
function totalFor(valueCurrency: BillingCurrency, valueType: BillingType) {
  return statistics.value?.totals.find(
    (item) => item.currency === valueCurrency && item.billingType === valueType,
  )
}
async function openDocument(id: string) {
  const revision = ++detailRevision
  detailOpen.value = true
  detail.value = null
  detailError.value = ''
  detailLoading.value = true
  try {
    const result = await api<BillingDocumentDetail>(`/billing/documents/${id}`)
    if (revision === detailRevision) detail.value = result
  } catch (error) {
    if (revision === detailRevision) detailError.value = errorText(error)
  } finally {
    if (revision === detailRevision) detailLoading.value = false
  }
}
function closeDetail() {
  detailRevision++
  detailOpen.value = false
  detail.value = null
}

onMounted(() => {
  resetDates()
  search()
})
onBeforeUnmount(() => {
  statisticsRevision++
  detailRevision++
})
</script>

<template>
  <PageHeader name="billing" />
  <div class="view-tabs billing-tabs" role="tablist" :aria-label="t('nav.billing')">
    <button
      v-for="view in ['overview', 'documents', 'exceptions'] as BillingView[]"
      :key="view"
      type="button"
      role="tab"
      :aria-selected="activeView === view"
      :class="{ selected: activeView === view }"
      :disabled="loading"
      @click="selectView(view)"
    >
      {{ t(`billing.${view}`) }}
    </button>
  </div>

  <section class="panel billing-filter-panel">
    <form class="usage-filters billing-filters" @submit.prevent="search">
      <label>
        <span class="filter-label">{{ t('billing.periodFrom') }}</span>
        <input v-model="from" type="date" required />
      </label>
      <label>
        <span class="filter-label">{{ t('billing.periodTo') }}</span>
        <input v-model="to" type="date" required />
      </label>
      <label v-if="activeView !== 'exceptions'">
        <span class="filter-label">{{ t('billing.billingType') }}</span>
        <select v-model="billingType">
          <option value="">{{ t('common.all') }}</option>
          <option value="SUBSCRIPTION">{{ t('billing.subscription') }}</option>
          <option value="API_KEY">{{ t('billing.apiKey') }}</option>
        </select>
      </label>
      <label v-if="activeView === 'documents'">
        <span class="filter-label">{{ t('billing.documentType') }}</span>
        <select v-model="documentType">
          <option value="">{{ t('common.all') }}</option>
          <option value="CHARGE">{{ t('billing.charge') }}</option>
          <option value="ADJUSTMENT">{{ t('billing.adjustment') }}</option>
        </select>
      </label>
      <label v-if="activeView === 'documents'">
        <span class="filter-label">{{ t('billing.currency') }}</span>
        <select v-model="currency">
          <option value="">{{ t('common.all') }}</option>
          <option value="CNY">CNY</option>
          <option value="USD">USD</option>
        </select>
      </label>
      <label v-if="activeView === 'exceptions'">
        <span class="filter-label">{{ t('billing.reason') }}</span>
        <select v-model="reason">
          <option value="">{{ t('common.all') }}</option>
          <option value="INCOMPLETE_TOKENS">{{ t('billing.reasons.INCOMPLETE_TOKENS') }}</option>
          <option value="MISSING_PRICE">{{ t('billing.reasons.MISSING_PRICE') }}</option>
          <option value="PENDING_RATING">{{ t('billing.reasons.PENDING_RATING') }}</option>
        </select>
      </label>
      <button class="button primary" :disabled="loading">{{ t('billing.search') }}</button>
    </form>
  </section>

  <template v-if="activeView === 'overview'">
    <div v-if="statisticsError" class="alert error billing-alert" role="alert">
      {{ statisticsError }}
      <button type="button" class="text-button" @click="loadStatistics">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-else-if="statisticsLoading" class="empty-state">{{ t('common.loading') }}</p>
    <template v-else-if="statistics">
      <section class="panel cost-ledger" aria-live="polite">
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
            <strong>{{
              amount(totalFor(valueCurrency, valueType)?.totalAmount || '0', valueCurrency)
            }}</strong>
            <small>{{ count(totalFor(valueCurrency, valueType)?.totalTokens ?? 0) }} Token</small>
          </div>
        </div>
      </section>
      <section class="panel billing-table-panel">
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
                <td colspan="6" class="empty-cell">{{ t('billing.emptyOverview') }}</td>
              </tr>
            </tbody>
          </table>
        </TableScroll>
      </section>
    </template>
  </template>

  <section v-else-if="activeView === 'documents'" class="panel billing-table-panel">
    <div v-if="documentsError" class="alert error" role="alert">
      {{ documentsError }}
      <button type="button" class="text-button" @click="retryDocuments">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-if="documentsLoading" class="empty-state">{{ t('common.loading') }}</p>
    <TableScroll v-else>
      <table class="billing-table">
        <thead>
          <tr>
            <th>{{ t('billing.documentId') }}</th>
            <th>{{ t('billing.documentType') }}</th>
            <th>{{ t('billing.billingType') }}</th>
            <th>{{ t('billing.credential') }}</th>
            <th>{{ t('billing.period') }}</th>
            <th class="numeric">{{ t('billing.tokens') }}</th>
            <th class="numeric">{{ t('billing.amount') }}</th>
            <th>{{ t('billing.createdAt') }}</th>
            <th>{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in documents" :key="item.id">
            <td>
              <strong>{{ item.id }}</strong
              ><small v-if="item.originalDocumentId" class="subline"
                >{{ t('billing.originalDocument') }} {{ item.originalDocumentId }}</small
              >
            </td>
            <td>{{ documentLabel(item.documentType) }}</td>
            <td>{{ billingLabel(item.billingType) }}</td>
            <td>
              <strong>{{ item.credentialName }}</strong
              ><small class="subline">ID {{ item.credentialId }}</small>
            </td>
            <td>{{ dateOnly(item.periodStart) }} – {{ dateOnly(item.periodEnd) }}</td>
            <td class="numeric">{{ count(item.totalTokens) }}</td>
            <td class="numeric amount-cell" :class="{ negative: item.totalAmount.startsWith('-') }">
              {{ amount(item.totalAmount, item.currency, item.documentType === 'ADJUSTMENT') }}
            </td>
            <td>{{ date(item.createdAt) }}</td>
            <td>
              <button type="button" class="text-button" @click="openDocument(item.id)">
                {{ t('billing.details') }}
              </button>
            </td>
          </tr>
          <tr v-if="!documents.length">
            <td colspan="9" class="empty-cell">{{ t('billing.emptyDocuments') }}</td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <ListFooter
      :cursor="documentCursor"
      :page="documentPage"
      :page-size="documentPageSize"
      :total="documentTotal"
      :loading="documentsLoading"
      @first="loadDocuments()"
      @previous="previousDocuments"
      @more="loadDocuments(true)"
      @page-size="setDocumentPageSize"
    />
  </section>

  <section v-else class="panel billing-table-panel">
    <div v-if="exceptionsError" class="alert error" role="alert">
      {{ exceptionsError }}
      <button type="button" class="text-button" @click="retryExceptions">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-if="exceptionsLoading" class="empty-state">{{ t('common.loading') }}</p>
    <TableScroll v-else>
      <table class="billing-table">
        <thead>
          <tr>
            <th>{{ t('billing.reason') }}</th>
            <th>{{ t('billing.principal') }}</th>
            <th>{{ t('billing.credential') }}</th>
            <th>{{ t('billing.callTime') }}</th>
            <th class="numeric">{{ t('billing.inputTokens') }}</th>
            <th class="numeric">{{ t('billing.cachedTokens') }}</th>
            <th class="numeric">{{ t('billing.outputTokens') }}</th>
            <th>{{ t('billing.modelMapping') }}</th>
            <th>{{ t('billing.waitingSince') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in exceptions" :key="item.usageRecordId">
            <td>
              <span class="issue-badge" :class="item.reason.toLowerCase()">{{
                reasonLabel(item.reason)
              }}</span>
            </td>
            <td>
              <strong>{{ item.principalName }}</strong
              ><small class="subline"
                >{{ principalLabel(item.principalType) }} · ID {{ item.principalId }}</small
              >
            </td>
            <td>
              <strong>{{ item.credentialName }}</strong
              ><small class="subline">ID {{ item.credentialId }}</small>
            </td>
            <td>{{ date(item.startedAt) }}</td>
            <td class="numeric">{{ count(item.inputTokens) }}</td>
            <td class="numeric">{{ count(item.cachedInputTokens) }}</td>
            <td class="numeric">{{ count(item.outputTokens) }}</td>
            <td>{{ item.providerModelId }}</td>
            <td>{{ date(item.waitingSince) }}</td>
          </tr>
          <tr v-if="!exceptions.length">
            <td colspan="9" class="empty-cell">{{ t('billing.emptyExceptions') }}</td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <ListFooter
      :cursor="exceptionCursor"
      :page="exceptionPage"
      :page-size="exceptionPageSize"
      :total="exceptionTotal"
      :loading="exceptionsLoading"
      @first="loadExceptions()"
      @previous="previousExceptions"
      @more="loadExceptions(true)"
      @page-size="setExceptionPageSize"
    />
  </section>

  <Modal
    v-if="detailOpen"
    :title="t('billing.documentDetails')"
    wide
    :busy="detailLoading"
    @close="closeDetail"
  >
    <p v-if="detailLoading" class="empty-state">{{ t('common.loading') }}</p>
    <div v-else-if="detailError" class="alert error" role="alert">{{ detailError }}</div>
    <template v-else-if="detail">
      <dl class="detail-grid billing-detail-grid">
        <div>
          <dt>{{ t('billing.documentId') }}</dt>
          <dd>{{ detail.id }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.documentType') }}</dt>
          <dd>{{ documentLabel(detail.documentType) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.billingType') }}</dt>
          <dd>{{ billingLabel(detail.billingType) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.credential') }}</dt>
          <dd>{{ detail.credentialName }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.period') }}</dt>
          <dd>{{ dateOnly(detail.periodStart) }} – {{ dateOnly(detail.periodEnd) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.amount') }}</dt>
          <dd class="amount-cell" :class="{ negative: detail.totalAmount.startsWith('-') }">
            {{ amount(detail.totalAmount, detail.currency, detail.documentType === 'ADJUSTMENT') }}
          </dd>
        </div>
        <div>
          <dt>{{ t('billing.tokens') }}</dt>
          <dd>{{ count(detail.totalTokens) }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.ratingCount') }}</dt>
          <dd>{{ detail.billingType === 'API_KEY' ? count(detail.ratingCount) : '-' }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.originalDocument') }}</dt>
          <dd>{{ detail.original?.id || '-' }}</dd>
        </div>
        <div>
          <dt>{{ t('billing.relatedAdjustments') }}</dt>
          <dd>
            {{
              detail.adjustments.length ? detail.adjustments.map((item) => item.id).join(', ') : '-'
            }}
          </dd>
        </div>
      </dl>
      <TableScroll>
        <table class="billing-table detail-items-table">
          <thead>
            <tr>
              <th>{{ t('billing.principal') }}</th>
              <th>{{ t('billing.principalType') }}</th>
              <th class="numeric">{{ t('billing.usageTokens') }}</th>
              <th class="numeric">{{ t('billing.allocationRatio') }}</th>
              <th class="numeric">{{ t('billing.amount') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in detail.items" :key="item.id">
              <td>
                <strong>{{ item.principalName }}</strong
                ><small class="subline">ID {{ item.principalId }}</small>
              </td>
              <td>{{ principalLabel(item.principalType) }}</td>
              <td class="numeric">{{ count(item.usageTokens) }}</td>
              <td class="numeric">{{ ratio(item.allocationRatio) }}</td>
              <td class="numeric amount-cell" :class="{ negative: item.amount.startsWith('-') }">
                {{ amount(item.amount, detail.currency, detail.documentType === 'ADJUSTMENT') }}
              </td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </template>
  </Modal>
</template>
