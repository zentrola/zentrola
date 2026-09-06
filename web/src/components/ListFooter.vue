<script setup lang="ts">
import { t } from '../i18n'
defineProps<{ count: number; cursor: string | null; loading: boolean }>()
defineEmits<{ more: [] }>()
</script>
<template>
  <div class="list-footer">
    <span>{{ t('common.loaded', { count }) }}</span
    ><button v-if="cursor" class="button subtle" :disabled="loading" @click="$emit('more')">
      {{ t(loading ? 'common.loading' : 'common.more') }}</button
    ><span v-else-if="count">{{ t('common.end') }}</span>
  </div>
</template>
