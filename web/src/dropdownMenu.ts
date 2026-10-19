import { nextTick, onMounted, onUnmounted, ref, useId } from 'vue'

// 下拉菜单的通用交互：打开后聚焦首项（或末项）、方向键 / Home / End 循环移动焦点、
// Esc 关闭并把焦点还给触发按钮、点击外部或焦点移出时收起。
export function useDropdownMenu(itemSelector = '[role="menuitem"]') {
  const open = ref(false)
  const root = ref<HTMLElement>()
  const trigger = ref<HTMLButtonElement>()
  const menu = ref<HTMLElement>()
  const menuId = useId()

  function items() {
    return Array.from(menu.value?.querySelectorAll<HTMLElement>(itemSelector) ?? [])
  }
  function close(restoreFocus = false) {
    open.value = false
    if (restoreFocus) trigger.value?.focus()
  }
  async function show(last = false) {
    open.value = true
    await nextTick()
    const candidates = items()
    candidates[last ? candidates.length - 1 : 0]?.focus()
  }
  function toggle() {
    if (open.value) close()
    else void show()
  }
  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault()
      close(true)
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const candidates = items()
    const current = candidates.indexOf(document.activeElement as HTMLElement)
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? candidates.length - 1
          : (current + (event.key === 'ArrowDown' ? 1 : -1) + candidates.length) % candidates.length
    candidates[next]?.focus()
  }
  function onOutside(event: PointerEvent) {
    if (!root.value?.contains(event.target as Node)) close()
  }
  function onFocusOut(event: FocusEvent) {
    if (!root.value?.contains(event.relatedTarget as Node | null)) close()
  }
  onMounted(() => document.addEventListener('pointerdown', onOutside))
  onUnmounted(() => document.removeEventListener('pointerdown', onOutside))
  return { open, root, trigger, menu, menuId, close, show, toggle, onKeydown, onFocusOut }
}
