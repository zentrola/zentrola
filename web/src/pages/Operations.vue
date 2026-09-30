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
import TableScroll from '../components/TableScroll.vue'
const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<Operation>(() => '/operation-logs')
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
const { keyword, query, visible, search, reset, searching, searchingAll } = useListSearch(
  items,
  (row) =>
    `${row.operatorName} ${row.type} ${operationLabel(row.type)} ${row.targetType} ${targetLabel(row.targetType)} ${row.targetName || ''} ${row.targetId} ${row.requestId || ''} ${row.result} ${t(`state.${row.result}`)}`,
  () => '/operation-logs',
  loading,
)
onMounted(() => load())
</script>
<template>
  <PageHeader name="operations" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading || searching"
      :placeholder="t('operations.searchPlaceholder')"
      @search="search"
      @reset="reset"
    />
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </p>
    <TableScroll has-actions>
      <table class="operations-table">
        <colgroup>
          <col class="operation-time-column" />
          <col class="operation-operator-column" />
          <col class="operation-type-column" />
          <col class="operation-target-column" />
          <col class="operation-status-column" />
          <col class="operation-action-column" />
        </colgroup>
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
            <td class="table-time">{{ date(row.createdAt) }}</td>
            <td>{{ row.operatorName }}</td>
            <td>{{ operationLabel(row.type) }}</td>
            <td>
              <div class="operation-target">
                <span class="operation-target-name">{{
                  row.targetName || targetLabel(row.targetType)
                }}</span>
                <span v-if="row.targetId" class="operation-target-id">
                  {{ targetLabel(row.targetType) }} · ID {{ row.targetId }}
                </span>
              </div>
            </td>
            <td><Status :value="row.result" /></td>
            <td class="align-right">
              <button class="text-button" @click="selected = row">{{ t('common.details') }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="operations" :size="32" />
      <p>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'operations.empty') }}</p>
      <button
        v-if="!loading"
        type="button"
        class="button primary empty-state-action"
        @click="query ? reset() : load()"
      >
        <Icon name="refresh" :size="16" />{{ t(query ? 'common.reset' : 'common.retry') }}
      </button>
    </div>
    <ListFooter
      v-if="!searchingAll"
      :cursor="cursor"
      :page="page"
      :page-size="pageSize"
      :total="total"
      :loading="loading"
      @first="load()"
      @previous="previous"
      @more="load(true)"
      @page-size="setPageSize"
    />
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
        <dd>
          <code>{{ errorLabel(selected.errorCode) }}</code>
        </dd>
      </template>
      <template v-if="selected.requestId">
        <dt>{{ t('common.requestId') }}</dt>
        <dd>
          <code>{{ selected.requestId }}</code>
        </dd>
      </template>
    </dl>
  </Modal>
</template>

<style scoped>
.operations-table {
  min-width: 860px;
  table-layout: fixed;
}
.operation-time-column {
  width: 164px;
}
.operation-operator-column {
  width: 142px;
}
.operation-type-column {
  width: 172px;
}
.operation-target-column {
  width: auto;
}
.operation-status-column {
  width: 112px;
}
.operation-action-column {
  width: 92px;
}
.operation-operator-column,
.operations-table td:nth-child(2),
.operations-table td:nth-child(3) {
  overflow: hidden;
  text-overflow: ellipsis;
}
.operation-target-name {
  overflow: hidden;
  color: var(--color-text);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.operation-target {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.operation-target-id {
  overflow: hidden;
  color: var(--color-text-muted);
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
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
