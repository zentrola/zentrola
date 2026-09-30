<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { all, api, errorText } from '../api'
import { useCollection, useAction, useListSearch, validText } from '../composables'
import { t } from '../i18n'
import { showErrorToast } from '../toast'
import type { Model, Modality, Provider } from '../types'
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
} = useCollection<Model>(() => '/models')
const { busy, error: actionError, run } = useAction()
const editing = ref(false)
const editTarget = ref<Model | null>(null)
const deleteTarget = ref<Model | null>(null)
const providers = ref<Provider[]>([])
const providerLoading = ref(false)
const providerError = ref('')
const modalities: Modality[] = ['TEXT', 'IMAGE', 'AUDIO', 'VIDEO']
const form = reactive({
  name: '',
  code: '',
  publisherProviderId: '',
  inputModalities: [] as Modality[],
  outputModalities: [] as Modality[],
  remark: '',
})
function openEdit(model: Model | null = null) {
  editTarget.value = model
  Object.assign(form, {
    name: model?.name ?? '',
    code: model?.code ?? '',
    publisherProviderId: model?.publisherProviderId ?? '',
    inputModalities: [...(model?.inputModalities ?? [])],
    outputModalities: [...(model?.outputModalities ?? [])],
    remark: model?.remark ?? '',
  })
  actionError.value = ''
  editing.value = true
}
function save() {
  if (
    !validText(form.name, 128) ||
    !validText(form.code, 128) ||
    (form.remark !== '' && !validText(form.remark, 2000))
  ) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  if (!form.inputModalities.length || !form.outputModalities.length) {
    showErrorToast(t('models.modalityRequired'))
    return
  }
  void run(async () => {
    await api(
      editTarget.value ? `/models/${editTarget.value.id}` : '/models',
      editTarget.value ? 'PUT' : 'POST',
      { ...form, publisherProviderId: form.publisherProviderId || null },
    )
    editing.value = false
    await load()
  })
}
const statusTarget = ref<Model | null>(null)
const publisherProviderId = ref('')
const appliedPublisherProviderId = ref('')
const {
  keyword,
  query,
  visible: searchVisible,
  search: searchKeyword,
  reset: resetKeyword,
  searching,
  searchingAll,
} = useListSearch(
  items,
  (model) =>
    `${model.name} ${model.code} ${model.id} ${model.publisherProviderName ?? ''} ${model.remark ?? ''} ${[...(model.inputModalities ?? []), ...(model.outputModalities ?? [])].map((value) => t(`models.${value}`)).join(' ')}`,
  () => '/models',
  loading,
)
const visible = computed(() =>
  searchVisible.value.filter(
    (model) =>
      !appliedPublisherProviderId.value ||
      model.publisherProviderId === appliedPublisherProviderId.value,
  ),
)
function search() {
  appliedPublisherProviderId.value = publisherProviderId.value
  void searchKeyword(Boolean(appliedPublisherProviderId.value))
}
function reset() {
  resetKeyword()
  publisherProviderId.value = ''
  appliedPublisherProviderId.value = ''
}
async function loadProviders() {
  providerLoading.value = true
  providerError.value = ''
  try {
    providers.value = await all<Provider>('/providers?type=OFFICIAL')
  } catch (error) {
    providerError.value = errorText(error)
  } finally {
    providerLoading.value = false
  }
}
function retryAll() {
  void retry()
  void loadProviders()
}
onMounted(() => {
  void load()
  void loadProviders()
})
function changeStatus(model: Model) {
  actionError.value = ''
  statusTarget.value = model
  void run(async () => {
    try {
      await api(`/models/${model.id}/status`, 'PATCH', {
        status: model.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
      })
      await refresh()
    } finally {
      statusTarget.value = null
    }
  })
}
function openDelete(model: Model) {
  actionError.value = ''
  deleteTarget.value = model
}
function deleteModel() {
  if (!deleteTarget.value) return
  const modelID = deleteTarget.value.id
  void run(async () => {
    await api(`/models/${modelID}`, 'DELETE')
    deleteTarget.value = null
    await load()
  })
}
</script>
<template>
  <PageHeader name="models" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading || searching"
      :label="t('models.searchLabel')"
      :placeholder="t('models.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #filters>
        <label class="model-publisher-filter">
          <span>{{ t('models.publisher') }}</span>
          <select
            v-model="publisherProviderId"
            :aria-label="t('models.publisher')"
            :disabled="loading || providerLoading"
          >
            <option value="">{{ t('common.all') }}</option>
            <option v-for="provider in providers" :key="provider.id" :value="provider.id">
              {{ provider.name }}
            </option>
          </select>
        </label>
      </template>
      <template #actions>
        <div class="list-toolbar-actions">
          <button type="button" class="button primary" :disabled="busy" @click="openEdit()">
            <Icon name="plus" :size="18" />{{ t('models.create') }}
          </button>
        </div>
      </template>
    </ListSearch>
    <p v-if="error || providerError" class="alert error" role="alert">
      {{ error || providerError
      }}<button class="text-button" @click="retryAll">{{ t('common.retry') }}</button>
    </p>
    <TableScroll has-actions>
      <table>
        <thead>
          <tr>
            <th>{{ t('models.name') }}</th>
            <th>{{ t('models.publisher') }}</th>
            <th>{{ t('common.enableStatus') }}</th>
            <th>{{ t('models.input') }}</th>
            <th>{{ t('models.output') }}</th>
            <th>{{ t('common.remark') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="model in visible" :key="model.id">
            <td>
              <div class="person">
                <span class="avatar">{{ model.name.slice(0, 1) }}</span>
                <div>
                  <strong class="model-name-regular">{{ model.name }}</strong
                  ><small>{{ model.code }}</small>
                </div>
              </div>
            </td>
            <td>{{ model.publisherProviderName || t('common.none') }}</td>
            <td>
              <StatusSwitch
                :value="model.status"
                :name="model.name"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === model.id"
                @change="changeStatus(model)"
              />
            </td>
            <td>
              <div class="modality-tags">
                <span v-for="value in model.inputModalities" :key="value" class="modality-tag">{{
                  t(`models.${value}`)
                }}</span>
              </div>
            </td>
            <td>
              <div class="modality-tags">
                <span v-for="value in model.outputModalities" :key="value" class="modality-tag">{{
                  t(`models.${value}`)
                }}</span>
              </div>
            </td>
            <td>
              <div class="model-remark">{{ model.remark || t('common.none') }}</div>
            </td>
            <td>
              <div class="row-actions">
                <button class="text-button danger" :disabled="busy" @click="openDelete(model)">
                  {{ t('models.delete') }}
                </button>
                <button class="text-button" :disabled="busy" @click="openEdit(model)">
                  {{ t('models.editAction') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="models" :size="32" />
      <p>
        {{
          t(
            loading
              ? 'common.loading'
              : query || appliedPublisherProviderId
                ? 'common.noResults'
                : 'models.empty',
          )
        }}
      </p>
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
    v-if="editing"
    :title="t(editTarget ? 'models.edit' : 'models.create')"
    :busy="busy"
    medium
    @close="editing = false"
  >
    <p v-if="!editTarget" class="model-form-hint">{{ t('models.createHint') }}</p>
    <form class="model-form" @submit.prevent="save">
      <div class="model-form-row">
        <label class="model-form-label required-label" for="model-code-input">{{
          t('models.code')
        }}</label>
        <div class="model-form-control">
          <input
            id="model-code-input"
            v-model="form.code"
            required
            :disabled="busy"
            spellcheck="false"
            autofocus
          />
        </div>
      </div>
      <div class="model-form-row">
        <label class="model-form-label required-label" for="model-name-input">{{
          t('models.name')
        }}</label>
        <div class="model-form-control">
          <input id="model-name-input" v-model="form.name" required :disabled="busy" />
        </div>
      </div>
      <p v-if="editTarget && form.code !== editTarget.code" class="alert" role="status">
        {{ t('models.codeChange') }}
      </p>
      <div class="model-form-row">
        <label class="model-form-label" for="model-publisher-input">{{
          t('models.publisher')
        }}</label>
        <div class="model-form-control">
          <select
            id="model-publisher-input"
            v-model="form.publisherProviderId"
            :disabled="busy || providerLoading"
          >
            <option value="">{{ t('models.publisherPlaceholder') }}</option>
            <option v-for="provider in providers" :key="provider.id" :value="provider.id">
              {{ provider.name }}
            </option>
          </select>
          <small class="model-field-hint">{{ t('models.publisherHint') }}</small>
        </div>
      </div>
      <div class="model-form-row">
        <div id="model-input-title" class="model-form-label required-label">
          {{ t('models.input') }}
        </div>
        <div
          class="model-form-control modality-options"
          role="group"
          aria-labelledby="model-input-title"
        >
          <label v-for="value in modalities" :key="value"
            ><input
              v-model="form.inputModalities"
              type="checkbox"
              :value="value"
              :disabled="busy"
            />{{ t(`models.${value}`) }}</label
          >
        </div>
      </div>
      <div class="model-form-row">
        <div id="model-output-title" class="model-form-label required-label">
          {{ t('models.output') }}
        </div>
        <div
          class="model-form-control modality-options"
          role="group"
          aria-labelledby="model-output-title"
        >
          <label v-for="value in modalities" :key="value"
            ><input
              v-model="form.outputModalities"
              type="checkbox"
              :value="value"
              :disabled="busy"
            />{{ t(`models.${value}`) }}</label
          >
        </div>
      </div>
      <div class="model-form-row">
        <label class="model-form-label" for="model-remark-input">{{ t('common.remark') }}</label>
        <div class="model-form-control">
          <textarea
            id="model-remark-input"
            v-model="form.remark"
            :disabled="busy"
            :placeholder="t('models.remarkPlaceholder')"
            rows="3"
          />
        </div>
      </div>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="editing = false">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'common.save') }}
        </button>
      </footer>
    </form>
  </Modal>
  <ConfirmDialog
    v-if="deleteTarget"
    :title="t('models.deleteTitle')"
    :message="t('models.deleteQuestion', { name: deleteTarget.name })"
    :hint="t('models.deleteConsequence')"
    :confirm-label="t('models.delete')"
    :busy="busy"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteModel"
  />
</template>
<style scoped>
.model-publisher-filter {
  display: flex;
  flex: none;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
}
.model-publisher-filter select {
  width: 144px;
  padding-top: 9px;
  padding-bottom: 9px;
  background: var(--color-surface);
  border-color: var(--color-border);
  font-size: 12px;
}
.model-form {
  display: grid;
  gap: 16px;
}
.model-form label {
  margin-bottom: 0;
}
.model-form .form-footer {
  margin-top: 0;
}
.model-form-hint {
  margin: 0 0 20px;
  color: var(--muted);
}
.model-form-row {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.model-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.model-form-control {
  min-width: 0;
}
.model-field-hint {
  display: block;
  margin-top: 6px;
  color: var(--muted);
  line-height: 1.5;
}
.required-label::after {
  content: '*';
  margin-left: 4px;
  color: var(--danger);
}
.modality-options,
.modality-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.modality-options label {
  display: inline-flex;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  margin: 0;
  font-weight: 400;
  cursor: pointer;
}
.modality-options {
  min-height: 38px;
}
.modality-options input {
  width: 16px;
  height: 16px;
  margin: 0;
  padding: 0;
  accent-color: var(--blue);
}
.modality-tag {
  display: inline-block;
  padding: 3px 7px;
  border-radius: 6px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  font-size: 11px;
  white-space: nowrap;
}
.model-remark {
  min-width: 100px;
  max-width: 240px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
@media (max-width: 640px) {
  .model-publisher-filter {
    width: 100%;
  }
  .model-publisher-filter select {
    flex: 1;
    width: auto;
  }
  .model-form-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .model-form-label {
    padding-top: 0;
    text-align: left;
  }
}
</style>
