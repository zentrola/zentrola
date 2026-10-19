<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { all, errorText } from '../api'
import { useCollection, date, count } from '../composables'
import { utcDateValue as dateValue, utcDayRangeParams } from '../dateRange'
import { activeLocale, codeText, t } from '../i18n'
import { usePrincipalAutocomplete } from '../principalAutocomplete'
import { showErrorToast } from '../toast'
import type { Usage, UsageStatistic, Model, Provider, Resource } from '../types'
import DateRangePicker from '../components/DateRangePicker.vue'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import TableScroll from '../components/TableScroll.vue'
import TechnicalValue from '../components/TechnicalValue.vue'

type UsageView = 'statistics' | 'records'
type StatisticDimension = 'member' | 'application' | 'model' | 'provider'
const statisticDimensions: StatisticDimension[] = ['member', 'application', 'model', 'provider']

const route = useRoute(),
  router = useRouter()
const activeView = ref<UsageView>('statistics'),
  statisticDimension = ref<StatisticDimension>('member'),
  statisticsQuery = ref('')
const memberID = ref(''),
  memberName = ref(''),
  principalKind = ref<'member' | 'application'>('member'),
  modelID = ref(''),
  providerID = ref(''),
  from = ref(''),
  to = ref(''),
  query = ref(''),
  lookupError = ref(''),
  lookupsReady = ref(false)
const models = ref<Model[]>([]),
  providers = ref<Provider[]>([]),
  resources = ref<Resource[]>([]),
  selected = ref<Usage | null>(null)
const {
  root: memberAutocomplete,
  open: memberSuggestionsOpen,
  loading: memberSuggestionsLoading,
  error: memberSearchError,
  suggestions: memberSuggestions,
  onInput: onMemberInput,
  openSuggestions: openMemberSuggestions,
  select: selectMember,
  reset: resetMemberAutocomplete,
} = usePrincipalAutocomplete({ name: memberName, id: memberID, kind: () => principalKind.value })
let lookupPromise: Promise<void> | null = null

function errorDescription(errorType: string | null) {
  if (!errorType) return t('common.none')
  const upstreamHTTPStatus = errorType.match(/^UPSTREAM_HTTP_(\d{3})$/)?.[1]
  return codeText(
    'errors',
    errorType,
    upstreamHTTPStatus
      ? t('usage.upstreamHTTPError', { status: upstreamHTTPStatus })
      : t('errors.UNKNOWN'),
  )
}
function protocolLabel(protocol: string) {
  const labels: Record<string, string> = {
    OPENAI: 'OpenAI',
    OPENAI_CHAT: 'OpenAI Chat',
    OPENAI_RESPONSES: 'OpenAI Responses',
    OPENAI_IMAGES: 'OpenAI Images',
    ANTHROPIC: 'Anthropic',
    ANTHROPIC_MESSAGES: 'Anthropic',
  }
  return labels[protocol] || protocol
}
const {
  items: recordItems,
  cursor: recordCursor,
  page: recordPage,
  pageSize: recordPageSize,
  total: recordTotal,
  loading: recordsLoading,
  error: recordsError,
  load: loadRecords,
  previous: previousRecords,
  retry: retryRecords,
  setPageSize: setRecordPageSize,
} = useCollection<Usage>(() => `/usage?${query.value}`)
const {
  items: statistics,
  cursor: statisticCursor,
  page: statisticPage,
  pageSize: statisticPageSize,
  total: statisticTotal,
  loading: statisticsLoading,
  error: statisticsError,
  load: loadStatistics,
  previous: previousStatistics,
  retry: retryStatistics,
  setPageSize: setStatisticPageSize,
} = useCollection<UsageStatistic>(() => `/usage/statistics?${statisticsQuery.value}`)
const loading = computed(() =>
  activeView.value === 'statistics' ? statisticsLoading.value : recordsLoading.value,
)
function resetTimes() {
  const now = new Date()
  const weekStart = new Date(now)
  weekStart.setUTCDate(weekStart.getUTCDate() - 6)
  to.value = dateValue(now)
  from.value = dateValue(weekStart)
}
// 用量按 UTC 自然日统计，且起止不能为同一天。
const maxRangeDays = 366
function rangeParams() {
  const params = utcDayRangeParams(from.value, to.value, maxRangeDays, false)
  if (!params) showErrorToast(t('usage.invalidRange', { days: maxRangeDays }))
  return params
}
function syncRoute() {
  const routeQuery: Record<string, string> = {
    view: activeView.value,
    from: from.value,
    to: to.value,
  }
  if (activeView.value === 'statistics') routeQuery.dimension = statisticDimension.value
  if (activeView.value === 'records') {
    if (memberID.value) {
      routeQuery[`${principalKind.value}Id`] = memberID.value
      routeQuery[`${principalKind.value}Name`] = memberName.value
    }
    routeQuery.principalKind = principalKind.value
    if (modelID.value) routeQuery.modelId = modelID.value
    if (providerID.value) routeQuery.providerId = providerID.value
  }
  void router.replace({ name: 'usage', query: routeQuery })
}
function searchRecords() {
  if (memberName.value && !memberID.value) {
    showErrorToast(
      t(principalKind.value === 'application' ? 'usage.selectApplication' : 'usage.selectMember'),
    )
    return
  }
  const params = rangeParams()
  if (!params) return
  for (const [key, value] of [
    [`${principalKind.value}Id`, memberID.value],
    ['modelId', modelID.value],
    ['providerId', providerID.value],
  ])
    if (value) params.set(key, value)
  query.value = params.toString()
  syncRoute()
  void loadRecords()
}
function searchStatistics() {
  const params = rangeParams()
  if (!params) return
  params.set('dimension', statisticDimension.value)
  statisticsQuery.value = params.toString()
  syncRoute()
  void loadStatistics()
}
function search() {
  if (activeView.value === 'statistics') searchStatistics()
  else searchRecords()
}
function reset() {
  memberID.value = ''
  memberName.value = ''
  principalKind.value = 'member'
  modelID.value = ''
  providerID.value = ''
  resetTimes()
  search()
}
function selectView(view: UsageView) {
  if (loading.value || activeView.value === view) return
  activeView.value = view
  if (view === 'records') void ensureLookups()
  search()
}
function selectStatisticDimension(dimension: StatisticDimension) {
  if (statisticsLoading.value || statisticDimension.value === dimension) return
  statisticDimension.value = dimension
  searchStatistics()
}
function viewStatisticRecords(item: UsageStatistic) {
  memberID.value = ''
  memberName.value = ''
  modelID.value = ''
  providerID.value = ''
  if (statisticDimension.value === 'member' || statisticDimension.value === 'application') {
    principalKind.value = statisticDimension.value
    memberID.value = item.entityId
    memberName.value = item.name
  } else if (statisticDimension.value === 'model') modelID.value = item.entityId
  else providerID.value = item.entityId
  activeView.value = 'records'
  void ensureLookups()
  searchRecords()
}
function percentage(value: number, totalValue: number) {
  if (totalValue <= 0) return '0%'
  return new Intl.NumberFormat(activeLocale.value, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(value / totalValue)
}
function statisticRank(index: number) {
  return (statisticPage.value - 1) * statisticPageSize.value + index + 1
}
function changePrincipalKind() {
  resetMemberAutocomplete()
}
async function lookups() {
  lookupError.value = ''
  try {
    ;[models.value, providers.value, resources.value] = await Promise.all([
      all<Model>('/models'),
      all<Provider>('/providers'),
      all<Resource>('/resources'),
    ])
    lookupsReady.value = true
  } catch (e) {
    lookupError.value = errorText(e)
  }
}
function ensureLookups() {
  if (lookupsReady.value) return Promise.resolve()
  if (!lookupPromise) lookupPromise = lookups().finally(() => (lookupPromise = null))
  return lookupPromise
}
function label(list: { id: string; name: string }[], id: string | null) {
  return id === null ? t('common.none') : list.find((x) => x.id === id)?.name || id
}
function routeValue(name: string) {
  const value = route.query[name]
  return typeof value === 'string' ? value : ''
}
function applyRoute() {
  activeView.value = routeValue('view') === 'records' ? 'records' : 'statistics'
  const dimension = routeValue('dimension')
  if (dimension === 'application' || dimension === 'model' || dimension === 'provider')
    statisticDimension.value = dimension
  const routeFrom = routeValue('from'),
    routeTo = routeValue('to')
  if (/^\d{4}-\d{2}-\d{2}$/.test(routeFrom) && /^\d{4}-\d{2}-\d{2}$/.test(routeTo)) {
    from.value = routeFrom
    to.value = routeTo
  } else resetTimes()
  principalKind.value =
    routeValue('principalKind') === 'application' || !!routeValue('applicationId')
      ? 'application'
      : 'member'
  memberID.value = routeValue(`${principalKind.value}Id`)
  memberName.value = routeValue(`${principalKind.value}Name`)
  modelID.value = routeValue('modelId')
  providerID.value = routeValue('providerId')
}
onMounted(() => {
  applyRoute()
  if (activeView.value === 'records') void ensureLookups()
  search()
})
</script>
<template>
  <PageHeader name="usage" />
  <div class="view-tabs" role="tablist" :aria-label="t('nav.usage')">
    <button
      type="button"
      role="tab"
      :disabled="loading"
      :aria-selected="activeView === 'statistics'"
      :class="{ selected: activeView === 'statistics' }"
      @click="selectView('statistics')"
    >
      <Icon name="home" :size="17" />{{ t('usage.statistics') }}
    </button>
    <button
      type="button"
      role="tab"
      :disabled="loading"
      :aria-selected="activeView === 'records'"
      :class="{ selected: activeView === 'records' }"
      @click="selectView('records')"
    >
      <Icon name="usage" :size="17" />{{ t('usage.records') }}
    </button>
  </div>
  <section class="panel usage-list-panel">
    <form
      class="table-toolbar usage-filters"
      :aria-label="t(activeView === 'statistics' ? 'usage.statisticsFilters' : 'usage.filters')"
      @submit.prevent="search"
    >
      <label v-if="activeView === 'records'" class="filter-field">
        <span class="filter-label">{{ t('usage.principalType') }}</span>
        <select v-model="principalKind" @change="changePrincipalKind">
          <option value="member">{{ t('usage.member') }}</option>
          <option value="application">{{ t('usage.application') }}</option>
        </select>
      </label>
      <div
        v-if="activeView === 'records'"
        ref="memberAutocomplete"
        class="filter-field member-filter-field"
      >
        <span class="filter-label">{{
          t(principalKind === 'application' ? 'usage.application' : 'usage.member')
        }}</span>
        <div class="member-autocomplete" @keydown.esc="memberSuggestionsOpen = false">
          <input
            id="usage-member"
            v-model="memberName"
            type="search"
            role="combobox"
            autocomplete="off"
            aria-autocomplete="list"
            aria-controls="usage-member-options"
            :aria-expanded="memberSuggestionsOpen"
            :aria-label="t(principalKind === 'application' ? 'usage.application' : 'usage.member')"
            :placeholder="
              t(
                principalKind === 'application'
                  ? 'usage.applicationPlaceholder'
                  : 'usage.memberPlaceholder',
              )
            "
            @input="onMemberInput"
            @focus="openMemberSuggestions"
          />
          <div
            v-if="memberSuggestionsOpen"
            id="usage-member-options"
            class="member-suggestions"
            role="listbox"
          >
            <button
              v-for="member in memberSuggestions"
              :key="member.id"
              type="button"
              role="option"
              :aria-selected="member.id === memberID"
              @click="selectMember(member)"
            >
              <strong>{{ member.name }}</strong>
            </button>
            <p v-if="memberSuggestionsLoading" class="autocomplete-empty">
              {{ t('common.loading') }}
            </p>
            <p v-else-if="memberSearchError" class="autocomplete-empty error-text">
              {{ memberSearchError }}
            </p>
            <p v-else-if="!memberSuggestions.length" class="autocomplete-empty">
              {{
                t(
                  principalKind === 'application'
                    ? 'usage.noApplicationMatches'
                    : 'usage.noMemberMatches',
                )
              }}
            </p>
          </div>
        </div>
      </div>
      <label v-if="activeView === 'records'" class="model-filter-field">
        <span class="filter-label">{{ t('usage.model') }}</span>
        <select v-model="modelID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="model in models" :key="model.id" :value="model.id">
            {{ model.name }}
          </option>
        </select>
      </label>
      <label v-if="activeView === 'records'" class="provider-filter-field">
        <span class="filter-label">{{ t('usage.provider') }}</span>
        <select v-model="providerID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="provider in providers" :key="provider.id" :value="provider.id">
            {{ provider.name }}
          </option>
        </select>
      </label>
      <DateRangePicker v-model:from="from" v-model:to="to" :allow-same-day="false" />
      <div class="filter-actions">
        <button class="button primary" :disabled="loading">{{ t('common.searchAction') }}</button>
        <button type="button" class="button" :disabled="loading" @click="reset">
          {{ t('common.reset') }}
        </button>
      </div>
    </form>
    <p v-if="lookupError" class="alert error" role="alert">
      {{ lookupError
      }}<button class="text-button" @click="ensureLookups">{{ t('common.retry') }}</button>
    </p>
    <p v-if="activeView === 'statistics' && statisticsError" class="alert error" role="alert">
      {{ statisticsError
      }}<button class="text-button" @click="retryStatistics">{{ t('common.retry') }}</button>
    </p>
    <p v-if="activeView === 'records' && recordsError" class="alert error" role="alert">
      {{ recordsError
      }}<button class="text-button" @click="retryRecords">{{ t('common.retry') }}</button>
    </p>
    <template v-if="activeView === 'statistics'">
      <div class="statistics-heading">
        <div>
          <h2>{{ t('usage.statistics') }}</h2>
          <p>{{ t('usage.rankingDescription') }}</p>
        </div>
        <div class="dimension-tabs" role="tablist" :aria-label="t('usage.statistics')">
          <button
            v-for="dimension in statisticDimensions"
            :key="dimension"
            type="button"
            role="tab"
            :disabled="statisticsLoading"
            :aria-selected="statisticDimension === dimension"
            :class="{ selected: statisticDimension === dimension }"
            @click="selectStatisticDimension(dimension)"
          >
            {{ t(`usage.${dimension}Ranking`) }}
          </button>
        </div>
      </div>
      <TableScroll has-actions>
        <table class="statistics-table">
          <colgroup>
            <col class="usage-rank-column" />
            <col class="usage-name-column" />
            <col span="8" class="usage-metric-column" />
            <col class="usage-action-column" />
          </colgroup>
          <thead>
            <tr>
              <th class="rank-column">{{ t('usage.rank') }}</th>
              <th>{{ t(`usage.${statisticDimension}`) }}</th>
              <th class="numeric">
                {{ t(statisticDimension === 'provider' ? 'usage.calls' : 'usage.requests') }}
              </th>
              <th class="numeric">{{ t('usage.input') }}</th>
              <th class="numeric">{{ t('usage.output') }}</th>
              <th class="numeric">{{ t('usage.cached') }}</th>
              <th class="numeric">{{ t('usage.totalTokens') }}</th>
              <th class="numeric">{{ t('usage.share') }}</th>
              <th class="numeric">{{ t('usage.successRate') }}</th>
              <th class="numeric">{{ t('usage.averageLatency') }}</th>
              <th class="align-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, index) in statistics" :key="row.entityId">
              <td class="rank-column">
                <span class="table-rank">{{ statisticRank(index) }}</span>
              </td>
              <td>
                <strong>{{ row.name }}</strong
                ><small v-if="row.code" class="subline">{{ row.code }}</small>
              </td>
              <td class="numeric">{{ count(row.count) }}</td>
              <td class="numeric">{{ count(row.inputTokens) }}</td>
              <td class="numeric">{{ count(row.outputTokens) }}</td>
              <td class="numeric">{{ count(row.cachedInputTokens) }}</td>
              <td class="numeric statistic-total">{{ count(row.tokens) }}</td>
              <td class="numeric">{{ percentage(row.tokens, row.overallTokens) }}</td>
              <td class="numeric">{{ percentage(row.successful, row.count) }}</td>
              <td class="numeric">{{ count(row.averageLatencyMs) }} ms</td>
              <td class="align-right">
                <button class="text-button" @click="viewStatisticRecords(row)">
                  {{ t('usage.viewRecords') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
      <div v-if="!statistics.length" class="empty-state">
        <Icon name="usage" :size="32" />
        <h3>{{ t(statisticsLoading ? 'common.loading' : 'common.empty') }}</h3>
        <p v-if="!statisticsLoading">{{ t('usage.statisticsEmpty') }}</p>
        <button
          v-if="!statisticsLoading"
          type="button"
          class="button primary empty-state-action"
          @click="reset"
        >
          <Icon name="refresh" :size="16" />{{ t('common.reset') }}
        </button>
      </div>
      <ListFooter
        :cursor="statisticCursor"
        :page="statisticPage"
        :page-size="statisticPageSize"
        :total="statisticTotal"
        :loading="statisticsLoading"
        @first="loadStatistics()"
        @previous="previousStatistics"
        @more="loadStatistics(true)"
        @page-size="setStatisticPageSize"
      />
    </template>
    <template v-else>
      <TableScroll has-actions>
        <table class="usage-table">
          <colgroup>
            <col class="usage-time-column" />
            <col class="usage-member-column" />
            <col class="usage-model-column" />
            <col class="usage-protocol-column" />
            <col class="usage-status-column" />
            <col span="3" class="usage-token-column" />
            <col class="usage-action-column" />
          </colgroup>
          <thead>
            <tr>
              <th>{{ t('usage.time') }}</th>
              <th>{{ t('usage.principal') }}</th>
              <th>{{ t('usage.model') }}</th>
              <th>{{ t('usage.protocol') }}</th>
              <th>{{ t('common.status') }}</th>
              <th class="numeric">{{ t('usage.input') }}</th>
              <th class="numeric">{{ t('usage.output') }}</th>
              <th class="numeric">{{ t('usage.cached') }}</th>
              <th class="align-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in recordItems" :key="row.id">
              <td class="table-time">
                {{ date(row.requestAt) }}<small class="subline">{{ row.latencyMs }} ms</small>
              </td>
              <td>
                <strong>{{ row.principalName || row.principalId }}</strong>
                <small class="subline">{{
                  t(row.principalType === 'APPLICATION' ? 'usage.application' : 'usage.member')
                }}</small>
                <TechnicalValue
                  v-if="row.principalName"
                  :value="row.principalId"
                  :copyable="false"
                  muted
                />
              </td>
              <td>
                {{ label(models, row.modelId)
                }}<small class="subline">{{ label(resources, row.resourceId) }}</small>
              </td>
              <td>
                <span class="protocol-label" :title="protocolLabel(row.clientProtocol)">
                  {{ protocolLabel(row.clientProtocol) }}
                </span>
              </td>
              <td><Status :value="row.status" /></td>
              <td class="numeric">{{ count(row.inputTokens) }}</td>
              <td class="numeric">{{ count(row.outputTokens) }}</td>
              <td class="numeric">{{ count(row.cachedInputTokens) }}</td>
              <td class="align-right">
                <button class="text-button" @click="selected = row">
                  {{ t('common.details') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
      <div v-if="!recordItems.length" class="empty-state">
        <Icon name="usage" :size="32" />
        <h3>{{ t(recordsLoading ? 'common.loading' : 'common.empty') }}</h3>
        <p v-if="!recordsLoading">{{ t('usage.empty') }}</p>
        <button
          v-if="!recordsLoading"
          type="button"
          class="button primary empty-state-action"
          @click="reset"
        >
          <Icon name="refresh" :size="16" />{{ t('common.reset') }}
        </button>
      </div>
      <ListFooter
        :cursor="recordCursor"
        :page="recordPage"
        :page-size="recordPageSize"
        :total="recordTotal"
        :loading="recordsLoading"
        @first="loadRecords()"
        @previous="previousRecords"
        @more="loadRecords(true)"
        @page-size="setRecordPageSize"
      />
    </template>
  </section>
  <Modal v-if="selected" :title="t('common.details')" wide @close="selected = null"
    ><dl class="detail-grid">
      <dt>{{ t('common.requestId') }}</dt>
      <dd>
        <TechnicalValue :value="selected.requestId" />
      </dd>
      <dt>
        {{ t(selected.principalType === 'APPLICATION' ? 'usage.application' : 'usage.member') }}
      </dt>
      <dd>
        {{ selected.principalName || selected.principalId
        }}<small class="subline">{{ selected.principalId }}</small>
      </dd>
      <dt>{{ t('usage.model') }}</dt>
      <dd>{{ label(models, selected.modelId) }}</dd>
      <dt>{{ t('usage.resource') }}</dt>
      <dd>
        {{ label(resources, selected.resourceId)
        }}<small class="subline">{{ selected.resourceId }}</small>
      </dd>
      <dt>{{ t('usage.protocol') }}</dt>
      <dd>{{ selected.clientProtocol }}</dd>
      <dt>{{ t('common.status') }}</dt>
      <dd><Status :value="selected.status" /></dd>
      <dt>{{ t('usage.errorType') }}</dt>
      <dd>
        <TechnicalValue
          :value="selected.errorType || t('common.none')"
          :copyable="!!selected.errorType"
        />
      </dd>
      <dt>{{ t('usage.errorInfo') }}</dt>
      <dd>{{ errorDescription(selected.errorType) }}</dd>
      <dt>{{ t('usage.time') }}</dt>
      <dd>{{ date(selected.requestAt) }}</dd>
      <dt>{{ t('usage.latency') }}</dt>
      <dd>{{ selected.latencyMs }} ms</dd>
      <dt>{{ t('usage.input') }}</dt>
      <dd>{{ count(selected.inputTokens) }}</dd>
      <dt>{{ t('usage.output') }}</dt>
      <dd>{{ count(selected.outputTokens) }}</dd>
      <dt>{{ t('usage.cached') }}</dt>
      <dd>{{ count(selected.cachedInputTokens) }}</dd>
    </dl></Modal
  >
</template>
