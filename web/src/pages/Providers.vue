<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { all, api, errorText } from '../api'
import { useAction, useCollection, useListSearch, validText } from '../composables'
import { i18n, t } from '../i18n'
import type {
  ConnectionResult,
  Model,
  ModelSyncResult,
  Provider,
  ProviderDetail,
  ProviderInitializeResult,
  ProviderProtocol,
  Resource,
} from '../types'
import Icon from '../components/Icon.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'

const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<Provider>(() => '/providers')
const { busy, error: actionError, run } = useAction()
const editing = ref(false)
const editTarget = ref<Provider | null>(null)
const statusTarget = ref<Provider | null>(null)
const deleteTarget = ref<Provider | null>(null)
const resources = ref<Resource[]>([])
const resourceError = ref('')
const models = ref<Model[]>([])
const modelError = ref('')
const credentialTarget = ref<Provider | null>(null)
const testTarget = ref<{ provider: Provider; resource: Resource } | null>(null)
const testResult = ref<ConnectionResult | null>(null)
const syncTarget = ref<{ provider: Provider; resource: Resource } | null>(null)
const syncResult = ref<ModelSyncResult | null>(null)
const credential = ref('')
const validation = ref('')
const initializeNotice = ref('')
const activeConfigTab = ref<'models' | 'proxy'>('models')
const modelConfigTab = ref<HTMLButtonElement | null>(null)
const proxyConfigTab = ref<HTMLButtonElement | null>(null)
type MappingDraft = {
  modelId: string
  upstreamModelCode: string
}
type ProxyHeaderDraft = {
  key: string
  value: string
  configured: boolean
  originalKey: string
}
const form = reactive({
  name: '',
  website: '',
  anthropicBaseUrl: '',
  openaiBaseUrl: '',
  proxyEnabled: false,
  proxyUrl: '',
  proxyHeaders: [] as ProxyHeaderDraft[],
  mappings: [] as MappingDraft[],
})
const enabledModels = computed(() => models.value.filter((model) => model.status === 'ACTIVE'))
const mappingRows = computed(() =>
  enabledModels.value.map((model) => ({
    model,
    mapping: form.mappings.find((mapping) => mapping.modelId === model.id),
  })),
)
const selectedMappingCount = computed(() => mappingRows.value.filter((row) => row.mapping).length)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (provider) =>
    `${provider.name} ${provider.code} ${provider.website ?? ''} ${provider.endpoints.map((endpoint) => endpoint.baseUrl).join(' ')}`,
  load,
)

function validURL(value: string, endpoint = false) {
  if (!value) return true
  try {
    const parsed = new URL(value)
    return (
      parsed.username === '' &&
      parsed.password === '' &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (endpoint ? parsed.protocol === 'https:' : ['http:', 'https:'].includes(parsed.protocol))
    )
  } catch {
    return false
  }
}

function normalizeURL(value: string) {
  return value.trim().replace(/\/+$/, '')
}

function assignForm(provider: Provider | null, mappings: MappingDraft[] = []) {
  editTarget.value = provider
  Object.assign(form, {
    name: provider?.name ?? '',
    website: provider?.website ?? '',
    anthropicBaseUrl:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'ANTHROPIC')?.baseUrl ?? '',
    openaiBaseUrl:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'OPENAI')?.baseUrl ?? '',
    proxyEnabled: provider?.proxyEnabled ?? false,
    proxyUrl: provider?.proxyUrl ?? '',
    proxyHeaders: (provider?.proxyHeaders ?? []).map((header) => ({
      key: header.key,
      value: '',
      configured: header.configured,
      originalKey: header.key,
    })),
    mappings,
  })
  validation.value = ''
  actionError.value = ''
}

function addProxyHeader() {
  if (form.proxyHeaders.length >= 32) return
  form.proxyHeaders.push({ key: '', value: '', configured: false, originalKey: '' })
}

function removeProxyHeader(index: number) {
  form.proxyHeaders.splice(index, 1)
}

function validProxyURL(value: string) {
  try {
    const parsed = new URL(value.trim())
    return (
      ['http:', 'https:'].includes(parsed.protocol) &&
      Boolean(parsed.hostname) &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (parsed.pathname === '' || parsed.pathname === '/')
    )
  } catch {
    return false
  }
}

function validProxyHeaders() {
  if (form.proxyHeaders.length > 32) return false
  const names = new Set<string>()
  const denied = new Set([
    'host',
    'connection',
    'content-length',
    'proxy-connection',
    'transfer-encoding',
    'upgrade',
  ])
  for (const header of form.proxyHeaders) {
    const key = header.key.trim()
    const normalized = key.toLowerCase()
    const unchanged = header.configured && normalized === header.originalKey.toLowerCase()
    if (
      !/^[!#$%&'*+\-.^_`|~0-9A-Za-z]{1,128}$/.test(key) ||
      denied.has(normalized) ||
      names.has(normalized) ||
      (!header.value && !unchanged) ||
      header.value.length > 4096 ||
      /[\r\n\0]/.test(header.value)
    )
      return false
    names.add(normalized)
  }
  return true
}

function openEdit(provider: Provider | null = null) {
  activeConfigTab.value = 'models'
  if (!provider) {
    assignForm(null)
    editing.value = true
    return
  }
  actionError.value = ''
  void run(async () => {
    const detail = await api<ProviderDetail>(`/providers/${provider.id}`)
    assignForm(
      detail,
      detail.mappings
        .map((mapping) => ({
          modelId: mapping.modelId,
          upstreamModelCode:
            mapping.upstreamModelCode === modelCode(mapping.modelId)
              ? ''
              : mapping.upstreamModelCode,
        }))
        .filter(
          (mapping, index, mappings) =>
            mappings.findIndex((candidate) => candidate.modelId === mapping.modelId) === index,
        ),
    )
    editing.value = true
  })
}

function activateConfigTab(tab: 'models' | 'proxy', focus = false) {
  activeConfigTab.value = tab
  if (!focus) return
  void nextTick(() => {
    const button = tab === 'models' ? modelConfigTab.value : proxyConfigTab.value
    button?.focus()
  })
}

function toggleMapping(model: Model) {
  validation.value = ''
  const index = form.mappings.findIndex((mapping) => mapping.modelId === model.id)
  if (index < 0) {
    form.mappings.push({
      modelId: model.id,
      upstreamModelCode: '',
    })
  } else {
    form.mappings.splice(index, 1)
  }
}

function updateUpstreamModelCode(modelId: string, event: Event) {
  const mapping = form.mappings.find((candidate) => candidate.modelId === modelId)
  if (mapping) mapping.upstreamModelCode = (event.target as HTMLInputElement).value
}

function modelCode(modelId: string) {
  return models.value.find((model) => model.id === modelId)?.code ?? ''
}

function endpointURL(provider: Provider, protocolType: ProviderProtocol) {
  return (
    provider.endpoints.find((endpoint) => endpoint.protocolType === protocolType)?.baseUrl ?? ''
  )
}

function resolvedUpstreamModelCode(mapping: MappingDraft) {
  return mapping.upstreamModelCode.trim() || modelCode(mapping.modelId)
}

function normalizeUpstreamModelCode(mapping: MappingDraft) {
  if (mapping.upstreamModelCode.trim() === modelCode(mapping.modelId)) {
    mapping.upstreamModelCode = ''
  }
}

function resourceFor(provider: Provider) {
  return resources.value.find((resource) => resource.providerId === provider.id)
}

async function loadResources() {
  resourceError.value = ''
  try {
    resources.value = await all<Resource>('/resources')
  } catch (error) {
    resourceError.value = errorText(error)
  }
}

async function loadModels() {
  modelError.value = ''
  try {
    models.value = await all<Model>('/models')
  } catch (error) {
    modelError.value = errorText(error)
  }
}

function reload() {
  void retry()
  void loadResources()
  void loadModels()
}

function initializeProviders() {
  initializeNotice.value = ''
  actionError.value = ''
  void run(async () => {
    const result = await api<ProviderInitializeResult>('/providers/initialize', 'POST')
    initializeNotice.value = t(
      result.created > 0 ? 'providers.initializeCompleted' : 'providers.initializeUnchanged',
      { created: result.created, total: result.total },
    )
    await load()
  })
}

function configureCredential(provider: Provider) {
  credentialTarget.value = provider
  credential.value = ''
  validation.value = ''
  actionError.value = ''
}

function openDelete(provider: Provider) {
  actionError.value = ''
  deleteTarget.value = provider
}

function deleteProvider() {
  if (!deleteTarget.value) return
  const providerID = deleteTarget.value.id
  void run(async () => {
    await api(`/providers/${providerID}`, 'DELETE')
    deleteTarget.value = null
    await Promise.all([load(), loadResources()])
  })
}

function closeCredential() {
  credentialTarget.value = null
  credential.value = ''
}

function saveCredential() {
  validation.value = ''
  if (!/^[\x21-\x7e]{1,4096}$/.test(credential.value)) {
    validation.value = t('resources.credentialRequired')
    return
  }
  void run(async () => {
    const provider = credentialTarget.value!
    let resource = resourceFor(provider)
    if (resource) {
      await api(`/resources/${resource.id}/credential`, 'PUT', { credential: credential.value })
    } else {
      resource = await api<Resource>('/resources', 'POST', {
        providerId: provider.id,
        name: `${provider.name} ${t('providers.credential')}`,
        credential: credential.value,
      })
    }
    closeCredential()
    await loadResources()
    syncTarget.value = { provider, resource }
    syncResult.value = null
    const result = await api<ModelSyncResult>(`/resources/${resource.id}/sync-models`, 'POST')
    syncResult.value = result
    if (result.ok) await loadModels()
  })
}

function testConnection(provider: Provider) {
  const resource = resourceFor(provider)
  if (!resource) return
  testTarget.value = { provider, resource }
  testResult.value = null
  actionError.value = ''
  void run(async () => {
    testResult.value = await api(`/resources/${resource.id}/test-connection`, 'POST')
  })
}

function syncModels(provider: Provider) {
  const resource = resourceFor(provider)
  if (!resource) return
  syncTarget.value = { provider, resource }
  syncResult.value = null
  actionError.value = ''
  void run(async () => {
    const result = await api<ModelSyncResult>(`/resources/${resource.id}/sync-models`, 'POST')
    syncResult.value = result
    if (result.ok) await loadModels()
  })
}

function resultMessage(result: ConnectionResult) {
  return result.ok
    ? t('resources.testPassed')
    : t(i18n.global.te(`errors.${result.code}`) ? `errors.${result.code}` : 'errors.UNKNOWN')
}

function save() {
  validation.value = ''
  const endpointDrafts: Array<{ protocolType: ProviderProtocol; baseUrl: string }> = [
    { protocolType: 'OPENAI', baseUrl: normalizeURL(form.openaiBaseUrl) },
    { protocolType: 'ANTHROPIC', baseUrl: normalizeURL(form.anthropicBaseUrl) },
  ]
  const input = {
    name: form.name.trim(),
    website: normalizeURL(form.website),
    endpoints: endpointDrafts.filter((endpoint) => endpoint.baseUrl),
    proxyEnabled: form.proxyEnabled,
    proxyUrl: form.proxyEnabled ? form.proxyUrl.trim() : '',
    proxyHeaders: form.proxyEnabled
      ? form.proxyHeaders.map((header) => ({ key: header.key.trim(), value: header.value }))
      : [],
    mappings: form.mappings.map((mapping) => ({
      modelId: mapping.modelId,
      upstreamModelCode: resolvedUpstreamModelCode(mapping),
    })),
  }
  if (!validText(input.name, 128)) validation.value = t('common.byteLimit')
  else if (
    !validURL(input.website) ||
    input.endpoints.some((endpoint) => !validURL(endpoint.baseUrl, true))
  )
    validation.value = t('providers.urlInvalid')
  else if (!input.endpoints.length) validation.value = t('providers.endpointRequired')
  else if (input.proxyEnabled && !validProxyURL(input.proxyUrl))
    validation.value = t('providers.proxyUrlInvalid')
  else if (input.proxyEnabled && !validProxyHeaders())
    validation.value = t('providers.proxyHeadersInvalid')
  else if (!form.mappings.length) validation.value = t('providers.mappingRequired')
  else if (
    form.mappings.some(
      (mapping) =>
        !models.value.some((model) => model.id === mapping.modelId) ||
        !validText(resolvedUpstreamModelCode(mapping), 128),
    )
  )
    validation.value = t('providers.mappingInvalid')
  else if (new Set(form.mappings.map((mapping) => mapping.modelId)).size !== form.mappings.length)
    validation.value = t('providers.mappingDuplicate')
  if (validation.value) {
    if (input.proxyEnabled && (!validProxyURL(input.proxyUrl) || !validProxyHeaders()))
      activeConfigTab.value = 'proxy'
    else if (
      !form.mappings.length ||
      form.mappings.some(
        (mapping) =>
          !models.value.some((model) => model.id === mapping.modelId) ||
          !validText(resolvedUpstreamModelCode(mapping), 128),
      )
    )
      activeConfigTab.value = 'models'
    return
  }
  void run(async () => {
    await api(
      editTarget.value ? `/providers/${editTarget.value.id}` : '/providers',
      editTarget.value ? 'PUT' : 'POST',
      input,
    )
    editing.value = false
    await load()
  })
}

function changeStatus() {
  void run(async () => {
    const provider = statusTarget.value!
    await api(`/providers/${provider.id}/status`, 'PATCH', {
      status: provider.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
    })
    statusTarget.value = null
    await load()
  })
}

onMounted(() => {
  void load()
  void loadResources()
  void loadModels()
})
</script>

<template>
  <PageHeader name="providers" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :label="t('providers.searchLabel')"
      :placeholder="t('providers.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #actions>
        <div class="provider-toolbar-actions">
          <button type="button" class="button primary" :disabled="busy" @click="openEdit()">
            <Icon name="plus" :size="18" />{{ t('providers.create') }}
          </button>
          <button type="button" class="button" :disabled="busy" @click="initializeProviders">
            <Icon name="refresh" :size="17" />{{ t('providers.initialize') }}
          </button>
        </div>
      </template>
    </ListSearch>
    <p v-if="initializeNotice" class="provider-initialize-notice notice" role="status">
      {{ initializeNotice }}
    </p>
    <p v-if="error || resourceError || modelError" class="alert error" role="alert">
      {{ error || resourceError || modelError
      }}<button class="text-button" @click="reload">
        {{ t('common.retry') }}
      </button>
    </p>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>{{ t('providers.name') }}</th>
            <th>{{ t('common.status') }}</th>
            <th>{{ t('providers.keyConfiguration') }}</th>
            <th>{{ t('providers.proxyAccess') }}</th>
            <th>{{ t('providers.endpoints') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="provider in visible" :key="provider.id">
            <td>
              <div class="person">
                <span class="avatar">{{ provider.name.slice(0, 1) }}</span>
                <div>
                  <span class="provider-name-line"
                    ><strong>{{ provider.name }}</strong
                    ><a
                      v-if="provider.website"
                      class="provider-quick-action"
                      :href="provider.website"
                      target="_blank"
                      rel="noopener noreferrer"
                      :aria-label="t('providers.openWebsiteFor', { name: provider.name })"
                      :title="t('providers.visitWebsite')"
                    >
                      <Icon name="external" :size="14" /></a
                    ><button
                      v-if="resourceFor(provider)"
                      type="button"
                      class="provider-quick-action provider-direct-action"
                      :aria-label="t('providers.testConnectionFor', { name: provider.name })"
                      :title="t('resources.test')"
                      :disabled="busy"
                      @click="testConnection(provider)"
                    >
                      <Icon name="activity" :size="16" /></button
                    ><button
                      v-if="resourceFor(provider)"
                      type="button"
                      class="provider-quick-action provider-direct-action"
                      :aria-label="t('providers.syncModelsFor', { name: provider.name })"
                      :title="t('resources.syncModels')"
                      :disabled="busy"
                      @click="syncModels(provider)"
                    >
                      <Icon name="refresh" :size="16" /></button
                  ></span>
                  <small>{{ provider.code }}</small>
                </div>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="provider.status"
                :name="provider.name"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === provider.id"
                @change="statusTarget = provider"
              />
            </td>
            <td>
              <span
                class="key-configuration"
                :class="resourceFor(provider) ? 'is-configured' : 'is-missing'"
              >
                <span>{{ t(resourceFor(provider) ? 'common.yes' : 'common.no') }}</span>
                <button
                  type="button"
                  class="icon-button credential-edit"
                  :disabled="busy"
                  :aria-label="t('providers.configureKeyFor', { name: provider.name })"
                  :title="t('providers.configureKey')"
                  @click="configureCredential(provider)"
                >
                  <Icon name="edit" :size="15" />
                </button>
              </span>
            </td>
            <td>
              <span
                class="proxy-access-state"
                :class="provider.proxyEnabled ? 'is-enabled' : 'is-direct'"
              >
                {{ t(provider.proxyEnabled ? 'common.yes' : 'common.no') }}
              </span>
            </td>
            <td>
              <div class="endpoint-stack">
                <span
                  ><b>OpenAI</b
                  ><code class="endpoint" :title="endpointURL(provider, 'OPENAI') || undefined">{{
                    endpointURL(provider, 'OPENAI') || '-'
                  }}</code></span
                ><span
                  ><b>Anthropic</b
                  ><code
                    class="endpoint"
                    :title="endpointURL(provider, 'ANTHROPIC') || undefined"
                    >{{ endpointURL(provider, 'ANTHROPIC') || '-' }}</code
                  ></span
                >
              </div>
            </td>
            <td class="align-right">
              <div class="provider-actions">
                <button class="text-button danger" :disabled="busy" @click="openDelete(provider)">
                  {{ t('providers.delete') }}
                </button>
                <button class="text-button" :disabled="busy" @click="openEdit(provider)">
                  {{ t('providers.edit') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="providers" :size="32" />
      <p>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'providers.empty') }}</p>
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
    :title="t(editTarget ? 'providers.editTitle' : 'providers.create')"
    :busy="busy"
    wide
    @close="editing = false"
  >
    <form id="provider-form" class="provider-form" @submit.prevent="save">
      <section class="connection-editor" aria-labelledby="provider-connection-title">
        <header class="provider-section-head">
          <div>
            <h3 id="provider-connection-title">{{ t('providers.connectionTitle') }}</h3>
            <p>{{ t('providers.endpointHint') }}</p>
          </div>
        </header>
        <div class="connection-fields">
          <label
            >{{ t('providers.name')
            }}<input v-model="form.name" required autofocus :disabled="busy"
          /></label>
          <label
            >{{ t('providers.website')
            }}<input
              v-model="form.website"
              type="url"
              placeholder="https://example.com"
              :disabled="busy"
          /></label>
          <label
            >{{ t('providers.openaiEndpoint')
            }}<input
              v-model="form.openaiBaseUrl"
              type="url"
              placeholder="https://api.example.com/v1"
              spellcheck="false"
              :disabled="busy"
          /></label>
          <label
            >{{ t('providers.anthropicEndpoint')
            }}<input
              v-model="form.anthropicBaseUrl"
              type="url"
              placeholder="https://api.example.com/anthropic"
              spellcheck="false"
              :disabled="busy"
          /></label>
        </div>
      </section>
      <section class="config-editor">
        <div class="config-tabs" role="tablist" :aria-label="t('providers.configTabs')">
          <button
            id="provider-models-tab"
            ref="modelConfigTab"
            type="button"
            role="tab"
            class="config-tab"
            :class="{ 'is-active': activeConfigTab === 'models' }"
            :aria-selected="activeConfigTab === 'models'"
            aria-controls="provider-models-panel"
            :tabindex="activeConfigTab === 'models' ? 0 : -1"
            @click="activateConfigTab('models')"
            @keydown.left.prevent="activateConfigTab('proxy', true)"
            @keydown.right.prevent="activateConfigTab('proxy', true)"
            @keydown.home.prevent="activateConfigTab('models', true)"
            @keydown.end.prevent="activateConfigTab('proxy', true)"
          >
            {{ t('providers.modelConfig') }}
          </button>
          <button
            id="provider-proxy-tab"
            ref="proxyConfigTab"
            type="button"
            role="tab"
            class="config-tab"
            :class="{ 'is-active': activeConfigTab === 'proxy' }"
            :aria-selected="activeConfigTab === 'proxy'"
            aria-controls="provider-proxy-panel"
            :tabindex="activeConfigTab === 'proxy' ? 0 : -1"
            @click="activateConfigTab('proxy')"
            @keydown.left.prevent="activateConfigTab('models', true)"
            @keydown.right.prevent="activateConfigTab('models', true)"
            @keydown.home.prevent="activateConfigTab('models', true)"
            @keydown.end.prevent="activateConfigTab('proxy', true)"
          >
            {{ t('providers.proxyConfig') }}
          </button>
        </div>
        <section
          v-show="activeConfigTab === 'models'"
          id="provider-models-panel"
          class="config-panel mapping-editor"
          role="tabpanel"
          aria-labelledby="provider-models-tab"
          tabindex="0"
        >
          <header class="mapping-editor-head">
            <div>
              <h3 id="provider-mapping-title">{{ t('providers.mappingTitle') }}</h3>
              <p>{{ t('providers.mappingHint') }}</p>
            </div>
            <span v-if="enabledModels.length" class="mapping-selection-count">
              {{
                t('providers.mappingSelectionCount', {
                  count: selectedMappingCount,
                  total: enabledModels.length,
                })
              }}
            </span>
          </header>
          <div v-if="enabledModels.length" class="mapping-list">
            <div class="mapping-grid mapping-grid-head" aria-hidden="true">
              <span></span>
              <span>{{ t('providers.logicalModel') }}</span>
              <span>{{ t('providers.upstreamModelCode') }}</span>
            </div>
            <div
              v-for="row in mappingRows"
              :key="row.model.id"
              class="mapping-grid mapping-row"
              :class="{ 'is-selected': row.mapping }"
            >
              <label class="mapping-check">
                <input
                  type="checkbox"
                  :checked="Boolean(row.mapping)"
                  :disabled="busy"
                  :aria-label="t('providers.mappingSelectionFor', { name: row.model.name })"
                  @change="toggleMapping(row.model)"
                />
              </label>
              <strong class="mapping-model-name model-name-regular">{{ row.model.name }}</strong>
              <label class="mapping-control">
                <span>{{ t('providers.upstreamModelCode') }}</span>
                <input
                  :value="row.mapping?.upstreamModelCode ?? ''"
                  spellcheck="false"
                  :disabled="busy || !row.mapping"
                  :placeholder="t('providers.upstreamModelPlaceholder', { code: row.model.code })"
                  :aria-label="t('providers.mappingCodeFor', { name: row.model.name })"
                  @input="updateUpstreamModelCode(row.model.id, $event)"
                  @blur="row.mapping && normalizeUpstreamModelCode(row.mapping)"
                />
              </label>
            </div>
          </div>
          <p v-else class="mapping-empty">
            {{ t('providers.noModels') }}
          </p>
        </section>
        <section
          v-show="activeConfigTab === 'proxy'"
          id="provider-proxy-panel"
          class="config-panel proxy-editor"
          role="tabpanel"
          aria-labelledby="provider-proxy-tab"
          tabindex="0"
        >
          <div class="proxy-switch-row">
            <div>
              <strong>{{ t('providers.proxyEnabled') }}</strong>
              <p>{{ t('providers.proxyHint') }}</p>
            </div>
            <label class="proxy-switch-control">
              <input
                v-model="form.proxyEnabled"
                type="checkbox"
                role="switch"
                :aria-label="t('providers.proxyEnabled')"
                :disabled="busy"
              />
              <span class="proxy-switch-track" aria-hidden="true"></span>
            </label>
          </div>
          <div v-if="form.proxyEnabled" class="proxy-fields">
            <label
              >{{ t('providers.proxyUrl') }}
              <input
                v-model="form.proxyUrl"
                type="text"
                placeholder="http://username:password@proxy.example.com:8080"
                autocomplete="off"
                spellcheck="false"
                :disabled="busy"
              />
            </label>
            <p class="field-hint">{{ t('providers.proxyUrlHint') }}</p>
            <div class="proxy-headers-head">
              <div>
                <strong>{{ t('providers.proxyHeaders') }}</strong>
                <span>{{ t('providers.proxyHeadersHint') }}</span>
              </div>
              <button
                type="button"
                class="button compact"
                :disabled="busy || form.proxyHeaders.length >= 32"
                @click="addProxyHeader"
              >
                <Icon name="plus" :size="15" />{{ t('providers.addProxyHeader') }}
              </button>
            </div>
            <div v-if="form.proxyHeaders.length" class="proxy-header-list">
              <div
                v-for="(header, index) in form.proxyHeaders"
                :key="index"
                class="proxy-header-row"
              >
                <label>
                  <span>{{ t('providers.proxyHeaderKey') }}</span>
                  <input
                    v-model="header.key"
                    spellcheck="false"
                    maxlength="128"
                    :disabled="busy"
                    placeholder="Proxy-Authorization"
                  />
                </label>
                <label>
                  <span>{{ t('providers.proxyHeaderValue') }}</span>
                  <input
                    v-model="header.value"
                    type="text"
                    autocomplete="off"
                    spellcheck="false"
                    :disabled="busy"
                    :placeholder="
                      header.configured
                        ? t('providers.proxyHeaderValueConfigured')
                        : t('providers.proxyHeaderValuePlaceholder')
                    "
                  />
                </label>
                <button
                  type="button"
                  class="icon-button proxy-header-remove"
                  :disabled="busy"
                  :aria-label="t('providers.removeProxyHeader', { index: index + 1 })"
                  :title="t('common.remove')"
                  @click="removeProxyHeader(index)"
                >
                  <Icon name="trash" :size="17" />
                </button>
              </div>
            </div>
          </div>
        </section>
      </section>
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="editing = false">
        {{ t('common.cancel') }}</button
      ><button type="submit" form="provider-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>

  <ConfirmDialog
    v-if="deleteTarget"
    :title="t('providers.deleteTitle')"
    :message="t('providers.deleteQuestion', { name: deleteTarget.name })"
    :hint="t('providers.deleteConsequence')"
    :confirm-label="t('providers.delete')"
    :busy="busy"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteProvider"
  />

  <Modal
    v-if="credentialTarget"
    :title="t(resourceFor(credentialTarget) ? 'providers.editKey' : 'providers.configureKey')"
    :busy="busy"
    @close="closeCredential"
  >
    <p class="muted">
      {{
        t(resourceFor(credentialTarget) ? 'resources.replaceHint' : 'providers.credentialHint', {
          name: credentialTarget.name,
        })
      }}
    </p>
    <form @submit.prevent="saveCredential">
      <label
        >{{ t('resources.credential')
        }}<input
          v-model="credential"
          type="password"
          autocomplete="new-password"
          required
          autofocus
          :disabled="busy"
          spellcheck="false"
      /></label>
      <p class="field-hint">{{ t('resources.credentialHint') }}</p>
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="closeCredential">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'common.save') }}
        </button>
      </footer>
    </form>
  </Modal>

  <Modal
    v-if="testTarget"
    :title="`${testTarget.provider.name} / ${t('resources.result')}`"
    :busy="busy"
    @close="testTarget = null"
  >
    <p v-if="busy" role="status">{{ t('resources.testing') }}</p>
    <template v-if="testResult">
      <div class="alert" :class="testResult.ok ? 'success' : 'error'" role="status">
        {{ resultMessage(testResult) }}
      </div>
      <dl class="detail-grid">
        <dt>{{ t('common.code') }}</dt>
        <dd>{{ testResult.code }}</dd>
        <dt>HTTP</dt>
        <dd>{{ testResult.httpStatus || t('common.none') }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ testResult.latencyMs }} ms</dd>
      </dl>
    </template>
    <p class="muted">{{ t('resources.testHint') }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="testTarget = null">{{ t('close') }}</button>
    </footer>
  </Modal>

  <Modal
    v-if="syncTarget"
    :title="`${syncTarget.provider.name} / ${t('resources.syncResult')}`"
    :busy="busy"
    @close="syncTarget = null"
  >
    <p v-if="busy && !syncResult" role="status">{{ t('resources.syncingModels') }}</p>
    <template v-if="syncResult">
      <div class="alert" :class="syncResult.ok ? 'success' : 'error'" role="status">
        {{
          syncResult.ok
            ? t('resources.syncPassed')
            : t(
                i18n.global.te(`errors.${syncResult.code}`)
                  ? `errors.${syncResult.code}`
                  : 'errors.UNKNOWN',
              )
        }}
      </div>
      <dl class="detail-grid">
        <dt>{{ t('resources.discoveredModels') }}</dt>
        <dd>{{ syncResult.discovered }}</dd>
        <dt>{{ t('resources.createdModels') }}</dt>
        <dd>{{ syncResult.created }}</dd>
        <dt>{{ t('resources.createdMappings') }}</dt>
        <dd>{{ syncResult.mapped }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ syncResult.latencyMs }} ms</dd>
      </dl>
    </template>
    <p class="muted">{{ t('resources.syncHint') }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="syncTarget = null">{{ t('close') }}</button>
    </footer>
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
    :hint="statusTarget.status === 'ACTIVE' ? t('providers.disableHint') : undefined"
    :confirm-label="
      t(statusTarget.status === 'ACTIVE' ? 'common.disableAction' : 'common.enableAction')
    "
    :busy="busy"
    :tone="statusTarget.status === 'ACTIVE' ? 'warning' : 'success'"
    @close="statusTarget = null"
    @confirm="changeStatus"
  />
</template>

<style scoped>
.provider-toolbar-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.provider-initialize-notice {
  margin: 0 22px 16px;
}
.panel table {
  min-width: 860px;
  table-layout: fixed;
}
.panel th,
.panel td {
  padding-left: 14px;
  padding-right: 14px;
}
.panel th:nth-child(1) {
  width: 210px;
}
.panel th:nth-child(2) {
  width: 96px;
}
.panel th:nth-child(3) {
  width: 120px;
}
.panel th:nth-child(4) {
  width: 100px;
}
.panel th:nth-child(5) {
  width: 260px;
}
.panel th:nth-child(6) {
  width: 150px;
}
.endpoint {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.provider-name-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.provider-name-line strong {
  overflow: hidden;
  text-overflow: ellipsis;
}
.provider-quick-action {
  display: inline-grid;
  flex: none;
  width: 25px;
  height: 25px;
  place-items: center;
  padding: 0;
  border: 1px solid #c8d7e6;
  border-radius: 50%;
  color: var(--blue);
  background: #f4f8fc;
  text-decoration: none;
}
.provider-quick-action:hover:not(:disabled) {
  border-color: var(--blue);
  background: #eaf2ff;
}
.provider-direct-action,
.provider-direct-action:hover:not(:disabled) {
  border-color: transparent;
}
.provider-direct-action {
  background: transparent;
}
.endpoint-stack {
  display: grid;
  gap: 7px;
}
.endpoint-stack > span {
  display: grid;
  grid-template-columns: 58px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
}
.endpoint-stack b {
  color: #778a9d;
  font-size: 10px;
  font-weight: 600;
}
.key-configuration {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  font-weight: 600;
}
.key-configuration.is-configured {
  color: #20714f;
}
.key-configuration.is-missing {
  color: var(--muted);
}
.key-configuration > span {
  display: inline-flex;
  align-items: center;
  white-space: nowrap;
}
.proxy-access-state {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 600;
}
.proxy-access-state.is-enabled {
  color: #20714f;
}
.proxy-access-state.is-direct {
  color: var(--muted);
}
.credential-edit {
  width: 24px;
  height: 24px;
  color: #60788d;
}
.credential-edit:hover:not(:disabled) {
  background: #eaf1fb;
  color: #2463c4;
}
.provider-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.provider-form {
  display: grid;
  gap: 20px;
}
.provider-form label {
  margin: 0;
}
.connection-editor,
.config-editor,
.config-panel {
  min-width: 0;
}
.provider-section-head {
  padding-bottom: 14px;
}
.provider-section-head,
.mapping-editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding-bottom: 14px;
}
.provider-section-head h3,
.mapping-editor-head h3 {
  margin: 0;
  color: #183247;
  font-size: 15px;
}
.provider-section-head p,
.mapping-editor-head p {
  margin: 5px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.connection-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.connection-fields label {
  min-width: 0;
}
.config-editor {
  margin-top: 2px;
}
.config-tabs {
  display: flex;
  gap: 4px;
  overflow-x: auto;
  overflow-y: hidden;
  border-bottom: 1px solid #d8e2ec;
}
.config-tab {
  position: relative;
  flex: none;
  min-width: 112px;
  padding: 11px 16px 12px;
  border: 0;
  background: transparent;
  color: #60788d;
  font-size: 13px;
  font-weight: 600;
}
.config-tab::after {
  position: absolute;
  right: 12px;
  bottom: -1px;
  left: 12px;
  height: 2px;
  background: transparent;
  content: '';
}
.config-tab:hover {
  color: #315f87;
}
.config-tab.is-active {
  color: #174f9d;
}
.config-tab.is-active::after {
  background: var(--blue);
}
.config-panel {
  padding-top: 18px;
}
.config-panel:focus-visible {
  outline: 3px solid #90b7fb;
  outline-offset: 4px;
}
.proxy-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}
.proxy-switch-row strong {
  color: #183247;
  font-size: 15px;
}
.proxy-switch-row p {
  margin-top: 5px;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.proxy-switch-control {
  position: relative;
  display: inline-flex;
  flex: none;
  flex-direction: row;
  width: 40px;
  height: 22px;
  margin: 0;
  cursor: pointer;
}
.proxy-switch-control input {
  position: absolute;
  inset: 0;
  z-index: 1;
  width: 100%;
  height: 100%;
  margin: 0;
  opacity: 0;
  cursor: inherit;
}
.proxy-switch-track {
  width: 100%;
  height: 100%;
  border: 1px solid #b8c5d2;
  border-radius: 999px;
  background: #cbd4dd;
  transition:
    border-color 0.15s,
    background 0.15s;
}
.proxy-switch-track::after {
  display: block;
  width: 16px;
  height: 16px;
  margin: 2px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px #18324738;
  transition: transform 0.15s;
  content: '';
}
.proxy-switch-control input:checked + .proxy-switch-track {
  border-color: var(--blue);
  background: var(--blue);
}
.proxy-switch-control input:checked + .proxy-switch-track::after {
  transform: translateX(18px);
}
.proxy-switch-control input:focus-visible + .proxy-switch-track {
  outline: 3px solid #90b7fb;
  outline-offset: 3px;
}
.proxy-switch-control input:focus-visible {
  outline: 0;
}
.proxy-switch-control input:disabled + .proxy-switch-track {
  opacity: 0.5;
}
.proxy-fields {
  display: grid;
  gap: 16px;
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid #e4eaf1;
}
.proxy-fields > label {
  margin: 0;
}
.proxy-fields .field-hint {
  margin: -9px 0 0;
}
.proxy-headers-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.proxy-headers-head > div {
  display: grid;
  gap: 3px;
}
.proxy-headers-head strong {
  color: #183247;
  font-size: 13px;
}
.proxy-headers-head span {
  color: var(--muted);
  font-size: 11px;
}
.button.compact {
  min-height: 34px;
  padding: 6px 11px;
  font-size: 12px;
}
.proxy-header-list {
  display: grid;
  gap: 9px;
}
.proxy-header-row {
  display: grid;
  grid-template-columns: minmax(100px, 0.7fr) minmax(160px, 1.3fr) 36px;
  align-items: end;
  gap: 10px;
}
.proxy-header-row label:nth-child(2) {
  grid-column: 2;
}
.proxy-header-remove {
  grid-column: 3;
}
.proxy-header-row label {
  min-width: 0;
  margin: 0;
}
.proxy-header-row label > span {
  display: block;
  margin-bottom: 5px;
  color: #60788d;
  font-size: 11px;
  font-weight: 600;
}
.proxy-header-remove {
  width: 36px;
  height: 40px;
  color: #b44b4b;
}
.proxy-header-remove:hover:not(:disabled) {
  color: #8f2f35;
}
.mapping-selection-count {
  flex: none;
  padding-top: 2px;
  color: #60788d;
  font-size: 12px;
  white-space: nowrap;
}
.mapping-list {
  min-width: 0;
  max-height: min(420px, calc(100dvh - 410px));
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-gutter: stable;
  border: 1px solid #d8e2ec;
  border-radius: 9px;
}
.mapping-grid {
  display: grid;
  grid-template-columns: 46px minmax(200px, 0.8fr) minmax(300px, 1.4fr);
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border-top: 1px solid #e5ebf1;
}
.mapping-grid-head {
  position: sticky;
  top: 0;
  z-index: 1;
  padding-top: 9px;
  padding-bottom: 9px;
  border-top: 0;
  background: #fff;
  color: #60788d;
  font-size: 11px;
  font-weight: 600;
}
.mapping-row {
  min-height: 62px;
  background: #fff;
}
.mapping-row.is-selected {
  background: #fff;
  box-shadow: inset 3px 0 #4e82d8;
}
.mapping-check {
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0;
}
.mapping-check input {
  width: 18px;
  height: 18px;
  margin: 0;
  padding: 0;
  accent-color: var(--blue);
  cursor: pointer;
}
.mapping-model-name {
  min-width: 0;
  overflow: hidden;
  color: #183247;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mapping-control {
  display: block;
  min-width: 0;
  margin: 0;
}
.mapping-control > span {
  display: none;
}
.mapping-control input {
  width: 100%;
  min-width: 0;
  margin: 0;
}
.mapping-row:not(.is-selected) .mapping-control input {
  border-color: #e2e9f0;
  background: #f5f8fb;
  color: #8a9aa9;
}
.mapping-empty {
  margin: 0;
  padding: 18px 0;
  border-top: 1px solid #e5ebf1;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}
@media (max-width: 760px) {
  .provider-toolbar-actions {
    width: 100%;
    margin-left: 0;
  }
  .provider-toolbar-actions .button {
    flex: 1;
  }
  .connection-fields {
    grid-template-columns: 1fr;
  }
  .mapping-editor-head {
    align-items: stretch;
    flex-direction: column;
  }
  .proxy-header-row {
    grid-template-columns: minmax(0, 1fr) 36px;
  }
  .proxy-header-row label:nth-child(2) {
    grid-column: 1;
  }
  .proxy-header-remove {
    grid-column: 2;
    grid-row: 1 / span 2;
    align-self: center;
  }
  .mapping-selection-count {
    padding-top: 0;
  }
  .mapping-grid-head {
    display: none;
  }
  .mapping-grid {
    grid-template-columns: 32px minmax(0, 1fr);
    gap: 12px;
    padding: 14px;
  }
  .mapping-control {
    grid-column: 1 / -1;
  }
  .mapping-control > span {
    display: block;
    margin-bottom: 6px;
    color: #60788d;
    font-size: 11px;
    font-weight: 600;
  }
}
</style>
