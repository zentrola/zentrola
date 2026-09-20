<script setup lang="ts">
import { useId } from 'vue'
import { t } from '../i18n'

const keyword = defineModel<string>({ required: true })
defineProps<{ loading?: boolean; placeholder?: string; label?: string }>()
defineEmits<{ search: []; reset: [] }>()
const inputID = useId()
</script>

<template>
  <form class="table-toolbar list-search" role="search" @submit.prevent="$emit('search')">
    <label v-if="label" class="list-search-label" :for="inputID">{{ label }}</label>
    <div class="search-box">
      <input
        :id="label ? inputID : undefined"
        v-model="keyword"
        type="search"
        :aria-label="label || placeholder || t('common.search')"
        :placeholder="placeholder || t('common.search')"
        :disabled="loading"
      />
    </div>
    <slot name="filters" />
    <div class="list-search-actions">
      <button class="button primary" :disabled="loading">
        {{ t('common.searchAction') }}
      </button>
      <button type="button" class="button" :disabled="loading" @click="$emit('reset')">
        {{ t('common.reset') }}
      </button>
    </div>
    <slot name="actions" />
  </form>
</template>
