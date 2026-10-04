<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

withDefaults(defineProps<{ hasActions?: boolean }>(), {
  hasActions: false,
})

const container = ref<HTMLElement | null>(null)
const overflowing = ref(false)
const atStart = ref(true)
const atEnd = ref(true)
let resizeObserver: ResizeObserver | undefined

function updateEdges() {
  const element = container.value
  if (!element) return

  const scrollbarGutter = Math.max(0, element.offsetWidth - element.clientWidth)
  const maximum = Math.max(0, element.scrollWidth - element.clientWidth - scrollbarGutter)
  overflowing.value = maximum > 1
  atStart.value = element.scrollLeft <= 1
  atEnd.value = element.scrollLeft >= maximum - 1
}

onMounted(async () => {
  await nextTick()
  const element = container.value
  if (!element) return
  updateEdges()
  resizeObserver = new ResizeObserver(updateEdges)
  resizeObserver.observe(element)
  const table = element.querySelector('table')
  if (table) resizeObserver.observe(table)
})

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<template>
  <div
    ref="container"
    class="table-scroll"
    :class="{
      'table-scroll--actions': hasActions,
      'is-overflowing': overflowing,
      'is-at-start': atStart,
      'is-at-end': atEnd,
    }"
    @scroll.passive="updateEdges"
  >
    <slot />
  </div>
</template>
