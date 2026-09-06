<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, useId } from 'vue'
import Icon from './Icon.vue'
import { t } from '../i18n'
const props = defineProps<{
  title: string
  busy?: boolean
  wide?: boolean
  medium?: boolean
  locked?: boolean
}>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>(),
  label = useId()
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
      :class="{ wide, medium }"
      :aria-labelledby="label"
      @cancel.prevent="close"
    >
      <header class="modal-head">
        <h2 :id="label">{{ title }}</h2>
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
      <div class="modal-body"><slot /></div></dialog
  ></Teleport>
</template>
