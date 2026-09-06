import { computed, ref, onBeforeUnmount, type Ref } from 'vue'
import { api, errorText } from './api'
import type { Page } from './types'
import { t } from './i18n'

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
    loading = ref(false),
    error = ref('')
  let revision = 0
  onBeforeUnmount(() => revision++)
  async function load(more = false) {
    if (more && (loading.value || !cursor.value)) return
    const current = ++revision
    loading.value = true
    error.value = ''
    if (!more) {
      items.value = []
      cursor.value = null
    }
    try {
      const route = path()
      const result = await api<Page<T>>(
        `${route}${route.includes('?') ? '&' : '?'}limit=50${more ? `&after=${cursor.value}` : ''}`,
      )
      if (current !== revision) return
      items.value = more ? [...items.value, ...result.items] : result.items
      cursor.value = result.nextCursor
    } catch (e) {
      if (current === revision) error.value = errorText(e)
    } finally {
      if (current === revision) loading.value = false
    }
  }
  return { items, cursor, loading, error, load }
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
    } finally {
      busy.value = false
    }
  }
  return { busy, error, run }
}
export function date(value: string | null | undefined) {
  return value
    ? new Intl.DateTimeFormat('zh-CN', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }).format(new Date(value))
    : t('common.none')
}
export function dateOnly(value: string | null | undefined) {
  return value
    ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(new Date(value))
    : t('common.none')
}
export function count(value: number | null) {
  return value === null ? '—' : new Intl.NumberFormat('zh-CN').format(value)
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
