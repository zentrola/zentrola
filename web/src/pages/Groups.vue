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
import GroupModelSelector from '../components/GroupModelSelector.vue'
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
)
const grantedModelIDs = computed(() => new Set(grantedModels.value.map((model) => model.id)))
const createSelectableModelIDs = computed(() =>
  creationModels.value.filter((model) => model.status === 'ACTIVE').map((model) => model.id),
)
const editSelectableModelIDs = computed(() =>
  modelCandidates.value.filter(canEditModelSelection).map((model) => model.id),
)
onMounted(() => load())
function canEditModelSelection(model: Model) {
  return (
    grantedModelIDs.value.has(model.id) ||
    (model.status === 'ACTIVE' && selected.value?.status === 'ACTIVE')
  )
}
function newGroup() {
  name.value = ''
  remark.value = ''
  creationModels.value = []
  createModelIDs.value = []
  createModelsReady.value = false
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
  <Modal v-if="creating" :title="t('groups.create')" :busy="busy" wide @close="creating = false"
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
            <GroupModelSelector
              v-if="createModelsReady && creationModels.length"
              v-model:model-ids="createModelIDs"
              :models="creationModels"
              :selectable-model-ids="createSelectableModelIDs"
              :disabled="busy"
            />
            <p v-else-if="busy && !createModelsReady" class="empty-compact">
              {{ t('common.loading') }}
            </p>
            <p v-else-if="createModelsReady" class="empty-compact">
              {{ t('groups.noModelCatalog') }}
            </p>
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
    wide
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
            <GroupModelSelector
              v-if="relationReady && modelCandidates.length"
              v-model:model-ids="editModelIDs"
              :models="modelCandidates"
              :selectable-model-ids="editSelectableModelIDs"
              :disabled="busy"
            />
            <p v-else-if="busy && !relationReady" class="empty-compact">
              {{ t('common.loading') }}
            </p>
            <p v-else-if="relationReady" class="empty-compact">
              {{ t('groups.noModelCatalog') }}
            </p>
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
.group-model-field {
  container: group-model-selector / inline-size;
}
.group-model-label {
  padding-top: 2px;
}
.required-label::after {
  content: '*';
  margin-left: 4px;
  color: var(--danger);
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
