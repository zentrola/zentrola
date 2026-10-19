import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import { api, errorText } from './api'
import type { Member, Page } from './types'

export type PrincipalKind = 'member' | 'application'

// 用户 / 应用名称联想：输入防抖、按版本号丢弃过期响应，并在点击外部时收起候选列表。
export function usePrincipalAutocomplete(options: {
  name: Ref<string>
  id: Ref<string>
  // 返回空字符串表示当前未限定主体类型，此时不发起联想查询。
  kind: () => PrincipalKind | ''
}) {
  const { name, id, kind } = options
  const root = ref<HTMLElement | null>(null)
  const open = ref(false)
  const loading = ref(false)
  const error = ref('')
  const suggestions = ref<Member[]>([])
  let timer: ReturnType<typeof setTimeout> | undefined
  let revision = 0

  function clear() {
    clearTimeout(timer)
    revision++
    suggestions.value = []
    open.value = false
    loading.value = false
    error.value = ''
  }
  function reset() {
    id.value = ''
    name.value = ''
    clear()
  }
  function schedule() {
    clearTimeout(timer)
    const keyword = name.value.trim()
    const current = ++revision
    if (!keyword || !kind()) {
      open.value = false
      loading.value = false
      return
    }
    open.value = true
    loading.value = true
    timer = setTimeout(() => void fetchSuggestions(keyword, current), 250)
  }
  async function fetchSuggestions(keyword: string, current: number) {
    const path = kind() === 'application' ? 'applications' : 'members'
    const params = new URLSearchParams({ name: keyword, limit: '8' })
    try {
      const result = await api<Page<Member>>(`/${path}/suggestions?${params}`)
      if (current !== revision || keyword !== name.value.trim()) return
      suggestions.value = result.items
    } catch (reason) {
      if (current !== revision) return
      suggestions.value = []
      error.value = errorText(reason)
    } finally {
      if (current === revision) loading.value = false
    }
  }
  function onInput() {
    id.value = ''
    suggestions.value = []
    error.value = ''
    schedule()
  }
  function openSuggestions() {
    if (!name.value.trim() || id.value) return
    open.value = true
    if (!suggestions.value.length) schedule()
  }
  function select(principal: Member) {
    id.value = principal.id
    name.value = principal.name
    clear()
  }
  function closeOnOutsideClick(event: PointerEvent) {
    if (event.target instanceof Node && !root.value?.contains(event.target)) open.value = false
  }
  onMounted(() => document.addEventListener('pointerdown', closeOnOutsideClick))
  onBeforeUnmount(() => {
    clearTimeout(timer)
    revision++
    document.removeEventListener('pointerdown', closeOnOutsideClick)
  })
  return { root, open, loading, error, suggestions, onInput, openSuggestions, select, clear, reset }
}
