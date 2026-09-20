<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, all, errorText } from '../api'
import { useCollection, date, count } from '../composables'
import { activeLocale, i18n, t } from '../i18n'
import { showErrorToast } from '../toast'
import type { Usage, UsageStatistic, Member, Model, Page, Provider, Resource } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import TableScroll from '../components/TableScroll.vue'
import TechnicalValue from '../components/TechnicalValue.vue'

type UsageView = 'statistics' | 'records'
type StatisticDimension = 'member' | 'model' | 'provider'
const statisticDimensions: StatisticDimension[] = ['member', 'model', 'provider']

const route = useRoute(),
  router = useRouter()
const activeView = ref<UsageView>('statistics'),
  statisticDimension = ref<StatisticDimension>('member'),
  statisticsQuery = ref('')
const memberID = ref(''),
  memberName = ref(''),
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
const memberAutocomplete = ref<HTMLElement | null>(null),
  dateRangeControl = ref<HTMLElement | null>(null),
  memberSuggestionsOpen = ref(false),
  memberSuggestionsLoading = ref(false),
  memberSearchError = ref(''),
  memberSuggestions = ref<Member[]>([]),
  datePickerOpen = ref(false),
  draftFrom = ref(''),
  draftTo = ref(''),
  calendarMonth = ref(new Date()),
  rangeSelectionStarted = ref(false),
  hoveredDate = ref(''),
  datePickerValidation = ref('')
let memberSearchTimer: ReturnType<typeof setTimeout> | undefined,
  memberSearchRevision = 0,
  lookupPromise: Promise<void> | null = null

function errorDescription(errorType: string | null) {
  if (!errorType) return t('common.none')
  const key = `errors.${errorType}`
  if (i18n.global.te(key)) return t(key)
  const upstreamHTTPStatus = errorType.match(/^UPSTREAM_HTTP_(\d{3})$/)?.[1]
  return upstreamHTTPStatus
    ? t('usage.upstreamHTTPError', { status: upstreamHTTPStatus })
    : t('errors.UNKNOWN')
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
const dateRangeText = computed(() => {
  if (!from.value || !to.value) return t('usage.dateRangePlaceholder')
  return `${from.value.replaceAll('-', '/')} — ${to.value.replaceAll('-', '/')}`
})
const calendarMonths = computed(() => [calendarMonth.value, addMonths(calendarMonth.value, 1)])
const weekdays = computed(() =>
  Array.from({ length: 7 }, (_, index) => t(`usage.weekdays.${index}`)),
)
function resetTimes() {
  const now = new Date()
  const weekStart = new Date(now)
  weekStart.setUTCDate(weekStart.getUTCDate() - 6)
  to.value = dateValue(now)
  from.value = dateValue(weekStart)
}
function dateValue(value: Date) {
  return value.toISOString().slice(0, 10)
}
function dateFromValue(value: string) {
  const [year, month, day] = value.split('-').map(Number)
  return new Date(Date.UTC(year, month - 1, day))
}
function startOfMonth(value: Date) {
  return new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), 1))
}
function addMonths(value: Date, amount: number) {
  return new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth() + amount, 1))
}
function monthTitle(value: Date) {
  return t('usage.monthTitle', { year: value.getUTCFullYear(), month: value.getUTCMonth() + 1 })
}
function calendarDays(month: Date) {
  const first = startOfMonth(month),
    start = new Date(first)
  start.setUTCDate(start.getUTCDate() - start.getUTCDay())
  return Array.from({ length: 42 }, (_, index) => {
    const value = new Date(start)
    value.setUTCDate(start.getUTCDate() + index)
    return {
      value: dateValue(value),
      day: value.getUTCDate(),
      currentMonth:
        value.getUTCMonth() === month.getUTCMonth() &&
        value.getUTCFullYear() === month.getUTCFullYear(),
    }
  })
}
function draftRange() {
  if (!draftFrom.value) return ['', '']
  const candidateEnd = draftTo.value || (rangeSelectionStarted.value ? hoveredDate.value : '')
  if (!candidateEnd) return [draftFrom.value, draftFrom.value]
  return draftFrom.value <= candidateEnd
    ? [draftFrom.value, candidateEnd]
    : [candidateEnd, draftFrom.value]
}
function calendarDayClass(value: string, currentMonth: boolean) {
  const [start, end] = draftRange()
  return {
    muted: !currentMonth,
    'range-start': value === start,
    'range-end': value === end,
    'in-range': Boolean(start && end && value >= start && value <= end),
  }
}
function dateBounds(startValue: string, endValue: string) {
  const start = Date.parse(`${startValue}T00:00:00Z`),
    endDate = new Date(`${endValue}T00:00:00Z`)
  endDate.setUTCDate(endDate.getUTCDate() + 1)
  const end = endDate.getTime()
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    startValue === endValue ||
    end <= start ||
    end - start > 366 * 86400000
  )
    return null
  return { start, end }
}
function rangeParams() {
  const bounds = dateBounds(from.value, to.value)
  if (!bounds) {
    showErrorToast(t('usage.invalidRange'))
    return null
  }
  return new URLSearchParams({
    from: new Date(bounds.start).toISOString(),
    to: new Date(bounds.end).toISOString(),
  })
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
      routeQuery.memberId = memberID.value
      routeQuery.memberName = memberName.value
    }
    if (modelID.value) routeQuery.modelId = modelID.value
    if (providerID.value) routeQuery.providerId = providerID.value
  }
  void router.replace({ name: 'usage', query: routeQuery })
}
function searchRecords() {
  if (memberName.value && !memberID.value) {
    showErrorToast(t('usage.selectMember'))
    return
  }
  const params = rangeParams()
  if (!params) return
  for (const [key, value] of [
    ['memberId', memberID.value],
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
  if (statisticDimension.value === 'member') {
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
function onMemberInput() {
  memberID.value = ''
  memberSuggestions.value = []
  memberSearchError.value = ''
  scheduleMemberSearch()
}
function openMemberSuggestions() {
  if (!memberName.value.trim()) return
  memberSuggestionsOpen.value = true
  if (!memberSuggestions.value.length) scheduleMemberSearch()
}
function scheduleMemberSearch() {
  clearTimeout(memberSearchTimer)
  const keyword = memberName.value.trim(),
    revision = ++memberSearchRevision
  if (!keyword) {
    memberSuggestionsOpen.value = false
    memberSuggestionsLoading.value = false
    return
  }
  memberSuggestionsOpen.value = true
  memberSuggestionsLoading.value = true
  memberSearchTimer = setTimeout(() => void loadMemberSuggestions(keyword, revision), 250)
}
async function loadMemberSuggestions(keyword: string, revision: number) {
  const params = new URLSearchParams({ name: keyword, limit: '8' })
  try {
    const result = await api<Page<Member>>(`/members/suggestions?${params}`)
    if (revision !== memberSearchRevision || keyword !== memberName.value.trim()) return
    memberSuggestions.value = result.items
  } catch (e) {
    if (revision !== memberSearchRevision) return
    memberSuggestions.value = []
    memberSearchError.value = errorText(e)
  } finally {
    if (revision === memberSearchRevision) memberSuggestionsLoading.value = false
  }
}
function selectMember(member: Member) {
  memberID.value = member.id
  memberName.value = member.name
  memberSuggestionsOpen.value = false
}
function toggleDatePicker() {
  if (datePickerOpen.value) {
    datePickerOpen.value = false
    return
  }
  draftFrom.value = from.value
  draftTo.value = to.value
  calendarMonth.value = startOfMonth(dateFromValue(from.value))
  rangeSelectionStarted.value = false
  hoveredDate.value = ''
  datePickerValidation.value = ''
  datePickerOpen.value = true
}
function selectDate(value: string) {
  if (!rangeSelectionStarted.value) {
    draftFrom.value = value
    draftTo.value = ''
    hoveredDate.value = value
    rangeSelectionStarted.value = true
    datePickerValidation.value = ''
    return
  }
  const start = value < draftFrom.value ? value : draftFrom.value,
    end = value < draftFrom.value ? draftFrom.value : value
  draftFrom.value = start
  draftTo.value = end
  rangeSelectionStarted.value = false
  hoveredDate.value = ''
  if (!dateBounds(start, end)) {
    datePickerValidation.value = t('usage.invalidRange')
    return
  }
  from.value = start
  to.value = end
  datePickerValidation.value = ''
  datePickerOpen.value = false
}
function changeCalendarMonth(amount: number) {
  calendarMonth.value = addMonths(calendarMonth.value, amount)
}
function closeFloatingPanels(event: PointerEvent) {
  if (!(event.target instanceof Node)) return
  if (!memberAutocomplete.value?.contains(event.target)) memberSuggestionsOpen.value = false
  if (!dateRangeControl.value?.contains(event.target)) datePickerOpen.value = false
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
  if (dimension === 'model' || dimension === 'provider') statisticDimension.value = dimension
  const routeFrom = routeValue('from'),
    routeTo = routeValue('to')
  if (/^\d{4}-\d{2}-\d{2}$/.test(routeFrom) && /^\d{4}-\d{2}-\d{2}$/.test(routeTo)) {
    from.value = routeFrom
    to.value = routeTo
  } else resetTimes()
  memberID.value = routeValue('memberId')
  memberName.value = routeValue('memberName')
  modelID.value = routeValue('modelId')
  providerID.value = routeValue('providerId')
}
onMounted(() => {
  document.addEventListener('pointerdown', closeFloatingPanels)
  applyRoute()
  if (activeView.value === 'records') void ensureLookups()
  search()
})
onBeforeUnmount(() => {
  clearTimeout(memberSearchTimer)
  memberSearchRevision++
  document.removeEventListener('pointerdown', closeFloatingPanels)
})
</script>
<template>
  <PageHeader name="usage" />
  <div class="usage-view-tabs" role="tablist" :aria-label="t('nav.usage')">
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
      <div
        v-if="activeView === 'records'"
        ref="memberAutocomplete"
        class="filter-field member-filter-field"
      >
        <span class="filter-label">{{ t('usage.member') }}</span>
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
            :aria-label="t('usage.member')"
            :placeholder="t('usage.memberPlaceholder')"
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
              {{ t('usage.noMemberMatches') }}
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
      <div ref="dateRangeControl" class="filter-field date-range-field">
        <span class="filter-label">{{ t('usage.dateRange') }}</span>
        <div class="date-range-control" @keydown.esc="datePickerOpen = false">
          <button
            type="button"
            class="date-range-trigger"
            aria-haspopup="dialog"
            :aria-expanded="datePickerOpen"
            :aria-label="t('usage.chooseDateRange')"
            @click="toggleDatePicker"
          >
            <span>{{ dateRangeText }}</span
            ><Icon name="calendar" :size="18" />
          </button>
          <div
            v-if="datePickerOpen"
            class="date-range-popover"
            role="dialog"
            :aria-label="t('usage.chooseDateRange')"
          >
            <div class="calendar-panels">
              <section
                v-for="(month, monthIndex) in calendarMonths"
                :key="dateValue(month)"
                class="calendar-panel"
                :aria-label="monthTitle(month)"
              >
                <header class="calendar-header">
                  <button
                    v-if="monthIndex === 0"
                    type="button"
                    class="calendar-nav"
                    :aria-label="t('usage.previousMonth')"
                    @click="changeCalendarMonth(-1)"
                  >
                    ‹
                  </button>
                  <span v-else></span>
                  <strong>{{ monthTitle(month) }}</strong>
                  <button
                    v-if="monthIndex === 1"
                    type="button"
                    class="calendar-nav"
                    :aria-label="t('usage.nextMonth')"
                    @click="changeCalendarMonth(1)"
                  >
                    ›
                  </button>
                  <span v-else></span>
                </header>
                <div class="calendar-weekdays" aria-hidden="true">
                  <span v-for="weekday in weekdays" :key="weekday">{{ weekday }}</span>
                </div>
                <div class="calendar-days">
                  <button
                    v-for="day in calendarDays(month)"
                    :key="day.value"
                    type="button"
                    :class="calendarDayClass(day.value, day.currentMonth)"
                    :aria-label="t('usage.selectDate', { date: day.value })"
                    :aria-pressed="day.value === draftFrom || day.value === draftTo"
                    @mouseenter="hoveredDate = day.value"
                    @focus="hoveredDate = day.value"
                    @click="selectDate(day.value)"
                  >
                    {{ day.day }}
                  </button>
                </div>
              </section>
            </div>
            <p class="date-range-guidance" aria-live="polite">
              {{ t(rangeSelectionStarted ? 'usage.selectRangeEnd' : 'usage.selectRangeStart') }}
            </p>
            <p v-if="datePickerValidation" class="date-range-error" role="alert">
              {{ datePickerValidation }}
            </p>
          </div>
        </div>
      </div>
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
              <th>{{ t('usage.member') }}</th>
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
      <dt>{{ t('usage.member') }}</dt>
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
