<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { all, api, errorText } from '../api'
import { date, useAction, useCollection, useListSearch, validText } from '../composables'
import { i18n, t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
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
import Status from '../components/Status.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
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
} = useCollection<Provider>(() => '/providers')
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
const credentialDeleteTarget = ref<Resource | null>(null)
const credentialCreating = ref(false)
const credentialAuthType = ref<'API_KEY' | 'SUBSCRIPTION'>('API_KEY')
const testTarget = ref<{ provider: Provider; resource: Resource } | null>(null)
const testResult = ref<ConnectionResult | null>(null)
const credentialVerifiedAt = reactive<Record<string, string>>({})
const syncTarget = ref<{ provider: Provider; resource: Resource } | null>(null)
const syncResult = ref<ModelSyncResult | null>(null)
const credential = ref('')
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
const allMappingsSelected = computed(
  () => enabledModels.value.length > 0 && selectedMappingCount.value === enabledModels.value.length,
)
const someMappingsSelected = computed(
  () => selectedMappingCount.value > 0 && !allMappingsSelected.value,
)
const route = useRoute()
const router = useRouter()
type RuntimeFilter = '' | 'HEALTHY' | 'ABNORMAL'
function routeRuntimeFilter(value: unknown): RuntimeFilter {
  return value === 'HEALTHY' || value === 'ABNORMAL' ? value : ''
}
const runtimeFilter = ref<RuntimeFilter>(routeRuntimeFilter(route.query.runtimeStatus))
const appliedRuntimeFilter = ref<RuntimeFilter>(runtimeFilter.value)
const {
  keyword,
  query,
  visible: searchVisible,
  search: searchKeyword,
  reset: resetKeyword,
} = useListSearch(
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

function toggleAllMappings(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const enabledModelIDs = new Set(enabledModels.value.map((model) => model.id))
  if (checked) {
    for (const model of enabledModels.value) {
      if (!form.mappings.some((mapping) => mapping.modelId === model.id)) {
        form.mappings.push({ modelId: model.id, upstreamModelCode: '' })
      }
    }
    return
  }
  const remaining = form.mappings.filter((mapping) => !enabledModelIDs.has(mapping.modelId))
  form.mappings.splice(0, form.mappings.length, ...remaining)
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
  return [...resourcesFor(provider)].sort((left, right) => {
    const preferred = (resource: Resource) =>
      resource.authType === 'SUBSCRIPTION' &&
      (resource.quotaStatus === 'AVAILABLE' || resource.quotaStatus === 'NEAR_LIMIT')
        ? 0
        : resource.authType === 'API_KEY' || !resource.authType
          ? 1
          : 2
    return preferred(left) - preferred(right) || left.priority - right.priority
  })[0]
}

function resourcesFor(provider: Provider) {
  return resources.value.filter((resource) => resource.providerId === provider.id)
}

function apiKeyResourceFor(provider: Provider) {
  return resourcesFor(provider).find(
    (resource) => resource.authType === 'API_KEY' || !resource.authType,
  )
}

function blockedResourceReason(resource: Resource) {
  const key = `resources.blockReasons.${resource.blockedReason || 'UNKNOWN_PERMANENT'}`
  const reason = t(i18n.global.te(key) ? key : 'resources.blockReasons.UNKNOWN_PERMANENT')
  return resource.lastHttpStatus ? `${reason} · HTTP ${resource.lastHttpStatus}` : reason
}

function verificationLabel(resource: Resource) {
  return t(
    resource.runtimeStatus === 'BLOCKED' ? 'resources.verifyAndRestoreFor' : 'resources.verifyFor',
    { name: resource.name },
  )
}

function providerRuntime(provider: Provider) {
  const configured = resourcesFor(provider)
  if (!configured.length) {
    return {
      status: 'UNCONFIGURED',
      reason: t('providers.runtimeReasons.UNCONFIGURED'),
      errorCode: '',
    }
  }

  const healthy = configured.filter((resource) => resource.runtimeStatus !== 'BLOCKED')
  const blocked = configured
    .filter((resource) => resource.runtimeStatus === 'BLOCKED')
    .sort((left, right) => (right.lastErrorAt || '').localeCompare(left.lastErrorAt || ''))

  if (healthy.length && !blocked.length) {
    return {
      status: provider.status === 'ACTIVE' ? 'HEALTHY' : 'DISABLED',
      reason: '',
      errorCode: '',
    }
  }

  const failed = blocked[0]
  return {
    status: healthy.length ? 'DEGRADED' : 'BLOCKED',
    reason: failed ? blockedResourceReason(failed) : t('providers.runtimeReasons.UNKNOWN'),
    errorCode: failed?.lastErrorCode || '',
  }
}

function providerRuntimeHint(provider: Provider) {
  const runtime = providerRuntime(provider)
  return [runtime.reason, runtime.errorCode].filter(Boolean).join(' · ')
}

function providerRuntimeLabel(provider: Provider) {
  const runtime = providerRuntime(provider)
  const stateKey = `state.${runtime.status}`
  const status = i18n.global.te(stateKey) ? t(stateKey) : runtime.status
  const hint = providerRuntimeHint(provider)
  return hint ? `${status}：${hint}` : status
}

function providerRuntimeActionLabel(provider: Provider) {
  return t('providers.openCredentialsFromRuntime', {
    name: provider.name,
    status: providerRuntimeLabel(provider),
  })
}

function maskedCredential(resource: Resource) {
  return resource.authType === 'SUBSCRIPTION'
    ? t('resources.maskedToken')
    : t('resources.maskedApiKey')
}

function lastCredentialVerification(resource: Resource) {
  return credentialVerifiedAt[resource.id] || resource.quotaCheckedAt || resource.lastErrorAt
}

function providerRuntimeAbnormal(provider: Provider) {
  if (provider.status !== 'ACTIVE') return false
  const configured = resourcesFor(provider)
  if (!configured.length) return true
  return !configured.some((resource) => resource.runtimeStatus !== 'BLOCKED')
}

const visible = computed(() =>
  searchVisible.value.filter((provider) => {
    if (!appliedRuntimeFilter.value) return true
    const abnormal = providerRuntimeAbnormal(provider)
    return appliedRuntimeFilter.value === 'ABNORMAL'
      ? abnormal
      : provider.status === 'ACTIVE' && !abnormal
  }),
)

function syncRuntimeFilterQuery(value: RuntimeFilter) {
  const nextQuery = { ...route.query }
  if (value) nextQuery.runtimeStatus = value
  else delete nextQuery.runtimeStatus
  void router.replace({ query: nextQuery })
}

function search() {
  searchKeyword()
  appliedRuntimeFilter.value = runtimeFilter.value
  syncRuntimeFilterQuery(runtimeFilter.value)
}

function reset() {
  resetKeyword()
  runtimeFilter.value = ''
  appliedRuntimeFilter.value = ''
  syncRuntimeFilterQuery('')
}

watch(
  () => route.query.runtimeStatus,
  (value) => {
    const next = routeRuntimeFilter(value)
    runtimeFilter.value = next
    appliedRuntimeFilter.value = next
  },
)

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
  actionError.value = ''
  void run(async () => {
    const result = await api<ProviderInitializeResult>('/providers/initialize', 'POST', {
      locale: i18n.global.locale.value,
    })
    const message = t(
      result.created > 0 || result.updated > 0
        ? 'providers.initializeCompleted'
        : 'providers.initializeUnchanged',
      { created: result.created, updated: result.updated, total: result.total },
    )
    await load()
    showSuccessToast(message)
  })
}

function configureCredential(provider: Provider) {
  credentialTarget.value = provider
  credentialCreating.value = false
  credentialAuthType.value = 'API_KEY'
  credential.value = ''
  actionError.value = ''
}

function addCredential() {
  credentialCreating.value = true
  credentialAuthType.value = 'API_KEY'
  credential.value = ''
}

function cancelCredentialCreation() {
  credentialCreating.value = false
  credential.value = ''
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
  credentialCreating.value = false
  credential.value = ''
}

function saveCredential() {
  const valid =
    credentialAuthType.value === 'API_KEY'
      ? /^[\x21-\x7e]{1,4096}$/.test(credential.value)
      : credential.value.length > 0 && new TextEncoder().encode(credential.value).length <= 65536
  if (!valid) {
    showErrorToast(
      t(
        credentialAuthType.value === 'API_KEY'
          ? 'resources.credentialRequired'
          : 'resources.subscriptionRequired',
      ),
    )
    return
  }
  void run(async () => {
    const provider = credentialTarget.value!
    await api('/resources', 'POST', {
      providerId: provider.id,
      name: `${provider.name} ${t(`resources.authTypes.${credentialAuthType.value}`)}`,
      credential: credential.value,
      authType: credentialAuthType.value,
      authAdapter: credentialAuthType.value === 'SUBSCRIPTION' ? 'OPENAI_CODEX' : 'API_KEY',
    })
    await loadResources()
    credentialCreating.value = false
    credential.value = ''
    showSuccessToast(t('common.saved'))
  })
}

async function importSubscription(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (file.size > 65536) {
    showErrorToast(t('resources.subscriptionRequired'))
    input.value = ''
    return
  }
  credential.value = await file.text()
}

function deleteCredential() {
  if (!credentialDeleteTarget.value) return
  const resourceID = credentialDeleteTarget.value.id
  void run(async () => {
    await api(`/resources/${resourceID}`, 'DELETE')
    delete credentialVerifiedAt[resourceID]
    credentialDeleteTarget.value = null
    await loadResources()
  })
}

function testConnection(provider: Provider, resource: Resource) {
  testTarget.value = { provider, resource }
  testResult.value = null
  actionError.value = ''
  void run(async () => {
    const result = await api<ConnectionResult>(`/resources/${resource.id}/test-connection`, 'POST')
    credentialVerifiedAt[resource.id] = new Date().toISOString()
    await loadResources()
    testResult.value = result
  })
}

function testCredentialFromModal(provider: Provider, resource: Resource) {
  closeCredential()
  testConnection(provider, resource)
}

function syncModels(provider: Provider) {
  const resource = apiKeyResourceFor(provider)
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
  let validation = ''
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
  if (!validText(input.name, 128)) validation = t('common.byteLimit')
  else if (
    !validURL(input.website) ||
    input.endpoints.some((endpoint) => !validURL(endpoint.baseUrl, true))
  )
    validation = t('providers.urlInvalid')
  else if (!input.endpoints.length) validation = t('providers.endpointRequired')
  else if (input.proxyEnabled && !validProxyURL(input.proxyUrl))
    validation = t('providers.proxyUrlInvalid')
  else if (input.proxyEnabled && !validProxyHeaders())
    validation = t('providers.proxyHeadersInvalid')
  else if (!form.mappings.length) validation = t('providers.mappingRequired')
  else if (
    form.mappings.some(
      (mapping) =>
        !models.value.some((model) => model.id === mapping.modelId) ||
        !validText(resolvedUpstreamModelCode(mapping), 128),
    )
  )
    validation = t('providers.mappingInvalid')
  else if (new Set(form.mappings.map((mapping) => mapping.modelId)).size !== form.mappings.length)
    validation = t('providers.mappingDuplicate')
  if (validation) {
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
    showErrorToast(validation)
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

function changeStatus(provider: Provider) {
  actionError.value = ''
  statusTarget.value = provider
  void run(async () => {
    try {
      await api(`/providers/${provider.id}/status`, 'PATCH', {
        status: provider.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
      })
      await refresh()
    } finally {
      statusTarget.value = null
    }
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
      <template #filters>
        <label class="provider-runtime-filter">
          <span>{{ t('providers.runtimeStatus') }}</span>
          <select v-model="runtimeFilter" :aria-label="t('providers.runtimeStatus')">
            <option value="">{{ t('common.all') }}</option>
            <option value="HEALTHY">{{ t('providers.runtimeHealthy') }}</option>
            <option value="ABNORMAL">{{ t('providers.runtimeAbnormal') }}</option>
          </select>
        </label>
      </template>
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
    <p v-if="error || resourceError || modelError" class="alert error" role="alert">
      {{ error || resourceError || modelError
      }}<button class="text-button" @click="reload">
        {{ t('common.retry') }}
      </button>
    </p>
    <TableScroll has-actions>
      <table>
        <colgroup>
          <col class="provider-name-column" />
          <col span="5" />
        </colgroup>
        <thead>
          <tr>
            <th>{{ t('providers.name') }}</th>
            <th>{{ t('common.enableStatus') }}</th>
            <th>{{ t('providers.runtimeStatus') }}</th>
            <th class="proxy-column">{{ t('providers.proxyAccess') }}</th>
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
                      class="provider-quick-action provider-website-action"
                      :href="provider.website"
                      target="_blank"
                      rel="noopener noreferrer"
                      :aria-label="t('providers.openWebsiteFor', { name: provider.name })"
                      :title="t('providers.visitWebsite')"
                    >
                      <Icon name="website" :size="15" /></a
                    ><button
                      v-if="provider.modelSyncSupported && apiKeyResourceFor(provider)"
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
                :disabled="
                  busy || loading || (provider.status !== 'ACTIVE' && !resourceFor(provider))
                "
                :busy="busy && statusTarget?.id === provider.id"
                :title="
                  provider.status !== 'ACTIVE' && !resourceFor(provider)
                    ? t('providers.credentialRequiredBeforeEnable')
                    : undefined
                "
                @change="changeStatus(provider)"
              />
            </td>
            <td class="provider-runtime">
              <button
                type="button"
                class="provider-runtime-state"
                :title="providerRuntimeActionLabel(provider)"
                :aria-label="providerRuntimeActionLabel(provider)"
                :disabled="busy"
                @click="configureCredential(provider)"
              >
                <Status :value="providerRuntime(provider).status" />
              </button>
            </td>
            <td class="proxy-column">
              <span
                class="proxy-access-state"
                :class="provider.proxyEnabled ? 'is-enabled' : 'is-direct'"
                role="img"
                :aria-label="
                  t(
                    provider.proxyEnabled
                      ? 'providers.proxyEnabledFor'
                      : 'providers.proxyDisabledFor',
                    { name: provider.name },
                  )
                "
                :title="
                  t(
                    provider.proxyEnabled
                      ? 'providers.proxyEnabledFor'
                      : 'providers.proxyDisabledFor',
                    { name: provider.name },
                  )
                "
              >
                <Icon :name="provider.proxyEnabled ? 'check' : 'close'" :size="17" />
              </span>
            </td>
            <td>
              <div
                v-if="endpointURL(provider, 'OPENAI') || endpointURL(provider, 'ANTHROPIC')"
                class="endpoint-stack"
              >
                <span v-if="endpointURL(provider, 'OPENAI')"
                  ><span class="endpoint-protocol">OpenAI</span
                  ><code class="endpoint" :title="endpointURL(provider, 'OPENAI')">{{
                    endpointURL(provider, 'OPENAI')
                  }}</code></span
                ><span v-if="endpointURL(provider, 'ANTHROPIC')"
                  ><span class="endpoint-protocol">Anthropic</span
                  ><code class="endpoint" :title="endpointURL(provider, 'ANTHROPIC')">{{
                    endpointURL(provider, 'ANTHROPIC')
                  }}</code></span
                >
              </div>
            </td>
            <td class="align-right">
              <div class="provider-actions">
                <button class="text-button" :disabled="busy" @click="openEdit(provider)">
                  {{ t('providers.edit') }}
                </button>
                <button
                  type="button"
                  class="text-button"
                  :disabled="busy"
                  :aria-label="t('providers.editCredentialFor', { name: provider.name })"
                  :title="t('providers.editCredentialFor', { name: provider.name })"
                  @click="configureCredential(provider)"
                >
                  {{ t('providers.credentialAction') }}
                </button>
                <details class="provider-more">
                  <summary
                    :aria-label="t('providers.moreActionsFor', { name: provider.name })"
                    :title="t('providers.moreActions')"
                  >
                    ⋯
                  </summary>
                  <div class="provider-more-menu">
                    <button
                      type="button"
                      class="text-button danger"
                      :disabled="busy"
                      @click="openDelete(provider)"
                    >
                      {{ t('providers.delete') }}
                    </button>
                  </div>
                </details>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="providers" :size="32" />
      <p>
        {{
          t(
            loading
              ? 'common.loading'
              : query || appliedRuntimeFilter
                ? 'common.noResults'
                : 'providers.empty',
          )
        }}
      </p>
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
      <section class="connection-editor" :aria-label="t('providers.connectionTitle')">
        <p class="provider-connection-hint">{{ t('providers.endpointHint') }}</p>
        <div class="connection-fields">
          <div class="provider-field-row">
            <label class="provider-field-label required-label" for="provider-name-input">{{
              t('providers.name')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-name-input"
                v-model="form.name"
                required
                autofocus
                :disabled="busy"
              />
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-website-input">{{
              t('providers.website')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-website-input"
                v-model="form.website"
                type="url"
                placeholder="https://example.com"
                :disabled="busy"
              />
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-openai-endpoint-input">{{
              t('providers.openaiEndpoint')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-openai-endpoint-input"
                v-model="form.openaiBaseUrl"
                type="url"
                placeholder="https://api.example.com/v1"
                spellcheck="false"
                :disabled="busy"
              />
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-anthropic-endpoint-input">{{
              t('providers.anthropicEndpoint')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-anthropic-endpoint-input"
                v-model="form.anthropicBaseUrl"
                type="url"
                placeholder="https://api.example.com/anthropic"
                spellcheck="false"
                :disabled="busy"
              />
            </div>
          </div>
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
            <div class="mapping-grid mapping-grid-head">
              <label class="mapping-check mapping-select-all">
                <input
                  type="checkbox"
                  :checked="allMappingsSelected"
                  :indeterminate="someMappingsSelected"
                  :disabled="busy"
                  :aria-label="t('providers.selectAllMappings')"
                  @change="toggleAllMappings"
                />
              </label>
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
          <div class="provider-field-row proxy-switch-row">
            <label class="provider-field-label" for="provider-proxy-enabled">{{
              t('providers.proxyEnabled')
            }}</label>
            <div class="provider-field-control proxy-switch-field">
              <span class="proxy-switch-control">
                <input
                  id="provider-proxy-enabled"
                  v-model="form.proxyEnabled"
                  type="checkbox"
                  role="switch"
                  :aria-label="t('providers.proxyEnabled')"
                  :title="t('providers.proxyHint')"
                  :disabled="busy"
                />
                <span class="proxy-switch-track" aria-hidden="true"></span>
              </span>
            </div>
          </div>
          <div v-if="form.proxyEnabled" class="proxy-fields">
            <div class="provider-field-row proxy-url-row">
              <div class="provider-field-label proxy-url-label">
                <label for="provider-proxy-url">{{ t('providers.proxyUrl') }}</label>
                <span
                  class="provider-field-help"
                  role="img"
                  tabindex="0"
                  :aria-label="t('providers.proxyUrlCredentialHint')"
                  :title="t('providers.proxyUrlCredentialHint')"
                  :data-tooltip="t('providers.proxyUrlCredentialHint')"
                  >i</span
                >
              </div>
              <div class="provider-field-control">
                <input
                  id="provider-proxy-url"
                  v-model="form.proxyUrl"
                  type="text"
                  placeholder="http://username:password@proxy.example.com:8080"
                  autocomplete="off"
                  spellcheck="false"
                  :disabled="busy"
                />
              </div>
            </div>
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
                <label class="proxy-header-field">
                  <span>{{ t('providers.proxyHeaderKey') }}</span>
                  <input
                    v-model="header.key"
                    spellcheck="false"
                    maxlength="128"
                    :disabled="busy"
                    placeholder="Proxy-Authorization"
                  />
                </label>
                <label class="proxy-header-field">
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
    :title="t('resources.configurationTitle')"
    :busy="busy"
    :wide="!credentialCreating"
    @close="closeCredential"
  >
    <template v-if="!credentialCreating">
      <div class="credential-list-head">
        <div>
          <h3>{{ credentialTarget.name }}</h3>
          <p>{{ t('resources.listHint') }}</p>
        </div>
        <button type="button" class="button primary" :disabled="busy" @click="addCredential">
          <Icon name="plus" :size="16" />{{ t('resources.addCredential') }}
        </button>
      </div>
      <dl class="credential-overview">
        <div>
          <dt>{{ t('resources.provider') }}</dt>
          <dd>{{ credentialTarget.name }}</dd>
        </div>
        <div>
          <dt>{{ t('common.status') }}</dt>
          <dd><Status :value="credentialTarget.status" /></dd>
        </div>
        <div class="credential-overview-endpoints">
          <dt>{{ t('resources.baseUrl') }}</dt>
          <dd>
            <span v-for="endpoint in credentialTarget.endpoints" :key="endpoint.protocolType">
              <b>{{ endpoint.protocolType === 'OPENAI' ? 'OpenAI' : 'Anthropic' }}</b>
              <code :title="endpoint.baseUrl">{{ endpoint.baseUrl }}</code>
            </span>
            <span v-if="!credentialTarget.endpoints.length">{{ t('common.none') }}</span>
          </dd>
        </div>
      </dl>
      <div v-if="resourcesFor(credentialTarget).length" class="credential-list">
        <table>
          <colgroup>
            <col class="credential-name-column" />
            <col class="credential-auth-column" />
            <col class="credential-runtime-column" />
            <col class="credential-error-column" />
            <col class="credential-operation-column" />
          </colgroup>
          <thead>
            <tr>
              <th>{{ t('resources.credential') }}</th>
              <th>{{ t('resources.authType') }}</th>
              <th>{{ t('resources.runtimeStatus') }}</th>
              <th>{{ t('resources.errorInfo') }}</th>
              <th class="credential-action-column">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="resource in resourcesFor(credentialTarget)" :key="resource.id">
              <td :data-label="t('resources.credential')">
                <strong>{{ resource.name }}</strong>
                <small v-if="resource.externalAccountRef" class="credential-detail">{{
                  resource.externalAccountRef
                }}</small>
                <small class="credential-detail">
                  {{ t('resources.lastVerifiedAt') }} ·
                  {{ date(lastCredentialVerification(resource)) }}
                </small>
              </td>
              <td class="credential-auth" :data-label="t('resources.authType')">
                {{ t(`resources.authTypes.${resource.authType || 'API_KEY'}`) }}
                <small v-if="resource.planCode" class="credential-detail">{{
                  resource.planCode
                }}</small>
                <small class="credential-detail credential-mask">
                  <code>{{ maskedCredential(resource) }}</code>
                </small>
                <Status
                  v-if="resource.authType === 'SUBSCRIPTION'"
                  :value="resource.quotaStatus || 'UNKNOWN'"
                />
              </td>
              <td class="credential-runtime" :data-label="t('resources.runtimeStatus')">
                <div class="credential-runtime-line">
                  <Status :value="resource.runtimeStatus || 'HEALTHY'" />
                  <button
                    type="button"
                    class="icon-button credential-verify-action"
                    :class="{ 'is-blocked': resource.runtimeStatus === 'BLOCKED' }"
                    :aria-label="verificationLabel(resource)"
                    :title="verificationLabel(resource)"
                    :disabled="busy"
                    @click="testCredentialFromModal(credentialTarget, resource)"
                  >
                    <Icon name="refresh" :size="15" />
                  </button>
                </div>
              </td>
              <td class="credential-error" :data-label="t('resources.errorInfo')">
                <template
                  v-if="
                    resource.runtimeStatus === 'BLOCKED' ||
                    resource.blockedReason ||
                    resource.lastErrorCode ||
                    resource.lastHttpStatus
                  "
                >
                  <span>{{ blockedResourceReason(resource) }}</span>
                  <small
                    v-if="resource.lastErrorCode"
                    class="credential-detail"
                    :title="resource.lastErrorCode"
                  >
                    {{ resource.lastErrorCode }}
                  </small>
                </template>
                <span v-else>{{ t('common.none') }}</span>
              </td>
              <td class="credential-action-column" :data-label="t('common.actions')">
                <button
                  type="button"
                  class="text-button danger"
                  :disabled="busy"
                  @click="credentialDeleteTarget = resource"
                >
                  {{ t('resources.delete') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="credential-empty">
        <Icon name="lock-open" :size="28" />
        <strong>{{ t('resources.emptyCredentials') }}</strong>
        <p>{{ t('resources.emptyCredentialsHint') }}</p>
      </div>
    </template>
    <form
      v-else
      id="credential-create-form"
      class="credential-create-form"
      @submit.prevent="saveCredential"
    >
      <p class="muted">{{ t('resources.createHint') }}</p>
      <div class="credential-form-row">
        <label class="credential-form-label" for="credential-auth-type">{{
          t('resources.authType')
        }}</label>
        <div class="credential-form-control">
          <select id="credential-auth-type" v-model="credentialAuthType" :disabled="busy">
            <option value="API_KEY">{{ t('resources.authTypes.API_KEY') }}</option>
            <option
              v-if="credentialTarget.authAdapters?.includes('OPENAI_CODEX')"
              value="SUBSCRIPTION"
            >
              {{ t('resources.authTypes.SUBSCRIPTION') }}
            </option>
          </select>
        </div>
      </div>
      <div class="credential-form-row">
        <label
          class="credential-form-label"
          :for="credentialAuthType === 'API_KEY' ? 'credential-api-key' : 'credential-auth-file'"
          >{{
            t(credentialAuthType === 'API_KEY' ? 'resources.apiKey' : 'resources.authFile')
          }}</label
        >
        <div class="credential-form-control">
          <input
            v-if="credentialAuthType === 'API_KEY'"
            id="credential-api-key"
            v-model="credential"
            type="password"
            autocomplete="new-password"
            required
            autofocus
            :disabled="busy"
            spellcheck="false"
          />
          <input
            v-else
            id="credential-auth-file"
            type="file"
            accept=".json,application/json"
            required
            :disabled="busy"
            @change="importSubscription"
          />
        </div>
      </div>
      <p class="field-hint credential-form-hint">
        {{
          t(
            credentialAuthType === 'API_KEY'
              ? 'resources.credentialHint'
              : 'resources.subscriptionHint',
          )
        }}
      </p>
    </form>
    <template #footer>
      <template v-if="credentialCreating">
        <button type="button" class="button" :disabled="busy" @click="cancelCredentialCreation">
          {{ t('common.cancel') }}</button
        ><button
          type="submit"
          form="credential-create-form"
          class="button primary"
          :disabled="busy"
        >
          {{ t(busy ? 'common.working' : 'common.save') }}
        </button>
      </template>
      <template v-else>
        <button type="button" class="button primary" :disabled="busy" @click="closeCredential">
          {{ t('common.save') }}
        </button>
      </template>
    </template>
  </Modal>

  <ConfirmDialog
    v-if="credentialDeleteTarget"
    :title="t('resources.deleteTitle')"
    :message="t('resources.deleteQuestion', { name: credentialDeleteTarget.name })"
    :hint="t('resources.deleteConsequence')"
    :confirm-label="t('resources.delete')"
    :busy="busy"
    tone="danger"
    @close="credentialDeleteTarget = null"
    @confirm="deleteCredential"
  />

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
        <dt>{{ t('resources.updatedModels') }}</dt>
        <dd>{{ syncResult.updated }}</dd>
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
</template>

<style scoped>
.provider-toolbar-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.provider-runtime-filter {
  display: flex;
  flex: none;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: #485e72;
  font-weight: 500;
  white-space: nowrap;
}
.provider-runtime-filter select {
  width: 120px;
  padding-top: 9px;
  padding-bottom: 9px;
  background: #fbfcfe;
  border-color: #dfe6ef;
  font-size: 12px;
}
.panel table {
  min-width: 840px;
  table-layout: fixed;
}
.panel th,
.panel td {
  padding-left: 14px;
  padding-right: 14px;
}
.provider-name-column {
  width: 230px;
}
.panel th:nth-child(2) {
  width: 90px;
}
.panel th:nth-child(3) {
  width: 125px;
}
.panel th:nth-child(4) {
  width: 64px;
}
.panel th:nth-child(5) {
  width: auto;
}
.panel th:nth-child(6) {
  width: 190px;
}
.panel th:nth-child(4),
.panel td:nth-child(4) {
  padding-left: 8px;
  padding-right: 8px;
}
.panel th:nth-child(2),
.panel td:nth-child(2) {
  padding-left: 12px;
  padding-right: 12px;
}
.panel th:nth-child(3),
.panel td:nth-child(3) {
  padding-left: 18px;
}
.provider-runtime-state {
  display: inline-flex;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  cursor: pointer;
}
.provider-runtime-state:hover:not(:disabled) {
  box-shadow: 0 0 0 2px #8eadd733;
}
.provider-runtime-state:focus-visible {
  outline: 2px solid #90b7fb;
  outline-offset: 2px;
}
.endpoint {
  display: block;
  min-width: 0;
  overflow-wrap: anywhere;
  white-space: normal;
}
.provider-name-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
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
.provider-website-action {
  border: 0;
  background: transparent;
}
.provider-website-action:hover:not(:disabled) {
  border: 0;
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
  align-items: start;
  gap: 8px;
}
.endpoint-protocol {
  display: inline-flex;
  min-height: 18px;
  align-items: center;
  justify-content: center;
  padding: 2px 5px;
  border-radius: 4px;
  color: #60788d;
  background: #eef3f7;
  font-size: 10px;
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
}
.proxy-column {
  text-align: center;
}
.proxy-access-state {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
}
.proxy-access-state.is-enabled {
  color: #20714f;
}
.proxy-access-state.is-direct {
  color: var(--muted);
}
.provider-runtime {
  min-width: 125px;
}
.provider-runtime .subline {
  max-width: 190px;
  font-size: 11px;
}
.credential-list-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}
.credential-list-head h3 {
  margin: 0;
  color: #183247;
  font-size: 15px;
}
.credential-list-head p {
  max-width: 430px;
  margin: 5px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.credential-overview {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0;
  margin: 0 0 16px;
  overflow: hidden;
  border: 1px solid #dce5ee;
  border-radius: 8px;
  background: #f8fafc;
}
.credential-overview > div {
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid #e3eaf1;
}
.credential-overview > div:nth-child(odd):not(.credential-overview-endpoints) {
  border-right: 1px solid #e3eaf1;
}
.credential-overview dt,
.credential-overview dd {
  min-width: 0;
  margin: 0;
}
.credential-overview dt {
  color: #687e92;
  font-size: 11px;
  font-weight: 600;
}
.credential-overview dd {
  color: #29445c;
  font-size: 12px;
}
.credential-overview-endpoints {
  grid-column: 1 / -1;
  border-bottom: 0 !important;
}
.credential-overview-endpoints dd {
  display: grid;
  gap: 6px;
}
.credential-overview-endpoints dd > span {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: 8px;
}
.credential-overview-endpoints b {
  color: #73889a;
  font-size: 10px;
  font-weight: 600;
}
.credential-overview-endpoints code {
  overflow: hidden;
  color: #38556f;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-list {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 8px;
}
.credential-list table {
  width: 100%;
  margin: 0;
  table-layout: fixed;
  white-space: normal;
}
.credential-name-column {
  width: 30%;
}
.credential-auth-column {
  width: 20%;
}
.credential-runtime-column {
  width: 17%;
}
.credential-operation-column {
  width: 64px;
}
.credential-list th,
.credential-list td {
  min-width: 0;
  padding: 11px 10px;
}
.credential-list td {
  white-space: normal;
}
.credential-list td > strong {
  overflow-wrap: anywhere;
}
.credential-auth .status {
  display: flex;
  width: max-content;
  max-width: 100%;
  margin: 5px 0 0;
  white-space: nowrap;
}
.credential-runtime .status {
  margin: 0;
  white-space: nowrap;
}
.credential-detail {
  display: block;
  max-width: 100%;
  margin-top: 4px;
  overflow: hidden;
  color: var(--muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-mask code {
  color: #536d84;
  letter-spacing: 0.04em;
}
.credential-error {
  color: #566d82;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.credential-error .credential-detail {
  max-width: 100%;
}
.credential-action-column {
  text-align: right;
}
.credential-runtime-line {
  display: flex;
  align-items: center;
  gap: 4px;
}
.credential-verify-action {
  width: 26px;
  height: 26px;
  color: var(--blue);
}
.credential-verify-action.is-blocked {
  color: #b46619;
}
.credential-empty {
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 42px 20px;
  border: 1px dashed #cad7e4;
  border-radius: 8px;
  color: #71869a;
  text-align: center;
}
.credential-empty strong {
  color: #385168;
  font-size: 14px;
}
.credential-empty p {
  margin: 0;
  font-size: 12px;
}
.credential-create-form {
  display: grid;
  gap: 14px;
}
.credential-create-form > .muted {
  margin: 0;
  line-height: 1.6;
}
.credential-form-row {
  display: grid;
  grid-template-columns: 86px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.credential-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: #485e72;
  line-height: 1.4;
  text-align: right;
}
.credential-form-control {
  min-width: 0;
}
.credential-form-hint {
  margin: -2px 0 0 100px;
}
.credential-create-form .form-footer {
  margin-top: 2px;
  padding-top: 16px;
}
.provider-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  white-space: nowrap;
}
.provider-more {
  position: relative;
  flex: none;
}
.provider-more summary {
  display: grid;
  width: 26px;
  height: 26px;
  place-items: center;
  border-radius: 5px;
  color: #60788d;
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  list-style: none;
}
.provider-more summary::-webkit-details-marker {
  display: none;
}
.provider-more summary:hover,
.provider-more summary:focus-visible {
  color: var(--blue);
  background: #edf4fc;
}
.provider-more summary:focus-visible {
  outline: 2px solid #90b7fb;
  outline-offset: 2px;
}
.provider-more-menu {
  position: absolute;
  z-index: 4;
  top: 50%;
  right: calc(100% + 4px);
  min-width: 80px;
  padding: 6px;
  border: 1px solid #d8e2ec;
  border-radius: 7px;
  background: #fff;
  box-shadow: 0 8px 24px #1832471f;
  transform: translateY(-50%);
}
.provider-more-menu .text-button {
  width: 100%;
  justify-content: flex-start;
  padding: 6px 8px;
}
.provider-form {
  --provider-field-label-width: 160px;

  display: grid;
  gap: 14px;
}
.provider-form label {
  margin: 0;
}
.connection-editor,
.config-editor,
.config-panel {
  min-width: 0;
}
.mapping-editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 8px;
}
.mapping-editor-head > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}
.mapping-editor-head p {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.provider-connection-hint {
  margin: 0 0 10px;
  padding: 7px 10px;
  border-left: 3px solid #8eadd7;
  border-radius: 5px;
  color: #60788d;
  background: #f5f8fc;
  font-size: 12px;
  line-height: 1.55;
}
.connection-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
}
.provider-field-row {
  display: grid;
  grid-template-columns: var(--provider-field-label-width) minmax(0, 1fr);
  align-items: start;
  gap: 12px;
}
.provider-field-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: #485e72;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.provider-field-control {
  min-width: 0;
}
.required-label::after {
  margin-left: 4px;
  color: var(--danger);
  content: '*';
}
.config-editor {
  margin-top: 0;
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
  padding: 9px 15px 10px;
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
  padding-top: 14px;
}
.config-panel:focus-visible {
  outline: 3px solid #90b7fb;
  outline-offset: 4px;
}
.proxy-editor {
  display: grid;
  grid-template-columns: var(--provider-field-label-width) minmax(0, 1fr);
  column-gap: 12px;
}
.proxy-switch-row {
  align-items: center;
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
}
.proxy-switch-row .provider-field-label {
  padding-top: 0;
  text-align: left;
}
.proxy-switch-field {
  display: flex;
  align-items: center;
  min-height: 40px;
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
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
  gap: 12px 0;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid #e4eaf1;
}
.proxy-fields > label {
  margin: 0;
}
.proxy-url-row {
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
}
.proxy-url-row .provider-field-label {
  text-align: left;
}
.proxy-url-label {
  display: flex;
  align-items: center;
  gap: 5px;
}
.provider-field-help {
  position: relative;
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 15px;
  height: 15px;
  border: 1px solid #8ca0b2;
  border-radius: 50%;
  color: #60788d;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  cursor: help;
}
.provider-field-help::after {
  position: absolute;
  top: 50%;
  left: calc(100% + 8px);
  z-index: 3;
  width: max-content;
  max-width: 220px;
  padding: 6px 8px;
  border-radius: 5px;
  background: #183247;
  box-shadow: 0 4px 12px #1832472b;
  color: #fff;
  content: attr(data-tooltip);
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
  opacity: 0;
  pointer-events: none;
  text-align: left;
  transform: translate(4px, -50%);
  transition:
    opacity 0.15s,
    transform 0.15s;
}
.provider-field-help:hover::after,
.provider-field-help:focus-visible::after {
  opacity: 1;
  transform: translate(0, -50%);
}
.provider-field-help:focus-visible {
  outline: 2px solid #90b7fb;
  outline-offset: 2px;
}
.proxy-headers-head {
  display: flex;
  grid-column: 1 / -1;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.proxy-headers-head > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
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
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
  column-gap: 12px;
  row-gap: 9px;
}
.proxy-header-row {
  display: grid;
  grid-column: 2;
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
.proxy-header-field {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
}
.proxy-header-field > span {
  display: block;
  margin: 0;
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
  overflow: clip;
  border: 1px solid #d8e2ec;
  border-radius: 9px;
}
.mapping-grid {
  display: grid;
  grid-template-columns: 46px minmax(200px, 0.8fr) minmax(300px, 1.4fr);
  align-items: center;
  gap: 12px;
  padding: 4px 10px;
  border-top: 1px solid #e5ebf1;
}
.mapping-grid-head {
  padding-top: 6px;
  padding-bottom: 6px;
  border-top: 0;
  background: #fff;
  color: #60788d;
  font-size: 11px;
  font-weight: 600;
}
.mapping-row {
  min-height: 44px;
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
.mapping-select-all {
  min-height: 24px;
}
.mapping-model-name {
  min-width: 0;
  overflow: hidden;
  color: #183247;
  font-size: 12px;
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
  padding: 5px 10px;
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
  .credential-overview {
    grid-template-columns: minmax(0, 1fr);
  }
  .credential-overview > div,
  .credential-overview > div:nth-child(odd):not(.credential-overview-endpoints) {
    grid-template-columns: 88px minmax(0, 1fr);
    border-right: 0;
  }
  .credential-overview-endpoints {
    grid-column: 1;
  }
  .credential-form-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }
  .credential-form-label {
    padding-top: 0;
    text-align: left;
  }
  .credential-form-hint {
    margin-left: 0;
  }
  .credential-list-head {
    align-items: stretch;
    flex-direction: column;
  }
  .credential-list-head .button {
    align-self: flex-start;
  }
  .credential-list {
    overflow: visible;
    border: 0;
    border-radius: 0;
  }
  .credential-list table,
  .credential-list tbody {
    display: block;
  }
  .credential-list colgroup,
  .credential-list thead {
    display: none;
  }
  .credential-list tr {
    display: grid;
    gap: 8px;
    margin-bottom: 10px;
    padding: 13px 14px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #fff;
  }
  .credential-list td,
  .credential-list .credential-action-column {
    display: grid;
    grid-template-columns: 88px minmax(0, 1fr);
    align-items: start;
    gap: 10px;
    width: auto;
    padding: 0;
    border: 0;
    text-align: left;
  }
  .credential-list td::before {
    content: attr(data-label);
    color: #687e92;
    font-size: 11px;
    font-weight: 500;
  }
  .credential-list .status {
    margin-top: 0;
  }
  .credential-list .text-button {
    justify-self: start;
  }
  .provider-runtime-filter {
    width: 100%;
  }
  .provider-runtime-filter select {
    flex: 1;
    width: auto;
  }
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
  .provider-field-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }
  .provider-field-label {
    padding-top: 0;
    text-align: left;
  }
  .proxy-editor,
  .proxy-fields,
  .proxy-header-list {
    grid-template-columns: minmax(0, 1fr);
  }
  .proxy-switch-row {
    grid-column: 1;
    grid-template-columns: auto 40px;
    gap: 12px;
  }
  .proxy-url-row {
    grid-column: 1;
    grid-template-columns: minmax(0, 1fr);
  }
  .proxy-headers-head {
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }
  .proxy-headers-head > div {
    flex-wrap: wrap;
  }
  .proxy-headers-head .button {
    align-self: flex-start;
  }
  .mapping-editor-head {
    align-items: stretch;
    flex-direction: column;
  }
  .mapping-editor-head > div {
    align-items: flex-start;
    flex-direction: column;
    gap: 2px;
  }
  .proxy-header-row {
    grid-column: 1;
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
