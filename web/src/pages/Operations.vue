<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useCollection, useListSearch, date } from '../composables'
import { t } from '../i18n'
import type { Operation } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
const { items, cursor, loading, error, load } = useCollection<Operation>(() => '/operation-logs')
const selected = ref<Operation | null>(null)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (row) =>
    `${row.operatorName} ${row.type} ${row.targetType} ${row.targetId} ${row.requestId || ''} ${row.result} ${t(`state.${row.result}`)}`,
  load,
)
onMounted(() => load())
</script>
<template>
  <PageHeader name="operations" />
  <section class="panel">
    <ListSearch v-model="keyword" :loading="loading" @search="search" @reset="reset" />
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="load()">{{ t('common.retry') }}</button>
    </p>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>{{ t('common.created') }}</th>
            <th>{{ t('operations.operator') }}</th>
            <th>{{ t('operations.type') }}</th>
            <th>{{ t('operations.target') }}</th>
            <th>{{ t('operations.result') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in visible" :key="row.id">
            <td>{{ date(row.createdAt) }}</td>
            <td>{{ row.operatorName }}</td>
            <td>
              <code>{{ row.type }}</code>
            </td>
            <td>
              {{ row.targetType }}<small class="subline">{{ row.targetId }}</small>
            </td>
            <td><Status :value="row.result" /></td>
            <td class="align-right">
              <button class="text-button" @click="selected = row">{{ t('common.details') }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="operations" :size="32" />
      <p>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'operations.empty') }}</p>
    </div>
    <ListFooter :count="items.length" :cursor="cursor" :loading="loading" @more="load(true)" />
  </section>
  <Modal v-if="selected" :title="t('common.details')" @close="selected = null"
    ><dl class="detail-grid">
      <dt>{{ t('common.id') }}</dt>
      <dd>{{ selected.id }}</dd>
      <dt>{{ t('operations.type') }}</dt>
      <dd>{{ selected.type }}</dd>
      <dt>{{ t('operations.operator') }}</dt>
      <dd>{{ selected.operatorName }}</dd>
      <dt>{{ t('operations.result') }}</dt>
      <dd><Status :value="selected.result" /></dd>
      <dt>{{ t('usage.errorType') }}</dt>
      <dd>{{ selected.errorCode || t('common.none') }}</dd>
      <dt>{{ t('common.requestId') }}</dt>
      <dd>{{ selected.requestId || t('common.none') }}</dd>
    </dl></Modal
  >
</template>
