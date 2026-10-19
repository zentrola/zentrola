<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { all, api, errorText } from '../api'
import { useCollection, useAction, useListSearch } from '../composables'
import { t } from '../i18n'
import { fillModelForm, modelFormError, type ModelForm } from '../modelForm'
import { showErrorToast } from '../toast'
import type { Model, Provider } from '../types'
import Icon from '../components/Icon.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ModelEditorModal from '../components/models/ModelEditorModal.vue'
import ModelPriorityModal from '../components/models/ModelPriorityModal.vue'
import ModelTable from '../components/models/ModelTable.vue'
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
const priorityTarget = ref<Model | null>(null)
const providerLoading = ref(false)
const providerError = ref('')
const form = reactive<ModelForm>({
  name: '',
  code: '',
  publisherProviderId: '',
  inputModalities: [],
  outputModalities: [],
  remark: '',
})
function openEdit(model: Model | null = null) {
  editTarget.value = model
  fillModelForm(form, model)
  actionError.value = ''
  editing.value = true
}
function save() {
  const invalid = modelFormError(form)
  if (invalid) {
    showErrorToast(invalid)
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
    <ModelTable
      :models="visible"
      :busy="busy"
      :loading="loading"
      :status-target-id="statusTarget?.id"
      @priorities="priorityTarget = $event"
      @change-status="changeStatus"
      @edit="openEdit"
      @delete="openDelete"
    />
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
  <ModelEditorModal
    v-if="editing"
    :edit-target="editTarget"
    :form="form"
    :busy="busy"
    :providers="providers"
    :provider-loading="providerLoading"
    @close="editing = false"
    @submit="save"
  />
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
  <ModelPriorityModal
    v-if="priorityTarget"
    :key="priorityTarget.id"
    :model="priorityTarget"
    @close="priorityTarget = null"
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
@media (max-width: 640px) {
  .model-publisher-filter {
    width: 100%;
  }
  .model-publisher-filter select {
    flex: 1;
    width: auto;
  }
}
</style>
