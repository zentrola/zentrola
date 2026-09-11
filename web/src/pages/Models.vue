<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api'
import { useCollection, useAction, useListSearch, validText } from '../composables'
import { t } from '../i18n'
import { showErrorToast } from '../toast'
import type { Model, Modality } from '../types'
import Icon from '../components/Icon.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
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
const modalities: Modality[] = ['TEXT', 'IMAGE', 'AUDIO', 'VIDEO']
const form = reactive({
  name: '',
  code: '',
  inputModalities: [] as Modality[],
  outputModalities: [] as Modality[],
  remark: '',
})
function openEdit(model: Model | null = null) {
  editTarget.value = model
  Object.assign(form, {
    name: model?.name ?? '',
    code: model?.code ?? '',
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
      { ...form },
    )
    editing.value = false
    await load()
  })
}
const statusTarget = ref<Model | null>(null)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (model) =>
    `${model.name} ${model.code} ${model.id} ${model.publisherProviderName ?? ''} ${model.remark ?? ''} ${[...(model.inputModalities ?? []), ...(model.outputModalities ?? [])].map((value) => t(`models.${value}`)).join(' ')}`,
  load,
)
onMounted(() => load())
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
</script>
<template>
  <PageHeader name="models" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :label="t('models.searchLabel')"
      :placeholder="t('models.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #actions>
        <div class="list-toolbar-actions">
          <button type="button" class="button primary" :disabled="busy" @click="openEdit()">
            <Icon name="plus" :size="18" />{{ t('models.create') }}
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
            <th>{{ t('models.name') }}</th>
            <th>{{ t('models.publisher') }}</th>
            <th>{{ t('common.status') }}</th>
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
            <td>{{ model.publisherProviderName || '-' }}</td>
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
              <div class="model-remark">{{ model.remark || '-' }}</div>
            </td>
            <td>
              <div class="row-actions">
                <button class="text-button" :disabled="busy" @click="openEdit(model)">
                  {{ t('models.editAction') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="models" :size="32" />
      <p>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'models.empty') }}</p>
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
</template>
<style scoped>
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
  color: #485e72;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.model-form-control {
  min-width: 0;
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
  border-radius: 5px;
  background: #edf3fb;
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
