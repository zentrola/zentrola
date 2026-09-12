<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, all, errorText } from '../api'
import { useCollection, date, count } from '../composables'
import { i18n, t } from '../i18n'
import { showErrorToast } from '../toast'
import type { Usage, Member, Model, Page, Resource } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import TableScroll from '../components/TableScroll.vue'
const memberID = ref(''),
  memberName = ref(''),
  modelID = ref(''),
  from = ref(''),
  to = ref(''),
  query = ref(''),
  lookupError = ref('')
const models = ref<Model[]>([]),
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
  memberSearchRevision = 0

function errorDescription(errorType: string | null) {
  if (!errorType) return t('common.none')
  const key = `errors.${errorType}`
  if (i18n.global.te(key)) return t(key)
  const upstreamHTTPStatus = errorType.match(/^UPSTREAM_HTTP_(\d{3})$/)?.[1]
  return upstreamHTTPStatus
    ? t('usage.upstreamHTTPError', { status: upstreamHTTPStatus })
    : t('errors.UNKNOWN')
}
const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<Usage>(() => `/usage?${query.value}`)
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
  weekStart.setDate(weekStart.getDate() - 6)
  to.value = dateValue(now)
  from.value = dateValue(weekStart)
}
function dateValue(value: Date) {
  const offset = value.getTimezoneOffset() * 60000
  return new Date(value.getTime() - offset).toISOString().slice(0, 10)
}
function dateFromValue(value: string) {
  const [year, month, day] = value.split('-').map(Number)
  return new Date(year, month - 1, day)
}
function startOfMonth(value: Date) {
  return new Date(value.getFullYear(), value.getMonth(), 1)
}
function addMonths(value: Date, amount: number) {
  return new Date(value.getFullYear(), value.getMonth() + amount, 1)
}
function monthTitle(value: Date) {
  return t('usage.monthTitle', { year: value.getFullYear(), month: value.getMonth() + 1 })
}
function calendarDays(month: Date) {
  const first = startOfMonth(month),
    start = new Date(first)
  start.setDate(start.getDate() - start.getDay())
  return Array.from({ length: 42 }, (_, index) => {
    const value = new Date(start)
    value.setDate(start.getDate() + index)
    return {
      value: dateValue(value),
      day: value.getDate(),
      currentMonth:
        value.getMonth() === month.getMonth() && value.getFullYear() === month.getFullYear(),
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
  const start = Date.parse(`${startValue}T00:00:00`),
    endDate = new Date(`${endValue}T00:00:00`)
  endDate.setDate(endDate.getDate() + 1)
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
function search() {
  if (memberName.value && !memberID.value) {
    showErrorToast(t('usage.selectMember'))
    return
  }
  const bounds = dateBounds(from.value, to.value)
  if (!bounds) {
    showErrorToast(t('usage.invalidRange'))
    return
  }
  const params = new URLSearchParams({
    from: new Date(bounds.start).toISOString(),
    to: new Date(bounds.end).toISOString(),
  })
  for (const [key, value] of [
    ['memberId', memberID.value],
    ['modelId', modelID.value],
  ])
    if (value) params.set(key, value)
  query.value = params.toString()
  void load()
}
function reset() {
  memberID.value = ''
  memberName.value = ''
  modelID.value = ''
  resetTimes()
  search()
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
    ;[models.value, resources.value] = await Promise.all([
      all<Model>('/models'),
      all<Resource>('/resources'),
    ])
  } catch (e) {
    lookupError.value = errorText(e)
  }
}
function label(list: { id: string; name: string }[], id: string | null) {
  return id === null ? t('common.none') : list.find((x) => x.id === id)?.name || id
}
onMounted(() => {
  document.addEventListener('pointerdown', closeFloatingPanels)
  void lookups()
  reset()
})
onBeforeUnmount(() => {
  clearTimeout(memberSearchTimer)
  memberSearchRevision++
  document.removeEventListener('pointerdown', closeFloatingPanels)
})
</script>
<template>
  <PageHeader name="usage" />
  <section class="panel usage-list-panel">
    <form
      class="table-toolbar usage-filters"
      :aria-label="t('usage.filters')"
      @submit.prevent="search"
    >
      <div ref="memberAutocomplete" class="filter-field member-filter-field">
        <span class="filter-label">{{ t('usage.member') }}</span>
        <div class="member-autocomplete" @keydown.esc="memberSuggestionsOpen = false">
          <Icon class="member-search-icon" name="search" :size="18" />
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
      <label class="model-filter-field">
        <span class="filter-label">{{ t('usage.model') }}</span>
        <select v-model="modelID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="model in models" :key="model.id" :value="model.id">
            {{ model.name }}
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
        <button class="button primary" :disabled="loading">
          <Icon name="search" :size="16" />{{ t('common.searchAction') }}</button
        ><button type="button" class="button" :disabled="loading" @click="reset">
          {{ t('common.reset') }}
        </button>
      </div>
    </form>
    <p v-if="lookupError" class="alert error" role="alert">
      {{ lookupError }}<button class="text-button" @click="lookups">{{ t('common.retry') }}</button>
    </p>
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </p>
    <TableScroll has-actions>
      <table class="usage-table">
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
          <tr v-for="row in items" :key="row.id">
            <td>
              {{ date(row.requestAt) }}<small class="subline">{{ row.latencyMs }} ms</small>
            </td>
            <td>{{ row.principalName || row.principalId }}</td>
            <td>
              {{ label(models, row.modelId)
              }}<small class="subline">{{ label(resources, row.resourceId) }}</small>
            </td>
            <td>
              <span class="protocol-label">{{
                row.clientProtocol === 'OPENAI'
                  ? 'OpenAI'
                  : row.clientProtocol === 'ANTHROPIC'
                    ? 'Anthropic'
                    : row.clientProtocol
              }}</span>
            </td>
            <td><Status :value="row.status" /></td>
            <td class="numeric">{{ count(row.inputTokens) }}</td>
            <td class="numeric">{{ count(row.outputTokens) }}</td>
            <td class="numeric">{{ count(row.cachedInputTokens) }}</td>
            <td class="align-right">
              <button class="text-button" @click="selected = row">{{ t('common.details') }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!items.length" class="empty-state">
      <Icon name="usage" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : 'common.empty') }}</h3>
      <p v-if="!loading">{{ t('usage.empty') }}</p>
    </div>
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
  <Modal v-if="selected" :title="t('common.details')" wide @close="selected = null"
    ><dl class="detail-grid">
      <dt>{{ t('common.requestId') }}</dt>
      <dd>
        <code>{{ selected.requestId }}</code>
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
      <dd>{{ selected.errorType || t('common.none') }}</dd>
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
