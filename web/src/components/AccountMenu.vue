<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { useRoute } from 'vue-router'
import { t } from '../i18n'
import Icon from './Icon.vue'

defineProps<{ username: string; displayName: string }>()
const emit = defineEmits<{ changePassword: []; logout: [] }>()
const open = ref(false)
const root = ref<HTMLElement>(),
  trigger = ref<HTMLButtonElement>(),
  menu = ref<HTMLElement>()
const menuId = useId()
const route = useRoute()

function close(restoreFocus = false) {
  open.value = false
  if (restoreFocus) trigger.value?.focus()
}
async function show(last = false) {
  open.value = true
  await nextTick()
  const items = menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  items?.[last ? items.length - 1 : 0]?.focus()
}
function select(action: 'changePassword' | 'logout') {
  // 弹窗打开前将焦点放回账号按钮，关闭弹窗后也能回到稳定的入口。
  close(true)
  if (action === 'changePassword') emit('changePassword')
  else emit('logout')
}
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    close(true)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = Array.from(
    menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [],
  )
  const current = items.indexOf(document.activeElement as HTMLButtonElement)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
  items[next]?.focus()
}
function onOutside(event: PointerEvent) {
  if (!root.value?.contains(event.target as Node)) close()
}
function onFocusOut(event: FocusEvent) {
  if (!root.value?.contains(event.relatedTarget as Node | null)) close()
}
watch(
  () => route.fullPath,
  () => close(),
)
onMounted(() => document.addEventListener('pointerdown', onOutside))
onUnmounted(() => document.removeEventListener('pointerdown', onOutside))
</script>

<template>
  <div ref="root" class="account-menu" @focusout="onFocusOut">
    <button
      ref="trigger"
      type="button"
      class="account-trigger"
      :aria-label="t('accountMenu.label')"
      aria-haspopup="menu"
      :aria-expanded="open"
      :aria-controls="menuId"
      @click="open ? close() : show()"
      @keydown.down.prevent="show()"
      @keydown.up.prevent="show(true)"
      @keydown.esc.prevent="close(true)"
    >
      <span class="avatar account-avatar" aria-hidden="true">{{
        (displayName || username).slice(0, 1)
      }}</span>
      <span class="account-name">{{ username }}</span>
      <Icon class="account-chevron" :class="{ expanded: open }" name="arrow" :size="14" />
    </button>
    <div
      v-if="open"
      :id="menuId"
      ref="menu"
      class="account-dropdown"
      role="menu"
      :aria-label="t('accountMenu.label')"
      @keydown="onKeydown"
    >
      <div class="account-summary" role="presentation">
        <strong>{{ displayName || username }}</strong
        ><span>{{ t('admin') }}</span>
      </div>
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        class="account-item"
        @click="select('changePassword')"
      >
        <Icon name="key" :size="17" />{{ t('accountMenu.changePassword') }}
      </button>
      <div class="account-divider" role="separator"></div>
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        class="account-item logout-item"
        @click="select('logout')"
      >
        <Icon name="logout" :size="17" />{{ t('logout') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.account-menu {
  position: relative;
  flex: none;
}
.account-trigger {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 5px 8px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  font-size: 12px;
  color: #4b6176;
}
.account-trigger:hover,
.account-trigger[aria-expanded='true'] {
  background: #f3f6fa;
}
.account-trigger:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: 2px;
}
.account-avatar {
  width: 30px;
  height: 30px;
  font-size: 12px;
}
.account-name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.account-chevron {
  transform: rotate(90deg);
  transition: transform 150ms;
  color: var(--muted);
}
.account-chevron.expanded {
  transform: rotate(-90deg);
}
.account-dropdown {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  z-index: 15;
  width: 220px;
  max-width: calc(100vw - 34px);
  padding: 6px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 8px 28px #172f461a;
}
.account-summary {
  display: flex;
  flex-direction: column;
  padding: 10px 11px 12px;
  margin-bottom: 4px;
  border-bottom: 1px solid var(--line);
}
.account-summary strong {
  overflow-wrap: anywhere;
  font-size: 13px;
  font-weight: 600;
}
.account-summary span {
  color: var(--muted);
  font-size: 11px;
  margin-top: 3px;
}
.account-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 11px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  text-align: left;
  font-size: 13px;
}
.account-item:hover,
.account-item:focus-visible {
  background: #edf3fd;
  outline: none;
}
.account-divider {
  height: 1px;
  margin: 4px 5px;
  background: var(--line);
}
.logout-item {
  color: var(--danger);
}
.logout-item:hover,
.logout-item:focus-visible {
  background: #fff2f2;
}
@media (max-width: 640px) {
  .account-name {
    display: none;
  }
}
</style>
