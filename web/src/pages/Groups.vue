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
const { items, cursor, loading, error, load } = useCollection<Group>(() => '/groups')
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
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    validation.value = t('common.byteLimit')
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
    await load()
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
  <PageHeader name="groups"
    ><button class="button primary" @click="newGroup">
      <Icon name="plus" :size="18" />{{ t('groups.create') }}
    </button></PageHeader
  >
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :placeholder="t('groups.searchPlaceholder')"
      @search="search"
      @reset="reset"
    />
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="load()">{{ t('common.retry') }}</button>
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
    <ListFooter :count="items.length" :cursor="cursor" :loading="loading" @more="load(true)" />
  </section>
  <Modal v-if="creating" :title="t('groups.create')" :busy="busy" medium @close="creating = false"
    ><form @submit.prevent="create">
      <label>
        {{ t('common.name') }}
        <input v-model="name" required :disabled="busy" autofocus />
      </label>
      <label>
        {{ t('common.remark') }}
        <textarea v-model="remark" rows="3" :disabled="busy"></textarea>
      </label>
      <section class="group-model-field" aria-labelledby="create-models-title">
        <div class="group-model-field-head">
          <h3 id="create-models-title">{{ t('groups.allowedModels') }}</h3>
          <span v-if="createModelsReady">{{
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
                <col class="model-code-column" />
                <col class="model-type-column" />
                <col class="model-type-column" />
              </colgroup>
              <thead>
                <tr>
                  <th class="model-check-cell">{{ t('common.select') }}</th>
                  <th>{{ t('common.name') }}</th>
                  <th>{{ t('common.code') }}</th>
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
                    <strong>{{ model.name }}</strong>
                  </td>
                  <td>
                    <code>{{ model.code }}</code>
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
      <p v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError
        }}<button
          v-if="actionError && !createModelsReady"
          type="button"
          class="text-button"
          :disabled="busy"
          @click="run(loadCreateModels)"
        >
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
    <form @submit.prevent="saveEdit">
      <label>
        {{ t('common.name') }}
        <input v-model="editName" required :disabled="busy" autofocus />
      </label>
      <label>
        {{ t('common.remark') }}
        <textarea v-model="editRemark" rows="3" :disabled="busy"></textarea>
      </label>
      <section class="group-model-field" aria-labelledby="edit-models-title">
        <div class="group-model-field-head">
          <h3 id="edit-models-title">{{ t('groups.allowedModels') }}</h3>
          <span v-if="relationReady">{{
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
                <col class="model-code-column" />
                <col class="model-type-column" />
                <col class="model-type-column" />
              </colgroup>
              <thead>
                <tr>
                  <th class="model-check-cell">{{ t('common.select') }}</th>
                  <th>{{ t('common.name') }}</th>
                  <th>{{ t('common.code') }}</th>
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
                  <td class="model-type-cell">
                    <strong>{{ model.name }}</strong>
                  </td>
                  <td class="model-type-cell">
                    <code>{{ model.code }}</code>
                  </td>
                  <td>
                    <div class="modality-tags">
                      <span
                        v-for="value in model.inputModalities"
                        :key="value"
                        class="modality-tag"
                        >{{ t(`models.${value}`) }}</span
                      >
                    </div>
                  </td>
                  <td>
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
      <p v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError
        }}<button
          v-if="actionError && !relationReady"
          type="button"
          class="text-button"
          :disabled="busy"
          @click="run(refreshModels)"
        >
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
  <Modal v-if="statusTarget" :title="t('common.status')" :busy="busy" @close="statusTarget = null">
    <p>
      {{
        t('common.confirmStatus', {
          name: statusTarget.name,
          status: t(statusTarget.status === 'ACTIVE' ? 'common.disable' : 'common.enable'),
        })
      }}
    </p>
    <p v-if="statusTarget.status === 'ACTIVE'" class="muted">{{ t('common.disableHint') }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="statusTarget = null">
        {{ t('common.cancel') }}
      </button>
      <button class="button primary" :disabled="busy" @click="changeStatus">
        {{ t('common.confirm') }}
      </button>
    </footer>
  </Modal>
  <Modal
    v-if="deleteTarget"
    :title="t('groups.deleteTitle')"
    :busy="busy"
    @close="deleteTarget = null"
  >
    <p>{{ t('groups.deleteHint', { name: deleteTarget.name }) }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="deleteTarget = null">
        {{ t('common.cancel') }}</button
      ><button class="button danger-fill" :disabled="busy" @click="deleteGroup">
        {{ t(busy ? 'common.working' : 'groups.delete') }}
      </button>
    </footer>
  </Modal>
</template>
<style scoped>
.group-model-field {
  margin-top: 20px;
}
.group-model-field-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 7px;
}
.group-model-field-head h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}
.group-model-field-head span {
  color: var(--muted);
  font-size: 12px;
  font-weight: 400;
}
.create-models {
  border: 1px solid var(--line);
  border-radius: 12px;
  overflow: hidden;
}
.create-model-list {
  max-height: 280px;
  overflow: auto;
}
.create-model-list table {
  table-layout: fixed;
  margin: 0;
}
.model-check-column {
  width: 10%;
}
.model-name-column {
  width: 27%;
}
.model-code-column {
  width: 31%;
}
.model-type-column {
  width: 16%;
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
  gap: 6px;
}
.modality-tag {
  display: inline-block;
  padding: 3px 7px;
  border-radius: 5px;
  background: #edf3fb;
  font-size: 11px;
  white-space: nowrap;
}
.model-check-cell {
  width: 60px;
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
</style>
