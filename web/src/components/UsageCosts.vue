<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { all, api, download, errorText } from '../api'
import { count, date, useCollection, utcDateOnly } from '../composables'
import { utcDateValue as dateValue, utcDayRangeParams } from '../dateRange'
import { saveBlob } from '../files'
import { t } from '../i18n'
import { formatMoney as amount } from '../money'
import { usePrincipalAutocomplete } from '../principalAutocomplete'
import { showErrorToast } from '../toast'
import type { Model, UsageCost, UsageCostDetail } from '../types'
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
const callerName = ref('')
const modelId = ref('')
const billingType = ref('')
const query = ref('')
const exporting = ref(false)
const models = ref<Model[]>([])
const lookupError = ref('')
const selected = ref<UsageCostDetail | null>(null)
const detailOpen = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
let detailRevision = 0
// 导出可能需要较长时间生成文件，单独放宽超时。
const exportTimeoutMs = 120000
const {
  root: callerAutocomplete,
  open: callerSuggestionsOpen,
  loading: callerSuggestionsLoading,
  error: callerSearchError,
  suggestions: callerSuggestions,
  onInput: onCallerInput,
  openSuggestions: openCallerSuggestions,
  select: selectCaller,
  clear: clearCallerSuggestions,
  reset: changeCallerType,
} = usePrincipalAutocomplete({
  name: callerName,
  id: principalId,
  kind: () =>
    principalType.value === 'APPLICATION'
      ? 'application'
      : principalType.value === 'MEMBER'
        ? 'member'
        : '',
})

const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<UsageCost>(() => `/billing/usage-costs?${query.value}`)

function resetDates() {
  const now = new Date()
  from.value = dateValue(new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1)))
  to.value = dateValue(now)
}
function params() {
  if (callerName.value.trim() && !principalId.value) {
    showErrorToast(
      t(principalType.value === 'APPLICATION' ? 'usage.selectApplication' : 'usage.selectMember'),
    )
    return null
  }
  const result = utcDayRangeParams(from.value, to.value)
  if (!result) {
    showErrorToast(t('billing.invalidRange'))
    return null
  }
  for (const [key, value] of [
    ['principalType', principalType.value],
    ['principalId', principalId.value.trim()],
    ['modelId', modelId.value],
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
  await load()
}
function reset() {
  principalType.value = ''
  principalId.value = ''
  callerName.value = ''
  clearCallerSuggestions()
  modelId.value = ''
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
function ratingLabel(value: string) {
  return t(`billing.ratingStatuses.${value}`)
}
function billingModeLabel(value: string) {
  return t(value === 'SUBSCRIPTION' ? 'billing.subscriptionBilling' : 'billing.tokenBilling')
}
function hasBillingIssue(value: string) {
  return value !== 'RATED' && value !== 'SUBSCRIPTION_SHARED'
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
    const blob = await download(`/billing/usage-costs/export?${result}`, {
      timeoutMs: exportTimeoutMs,
    })
    saveBlob(blob, `zentrola-usage-costs-${from.value}-${to.value}.csv`)
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
onBeforeUnmount(() => {
  detailRevision++
})
</script>

<template>
  <section class="panel billing-filter-panel usage-cost-panel">
    <form class="usage-filters usage-cost-filters" @submit.prevent="search">
      <DateRangePicker v-model:from="from" v-model:to="to" />
      <div
        class="filter-field usage-cost-principal-filter"
        role="group"
        :aria-label="t('billing.caller')"
      >
        <span class="filter-label">{{ t('billing.caller') }}</span>
        <div class="usage-cost-principal-controls">
          <select
            v-model="principalType"
            :aria-label="t('billing.callerType')"
            @change="changeCallerType"
          >
            <option value="">{{ t('common.all') }}</option>
            <option value="MEMBER">{{ t('billing.principalTypes.MEMBER') }}</option>
            <option value="APPLICATION">{{ t('billing.principalTypes.APPLICATION') }}</option>
          </select>
          <div
            ref="callerAutocomplete"
            class="member-autocomplete"
            @keydown.esc="callerSuggestionsOpen = false"
          >
            <input
              v-model="callerName"
              type="search"
              role="combobox"
              autocomplete="off"
              aria-autocomplete="list"
              aria-controls="billing-caller-options"
              :aria-expanded="callerSuggestionsOpen"
              :aria-label="
                t(principalType === 'APPLICATION' ? 'usage.application' : 'usage.member')
              "
              :placeholder="
                t(
                  principalType === 'APPLICATION'
                    ? 'usage.applicationPlaceholder'
                    : principalType === 'MEMBER'
                      ? 'usage.memberPlaceholder'
                      : 'billing.chooseCallerType',
                )
              "
              :disabled="!principalType"
              @input="onCallerInput"
              @focus="openCallerSuggestions"
            />
            <div
              v-if="callerSuggestionsOpen"
              id="billing-caller-options"
              class="member-suggestions"
              role="listbox"
            >
              <button
                v-for="caller in callerSuggestions"
                :key="caller.id"
                type="button"
                role="option"
                :aria-selected="caller.id === principalId"
                @click="selectCaller(caller)"
              >
                <strong>{{ caller.name }}</strong>
                <small>ID {{ caller.id }}</small>
              </button>
              <p v-if="callerSuggestionsLoading" class="autocomplete-empty">
                {{ t('common.loading') }}
              </p>
              <p v-else-if="callerSearchError" class="autocomplete-empty error-text">
                {{ callerSearchError }}
              </p>
              <p v-else-if="!callerSuggestions.length" class="autocomplete-empty">
                {{
                  t(
                    principalType === 'APPLICATION'
                      ? 'usage.noApplicationMatches'
                      : 'usage.noMemberMatches',
                  )
                }}
              </p>
            </div>
          </div>
        </div>
      </div>
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
        <span class="filter-label">{{ t('billing.billingType') }}</span>
        <select v-model="billingType">
          <option value="">{{ t('common.all') }}</option>
          <option value="API_KEY">{{ t('billing.tokenBilling') }}</option>
          <option value="SUBSCRIPTION">{{ t('billing.subscriptionBilling') }}</option>
        </select>
      </label>
      <div class="usage-cost-actions">
        <button class="button primary" :disabled="loading">
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
            <th>{{ t('billing.billingType') }}</th>
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
              {{ billingModeLabel(item.billingType) }}
              <small v-if="hasBillingIssue(item.ratingStatus)" class="subline">
                <span class="issue-badge" :class="item.ratingStatus.toLowerCase()">{{
                  ratingLabel(item.ratingStatus)
                }}</span>
              </small>
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
        <div v-if="hasBillingIssue(selected.ratingStatus)">
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
          <dt>{{ t('billing.cacheCreationTokens') }}</dt>
          <dd>{{ count(selected.cacheCreationInputTokens) }}</dd>
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
          <div v-if="selected.subscription.periodStart && selected.subscription.periodEnd">
            <dt>{{ t('billing.period') }}</dt>
            <dd>
              {{
                `${utcDateOnly(selected.subscription.periodStart)} – ${utcDateOnly(selected.subscription.periodEnd)}`
              }}
            </dd>
          </div>
          <div v-if="selected.subscription.principalAmount !== null">
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
              <th class="numeric">{{ t('billing.cacheCreationInputPrice') }}</th>
              <th class="numeric">{{ t('billing.outputPrice') }}</th>
              <th class="numeric">{{ t('billing.inputCost') }}</th>
              <th class="numeric">{{ t('billing.cachedInputCost') }}</th>
              <th class="numeric">{{ t('billing.cacheCreationInputCost') }}</th>
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
              <td
                class="numeric"
                :title="
                  rating.cacheCreationInputPrice
                    ? undefined
                    : t('billing.cacheCreationPriceFallback')
                "
              >
                {{ amount(rating.cacheCreationInputPrice, rating.currency) }}
              </td>
              <td class="numeric">{{ amount(rating.outputPrice, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.inputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.cachedInputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.cacheCreationInputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.outputCost, rating.currency) }}</td>
              <td class="numeric">{{ amount(rating.totalCost, rating.currency) }}</td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </template>
  </Modal>
</template>
