<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useCollection, date } from '../composables'
import { codeText, t } from '../i18n'
import enUS from '../locales/en-US'
import type { Operation } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import OperationDiff from '../components/OperationDiff.vue'
import TableScroll from '../components/TableScroll.vue'

type OperationFilters = { keyword: string; type: string; result: string }
// 关键词最长 100 字符，与后端校验保持一致。
const keywordMaxLength = 100
const operationTypes = Object.keys(enUS.operations.types)
const operationResults = ['SUCCESS', 'FAILED'] as const
const emptyFilters = (): OperationFilters => ({ keyword: '', type: '', result: '' })
const route = useRoute()
const keyword = ref(''),
  typeFilter = ref(''),
  resultFilter = ref(''),
  applied = ref<OperationFilters>(emptyFilters())
const filtered = computed(() =>
  Boolean(applied.value.keyword || applied.value.type || applied.value.result),
)
// 筛选交给服务端执行，列表仍按游标分页，避免把整张审计表拉到浏览器。
const operationPath = () => {
  const base =
    typeof route.query.applicationId === 'string' && /^\d+$/.test(route.query.applicationId)
      ? `/applications/${route.query.applicationId}/operations`
      : '/operation-logs'
  const params = new URLSearchParams()
  if (applied.value.keyword) params.set('keyword', applied.value.keyword)
  if (applied.value.type) params.set('type', applied.value.type)
  if (applied.value.result) params.set('result', applied.value.result)
  const query = params.toString()
  return query ? `${base}?${query}` : base
}
const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<Operation>(operationPath)
const selected = ref<Operation | null>(null)
const operationLabel = (value: string) => codeText('operations.types', value)
const targetLabel = (value: string) => codeText('operations.targets', value)
const errorLabel = (value: string) => codeText('errors', value)
function showSnapshot(row: Operation) {
  return row.type !== 'LOGIN_SUCCESS' && row.type !== 'LOGIN_FAILED'
}
function search() {
  applied.value = {
    keyword: keyword.value.trim().slice(0, keywordMaxLength),
    type: typeFilter.value,
    result: resultFilter.value,
  }
  void load()
}
function reset() {
  keyword.value = ''
  typeFilter.value = ''
  resultFilter.value = ''
  applied.value = emptyFilters()
  void load()
}
onMounted(() => load())
watch(
  () => route.query.applicationId,
  () => void load(),
)
</script>
<template>
  <PageHeader name="operations" />
  <p v-if="route.query.applicationId" class="context-note">
    {{ t('operations.applicationFilter') }}
    <RouterLink to="/operations" class="text-button">{{ t('operations.showAll') }}</RouterLink>
  </p>
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :maxlength="keywordMaxLength"
      :placeholder="t('operations.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #filters>
        <label class="operation-filter">
          <span>{{ t('operations.type') }}</span>
          <select v-model="typeFilter" :aria-label="t('operations.type')" :disabled="loading">
            <option value="">{{ t('common.all') }}</option>
            <option v-for="value in operationTypes" :key="value" :value="value">
              {{ operationLabel(value) }}
            </option>
          </select>
        </label>
        <label class="operation-filter">
          <span>{{ t('operations.result') }}</span>
          <select v-model="resultFilter" :aria-label="t('operations.result')" :disabled="loading">
            <option value="">{{ t('common.all') }}</option>
            <option v-for="value in operationResults" :key="value" :value="value">
              {{ t(`state.${value}`) }}
            </option>
          </select>
        </label>
      </template>
    </ListSearch>
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
          <tr v-for="row in items" :key="row.id">
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
    <div v-if="!items.length" class="empty-state">
      <Icon name="operations" :size="32" />
      <p>
        {{ t(loading ? 'common.loading' : filtered ? 'common.noResults' : 'operations.empty') }}
      </p>
      <button
        v-if="!loading"
        type="button"
        class="button primary empty-state-action"
        @click="filtered ? reset() : load()"
      >
        <Icon name="refresh" :size="16" />{{ t(filtered ? 'common.reset' : 'common.retry') }}
      </button>
    </div>
    <ListFooter
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
.operation-filter {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--color-text-secondary);
  font-weight: 500;
  white-space: nowrap;
}
.operation-filter select {
  width: 160px;
  padding-top: 9px;
  padding-bottom: 9px;
  background: var(--color-surface);
  border-color: var(--color-border);
  font-size: 12px;
}
@media (max-width: 760px) {
  .operation-filter {
    width: 100%;
  }
  .operation-filter select {
    flex: 1;
    width: auto;
  }
}
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
