import { computed, ref, onBeforeUnmount, type Ref } from 'vue'
import { api, errorText } from './api'
import type { Page } from './types'
import { activeLocale, t } from './i18n'
import { showErrorToast } from './toast'

export function useListSearch<T>(
  items: Ref<T[]>,
  text: (item: T) => string,
  reload?: () => Promise<void>,
) {
  const keyword = ref(''),
    query = ref('')
  const visible = computed(() =>
    items.value.filter((item) => text(item).toLowerCase().includes(query.value)),
  )
  function search() {
    query.value = keyword.value.trim().toLowerCase()
    void reload?.()
  }
  function reset() {
    keyword.value = ''
    query.value = ''
    void reload?.()
  }
  return { keyword, query, visible, search, reset }
}

export function useCollection<T>(path: () => string) {
  const items = ref<T[]>([]) as import('vue').Ref<T[]>
  const cursor = ref<string | null>(null),
    page = ref(1),
    pageSize = ref(20),
    total = ref(0),
    loading = ref(false),
    error = ref('')
  let pageStarts: Array<string | null> = [null]
  let failedRequest: { after: string | null; targetPage: number; reset: boolean } | null = null
  let revision = 0
  onBeforeUnmount(() => revision++)
  async function fetchPage(after: string | null, targetPage: number, reset: boolean) {
    if (loading.value) return
    const current = ++revision
    loading.value = true
    error.value = ''
    failedRequest = { after, targetPage, reset }
    if (reset) {
      items.value = []
      cursor.value = null
      page.value = 1
      total.value = 0
      pageStarts = [null]
    }
    try {
      const route = path()
      const result = await api<Page<T>>(
        `${route}${route.includes('?') ? '&' : '?'}limit=${pageSize.value}${after ? `&after=${after}` : ''}`,
      )
      if (current !== revision) return
      pageStarts[targetPage - 1] = after
      pageStarts.length = targetPage
      items.value = result.items
      cursor.value = result.nextCursor
      page.value = targetPage
      total.value = result.total
      failedRequest = null
    } catch (e) {
      if (current === revision) error.value = errorText(e)
    } finally {
      if (current === revision) loading.value = false
    }
  }
  async function load(more = false) {
    if (more) {
      if (!cursor.value) return
      await fetchPage(cursor.value, page.value + 1, false)
      return
    }
    await fetchPage(null, 1, true)
  }
  async function refresh() {
    await fetchPage(pageStarts[page.value - 1] ?? null, page.value, false)
  }
  async function previous() {
    if (page.value <= 1) return
    await fetchPage(pageStarts[page.value - 2] ?? null, page.value - 1, false)
  }
  async function retry() {
    const request = failedRequest
    if (!request) return
    await fetchPage(request.after, request.targetPage, request.reset)
  }
  async function setPageSize(value: number) {
    if (![20, 50, 100].includes(value) || value === pageSize.value) return
    pageSize.value = value
    await load()
  }
  return {
    items,
    cursor,
    page,
    pageSize,
    total,
    loading,
    error,
    load,
    refresh,
    previous,
    retry,
    setPageSize,
  }
}
export function useAction() {
  const busy = ref(false),
    error = ref('')
  async function run(work: () => Promise<void>) {
    if (busy.value) return
    busy.value = true
    error.value = ''
    try {
      await work()
    } catch (e) {
      error.value = errorText(e)
      showErrorToast(error.value)
    } finally {
      busy.value = false
    }
  }
  return { busy, error, run }
}
export function date(value: string | null | undefined) {
  return value
    ? new Intl.DateTimeFormat(activeLocale.value, {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }).format(new Date(value))
    : t('common.none')
}
export function dateOnly(value: string | null | undefined) {
  return value
    ? new Intl.DateTimeFormat(activeLocale.value, { dateStyle: 'medium' }).format(new Date(value))
    : t('common.none')
}
export function count(value: number | null) {
  return value === null ? '-' : new Intl.NumberFormat(activeLocale.value).format(value)
}
export function compactCount(value: number | null) {
  if (value === null) return '-'
  const absolute = Math.abs(value)
  if (absolute < 1000) return count(value)

  const units = ['K', 'M', 'B', 'T', 'P', 'E']
  let unitIndex = Math.min(Math.floor(Math.log10(absolute) / 3) - 1, units.length - 1)
  let scaled = value / 1000 ** (unitIndex + 1)
  if (Math.abs(scaled) >= 999.5 && unitIndex < units.length - 1) {
    unitIndex++
    scaled /= 1000
  }
  return `${new Intl.NumberFormat(activeLocale.value, { maximumFractionDigits: 1 }).format(scaled)}${units[unitIndex]}`
}
export function validText(value: string, bytes: number, required = true) {
  return (
    (!required && value === '') ||
    (value.length > 0 &&
      value === value.trim() &&
      !value.includes('\0') &&
      new TextEncoder().encode(value).length <= bytes)
  )
}
export function localTime(value: Date) {
  return new Date(value.getTime() - value.getTimezoneOffset() * 60000).toISOString().slice(0, 19)
}
