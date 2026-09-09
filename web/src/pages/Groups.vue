<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, all } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t } from '../i18n'
import type { Group, Model } from '../types'
import Icon from '../components/Icon.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
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
  validation = ref(''),
  creationModels = ref<Model[]>([]),
  createModelIDs = ref<string[]>([]),
  createModelsReady = ref(false)
const selected = ref<Group | null>(null),
  editName = ref(''),
  editRemark = ref(''),
  editModelIDs = ref<string[]>([]),
  grantedModels = ref<Model[]>([]),
  modelCandidates = ref<Model[]>([]),
  relationReady = ref(false),
  statusTarget = ref<Group | null>(null),
  deleteTarget = ref<Group | null>(null)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (g) => `${g.name} ${g.id} ${g.remark || ''}`,
  load,
)
const grantedModelIDs = computed(() => new Set(grantedModels.value.map((model) => model.id)))
onMounted(() => load())
function newGroup() {
  name.value = ''
  remark.value = ''
  validation.value = ''
  creationModels.value = []
  createModelIDs.value = []
  createModelsReady.value = false
  actionError.value = ''
  creating.value = true
  void run(loadCreateModels)
}
async function loadCreateModels() {
  createModelsReady.value = false
  creationModels.value = await all<Model>('/models?status=ACTIVE')
  createModelsReady.value = true
}
function create() {
  validation.value = ''
  if (!name.value) {
    validation.value = t('groups.nameRequired')
    return
  }
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    validation.value = t('common.byteLimit')
    return
  }
  if (!createModelIDs.value.length) {
    validation.value = t('groups.modelRequired')
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
  validation.value = ''
  actionError.value = ''
  void run(refreshModels)
}
function saveEdit() {
  validation.value = ''
  if (!selected.value) return
  if (!validText(editName.value, 128) || !validText(editRemark.value, 2000, false)) {
    validation.value = t('common.byteLimit')
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
function openStatus(group: Group) {
  actionError.value = ''
  statusTarget.value = group
}
function changeStatus() {
  void run(async () => {
    const group = statusTarget.value!
    await api(`/groups/${group.id}/status`, 'PATCH', {
      status: group.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
    })
    statusTarget.value = null
    await refresh()
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
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('common.status') }}</th>
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
                @change="openStatus(group)"
              />
            </td>
            <td>{{ date(group.createdAt) }}</td>
            <td class="remark-cell" :title="group.remark || ''">{{ group.remark || '-' }}</td>
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
    </div>
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
        <div class="group-form-row">
          <div id="create-models-title" class="group-form-label group-model-label required-label">
            {{ t('groups.allowedModels') }}
          </div>
          <section
            class="group-form-control group-model-field"
            aria-labelledby="create-models-title"
            aria-required="true"
          >
            <div v-if="createModelsReady" class="group-model-field-head">
              <span>{{
                t('groups.selectionCount', {
                  count: createModelIDs.length,
                  total: creationModels.length,
                })
              }}</span>
            </div>
            <div class="create-models">
              <div
                v-if="createModelsReady && creationModels.length"
                class="table-scroll create-model-list"
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
                    <tr v-for="model in creationModels" :key="model.id">
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
              </div>
              <p v-else-if="busy && !createModelsReady" class="empty-compact">
                {{ t('common.loading') }}
              </p>
              <p v-else-if="createModelsReady" class="empty-compact">
                {{ t('groups.noModelCatalog') }}
              </p>
            </div>
          </section>
        </div>
      </div>
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
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
        <div class="group-form-row">
          <div id="edit-models-title" class="group-form-label group-model-label">
            {{ t('groups.allowedModels') }}
          </div>
          <section class="group-form-control group-model-field" aria-labelledby="edit-models-title">
            <div v-if="relationReady" class="group-model-field-head">
              <span>{{
                t('groups.selectionCount', {
                  count: editModelIDs.length,
                  total: modelCandidates.length,
                })
              }}</span>
            </div>
            <div class="create-models">
              <div
                v-if="relationReady && modelCandidates.length"
                class="table-scroll create-model-list"
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
                    <tr v-for="model in modelCandidates" :key="model.id">
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
              </div>
              <p v-else-if="busy && !relationReady" class="empty-compact">
                {{ t('common.loading') }}
              </p>
              <p v-else-if="relationReady" class="empty-compact">
                {{ t('groups.noModelCatalog') }}
              </p>
            </div>
          </section>
        </div>
      </div>
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
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
    v-if="statusTarget"
    :title="t(statusTarget.status === 'ACTIVE' ? 'common.disableTitle' : 'common.enableTitle')"
    :message="
      t('common.confirmStatus', {
        name: statusTarget.name,
        status: t(statusTarget.status === 'ACTIVE' ? 'common.disable' : 'common.enable'),
      })
    "
    :hint="statusTarget.status === 'ACTIVE' ? t('common.disableHint') : undefined"
    :confirm-label="
      t(statusTarget.status === 'ACTIVE' ? 'common.disableAction' : 'common.enableAction')
    "
    :busy="busy"
    :tone="statusTarget.status === 'ACTIVE' ? 'warning' : 'success'"
    @close="statusTarget = null"
    @confirm="changeStatus"
  />
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
  color: #485e72;
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
  justify-content: flex-end;
  margin-bottom: 6px;
}
.group-model-field-head span {
  color: var(--muted);
  font-size: 12px;
  font-weight: 400;
}
.create-models {
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: hidden;
}
.create-model-list {
  max-height: 224px;
  overflow: auto;
}
.create-model-list table {
  table-layout: fixed;
  margin: 0;
  min-width: 420px;
}
.create-model-list th {
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
  border-radius: 5px;
  background: #edf3fb;
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
}
</style>
