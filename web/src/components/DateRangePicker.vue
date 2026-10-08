<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { t } from '../i18n'
import Icon from './Icon.vue'

const props = withDefaults(
  defineProps<{
    from: string
    to: string
    label?: string
    maxDays?: number
  }>(),
  { label: '', maxDays: 366 },
)
const emit = defineEmits<{
  'update:from': [value: string]
  'update:to': [value: string]
}>()

const root = ref<HTMLElement | null>(null)
const open = ref(false)
const draftFrom = ref('')
const draftTo = ref('')
const hoveredDate = ref('')
const selectingEnd = ref(false)
const validation = ref('')
const calendarMonth = ref(new Date())

const rangeText = computed(() => {
  if (!props.from || !props.to) return t('usage.dateRangePlaceholder')
  return `${props.from.replaceAll('-', '/')} — ${props.to.replaceAll('-', '/')}`
})
const calendarMonths = computed(() => [calendarMonth.value, addMonths(calendarMonth.value, 1)])
const weekdays = computed(() =>
  Array.from({ length: 7 }, (_, index) => t(`usage.weekdays.${index}`)),
)

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
  const first = startOfMonth(month)
  const start = new Date(first)
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
  const candidateEnd = draftTo.value || (selectingEnd.value ? hoveredDate.value : '')
  if (!candidateEnd) return [draftFrom.value, draftFrom.value]
  return draftFrom.value <= candidateEnd
    ? [draftFrom.value, candidateEnd]
    : [candidateEnd, draftFrom.value]
}
function dayClass(value: string, currentMonth: boolean) {
  const [start, end] = draftRange()
  return {
    muted: !currentMonth,
    'range-start': value === start,
    'range-end': value === end,
    'in-range': Boolean(start && end && value >= start && value <= end),
  }
}
function validRange(start: string, end: string) {
  const startTime = Date.parse(`${start}T00:00:00Z`)
  const endTime = Date.parse(`${end}T00:00:00Z`)
  return (
    Number.isFinite(startTime) &&
    Number.isFinite(endTime) &&
    endTime >= startTime &&
    endTime - startTime + 86400000 <= props.maxDays * 86400000
  )
}
function toggle() {
  if (open.value) {
    open.value = false
    return
  }
  draftFrom.value = props.from
  draftTo.value = props.to
  calendarMonth.value = startOfMonth(dateFromValue(props.from))
  selectingEnd.value = false
  hoveredDate.value = ''
  validation.value = ''
  open.value = true
}
function selectDate(value: string) {
  if (!selectingEnd.value) {
    draftFrom.value = value
    draftTo.value = ''
    hoveredDate.value = value
    selectingEnd.value = true
    validation.value = ''
    return
  }
  const start = value < draftFrom.value ? value : draftFrom.value
  const end = value < draftFrom.value ? draftFrom.value : value
  draftFrom.value = start
  draftTo.value = end
  selectingEnd.value = false
  hoveredDate.value = ''
  if (!validRange(start, end)) {
    validation.value = t('usage.invalidRange')
    return
  }
  emit('update:from', start)
  emit('update:to', end)
  validation.value = ''
  open.value = false
}
function changeMonth(amount: number) {
  calendarMonth.value = addMonths(calendarMonth.value, amount)
}
function closeOnOutsideClick(event: PointerEvent) {
  if (event.target instanceof Node && !root.value?.contains(event.target)) open.value = false
}

onMounted(() => document.addEventListener('pointerdown', closeOnOutsideClick))
onBeforeUnmount(() => document.removeEventListener('pointerdown', closeOnOutsideClick))
</script>

<template>
  <div ref="root" class="filter-field date-range-field">
    <span class="filter-label">{{ label || t('usage.dateRange') }}</span>
    <div class="date-range-control" @keydown.esc="open = false">
      <button
        type="button"
        class="date-range-trigger"
        aria-haspopup="dialog"
        :aria-expanded="open"
        :aria-label="t('usage.chooseDateRange')"
        @click="toggle"
      >
        <span>{{ rangeText }}</span>
        <Icon name="calendar" :size="18" />
      </button>
      <div
        v-if="open"
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
                @click="changeMonth(-1)"
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
                @click="changeMonth(1)"
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
                :class="dayClass(day.value, day.currentMonth)"
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
          {{ t(selectingEnd ? 'usage.selectRangeEnd' : 'usage.selectRangeStart') }}
        </p>
        <p v-if="validation" class="date-range-error" role="alert">{{ validation }}</p>
      </div>
    </div>
  </div>
</template>
