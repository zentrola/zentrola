<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { t } from '../i18n'
import { useToast } from '../toast'
import Icon from './Icon.vue'

const { toast, dismissToast } = useToast()
const host = ref<HTMLElement | null>(null)
const target = ref<string | HTMLElement>('body')

watch(
  () => toast.value?.id,
  async (id) => {
    await nextTick()
    target.value =
      Array.from(document.querySelectorAll<HTMLElement>('dialog[open]')).at(-1) ?? 'body'
    await nextTick()
    const element = host.value
    if (!element) return
    if (element.matches(':popover-open')) element.hidePopover()
    if (id !== undefined) element.showPopover()
  },
  { flush: 'post' },
)

onBeforeUnmount(() => {
  if (host.value?.matches(':popover-open')) host.value.hidePopover()
})
</script>

<template>
  <Teleport :to="target">
    <div ref="host" class="toast-host" popover="manual">
      <div
        v-if="toast"
        class="toast"
        :class="`toast-${toast.tone}`"
        :role="toast.tone === 'error' ? 'alert' : 'status'"
      >
        <span class="toast-icon"
          ><Icon :name="toast.tone === 'error' ? 'alert' : 'check'" :size="19"
        /></span>
        <span class="toast-message">{{ toast.message }}</span>
        <button
          type="button"
          class="icon-button toast-close"
          :aria-label="t('closeNotification')"
          @click="dismissToast(toast.id)"
        >
          <Icon name="close" :size="17" />
        </button>
      </div>
    </div>
  </Teleport>
</template>
