<script setup lang="ts">
import { computed, useId } from 'vue'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
import { t } from '../i18n'

const props = withDefaults(
  defineProps<{
    title: string
    message: string
    hint?: string
    error?: string
    confirmLabel: string
    cancelLabel?: string
    busy?: boolean
    tone?: 'neutral' | 'success' | 'warning' | 'danger'
  }>(),
  { tone: 'neutral' },
)

const emit = defineEmits<{ close: []; confirm: [] }>()
const descriptionId = useId()
const iconName = computed(() => {
  if (props.tone === 'success') return 'check'
  if (props.tone === 'warning' || props.tone === 'danger') return 'alert'
  return 'shield'
})
</script>

<template>
  <Modal
    :title="title"
    :busy="busy"
    :tone="tone"
    :description-id="descriptionId"
    confirm
    @close="emit('close')"
  >
    <div class="confirm-content">
      <Icon class="confirm-status-icon" :name="iconName" :size="24" />
      <div class="confirm-copy">
        <p :id="descriptionId" class="confirm-message">{{ message }}</p>
        <p v-if="hint" class="confirm-hint">{{ hint }}</p>
      </div>
    </div>
    <p v-if="error" class="alert error" role="alert">{{ error }}</p>
    <template #footer>
      <button class="button" :disabled="busy" autofocus @click="emit('close')">
        {{ cancelLabel || t('common.cancel') }}
      </button>
      <button
        class="button"
        :class="tone === 'danger' ? 'danger-fill' : 'primary'"
        :disabled="busy"
        @click="emit('confirm')"
      >
        {{ busy ? t('common.working') : confirmLabel }}
      </button>
    </template>
  </Modal>
</template>
