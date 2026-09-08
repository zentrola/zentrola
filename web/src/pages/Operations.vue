<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useCollection, useListSearch, date } from '../composables'
import { i18n, t } from '../i18n'
import type { Operation } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import OperationDiff from '../components/OperationDiff.vue'
const { items, cursor, loading, error, load } = useCollection<Operation>(() => '/operation-logs')
const selected = ref<Operation | null>(null)
function operationLabel(value: string) {
  const key = `operations.types.${value}`
  return i18n.global.te(key) ? t(key) : value
}
function targetLabel(value: string) {
  const key = `operations.targets.${value}`
  return i18n.global.te(key) ? t(key) : value
}
function errorLabel(value: string) {
  const key = `errors.${value}`
  return i18n.global.te(key) ? t(key) : value
}
function showSnapshot(row: Operation) {
  return row.type !== 'LOGIN_SUCCESS' && row.type !== 'LOGIN_FAILED'
}
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (row) =>
    `${row.operatorName} ${row.type} ${operationLabel(row.type)} ${row.targetType} ${targetLabel(row.targetType)} ${row.targetId} ${row.requestId || ''} ${row.result} ${t(`state.${row.result}`)}`,
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
            <td>{{ operationLabel(row.type) }}</td>
            <td class="operation-target">
              {{ targetLabel(row.targetType)
              }}<span class="operation-target-id">{{
                row.targetId ?? t('operations.emptyValue')
              }}</span>
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
  <Modal
    v-if="selected"
    :title="t('operations.logDetails')"
    :wide="showSnapshot(selected)"
    @close="selected = null"
  >
    <OperationDiff
      v-if="showSnapshot(selected)"
      :before="selected.before"
      :after="selected.after"
    />
    <dl
      v-if="selected.errorCode || selected.requestId"
      class="operation-trace"
      :class="{ 'operation-trace-only': !showSnapshot(selected) }"
    >
      <template v-if="selected.errorCode">
        <dt>{{ t('usage.errorType') }}</dt>
        <dd>{{ errorLabel(selected.errorCode) }}</dd>
      </template>
      <template v-if="selected.requestId">
        <dt>{{ t('common.requestId') }}</dt>
        <dd>{{ selected.requestId }}</dd>
      </template>
    </dl>
  </Modal>
</template>

<style scoped>
.operation-trace {
  display: grid;
  grid-template-columns: 110px minmax(0, 1fr);
  gap: 10px 16px;
  margin: 20px 0 0;
  padding-top: 18px;
  border-top: 1px solid var(--line);
  font-size: 12px;
}
.operation-trace dt {
  color: var(--muted);
}
.operation-trace dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.operation-trace-only {
  margin-top: 0;
  padding-top: 0;
  border-top: 0;
}
</style>
