import { computed, ref, onBeforeUnmount, watch, type Ref } from 'vue'
import { all, api, errorText } from './api'
import type { Page } from './types'
import { activeLocale, t } from './i18n'
import { showErrorToast } from './toast'

export function useListSearch<T>(
  items: Ref<T[]>,
  text: (item: T) => string,
  path?: () => string,
  sourceLoading: Ref<boolean> = ref(false),
) {
  const keyword = ref(''),
    query = ref(''),
    allItems = ref<T[] | null>(null) as Ref<T[] | null>,
    searching = ref(false)
  let revision = 0
  const source = computed(() => allItems.value ?? items.value)
  const visible = computed(() =>
    source.value.filter((item) => text(item).toLocaleLowerCase().includes(query.value)),
  )
  async function loadAll() {
    if (!path) return
    const current = ++revision
    allItems.value = []
    searching.value = true
    try {
      const result = await all<T>(path())
      if (current === revision) allItems.value = result
    } catch (error) {
      if (current === revision) showErrorToast(errorText(error))
    } finally {
      if (current === revision) searching.value = false
    }
  }
  async function search(forceAll = false) {
    query.value = keyword.value.trim().toLocaleLowerCase()
    if (!path || (!query.value && !forceAll)) {
      revision++
      allItems.value = null
      searching.value = false
      return
    }
    await loadAll()
  }
  function reset() {
    revision++
    keyword.value = ''
    query.value = ''
    allItems.value = null
    searching.value = false
  }
  watch([items, sourceLoading], ([, loading]) => {
    if (allItems.value && !searching.value && !loading) void loadAll()
  })
  onBeforeUnmount(() => revision++)
  return {
    keyword,
    query,
    visible,
    search,
    reset,
    searching,
    searchingAll: computed(() => allItems.value !== null),
  }
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
      const route = new URL(path(), 'http://zentrola.local')
      route.searchParams.set('limit', String(pageSize.value))
      if (after) route.searchParams.set('after', after)
      else route.searchParams.delete('after')
      const result = await api<Page<T>>(`${route.pathname}?${route.searchParams}`)
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
const dateTimeFormatters = new Map<string, Intl.DateTimeFormat>()
const dateFormatters = new Map<string, Intl.DateTimeFormat>()
const countFormatters = new Map<string, Intl.NumberFormat>()
const compactFormatters = new Map<string, Intl.NumberFormat>()

function formatter<K, T>(cache: Map<K, T>, key: K, create: () => T) {
  const cached = cache.get(key)
  if (cached) return cached
  const value = create()
  cache.set(key, value)
  return value
}

export function date(value: string | null | undefined) {
  return value
    ? formatter(
        dateTimeFormatters,
        activeLocale.value,
        () =>
          new Intl.DateTimeFormat(activeLocale.value, {
            dateStyle: 'medium',
            timeStyle: 'short',
            hour12: false,
          }),
      ).format(new Date(value))
    : t('common.none')
}
export function dateOnly(value: string | null | undefined) {
  return value
    ? formatter(
        dateFormatters,
        activeLocale.value,
        () => new Intl.DateTimeFormat(activeLocale.value, { dateStyle: 'medium' }),
      ).format(new Date(value))
    : t('common.none')
}
export function count(value: number | null) {
  return value === null
    ? t('common.none')
    : formatter(
        countFormatters,
        activeLocale.value,
        () => new Intl.NumberFormat(activeLocale.value),
      ).format(value)
}
export function compactCount(value: number | null) {
  if (value === null) return t('common.none')
  const absolute = Math.abs(value)
  if (absolute < 1000) return count(value)

  const units = ['K', 'M', 'B', 'T', 'P', 'E']
  let unitIndex = Math.min(Math.floor(Math.log10(absolute) / 3) - 1, units.length - 1)
  let scaled = value / 1000 ** (unitIndex + 1)
  if (Math.abs(scaled) >= 999.5 && unitIndex < units.length - 1) {
    unitIndex++
    scaled /= 1000
  }
  const formatted = formatter(
    compactFormatters,
    activeLocale.value,
    () => new Intl.NumberFormat(activeLocale.value, { maximumFractionDigits: 1 }),
  ).format(scaled)
  return `${formatted}${units[unitIndex]}`
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
