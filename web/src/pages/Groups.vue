<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, all } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t } from '../i18n'
import { showErrorToast } from '../toast'
import type { Group, Model } from '../types'
import Icon from '../components/Icon.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import TableScroll from '../components/TableScroll.vue'
const {
  items,
  cursor,
  page,
  pageSize,
  total,
  loading,
  error,
  load,
  refresh,
  previous,
  retry,
  setPageSize,
} = useCollection<Group>(() => '/groups')
const { busy, error: actionError, run } = useAction()
const creating = ref(false),
  name = ref(''),
  remark = ref(''),
  creationModels = ref<Model[]>([]),
  createModelIDs = ref<string[]>([]),
  createModelsReady = ref(false),
  createModelQuery = ref(''),
  createOnlySelected = ref(false)
const selected = ref<Group | null>(null),
  editName = ref(''),
  editRemark = ref(''),
  editModelIDs = ref<string[]>([]),
  grantedModels = ref<Model[]>([]),
  modelCandidates = ref<Model[]>([]),
  relationReady = ref(false),
  editModelQuery = ref(''),
  editOnlySelected = ref(false),
  statusTarget = ref<Group | null>(null),
  deleteTarget = ref<Group | null>(null)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (g) => `${g.name} ${g.id} ${g.remark || ''}`,
  load,
)
const grantedModelIDs = computed(() => new Set(grantedModels.value.map((model) => model.id)))
const filteredCreationModels = computed(() =>
  creationModels.value.filter(
    (model) =>
      matchesModel(model, createModelQuery.value) &&
      (!createOnlySelected.value || createModelIDs.value.includes(model.id)),
  ),
)
const filteredModelCandidates = computed(() =>
  modelCandidates.value.filter(
    (model) =>
      matchesModel(model, editModelQuery.value) &&
      (!editOnlySelected.value || editModelIDs.value.includes(model.id)),
  ),
)
const createModelFilterActive = computed(
  () => Boolean(createModelQuery.value.trim()) || createOnlySelected.value,
)
const editModelFilterActive = computed(
  () => Boolean(editModelQuery.value.trim()) || editOnlySelected.value,
)
onMounted(() => load())
function matchesModel(model: Model, keyword: string) {
  const normalized = keyword.trim().toLocaleLowerCase()
  if (!normalized) return true
  return [model.name, model.code, model.publisherProviderName || ''].some((value) =>
    value.toLocaleLowerCase().includes(normalized),
  )
}
function resetCreateModelFilter() {
  createModelQuery.value = ''
  createOnlySelected.value = false
}
function resetEditModelFilter() {
  editModelQuery.value = ''
  editOnlySelected.value = false
}
function newGroup() {
  name.value = ''
  remark.value = ''
  creationModels.value = []
  createModelIDs.value = []
  createModelsReady.value = false
  resetCreateModelFilter()
  actionError.value = ''
  creating.value = false
  void run(async () => {
    await loadCreateModels()
    if (!creationModels.value.length) {
      showErrorToast(t('groups.addModelFirst'))
      return
    }
    creating.value = true
  })
}
async function loadCreateModels() {
  createModelsReady.value = false
  creationModels.value = await all<Model>('/models?status=ACTIVE')
  createModelsReady.value = true
}
function create() {
  if (!name.value) {
    showErrorToast(t('groups.nameRequired'))
    return
  }
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  if (!createModelIDs.value.length) {
    showErrorToast(t('groups.modelRequired'))
    return
  }
  void run(async () => {
    await api('/groups', 'POST', {
      name: name.value,
      ...(remark.value ? { remark: remark.value } : {}),
      modelIds: createModelIDs.value,
    })
    creating.value = false
    await load()
  })
}
async function refreshModels() {
  relationReady.value = false
  const [current, available] = await Promise.all([
    all<Model>(`/groups/${selected.value!.id}/models`),
    all<Model>('/models?status=ACTIVE'),
  ])
  grantedModels.value = current
  modelCandidates.value = available
  const activeIDs = new Set(modelCandidates.value.map((model) => model.id))
  editModelIDs.value = current.filter((model) => activeIDs.has(model.id)).map((model) => model.id)
  relationReady.value = true
}
function manage(group: Group) {
  selected.value = group
  editName.value = group.name
  editRemark.value = group.remark || ''
  editModelIDs.value = []
  grantedModels.value = []
  modelCandidates.value = []
  resetEditModelFilter()
  actionError.value = ''
  void run(refreshModels)
}
function saveEdit() {
  if (!selected.value) return
  if (!validText(editName.value, 128) || !validText(editRemark.value, 2000, false)) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  const groupID = selected.value.id
  void run(async () => {
    await api(`/groups/${groupID}`, 'PUT', {
      name: editName.value,
      remark: editRemark.value,
      modelIds: editModelIDs.value,
    })
    selected.value = null
    await load()
  })
}
function openDelete(group: Group) {
  actionError.value = ''
  deleteTarget.value = group
}
function changeStatus(group: Group) {
  actionError.value = ''
  statusTarget.value = group
  void run(async () => {
    try {
      await api(`/groups/${group.id}/status`, 'PATCH', {
        status: group.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
      })
      await refresh()
    } finally {
      statusTarget.value = null
    }
  })
}
function deleteGroup() {
  if (!deleteTarget.value) return
  const groupID = deleteTarget.value.id
  void run(async () => {
    await api(`/groups/${groupID}`, 'DELETE')
    deleteTarget.value = null
    await load()
  })
}
</script>
<template>
  <PageHeader name="groups" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :label="t('groups.searchLabel')"
      :placeholder="t('groups.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #actions>
        <div class="list-toolbar-actions">
          <button type="button" class="button primary" @click="newGroup">
            <Icon name="plus" :size="18" />{{ t('groups.create') }}
          </button>
        </div>
      </template>
    </ListSearch>
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </p>
    <TableScroll has-actions>
      <table>
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('common.enableStatus') }}</th>
            <th>{{ t('common.created') }}</th>
            <th>{{ t('common.remark') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="group in visible" :key="group.id">
            <td>
              <div class="person">
                <span class="avatar">{{ group.name.slice(0, 1) }}</span>
                <strong>{{ group.name }}</strong>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="group.status"
                :name="group.name"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === group.id"
                @change="changeStatus(group)"
              />
            </td>
            <td>{{ date(group.createdAt) }}</td>
            <td class="remark-cell" :title="group.remark || ''">
              {{ group.remark || t('common.none') }}
            </td>
            <td>
              <div class="row-actions">
                <button class="text-button" @click="manage(group)">
                  {{ t('groups.models') }}
                </button>
                <button class="text-button danger" @click="openDelete(group)">
                  {{ t('groups.delete') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="groups" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'common.empty') }}</h3>
      <p v-if="!loading && !query">{{ t('groups.empty') }}</p>
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
  <Modal v-if="creating" :title="t('groups.create')" :busy="busy" medium @close="creating = false"
    ><form class="group-form" @submit.prevent="create">
      <div class="group-form-fields">
        <div class="group-form-row">
          <label class="group-form-label required-label" for="create-group-name-input">{{
            t('common.name')
          }}</label>
          <div class="group-form-control">
            <input
              id="create-group-name-input"
              v-model="name"
              required
              :disabled="busy"
              autofocus
            />
          </div>
        </div>
        <div class="group-form-row">
          <div id="create-models-title" class="group-form-label group-model-label required-label">
            {{ t('groups.allowedModels') }}
          </div>
          <section
            class="group-form-control group-model-field"
            aria-labelledby="create-models-title"
            aria-required="true"
          >
            <div v-if="createModelsReady" class="group-model-toolbar">
              <div class="model-search-box">
                <input
                  id="create-model-search"
                  v-model="createModelQuery"
                  type="search"
                  :aria-label="t('groups.modelSearch')"
                  :placeholder="t('groups.modelSearchPlaceholder')"
                  :disabled="busy"
                />
                <button
                  v-if="createModelQuery"
                  type="button"
                  class="model-search-clear"
                  :aria-label="t('groups.clearModelSearch')"
                  :disabled="busy"
                  @click="createModelQuery = ''"
                >
                  <Icon name="close" :size="14" />
                </button>
              </div>
              <button
                type="button"
                class="model-selected-filter"
                :class="{ active: createOnlySelected }"
                :aria-pressed="createOnlySelected"
                :disabled="busy"
                @click="createOnlySelected = !createOnlySelected"
              >
                <span class="model-filter-indicator"><Icon name="check" :size="12" /></span>
                {{ t('groups.onlySelected') }}
              </button>
            </div>
            <div v-if="createModelsReady" class="group-model-field-head">
              <span v-if="createModelFilterActive">{{
                t('groups.filteredModelCount', { count: filteredCreationModels.length })
              }}</span>
              <span class="model-selection-summary">{{
                t('groups.selectionCount', {
                  count: createModelIDs.length,
                  total: creationModels.length,
                })
              }}</span>
            </div>
            <div class="create-models">
              <TableScroll
                v-if="createModelsReady && filteredCreationModels.length"
                class="create-model-list"
              >
                <table>
                  <colgroup>
                    <col class="model-check-column" />
                    <col class="model-name-column" />
                    <col class="model-type-column" />
                    <col class="model-type-column" />
                  </colgroup>
                  <thead>
                    <tr>
                      <th class="model-check-cell">{{ t('common.select') }}</th>
                      <th>{{ t('common.name') }}</th>
                      <th class="model-type-cell">{{ t('models.input') }}</th>
                      <th class="model-type-cell">{{ t('models.output') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="model in filteredCreationModels" :key="model.id">
                      <td class="model-check-cell">
                        <input
                          v-model="createModelIDs"
                          class="model-checkbox"
                          type="checkbox"
                          :value="model.id"
                          :aria-label="t('groups.modelSelection', { name: model.name })"
                          :disabled="busy || model.status !== 'ACTIVE'"
                        />
                      </td>
                      <td>
                        <strong class="model-name-regular">{{ model.name }}</strong>
                      </td>
                      <td class="model-type-cell">
                        <div class="modality-tags">
                          <span
                            v-for="value in model.inputModalities"
                            :key="value"
                            class="modality-tag"
                            >{{ t(`models.${value}`) }}</span
                          >
                        </div>
                      </td>
                      <td class="model-type-cell">
                        <div class="modality-tags">
                          <span
                            v-for="value in model.outputModalities"
                            :key="value"
                            class="modality-tag"
                            >{{ t(`models.${value}`) }}</span
                          >
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </TableScroll>
              <p v-else-if="busy && !createModelsReady" class="empty-compact">
                {{ t('common.loading') }}
              </p>
              <p v-else-if="createModelsReady && !creationModels.length" class="empty-compact">
                {{ t('groups.noModelCatalog') }}
              </p>
              <p v-else-if="createModelsReady" class="empty-compact model-filter-empty">
                {{ t('groups.noMatchingModels') }}
                <button type="button" class="text-button" @click="resetCreateModelFilter">
                  {{ t('groups.clearModelFilters') }}
                </button>
              </p>
            </div>
          </section>
        </div>
        <div class="group-form-row">
          <label class="group-form-label" for="create-group-remark-input">{{
            t('common.remark')
          }}</label>
          <div class="group-form-control">
            <textarea
              id="create-group-remark-input"
              v-model="remark"
              rows="3"
              :disabled="busy"
            ></textarea>
          </div>
        </div>
      </div>
      <p v-if="actionError && !createModelsReady" class="form-retry">
        <button type="button" class="text-button" :disabled="busy" @click="run(loadCreateModels)">
          {{ t('common.retry') }}
        </button>
      </p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="creating = false">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy || !createModelsReady">
          {{ t(busy ? 'common.working' : 'common.create') }}
        </button>
      </footer>
    </form></Modal
  >
  <Modal
    v-if="selected"
    :title="t('groups.editTitle', { name: selected.name })"
    :busy="busy"
    medium
    @close="selected = null"
  >
    <form class="group-form" @submit.prevent="saveEdit">
      <div class="group-form-fields">
        <div class="group-form-row">
          <label class="group-form-label required-label" for="edit-group-name-input">{{
            t('common.name')
          }}</label>
          <div class="group-form-control">
            <input
              id="edit-group-name-input"
              v-model="editName"
              required
              :disabled="busy"
              autofocus
            />
          </div>
        </div>
        <div class="group-form-row">
          <div id="edit-models-title" class="group-form-label group-model-label">
            {{ t('groups.allowedModels') }}
          </div>
          <section class="group-form-control group-model-field" aria-labelledby="edit-models-title">
            <div v-if="relationReady" class="group-model-toolbar">
              <div class="model-search-box">
                <input
                  id="edit-model-search"
                  v-model="editModelQuery"
                  type="search"
                  :aria-label="t('groups.modelSearch')"
                  :placeholder="t('groups.modelSearchPlaceholder')"
                  :disabled="busy"
                />
                <button
                  v-if="editModelQuery"
                  type="button"
                  class="model-search-clear"
                  :aria-label="t('groups.clearModelSearch')"
                  :disabled="busy"
                  @click="editModelQuery = ''"
                >
                  <Icon name="close" :size="14" />
                </button>
              </div>
              <button
                type="button"
                class="model-selected-filter"
                :class="{ active: editOnlySelected }"
                :aria-pressed="editOnlySelected"
                :disabled="busy"
                @click="editOnlySelected = !editOnlySelected"
              >
                <span class="model-filter-indicator"><Icon name="check" :size="12" /></span>
                {{ t('groups.onlySelected') }}
              </button>
            </div>
            <div v-if="relationReady" class="group-model-field-head">
              <span v-if="editModelFilterActive">{{
                t('groups.filteredModelCount', { count: filteredModelCandidates.length })
              }}</span>
              <span class="model-selection-summary">{{
                t('groups.selectionCount', {
                  count: editModelIDs.length,
                  total: modelCandidates.length,
                })
              }}</span>
            </div>
            <div class="create-models">
              <TableScroll
                v-if="relationReady && filteredModelCandidates.length"
                class="create-model-list"
              >
                <table>
                  <colgroup>
                    <col class="model-check-column" />
                    <col class="model-name-column" />
                    <col class="model-type-column" />
                    <col class="model-type-column" />
                  </colgroup>
                  <thead>
                    <tr>
                      <th class="model-check-cell">{{ t('common.select') }}</th>
                      <th>{{ t('common.name') }}</th>
                      <th class="model-type-cell">{{ t('models.input') }}</th>
                      <th class="model-type-cell">{{ t('models.output') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="model in filteredModelCandidates" :key="model.id">
                      <td class="model-check-cell">
                        <input
                          v-model="editModelIDs"
                          class="model-checkbox"
                          type="checkbox"
                          :value="model.id"
                          :aria-label="t('groups.modelSelection', { name: model.name })"
                          :disabled="
                            busy ||
                            !relationReady ||
                            (!grantedModelIDs.has(model.id) &&
                              (model.status !== 'ACTIVE' || selected.status !== 'ACTIVE'))
                          "
                        />
                      </td>
                      <td>
                        <strong class="model-name-regular">{{ model.name }}</strong>
                      </td>
                      <td class="model-type-cell">
                        <div class="modality-tags">
                          <span
                            v-for="value in model.inputModalities"
                            :key="value"
                            class="modality-tag"
                            >{{ t(`models.${value}`) }}</span
                          >
                        </div>
                      </td>
                      <td class="model-type-cell">
                        <div class="modality-tags">
                          <span
                            v-for="value in model.outputModalities"
                            :key="value"
                            class="modality-tag"
                            >{{ t(`models.${value}`) }}</span
                          >
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </TableScroll>
              <p v-else-if="busy && !relationReady" class="empty-compact">
                {{ t('common.loading') }}
              </p>
              <p v-else-if="relationReady && !modelCandidates.length" class="empty-compact">
                {{ t('groups.noModelCatalog') }}
              </p>
              <p v-else-if="relationReady" class="empty-compact model-filter-empty">
                {{ t('groups.noMatchingModels') }}
                <button type="button" class="text-button" @click="resetEditModelFilter">
                  {{ t('groups.clearModelFilters') }}
                </button>
              </p>
            </div>
          </section>
        </div>
        <div class="group-form-row">
          <label class="group-form-label" for="edit-group-remark-input">{{
            t('common.remark')
          }}</label>
          <div class="group-form-control">
            <textarea
              id="edit-group-remark-input"
              v-model="editRemark"
              rows="3"
              :disabled="busy"
            ></textarea>
          </div>
        </div>
      </div>
      <p v-if="actionError && !relationReady" class="form-retry">
        <button type="button" class="text-button" :disabled="busy" @click="run(refreshModels)">
          {{ t('common.retry') }}
        </button>
      </p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="selected = null">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy || !relationReady">
          {{ t(busy ? 'common.working' : 'common.save') }}
        </button>
      </footer>
    </form>
  </Modal>
  <ConfirmDialog
    v-if="deleteTarget"
    :title="t('groups.deleteTitle')"
    :message="t('groups.deleteQuestion', { name: deleteTarget.name })"
    :hint="t('groups.deleteConsequence')"
    :confirm-label="t('groups.delete')"
    :busy="busy"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteGroup"
  />
</template>
<style scoped>
.group-form-fields {
  display: grid;
  gap: 16px;
}
.group-form-row {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.group-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.group-form-control {
  min-width: 0;
}
.group-model-label {
  padding-top: 2px;
}
.required-label::after {
  content: '*';
  margin-left: 4px;
  color: var(--danger);
}
.group-model-field-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 6px;
  padding: 0 2px;
}
.group-model-field-head span {
  color: var(--muted);
  font-size: 12px;
  font-weight: 400;
}
.model-selection-summary {
  margin-left: auto;
}
.group-model-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.model-search-box {
  position: relative;
  flex: 1 1 260px;
  min-width: 0;
  color: #92a0af;
}
.model-search-box input[type='search'] {
  min-height: 36px;
  padding: 7px 36px 7px 11px;
  font-size: 12px;
  background: #f8fafc;
}
.model-search-box input[type='search']::-webkit-search-cancel-button {
  display: none;
}
.model-search-clear {
  position: absolute;
  top: 5px;
  right: 5px;
  display: grid;
  width: 26px;
  height: 26px;
  padding: 0;
  place-items: center;
  color: var(--muted);
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
}
.model-search-clear:hover {
  color: var(--color-text);
  background: #e9eff6;
}
.model-selected-filter {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 7px;
  min-height: 36px;
  padding: 7px 11px;
  color: var(--color-text-secondary);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  font-size: 12px;
  cursor: pointer;
}
.model-selected-filter:hover {
  border-color: #b8c6d5;
  background: #f8fafc;
}
.model-selected-filter.active {
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border-color: #bfdbfe;
}
.model-filter-indicator {
  display: grid;
  width: 16px;
  height: 16px;
  place-items: center;
  color: transparent;
  background: var(--color-surface);
  border: 1px solid #b8c6d5;
  border-radius: 4px;
}
.model-selected-filter.active .model-filter-indicator {
  color: #fff;
  background: var(--color-primary);
  border-color: var(--color-primary);
}
.create-models {
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: hidden;
}
.create-model-list {
  max-height: min(38vh, 320px);
  overflow: auto;
}
.create-model-list table {
  table-layout: fixed;
  margin: 0;
  min-width: 420px;
}
.create-model-list th {
  position: sticky;
  top: 0;
  z-index: 3;
  padding: 8px 10px;
}
.create-model-list td {
  padding: 9px 10px;
}
.model-check-column {
  width: 58px;
}
.model-name-column {
  width: auto;
}
.model-type-column {
  width: 22%;
}
.model-type-cell {
  text-align: center;
}
.model-type-cell .modality-tags {
  justify-content: center;
}
.modality-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.modality-tag {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 6px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  font-size: 11px;
  white-space: nowrap;
}
.model-check-cell {
  text-align: center;
}
.model-checkbox {
  width: 18px;
  height: 18px;
  padding: 0;
  margin: 0;
  vertical-align: middle;
  accent-color: var(--blue);
  cursor: pointer;
}
.model-checkbox:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
.model-filter-empty .text-button {
  margin-left: 6px;
}
.model-selection-count {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--muted);
}
@media (max-width: 640px) {
  .group-form-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .group-form-label,
  .group-model-label {
    padding-top: 0;
    text-align: left;
  }
  .group-model-toolbar {
    align-items: stretch;
    flex-direction: column;
  }
  .model-search-box {
    flex-basis: auto;
  }
  .model-selected-filter {
    justify-content: center;
  }
}
</style>
