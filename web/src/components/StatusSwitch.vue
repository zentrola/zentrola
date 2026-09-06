<script setup lang="ts">
import { t } from '../i18n'

defineProps<{ value: string; name: string; disabled?: boolean; busy?: boolean }>()
defineEmits<{ change: [] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    class="status-switch"
    :aria-label="t('common.statusFor', { name })"
    :aria-checked="value === 'ACTIVE'"
    :aria-busy="!!busy"
    :disabled="disabled || busy"
    @click="$emit('change')"
  >
    <span class="status-switch-track" aria-hidden="true"><span class="status-switch-thumb" /></span>
    <span>{{ t(value === 'ACTIVE' ? 'state.ACTIVE' : 'state.DISABLED') }}</span>
  </button>
</template>
