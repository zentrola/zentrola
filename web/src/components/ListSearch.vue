<script setup lang="ts">
import { t } from '../i18n'
import Icon from './Icon.vue'

const keyword = defineModel<string>({ required: true })
defineProps<{ loading?: boolean }>()
defineEmits<{ search: []; reset: [] }>()
</script>

<template>
  <form class="table-toolbar list-search" role="search" @submit.prevent="$emit('search')">
    <div class="search-box">
      <Icon name="search" :size="18" />
      <input
        v-model="keyword"
        type="search"
        :aria-label="t('common.search')"
        :placeholder="t('common.search')"
        :disabled="loading"
      />
    </div>
    <div class="list-search-actions">
      <button class="button primary" :disabled="loading">
        <Icon name="search" :size="16" />{{ t('common.searchAction') }}
      </button>
      <button type="button" class="button" :disabled="loading" @click="$emit('reset')">
        {{ t('common.reset') }}
      </button>
    </div>
  </form>
</template>
