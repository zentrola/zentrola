<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, useId } from 'vue'
import Icon from './Icon.vue'
import { t } from '../i18n'
const props = withDefaults(
  defineProps<{
    title: string
    busy?: boolean
    wide?: boolean
    medium?: boolean
    locked?: boolean
    confirm?: boolean
    tone?: 'neutral' | 'success' | 'warning' | 'danger'
    descriptionId?: string
    bodyClass?: string
  }>(),
  { tone: 'neutral' },
)
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>(),
  label = useId()
const toneClass = computed(() => (props.confirm ? `confirm-${props.tone}` : undefined))
let previous: HTMLElement | null = null
function close() {
  if (!props.busy && !props.locked) emit('close')
}
onMounted(() => {
  previous = document.activeElement as HTMLElement
  dialog.value?.showModal()
})
onBeforeUnmount(() => {
  dialog.value?.close()
  previous?.focus()
})
</script>
<template>
  <Teleport to="body"
    ><dialog
      ref="dialog"
      class="modal"
      :class="[{ wide, medium, confirm }, toneClass]"
      :aria-labelledby="label"
      :aria-describedby="descriptionId"
      @cancel.prevent="close"
    >
      <header class="modal-head">
        <div class="modal-title">
          <h2 :id="label">{{ title }}</h2>
        </div>
        <button
          v-if="!locked"
          class="icon-button"
          :aria-label="t('close')"
          :disabled="busy"
          @click="close"
        >
          <Icon name="close" />
        </button>
      </header>
      <div class="modal-body" :class="bodyClass"><slot /></div>
      <footer v-if="$slots.footer" class="modal-footer"><slot name="footer" /></footer></dialog
  ></Teleport>
</template>
