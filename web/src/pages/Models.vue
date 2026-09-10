<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api'
import { useCollection, useAction, useListSearch, validText } from '../composables'
import { t } from '../i18n'
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
const editing = ref(false),
  editTarget = ref<Model | null>(null),
  validation = ref('')
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
  validation.value = ''
  actionError.value = ''
  editing.value = true
}
function save() {
  validation.value = ''
  if (
    !validText(form.name, 128) ||
    !validText(form.code, 128) ||
    (form.remark !== '' && !validText(form.remark, 2000))
  ) {
    validation.value = t('common.byteLimit')
    return
  }
  if (!form.inputModalities.length || !form.outputModalities.length) {
    validation.value = t('models.modalityRequired')
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
      <div class="model-fields">
        <label
          >{{ t('models.name') }}<input v-model="form.name" required :disabled="busy" autofocus
        /></label>
        <label
          >{{ t('models.code')
          }}<input v-model="form.code" required :disabled="busy" spellcheck="false"
        /></label>
      </div>
      <p v-if="editTarget && form.code !== editTarget.code" class="alert" role="status">
        {{ t('models.codeChange') }}
      </p>
      <div class="model-fields">
        <fieldset class="modality-field" :disabled="busy">
          <legend>{{ t('models.input') }}</legend>
          <div class="modality-options">
            <label v-for="value in modalities" :key="value"
              ><input v-model="form.inputModalities" type="checkbox" :value="value" />{{
                t(`models.${value}`)
              }}</label
            >
          </div>
        </fieldset>
        <fieldset class="modality-field" :disabled="busy">
          <legend>{{ t('models.output') }}</legend>
          <div class="modality-options">
            <label v-for="value in modalities" :key="value"
              ><input v-model="form.outputModalities" type="checkbox" :value="value" />{{
                t(`models.${value}`)
              }}</label
            >
          </div>
        </fieldset>
      </div>
      <label
        >{{ t('common.remark')
        }}<textarea
          v-model="form.remark"
          :disabled="busy"
          :placeholder="t('models.remarkPlaceholder')"
          rows="3"
        />
      </label>
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
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
  gap: 22px;
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
.model-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}
.modality-field {
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}
.modality-field legend {
  padding: 0;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
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
  .model-fields {
    grid-template-columns: 1fr;
    gap: 22px;
  }
}
</style>
