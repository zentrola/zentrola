<script setup lang="ts">
import { nextTick, onUnmounted, ref } from 'vue'
import type { CSSProperties } from 'vue'

defineProps<{ label: string; title: string }>()

const root = ref<HTMLDetailsElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
const menu = ref<HTMLElement | null>(null)
const menuStyle = ref<CSSProperties>({ top: '0px', left: '0px', visibility: 'hidden' })

function menuIsOpen() {
  return menu.value?.matches(':popover-open') ?? false
}

function removeViewportListeners() {
  window.removeEventListener('resize', closeOnViewportChange)
  window.removeEventListener('scroll', closeOnViewportChange, true)
}

function closeOnViewportChange() {
  close()
}

function close(restoreFocus = false) {
  if (menuIsOpen()) menu.value?.hidePopover()
  if (root.value?.open) root.value.open = false
  removeViewportListeners()
  if (restoreFocus) void nextTick(() => trigger.value?.focus())
}

function positionMenu() {
  if (!trigger.value || !menu.value || !menuIsOpen()) return
  const viewportPadding = 8
  const gap = 4
  const triggerBox = trigger.value.getBoundingClientRect()
  const menuBox = menu.value.getBoundingClientRect()
  let left = triggerBox.left - menuBox.width - gap
  if (left < viewportPadding) left = triggerBox.right + gap
  left = Math.min(
    Math.max(left, viewportPadding),
    window.innerWidth - menuBox.width - viewportPadding,
  )
  const centeredTop = triggerBox.top + (triggerBox.height - menuBox.height) / 2
  const top = Math.min(
    Math.max(centeredTop, viewportPadding),
    window.innerHeight - menuBox.height - viewportPadding,
  )
  menuStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    visibility: 'visible',
  }
}

async function openMenu() {
  if (!menu.value || menuIsOpen()) return
  menuStyle.value = { top: '0px', left: '0px', visibility: 'hidden' }
  menu.value.showPopover()
  await nextTick()
  positionMenu()
  window.addEventListener('resize', closeOnViewportChange)
  window.addEventListener('scroll', closeOnViewportChange, true)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !menuIsOpen()) return
  event.preventDefault()
  close(true)
}

function onToggle() {
  if (root.value?.open) void openMenu()
  else if (menuIsOpen()) menu.value?.hidePopover()
}

function onPopoverToggle() {
  if (menuIsOpen()) return
  menuStyle.value = { top: '0px', left: '0px', visibility: 'hidden' }
  removeViewportListeners()
  if (root.value?.open) root.value.open = false
}

onUnmounted(() => {
  removeViewportListeners()
  if (menuIsOpen()) menu.value?.hidePopover()
})
</script>

<template>
  <details ref="root" class="row-action-more" @toggle="onToggle" @keydown="onKeydown">
    <summary ref="trigger" :aria-label="label" :title="title">⋮</summary>
    <div
      ref="menu"
      popover="auto"
      class="row-action-more-menu"
      :style="menuStyle"
      @click="close()"
      @toggle="onPopoverToggle"
    >
      <slot />
    </div>
  </details>
</template>

<style scoped>
.row-action-more {
  position: relative;
  flex: none;
}
.row-action-more summary {
  display: grid;
  width: 26px;
  height: 26px;
  place-items: center;
  border-radius: 5px;
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  list-style: none;
}
.row-action-more summary::-webkit-details-marker {
  display: none;
}
.row-action-more summary:hover,
.row-action-more summary:focus-visible {
  color: var(--blue);
  background: var(--color-primary-soft);
}
.row-action-more summary:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.row-action-more-menu {
  position: fixed;
  z-index: 1000;
  inset: auto;
  display: grid;
  gap: 2px;
  min-width: 112px;
  box-sizing: border-box;
  margin: 0;
  padding: 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #fff;
  box-shadow: 0 12px 28px rgb(15 23 42 / 12%);
  white-space: nowrap;
}
.row-action-more-menu :deep(.text-button) {
  width: 100%;
  box-sizing: border-box;
  justify-content: flex-start;
  padding: 6px 8px;
}
</style>
