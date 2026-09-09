<script setup lang="ts">
import { t } from '../i18n'

defineProps<{
  value: string
  name: string
  disabled?: boolean
  busy?: boolean
  ariaLabel?: string
  activeLabel?: string
  inactiveLabel?: string
  title?: string
}>()
defineEmits<{ change: [] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    class="status-switch"
    :aria-label="ariaLabel || t('common.statusFor', { name })"
    :aria-checked="value === 'ACTIVE'"
    :aria-busy="!!busy"
    :title="title"
    :disabled="disabled || busy"
    @click="$emit('change')"
  >
    <span class="status-switch-track" aria-hidden="true"><span class="status-switch-thumb" /></span>
    <span>{{
      value === 'ACTIVE' ? activeLabel || t('state.ACTIVE') : inactiveLabel || t('state.DISABLED')
    }}</span>
  </button>
</template>
