<script setup lang="ts">
import { computed } from 'vue'
import { t } from '../i18n'
const props = defineProps<{
  cursor: string | null
  page: number
  pageSize: number
  total: number
  loading: boolean
}>()
const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const emit = defineEmits<{ first: []; previous: []; more: []; pageSize: [value: number] }>()
function changePageSize(event: Event) {
  emit('pageSize', Number((event.target as HTMLSelectElement).value))
}
</script>
<template>
  <div class="list-footer">
    <nav class="pagination-controls" :aria-label="t('common.pagination')">
      <span class="pagination-position" aria-live="polite">
        {{ t('common.pageNumber', { page, totalPages }) }}
      </span>
      <span class="pagination-total">{{ t('common.totalRecords', { total }) }}</span>
      <span class="page-size-control">
        <span>{{ t('common.perPageDisplay') }}</span>
        <select
          class="page-size-select"
          :value="pageSize"
          :disabled="loading"
          :aria-label="t('common.perPage')"
          @change="changePageSize"
        >
          <option v-for="size in [20, 50, 100]" :key="size" :value="size">{{ size }}</option>
        </select>
        <span>{{ t('common.recordUnit') }}</span>
      </span>
      <span class="pagination-divider" aria-hidden="true"></span>
      <span class="pagination-buttons">
        <button class="pagination-button" :disabled="loading || page <= 1" @click="$emit('first')">
          {{ t('common.firstPage') }}
        </button>
        <button
          class="pagination-button"
          :disabled="loading || page <= 1"
          @click="$emit('previous')"
        >
          {{ t('common.previousPage') }}
        </button>
        <button
          class="pagination-button"
          :disabled="loading || page >= totalPages || !cursor"
          @click="$emit('more')"
        >
          {{ t('common.nextPage') }}
        </button>
      </span>
    </nav>
  </div>
</template>
