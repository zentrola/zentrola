<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, useId, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { all, api, download, errorText } from '../api'
import { date, useAction, useCollection, useListSearch, validText } from '../composables'
import { i18n, t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import type {
  ConnectionResult,
  Model,
  ModelSyncResult,
  Provider,
  ProviderDetail,
  ProviderInitializeOption,
  ProviderInitializeResult,
  ProviderNetworkScope,
  ProviderProtocol,
  Resource,
  ResourceQuota,
  RateLimitResetCredits,
  ResetCreditConsumeResult,
} from '../types'
import Icon from '../components/Icon.vue'
import GuideTour from '../components/InitializationGuide.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import Status from '../components/Status.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CredentialPriceEditor from '../components/CredentialPriceEditor.vue'
import TableScroll from '../components/TableScroll.vue'
import TechnicalValue from '../components/TechnicalValue.vue'

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
const createMenuOpen = ref(false)
const providerGuideOpen = ref(false)
const providerGuideAllSteps = [
  {
    key: 'create',
    target: '#provider-guide-create',
    icon: 'providers',
    titleKey: 'providers.guideSteps.create.title',
    descriptionKey: 'providers.guideSteps.create.description',
  },
  {
    key: 'website',
    target: '#provider-guide-website',
    icon: 'website',
    titleKey: 'providers.guideSteps.website.title',
    descriptionKey: 'providers.guideSteps.website.description',
  },
  {
    key: 'credential',
    target: '#provider-guide-credential',
    icon: 'key',
    titleKey: 'providers.guideSteps.credential.title',
    descriptionKey: 'providers.guideSteps.credential.description',
  },
  {
    key: 'sync',
    target: '#provider-guide-sync',
    icon: 'refresh',
    titleKey: 'providers.guideSteps.sync.title',
    descriptionKey: 'providers.guideSteps.sync.description',
  },
  {
    key: 'test',
    target: '#provider-guide-test',
    icon: 'activity',
    titleKey: 'providers.guideSteps.test.title',
    descriptionKey: 'providers.guideSteps.test.description',
  },
  {
    key: 'edit',
    target: '#provider-guide-edit',
    icon: 'edit',
    titleKey: 'providers.guideSteps.edit.title',
    descriptionKey: 'providers.guideSteps.edit.description',
  },
] as const
const createMenuRoot = ref<HTMLElement>()
const createMenuTrigger = ref<HTMLButtonElement>()
const createMenu = ref<HTMLElement>()
const createMenuId = useId()
const providerInitializeOpen = ref(false)
const providerInitializeOptions = ref<ProviderInitializeOption[]>([])
const selectedProviderCodes = ref<string[]>([])
const allProviderOptionsSelected = computed(
  () =>
    providerInitializeOptions.value.length > 0 &&
    selectedProviderCodes.value.length === providerInitializeOptions.value.length,
)
const editing = ref(false)
const editTarget = ref<Provider | null>(null)
const statusTarget = ref<Provider | null>(null)
const deleteTarget = ref<Provider | null>(null)
const resources = ref<Resource[]>([])
const resourceError = ref('')
const models = ref<Model[]>([])
const emptyResources: Resource[] = []
const resourcesByProvider = computed(() => {
  const grouped = new Map<string, Resource[]>()
  for (const resource of resources.value) {
    const group = grouped.get(resource.providerId)
    if (group) group.push(resource)
    else grouped.set(resource.providerId, [resource])
  }
  return grouped
})
function resourcePreference(resource: Resource) {
  if (
    resource.authType === 'SUBSCRIPTION' &&
    (resource.quotaStatus === 'AVAILABLE' || resource.quotaStatus === 'NEAR_LIMIT')
  )
    return 0
  return resource.authType === 'API_KEY' || !resource.authType ? 1 : 2
}
const preferredResourceByProvider = computed(() => {
  const preferred = new Map<string, Resource>()
  for (const [providerID, candidates] of resourcesByProvider.value) {
    let selected: Resource | undefined
    for (const candidate of candidates) {
      if (
        !selected ||
        resourcePreference(candidate) < resourcePreference(selected) ||
        (resourcePreference(candidate) === resourcePreference(selected) &&
          candidate.priority < selected.priority)
      )
        selected = candidate
    }
    if (selected) preferred.set(providerID, selected)
  }
  return preferred
})
const credentialTarget = ref<Provider | null>(null)
const priceTarget = ref<Resource | null>(null)
const credentialDeleteTarget = ref<Resource | null>(null)
const credentialCreating = ref(false)
const credentialAuthType = ref<'API_KEY' | 'SUBSCRIPTION'>('API_KEY')
type SubscriptionInputMode = 'UPLOAD' | 'PASTE'
const subscriptionInputMode = ref<SubscriptionInputMode>('UPLOAD')
const subscriptionFileName = ref('')
const codexSubscriptionUploadCommands = [
  { platform: 'macOS', command: 'open ~/.codex' },
  {
    platform: 'Windows PowerShell',
    command: 'explorer.exe "$env:USERPROFILE\\.codex"',
  },
  { platform: 'Linux', command: 'xdg-open ~/.codex' },
] as const
const codexSubscriptionPasteCommands = [
  { platform: 'macOS', command: 'pbcopy < ~/.codex/auth.json' },
  {
    platform: 'Windows PowerShell',
    command: 'Get-Content -Raw "$env:USERPROFILE\\.codex\\auth.json" | Set-Clipboard',
  },
  { platform: 'Linux', command: 'cat ~/.codex/auth.json' },
] as const
const claudeSubscriptionCommands = [{ platform: '', command: 'claude setup-token' }] as const
const subscriptionAdapter = computed(
  () => credentialTarget.value?.authAdapters?.find((adapter) => adapter !== 'API_KEY') ?? '',
)
const claudeSubscription = computed(() => subscriptionAdapter.value === 'ANTHROPIC_CLAUDE_CODE')
const subscriptionCommands = computed(() => {
  if (claudeSubscription.value) return claudeSubscriptionCommands
  return subscriptionInputMode.value === 'UPLOAD'
    ? codexSubscriptionUploadCommands
    : codexSubscriptionPasteCommands
})
const testSelectionTarget = ref<Provider | null>(null)
const testSelectionDetail = ref<ProviderDetail | null>(null)
const selectedTestResourceID = ref('')
const selectedTestProtocol = ref<ProviderProtocol | ''>('')
const selectedTestMappingID = ref('')
const testTarget = ref<{
  provider: Provider
  resource: Resource
  protocol?: ProviderProtocol
} | null>(null)
const testResult = ref<ConnectionResult | null>(null)
const credentialVerifiedAt = reactive<Record<string, string>>({})
const credentialQuotas = reactive<Record<string, ResourceQuota[]>>({})
const credentialResetCredits = reactive<Record<string, RateLimitResetCredits>>({})
const quotaRefreshing = reactive<Record<string, boolean>>({})
const quotaRefreshError = reactive<Record<string, string>>({})
const resetCreditConsuming = reactive<Record<string, boolean>>({})
const resetCreditIdempotencyKeys = reactive<Record<string, string>>({})
const syncTarget = ref<{ provider: Provider } | null>(null)
const syncResult = ref<ModelSyncResult | null>(null)
const credential = ref('')
const activeConfigTab = ref<'models' | 'proxy'>('models')
const modelConfigTab = ref<HTMLButtonElement | null>(null)
const proxyConfigTab = ref<HTMLButtonElement | null>(null)
const subscriptionUploadTab = ref<HTMLButtonElement | null>(null)
const subscriptionPasteTab = ref<HTMLButtonElement | null>(null)
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
const proxySchemes = ['http', 'https', 'socks5', 'socks5h'] as const
type ProxyScheme = (typeof proxySchemes)[number]
const form = reactive({
  name: '',
  website: '',
  anthropicBaseUrl: '',
  anthropicNetworkScope: 'PUBLIC' as ProviderNetworkScope,
  openaiBaseUrl: '',
  openaiNetworkScope: 'PUBLIC' as ProviderNetworkScope,
  proxyEnabled: false,
  proxyScheme: 'http' as ProxyScheme,
  proxyUrl: '',
  updateProxyCredentials: false,
  proxyHeaders: [] as ProxyHeaderDraft[],
  mappings: [] as MappingDraft[],
})
const mappingQuery = ref('')
const availableMappingModels = computed(() => models.value)
const mappingByModelID = computed(
  () => new Map(form.mappings.map((mapping) => [mapping.modelId, mapping])),
)
const mappingRows = computed(() =>
  availableMappingModels.value.map((model) => ({
    model,
    mapping: mappingByModelID.value.get(model.id),
  })),
)
const filteredMappingRows = computed(() => {
  const keyword = mappingQuery.value.trim().toLocaleLowerCase()
  return mappingRows.value.filter(
    (row) =>
      !keyword ||
      [
        row.model.name,
        row.model.code,
        row.model.publisherProviderName || '',
        row.mapping?.upstreamModelCode || '',
      ].some((value) => value.toLocaleLowerCase().includes(keyword)),
  )
})
const mappingFilterActive = computed(() => Boolean(mappingQuery.value.trim()))
const selectedMappingRows = computed(() => mappingRows.value.filter((row) => row.mapping))
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
  searching,
  searchingAll,
} = useListSearch(
  items,
  (provider) =>
    `${provider.name} ${provider.code} ${provider.website ?? ''} ${provider.endpoints.map((endpoint) => endpoint.baseUrl).join(' ')}`,
  () => '/providers',
  loading,
)

function validURL(value: string, endpoint = false, networkScope: ProviderNetworkScope = 'PUBLIC') {
  if (!value) return true
  try {
    const parsed = new URL(value)
    return (
      parsed.username === '' &&
      parsed.password === '' &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (endpoint
        ? networkScope === 'PRIVATE'
          ? ['http:', 'https:'].includes(parsed.protocol)
          : parsed.protocol === 'https:'
        : ['http:', 'https:'].includes(parsed.protocol))
    )
  } catch {
    return false
  }
}

function normalizeURL(value: string) {
  return value.trim().replace(/\/+$/, '')
}

function assignForm(provider: Provider | null, mappings: MappingDraft[] = []) {
  resetMappingFilters()
  editTarget.value = provider
  Object.assign(form, {
    name: provider?.name ?? '',
    website: provider?.website ?? '',
    anthropicBaseUrl:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'ANTHROPIC')?.baseUrl ?? '',
    anthropicNetworkScope:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'ANTHROPIC')?.networkScope ??
      'PUBLIC',
    openaiBaseUrl:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'OPENAI')?.baseUrl ?? '',
    openaiNetworkScope:
      provider?.endpoints.find((endpoint) => endpoint.protocolType === 'OPENAI')?.networkScope ??
      'PUBLIC',
    proxyEnabled: provider?.proxyEnabled ?? false,
    proxyScheme: proxySchemeFromURL(provider?.proxyUrl ?? ''),
    proxyUrl: provider?.proxyUrl ?? '',
    updateProxyCredentials: false,
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

function resetMappingFilters() {
  mappingQuery.value = ''
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
      ['http:', 'https:', 'socks5:', 'socks5h:'].includes(parsed.protocol) &&
      Boolean(parsed.hostname) &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (parsed.pathname === '' || parsed.pathname === '/')
    )
  } catch {
    return false
  }
}

function splitProxyAddress(value: string, fallback: ProxyScheme) {
  let address = value.trimStart()
  let scheme = fallback
  let match = /^(https?|socks5h?):\/\//i.exec(address)
  while (match) {
    const candidate = match[1].toLowerCase()
    if (proxySchemes.includes(candidate as ProxyScheme)) scheme = candidate as ProxyScheme
    address = address.slice(match[0].length)
    match = /^(https?|socks5h?):\/\//i.exec(address)
  }
  return { scheme, address }
}

function proxySchemeFromURL(value: string): ProxyScheme {
  return splitProxyAddress(value, 'http').scheme
}

const proxyAddress = computed({
  get: () => splitProxyAddress(form.proxyUrl, form.proxyScheme).address,
  set: (value: string) => {
    const parsed = splitProxyAddress(value, form.proxyScheme)
    form.proxyScheme = parsed.scheme
    form.proxyUrl = `${parsed.scheme}://${parsed.address}`
  },
})

function changeProxyScheme(event: Event) {
  const scheme = (event.target as HTMLSelectElement).value as ProxyScheme
  const address = proxyAddress.value
  form.proxyScheme = scheme
  form.proxyUrl = `${scheme}://${address}`
}

function proxyCredentialSignature(value: string | null) {
  if (!value) return ''
  try {
    const parsed = new URL(value.trim())
    if (!parsed.username && !parsed.password) return ''
    return `${parsed.username}\u0000${parsed.password}`
  } catch {
    return ''
  }
}

function proxyURLWithoutCredentials(value: string) {
  try {
    const parsed = new URL(value.trim())
    parsed.username = ''
    parsed.password = ''
    return parsed.toString().replace(/\/$/, '')
  } catch {
    return value
  }
}

function proxyURLWithOriginalCredentials(value: string, original: string | null) {
  if (!original) return value
  try {
    const parsed = new URL(value.trim())
    const originalParsed = new URL(original)
    parsed.username = originalParsed.username
    parsed.password = originalParsed.password
    return parsed.toString().replace(/\/$/, '')
  } catch {
    return value
  }
}

const canUpdateProxyCredentials = computed(() => Boolean(editTarget.value?.proxyEnabled))

function toggleProxyCredentialUpdate(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  form.updateProxyCredentials = checked
  form.proxyUrl = checked
    ? proxyURLWithoutCredentials(form.proxyUrl)
    : proxyURLWithOriginalCredentials(form.proxyUrl, editTarget.value?.proxyUrl ?? null)
}

function isSocksProxyURL(value: string) {
  try {
    return ['socks5:', 'socks5h:'].includes(new URL(value.trim()).protocol)
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
  actionError.value = ''
  void run(async () => {
    const detail = provider ? await api<ProviderDetail>(`/providers/${provider.id}`) : null
    if (detail) models.value = detail.models
    else await loadActiveModels()
    const modelIDs = new Set(models.value.map((model) => model.id))
    assignForm(
      detail,
      (detail?.mappings ?? [])
        .map((mapping) => ({
          modelId: mapping.modelId,
          upstreamModelCode: mapping.upstreamModelCode,
        }))
        .filter((mapping) => modelIDs.has(mapping.modelId))
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

function selectAllMappings() {
  const visibleModels = filteredMappingRows.value.map((row) => row.model)
  for (const model of visibleModels) {
    if (!form.mappings.some((mapping) => mapping.modelId === model.id)) {
      form.mappings.push({ modelId: model.id, upstreamModelCode: '' })
    }
  }
}

function invertMappingSelection() {
  for (const row of filteredMappingRows.value) toggleMapping(row.model)
}

function updateUpstreamModelCode(modelId: string, event: Event) {
  const mapping = form.mappings.find((candidate) => candidate.modelId === modelId)
  if (mapping) mapping.upstreamModelCode = (event.target as HTMLInputElement).value
}

function endpointURL(provider: Provider, protocolType: ProviderProtocol) {
  return (
    provider.endpoints.find((endpoint) => endpoint.protocolType === protocolType)?.baseUrl ?? ''
  )
}

function endpointNetworkScope(provider: Provider, protocolType: ProviderProtocol) {
  return (
    provider.endpoints.find((endpoint) => endpoint.protocolType === protocolType)?.networkScope ??
    'PUBLIC'
  )
}

function resourceFor(provider: Provider) {
  return preferredResourceByProvider.value.get(provider.id)
}

function resourcesFor(provider: Provider) {
  return resourcesByProvider.value.get(provider.id) ?? emptyResources
}

function providerEnableBlockReason(provider: Provider) {
  if (provider.status === 'ACTIVE') return undefined
  const credentialConfigured = Boolean(resourceFor(provider))
  const modelConfigured = provider.modelCount > 0
  if (!credentialConfigured && !modelConfigured)
    return t('providers.configurationRequiredBeforeEnable')
  if (!modelConfigured) return t('providers.modelRequiredBeforeEnable')
  if (!credentialConfigured) return t('providers.credentialRequiredBeforeEnable')
  return undefined
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

function subscriptionPriceLabel(resource: Resource) {
  const price = resource.subscriptionPrice
  if (!price) return '-'
  const symbol = price.currency === 'CNY' ? '¥' : '$'
  return `${symbol}${price.periodAmount} / ${t(`resources.pricing.periods.${price.billingPeriod}`)}`
}

function subscriptionPriceEffectiveDate(resource: Resource) {
  return resource.subscriptionPrice?.effectiveAt.slice(0, 10) ?? '-'
}

function calculateProviderRuntime(provider: Provider) {
  const configured = resourcesFor(provider)
  if (!configured.length) {
    return {
      status: 'UNCONFIGURED',
      reason: t('providers.runtimeReasons.UNCONFIGURED'),
      errorCode: '',
    }
  }

  if (provider.modelCount === 0) {
    return {
      status: 'MISSING_MODEL',
      reason: t('providers.modelRequiredBeforeEnable'),
      errorCode: '',
    }
  }

  let healthy = false
  let failed: Resource | undefined
  for (const resource of configured) {
    if (resource.runtimeStatus !== 'BLOCKED') {
      healthy = true
      continue
    }
    if (!failed || (resource.lastErrorAt || '') > (failed.lastErrorAt || '')) failed = resource
  }

  if (healthy && !failed) {
    return {
      status: provider.status === 'ACTIVE' ? 'HEALTHY' : 'PENDING_ENABLE',
      reason: '',
      errorCode: '',
    }
  }

  return {
    status: healthy ? 'DEGRADED' : 'BLOCKED',
    reason: failed ? blockedResourceReason(failed) : t('providers.runtimeReasons.UNKNOWN'),
    errorCode: failed?.lastErrorCode || '',
  }
}
const providerRuntimeByID = computed(
  () => new Map(items.value.map((provider) => [provider.id, calculateProviderRuntime(provider)])),
)

function providerRuntime(provider: Provider) {
  return providerRuntimeByID.value.get(provider.id) ?? calculateProviderRuntime(provider)
}

function providerRuntimeLabel(provider: Provider) {
  const runtime = providerRuntime(provider)
  const stateKey = `state.${runtime.status}`
  const status = i18n.global.te(stateKey) ? t(stateKey) : runtime.status
  const hint = [runtime.reason, runtime.errorCode].filter(Boolean).join(' · ')
  return hint ? `${status}：${hint}` : status
}

function providerRuntimeActionLabel(provider: Provider) {
  if (providerRuntime(provider).status === 'MISSING_MODEL') {
    return t('providers.configureModelsFromRuntime', { name: provider.name })
  }
  return t('providers.openCredentialsFromRuntime', {
    name: provider.name,
    status: providerRuntimeLabel(provider),
  })
}

function lastCredentialVerification(resource: Resource) {
  return credentialVerifiedAt[resource.id] || resource.quotaCheckedAt || resource.lastErrorAt
}

function providerRuntimeAbnormal(provider: Provider) {
  if (provider.status !== 'ACTIVE') return false
  const runtime = providerRuntime(provider)
  return ['UNCONFIGURED', 'MISSING_MODEL', 'BLOCKED'].includes(runtime.status)
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

const guideCredentialProviderID = computed(() => visible.value[0]?.id ?? '')
const guideWebsiteProviderID = computed(
  () => visible.value.find((provider) => provider.website)?.id ?? '',
)
const guideSyncProviderID = computed(
  () => visible.value.find((provider) => provider.modelSyncSupported)?.id ?? '',
)
const guideTestProviderID = computed(
  () =>
    visible.value.find((provider) => resourceFor(provider) && provider.modelCount > 0)?.id ?? '',
)
const providerGuideSteps = computed(() =>
  providerGuideAllSteps.filter((step) => {
    if (step.key === 'create') return true
    if (!guideCredentialProviderID.value) return false
    if (step.key === 'website') return Boolean(guideWebsiteProviderID.value)
    if (step.key === 'sync') return Boolean(guideSyncProviderID.value)
    if (step.key === 'test') return Boolean(guideTestProviderID.value)
    return true
  }),
)

function syncRuntimeFilterQuery(value: RuntimeFilter) {
  const nextQuery = { ...route.query }
  if (value) nextQuery.runtimeStatus = value
  else delete nextQuery.runtimeStatus
  void router.replace({ query: nextQuery })
}

function search() {
  appliedRuntimeFilter.value = runtimeFilter.value
  void searchKeyword(Boolean(appliedRuntimeFilter.value))
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

function displayableQuotas(resourceID: string) {
  return (credentialQuotas[resourceID] || []).filter(
    (quota) =>
      quota.remainingValue !== null ||
      quota.usedPercent !== null ||
      quota.limitValue !== null ||
      quota.usedValue !== null,
  )
}

function quotaWindowLabel(quota: ResourceQuota) {
  const seconds = quota.windowDurationSeconds
  let window = ''
  if (seconds && seconds % 86400 === 0)
    window = t('resources.quotaWindowDays', { value: seconds / 86400 })
  else if (seconds && seconds % 3600 === 0)
    window = t('resources.quotaWindowHours', { value: seconds / 3600 })
  else if (seconds && seconds % 60 === 0)
    window = t('resources.quotaWindowMinutes', { value: seconds / 60 })
  const defaultWindow = quota.code === 'codex.primary' || quota.code === 'codex.secondary'
  const name = quota.name?.trim() || (defaultWindow ? '' : quota.code)
  return [name, window].filter(Boolean).join(' · ') || quota.code
}

function quotaRemainingLabel(quota: ResourceQuota) {
  if (quota.remainingValue !== null) {
    return t('resources.quotaRemainingValue', {
      value: quota.remainingValue,
      unit: quota.unit ? ` ${quota.unit}` : '',
    })
  }
  if (quota.usedPercent !== null) {
    const value = new Intl.NumberFormat(i18n.global.locale.value, {
      maximumFractionDigits: 1,
    }).format(Math.max(0, 100 - quota.usedPercent))
    return t('resources.quotaRemainingPercent', { value })
  }
  if (quota.limitValue !== null && quota.usedValue !== null) {
    return t('resources.quotaUsageValue', {
      used: quota.usedValue,
      limit: quota.limitValue,
      unit: quota.unit ? ` ${quota.unit}` : '',
    })
  }
  return t('resources.quotaAmountUnknown')
}

async function refreshSubscriptionQuota(resource: Resource) {
  if (quotaRefreshing[resource.id]) return
  quotaRefreshing[resource.id] = true
  delete quotaRefreshError[resource.id]
  delete credentialQuotas[resource.id]
  delete credentialResetCredits[resource.id]
  try {
    const result = await api<ConnectionResult>(`/resources/${resource.id}/test-connection`, 'POST')
    if (!result.ok) {
      quotaRefreshError[resource.id] = resultMessage(result, resource)
      return
    }
    if (result.resetCredits) credentialResetCredits[resource.id] = result.resetCredits
    credentialQuotas[resource.id] = await api<ResourceQuota[]>(`/resources/${resource.id}/quotas`)
  } catch (error) {
    quotaRefreshError[resource.id] = errorText(error)
  } finally {
    delete quotaRefreshing[resource.id]
  }
}

async function consumeResetCredit(resource: Resource) {
  if (resetCreditConsuming[resource.id]) return
  resetCreditConsuming[resource.id] = true
  try {
    const idempotencyKey =
      resetCreditIdempotencyKeys[resource.id] ||
      (resetCreditIdempotencyKeys[resource.id] = crypto.randomUUID())
    const creditID = credentialResetCredits[resource.id]?.credits.find(
      (credit) => credit.status === 'available',
    )?.id
    const result = await api<ResetCreditConsumeResult>(
      `/resources/${resource.id}/rate-limit-reset-credit/consume`,
      'POST',
      {
        idempotencyKey,
        ...(creditID ? { creditId: creditID } : {}),
      },
    )
    if (result.resetCredits) credentialResetCredits[resource.id] = result.resetCredits
    credentialQuotas[resource.id] = await api<ResourceQuota[]>(`/resources/${resource.id}/quotas`)
    await loadResources()
    if (result.outcome === 'reset' || result.outcome === 'alreadyRedeemed') {
      showSuccessToast(t('resources.resetCreditConsumed'))
    } else {
      showErrorToast(t(`resources.resetCreditOutcomes.${result.outcome}`))
    }
    delete resetCreditIdempotencyKeys[resource.id]
  } catch (error) {
    showErrorToast(errorText(error))
  } finally {
    delete resetCreditConsuming[resource.id]
  }
}

async function refreshProviderQuotas(provider: Provider) {
  await loadResources()
  const subscriptions = resourcesFor(provider).filter(
    (resource) => resource.authType === 'SUBSCRIPTION',
  )
  await Promise.all(subscriptions.map(refreshSubscriptionQuota))
  if (subscriptions.length) await loadResources()
}

async function loadActiveModels() {
  models.value = await all<Model>('/models?status=ACTIVE')
}

function reload() {
  void retry()
  void loadResources()
}

function openProviderInitialization() {
  actionError.value = ''
  void run(async () => {
    const locale = i18n.global.locale.value
    providerInitializeOptions.value = await api<ProviderInitializeOption[]>(
      `/providers/initialize-options?locale=${encodeURIComponent(locale)}`,
    )
    selectedProviderCodes.value = providerInitializeOptions.value.map((option) => option.code)
    providerInitializeOpen.value = true
  })
}

function resetProviderInitialization() {
  providerInitializeOpen.value = false
  providerInitializeOptions.value = []
  selectedProviderCodes.value = []
}

function closeProviderInitialization() {
  if (busy.value) return
  resetProviderInitialization()
}

function toggleAllProviderOptions() {
  selectedProviderCodes.value = allProviderOptionsSelected.value
    ? []
    : providerInitializeOptions.value.map((option) => option.code)
}

function initializeProviders() {
  if (!selectedProviderCodes.value.length) return
  actionError.value = ''
  void run(async () => {
    const result = await api<ProviderInitializeResult>('/providers/initialize', 'POST', {
      locale: i18n.global.locale.value,
      providerCodes: selectedProviderCodes.value,
    })
    const message = t(
      result.created > 0 || result.updated > 0
        ? 'providers.initializeCompleted'
        : 'providers.initializeUnchanged',
      { created: result.created, updated: result.updated, total: result.total },
    )
    resetProviderInitialization()
    await load()
    showSuccessToast(message)
  })
}

function closeCreateMenu(restoreFocus = false) {
  createMenuOpen.value = false
  if (restoreFocus) createMenuTrigger.value?.focus()
}

async function showCreateMenu(last = false) {
  createMenuOpen.value = true
  await nextTick()
  const items = createMenu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  items?.[last ? items.length - 1 : 0]?.focus()
}

function selectCreateAction(action: 'thirdParty' | 'modelVendor') {
  closeCreateMenu(true)
  if (action === 'thirdParty') openEdit()
  else openProviderInitialization()
}

function onCreateMenuKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closeCreateMenu(true)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = Array.from(
    createMenu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [],
  )
  const current = items.indexOf(document.activeElement as HTMLButtonElement)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
  items[next]?.focus()
}

function onCreateMenuOutside(event: PointerEvent) {
  if (!createMenuRoot.value?.contains(event.target as Node)) closeCreateMenu()
}

function onCreateMenuFocusOut(event: FocusEvent) {
  if (!createMenuRoot.value?.contains(event.relatedTarget as Node | null)) closeCreateMenu()
}

function configureCredential(provider: Provider) {
  credentialTarget.value = provider
  credentialCreating.value = false
  credentialAuthType.value = 'API_KEY'
  subscriptionInputMode.value = 'UPLOAD'
  subscriptionFileName.value = ''
  credential.value = ''
  actionError.value = ''
  void refreshProviderQuotas(provider)
}

function addCredential() {
  credentialCreating.value = true
  credentialAuthType.value = 'API_KEY'
  subscriptionInputMode.value = 'UPLOAD'
  subscriptionFileName.value = ''
  credential.value = ''
}

function cancelCredentialCreation() {
  credentialCreating.value = false
  subscriptionInputMode.value = 'UPLOAD'
  subscriptionFileName.value = ''
  credential.value = ''
}

function resetCredentialInput() {
  subscriptionInputMode.value = 'UPLOAD'
  subscriptionFileName.value = ''
  credential.value = ''
}

function selectSubscriptionInputMode(mode: SubscriptionInputMode) {
  if (subscriptionInputMode.value === mode) return
  subscriptionInputMode.value = mode
  subscriptionFileName.value = ''
  credential.value = ''
}

async function focusSubscriptionInputMode(mode: SubscriptionInputMode) {
  selectSubscriptionInputMode(mode)
  await nextTick()
  const tab = mode === 'UPLOAD' ? subscriptionUploadTab.value : subscriptionPasteTab.value
  tab?.focus()
}

async function copySubscriptionCommand(command: string) {
  try {
    await navigator.clipboard.writeText(command)
    showSuccessToast(t('common.copied'))
  } catch {
    showErrorToast(t('common.copyFailed'))
  }
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
  priceTarget.value = null
  credentialCreating.value = false
  subscriptionInputMode.value = 'UPLOAD'
  subscriptionFileName.value = ''
  credential.value = ''
}

function validSubscriptionCredential(value: string) {
  const size = new TextEncoder().encode(value).length
  if (!value.trim() || size > 65536) return false
  if (claudeSubscription.value) {
    const token = value.trim()
    return token.length <= 4096 && /^[\x21-\x7e]+$/.test(token)
  }
  try {
    const parsed = JSON.parse(value)
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
  } catch {
    return false
  }
}

function subscriptionRequiredKey() {
  return claudeSubscription.value
    ? 'resources.claudeSubscriptionRequired'
    : 'resources.subscriptionRequired'
}

function saveCredential() {
  const valid =
    credentialAuthType.value === 'API_KEY'
      ? /^[\x21-\x7e]{1,4096}$/.test(credential.value)
      : validSubscriptionCredential(credential.value)
  if (!valid) {
    showErrorToast(
      t(
        credentialAuthType.value === 'API_KEY'
          ? 'resources.credentialRequired'
          : subscriptionRequiredKey(),
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
      authAdapter:
        credentialAuthType.value === 'SUBSCRIPTION' ? subscriptionAdapter.value : 'API_KEY',
    })
    await loadResources()
    credentialCreating.value = false
    subscriptionInputMode.value = 'UPLOAD'
    subscriptionFileName.value = ''
    credential.value = ''
    showSuccessToast(t('common.saved'))
  })
}

async function importSubscription(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (file.size > 65536) {
    showErrorToast(t(subscriptionRequiredKey()))
    input.value = ''
    subscriptionFileName.value = ''
    credential.value = ''
    return
  }
  credential.value = await file.text()
  subscriptionFileName.value = file.name
}

function exportableSubscription(resource: Resource) {
  return (
    resource.authType === 'SUBSCRIPTION' &&
    resource.authAdapter === 'OPENAI_CODEX' &&
    resource.subscriptionType === 'PERSONAL'
  )
}

function exportSubscription(resource: Resource) {
  void run(async () => {
    const blob = await download(`/resources/${resource.id}/credential/export`)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'auth.json'
    document.body.append(link)
    link.click()
    link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 0)
    showSuccessToast(t('resources.exported'))
  })
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

function preferredTestProtocol(provider: Provider): ProviderProtocol | undefined {
  return (
    provider.endpoints.find((endpoint) => endpoint.protocolType === 'ANTHROPIC') ??
    provider.endpoints.find((endpoint) => endpoint.protocolType === 'OPENAI')
  )?.protocolType
}

function testConnection(
  provider: Provider,
  resource: Resource,
  protocol = resource.authType === 'API_KEY' ? preferredTestProtocol(provider) : undefined,
  providerModelMappingID?: string,
) {
  void run(() => submitConnectionTest(provider, resource, protocol, providerModelMappingID))
}

async function submitConnectionTest(
  provider: Provider,
  resource: Resource,
  protocol?: ProviderProtocol,
  providerModelMappingID?: string,
) {
  testTarget.value = { provider, resource, protocol }
  testResult.value = null
  actionError.value = ''
  try {
    const query = new URLSearchParams()
    if (protocol) query.set('protocol', protocol)
    if (providerModelMappingID) query.set('providerModelMappingId', providerModelMappingID)
    const suffix = query.size ? `?${query.toString()}` : ''
    const result = await api<ConnectionResult>(
      `/resources/${resource.id}/test-connection${suffix}`,
      'POST',
    )
    if (result.resetCredits) credentialResetCredits[resource.id] = result.resetCredits
    credentialVerifiedAt[resource.id] = new Date().toISOString()
    await loadResources()
    testResult.value = result
  } catch (error) {
    testTarget.value = null
    throw error
  }
}

const selectedTestResource = computed(() => {
  if (!testSelectionTarget.value) return undefined
  return resourcesFor(testSelectionTarget.value).find(
    (candidate) => candidate.id === selectedTestResourceID.value,
  )
})
const testProtocolOptions = computed(() => {
  if (!testSelectionTarget.value) return []
  return [...testSelectionTarget.value.endpoints].sort((left, right) => {
    if (left.protocolType === right.protocolType) return 0
    return left.protocolType === 'ANTHROPIC' ? -1 : 1
  })
})
function testModelOptionsFor(detail: ProviderDetail) {
  return detail.mappings
    .map((mapping) => ({
      mapping,
      model: detail.models.find((model) => model.id === mapping.modelId),
    }))
    .filter((option) => option.model?.status === 'ACTIVE')
}
const testModelOptions = computed(() =>
  testSelectionDetail.value ? testModelOptionsFor(testSelectionDetail.value) : [],
)
const testCredentialSelectionRequired = computed(
  () => !!testSelectionTarget.value && resourcesFor(testSelectionTarget.value).length > 1,
)
const testProtocolSelectionRequired = computed(
  () => selectedTestResource.value?.authType === 'API_KEY' && testProtocolOptions.value.length > 1,
)
const testModelSelectionRequired = computed(
  () => selectedTestResource.value?.authType === 'API_KEY' && testModelOptions.value.length > 1,
)
const testSelectionReady = computed(
  () =>
    !!selectedTestResource.value &&
    (!testProtocolSelectionRequired.value || !!selectedTestProtocol.value) &&
    (!testModelSelectionRequired.value || !!selectedTestMappingID.value),
)

function preferredTestMappingID(detail: ProviderDetail) {
  const options = testModelOptionsFor(detail)
  const preferred =
    detail.code === 'google-gemini-official'
      ? options.find(({ mapping, model }) => {
          return (
            (mapping.upstreamModelCode || model?.code || '').toLowerCase() === 'gemini-3.6-flash'
          )
        })
      : undefined
  return preferred?.mapping.id ?? options[0]?.mapping.id ?? ''
}

function openTestSelection(provider: Provider, detail: ProviderDetail, resource?: Resource) {
  testSelectionTarget.value = provider
  testSelectionDetail.value = detail
  selectedTestResourceID.value = resource?.id ?? resourceFor(provider)?.id ?? ''
  selectedTestProtocol.value = preferredTestProtocol(provider) ?? ''
  selectedTestMappingID.value = preferredTestMappingID(detail)
}

function prepareTestConnection(provider: Provider, resource?: Resource) {
  const candidates = resourcesFor(provider)
  if (!candidates.length) return
  void run(async () => {
    const detail = await api<ProviderDetail>(`/providers/${provider.id}`)
    const selectedResource = resource ?? candidates[0]
    const mappingID =
      selectedResource.authType === 'API_KEY' ? preferredTestMappingID(detail) : undefined
    const selectionRequired =
      (!resource && candidates.length > 1) ||
      (selectedResource.authType === 'API_KEY' &&
        (provider.endpoints.length > 1 || testModelOptionsFor(detail).length > 1))
    if (selectionRequired) {
      openTestSelection(provider, detail, resource)
      return
    }
    await submitConnectionTest(
      provider,
      selectedResource,
      selectedResource.authType === 'API_KEY' ? preferredTestProtocol(provider) : undefined,
      mappingID,
    )
  })
}

function testProviderConnection(provider: Provider) {
  prepareTestConnection(provider)
}

function closeTestSelection() {
  testSelectionTarget.value = null
  testSelectionDetail.value = null
  selectedTestResourceID.value = ''
  selectedTestProtocol.value = ''
  selectedTestMappingID.value = ''
}

function testSelectedProviderCredential() {
  if (!testSelectionTarget.value) return
  const provider = testSelectionTarget.value
  const resource = resourcesFor(provider).find(
    (candidate) => candidate.id === selectedTestResourceID.value,
  )
  if (!resource) return
  const protocol =
    resource.authType === 'API_KEY' ? selectedTestProtocol.value || undefined : undefined
  const mappingID =
    resource.authType === 'API_KEY' ? selectedTestMappingID.value || undefined : undefined
  closeTestSelection()
  testConnection(provider, resource, protocol, mappingID)
}

function testCredentialFromModal(provider: Provider, resource: Resource) {
  closeCredential()
  prepareTestConnection(provider, resource)
}

function syncModels(provider: Provider) {
  syncTarget.value = { provider }
  syncResult.value = null
  actionError.value = ''
  void run(async () => {
    try {
      const result = await api<ModelSyncResult>(`/providers/${provider.id}/sync-models`, 'POST')
      syncResult.value = result
    } catch (error) {
      syncTarget.value = null
      throw error
    }
  })
}

function resultMessage(result: ConnectionResult, resource?: Resource) {
  if (result.ok) {
    return t(
      resource?.authType === 'SUBSCRIPTION'
        ? 'resources.subscriptionTestPassed'
        : 'resources.testPassed',
    )
  }
  if (result.code === 'CODEX_APP_SERVER_UNAVAILABLE') {
    return t('errors.CODEX_APP_SERVER_UNAVAILABLE', { executable: 'CODEX_EXECUTABLE' })
  }
  return t(i18n.global.te(`errors.${result.code}`) ? `errors.${result.code}` : 'errors.UNKNOWN')
}

function save() {
  let validation = ''
  const endpointDrafts: Array<{
    protocolType: ProviderProtocol
    baseUrl: string
    networkScope: ProviderNetworkScope
  }> = [
    {
      protocolType: 'OPENAI',
      baseUrl: normalizeURL(form.openaiBaseUrl),
      networkScope: form.openaiNetworkScope,
    },
    {
      protocolType: 'ANTHROPIC',
      baseUrl: normalizeURL(form.anthropicBaseUrl),
      networkScope: form.anthropicNetworkScope,
    },
  ]
  const socksProxy = form.proxyEnabled && isSocksProxyURL(form.proxyUrl)
  const input = {
    name: form.name.trim(),
    website: normalizeURL(form.website),
    endpoints: endpointDrafts.filter((endpoint) => endpoint.baseUrl),
    proxyEnabled: form.proxyEnabled,
    proxyUrl: form.proxyEnabled ? form.proxyUrl.trim() : '',
    updateProxyCredentials: form.proxyEnabled && (!editTarget.value || form.updateProxyCredentials),
    proxyHeaders:
      form.proxyEnabled && !socksProxy
        ? form.proxyHeaders.map((header) => ({ key: header.key.trim(), value: header.value }))
        : [],
    mappings: form.mappings.map((mapping) => ({
      modelId: mapping.modelId,
      upstreamModelCode: mapping.upstreamModelCode.trim(),
    })),
  }
  if (!validText(input.name, 128)) validation = t('common.byteLimit')
  else if (
    !validURL(input.website) ||
    input.endpoints.some((endpoint) => !validURL(endpoint.baseUrl, true, endpoint.networkScope))
  )
    validation = t('providers.urlInvalid')
  else if (!input.endpoints.length) validation = t('providers.endpointRequired')
  else if (input.proxyEnabled && !validProxyURL(input.proxyUrl))
    validation = t('providers.proxyUrlInvalid')
  else if (
    input.proxyEnabled &&
    editTarget.value?.proxyEnabled &&
    !form.updateProxyCredentials &&
    proxyCredentialSignature(input.proxyUrl) !== proxyCredentialSignature(editTarget.value.proxyUrl)
  )
    validation = t('providers.proxyCredentialsUpdateRequired')
  else if (
    input.proxyEnabled &&
    form.updateProxyCredentials &&
    !proxyCredentialSignature(input.proxyUrl)
  )
    validation = t('providers.proxyCredentialsRequired')
  else if (input.proxyEnabled && !socksProxy && !validProxyHeaders())
    validation = t('providers.proxyHeadersInvalid')
  else if (
    form.mappings.some(
      (mapping) =>
        !models.value.some((model) => model.id === mapping.modelId) ||
        (mapping.upstreamModelCode.trim() !== '' &&
          !validText(mapping.upstreamModelCode.trim(), 128)),
    )
  )
    validation = t('providers.mappingInvalid')
  else if (new Set(form.mappings.map((mapping) => mapping.modelId)).size !== form.mappings.length)
    validation = t('providers.mappingDuplicate')
  if (validation) {
    if (
      input.proxyEnabled &&
      (!validProxyURL(input.proxyUrl) || (!socksProxy && !validProxyHeaders()))
    )
      activeConfigTab.value = 'proxy'
    else if (
      form.mappings.some(
        (mapping) =>
          !models.value.some((model) => model.id === mapping.modelId) ||
          (mapping.upstreamModelCode.trim() !== '' &&
            !validText(mapping.upstreamModelCode.trim(), 128)),
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
  if (appliedRuntimeFilter.value) void searchKeyword(true)
  document.addEventListener('pointerdown', onCreateMenuOutside)
})
onUnmounted(() => document.removeEventListener('pointerdown', onCreateMenuOutside))
</script>

<template>
  <PageHeader name="providers">
    <button type="button" class="button" aria-haspopup="dialog" @click="providerGuideOpen = true">
      <Icon name="guide" :size="17" />{{ t('providers.guideAction') }}
    </button>
  </PageHeader>
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading || searching"
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
          <div ref="createMenuRoot" class="provider-create-menu" @focusout="onCreateMenuFocusOut">
            <button
              ref="createMenuTrigger"
              id="provider-guide-create"
              type="button"
              class="button primary provider-create-trigger"
              :disabled="busy"
              aria-haspopup="menu"
              :aria-expanded="createMenuOpen"
              :aria-controls="createMenuId"
              @click="createMenuOpen ? closeCreateMenu() : showCreateMenu()"
              @keydown.down.prevent="showCreateMenu()"
              @keydown.up.prevent="showCreateMenu(true)"
              @keydown.esc.prevent="closeCreateMenu(true)"
            >
              <Icon name="plus" :size="18" />
              <span>{{ t('providers.createMenu') }}</span>
              <Icon
                class="provider-create-chevron"
                :class="{ expanded: createMenuOpen }"
                name="arrow"
                :size="14"
              />
            </button>
            <div
              v-if="createMenuOpen"
              :id="createMenuId"
              ref="createMenu"
              class="provider-create-dropdown"
              role="menu"
              :aria-label="t('providers.createMenu')"
              @keydown="onCreateMenuKeydown"
            >
              <button
                type="button"
                role="menuitem"
                tabindex="-1"
                class="provider-create-option"
                @click="selectCreateAction('thirdParty')"
              >
                <span class="provider-create-option-icon">
                  <Icon name="connection" :size="17" />
                </span>
                <span>
                  <strong>{{ t('providers.thirdPartyProvider') }}</strong>
                  <small>{{ t('providers.thirdPartyProviderHint') }}</small>
                </span>
              </button>
              <button
                type="button"
                role="menuitem"
                tabindex="-1"
                class="provider-create-option"
                @click="selectCreateAction('modelVendor')"
              >
                <span class="provider-create-option-icon"><Icon name="models" :size="17" /></span>
                <span>
                  <strong>{{ t('providers.modelVendor') }}</strong>
                  <small>{{ t('providers.modelVendorHint') }}</small>
                </span>
              </button>
            </div>
          </div>
        </div>
      </template>
    </ListSearch>
    <p v-if="error || resourceError" class="alert error" role="alert">
      {{ error || resourceError
      }}<button class="text-button" @click="reload">
        {{ t('common.retry') }}
      </button>
    </p>
    <TableScroll has-actions>
      <table class="providers-table">
        <colgroup>
          <col class="provider-id-column" />
          <col class="provider-name-column" />
          <col class="provider-enable-column" />
          <col class="provider-runtime-column" />
          <col class="provider-proxy-column" />
          <col class="provider-endpoint-column" />
          <col class="provider-action-column" />
        </colgroup>
        <thead>
          <tr>
            <th>{{ t('common.id') }}</th>
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
            <td class="record-id-cell">
              <TechnicalValue :value="provider.id" :copyable="false" />
            </td>
            <td>
              <div class="person">
                <span class="avatar">{{ provider.name.slice(0, 1) }}</span>
                <div>
                  <span class="provider-name-line"
                    ><strong>{{ provider.name }}</strong></span
                  >
                  <TechnicalValue :value="provider.code" :copyable="false" muted />
                </div>
                <span class="provider-name-actions"
                  ><a
                    v-if="provider.website"
                    :id="
                      provider.id === guideWebsiteProviderID ? 'provider-guide-website' : undefined
                    "
                    class="provider-quick-action provider-website-action"
                    :href="provider.website"
                    target="_blank"
                    rel="noopener noreferrer"
                    :aria-label="t('providers.openWebsiteFor', { name: provider.name })"
                    :title="t('providers.visitWebsite')"
                  >
                    <Icon name="website" :size="15" /></a
                  ><button
                    v-if="resourceFor(provider) && provider.modelCount > 0"
                    type="button"
                    class="provider-quick-action provider-direct-action"
                    :id="provider.id === guideTestProviderID ? 'provider-guide-test' : undefined"
                    :aria-label="t('providers.testConnectionFor', { name: provider.name })"
                    :title="t('resources.test')"
                    :disabled="busy"
                    @click="testProviderConnection(provider)"
                  >
                    <Icon name="activity" :size="16" /></button
                  ><button
                    v-if="provider.modelSyncSupported"
                    type="button"
                    class="provider-quick-action provider-direct-action"
                    :id="provider.id === guideSyncProviderID ? 'provider-guide-sync' : undefined"
                    :aria-label="t('providers.syncModelsFor', { name: provider.name })"
                    :title="t('resources.syncModels')"
                    :disabled="busy"
                    @click="syncModels(provider)"
                  >
                    <Icon name="refresh" :size="16" /></button
                ></span>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="provider.status"
                :name="provider.name"
                :disabled="busy || loading || Boolean(providerEnableBlockReason(provider))"
                :busy="busy && statusTarget?.id === provider.id"
                :title="providerEnableBlockReason(provider)"
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
                @click="
                  providerRuntime(provider).status === 'MISSING_MODEL'
                    ? openEdit(provider)
                    : configureCredential(provider)
                "
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
                  ><TechnicalValue class="endpoint" :value="endpointURL(provider, 'OPENAI')" /><span
                    v-if="endpointNetworkScope(provider, 'OPENAI') === 'PRIVATE'"
                    class="endpoint-private-badge"
                    >{{ t('providers.networkPrivate') }}</span
                  ></span
                ><span v-if="endpointURL(provider, 'ANTHROPIC')"
                  ><span class="endpoint-protocol">Anthropic</span
                  ><TechnicalValue
                    class="endpoint"
                    :value="endpointURL(provider, 'ANTHROPIC')"
                  /><span
                    v-if="endpointNetworkScope(provider, 'ANTHROPIC') === 'PRIVATE'"
                    class="endpoint-private-badge"
                    >{{ t('providers.networkPrivate') }}</span
                  ></span
                >
              </div>
            </td>
            <td class="align-right">
              <div class="provider-actions">
                <button
                  :id="
                    provider.id === guideCredentialProviderID ? 'provider-guide-edit' : undefined
                  "
                  class="text-button"
                  :disabled="busy"
                  @click="openEdit(provider)"
                >
                  {{ t('providers.edit') }}
                </button>
                <button
                  type="button"
                  class="text-button"
                  :disabled="busy"
                  :id="
                    provider.id === guideCredentialProviderID
                      ? 'provider-guide-credential'
                      : undefined
                  "
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
      <button
        v-if="!loading"
        type="button"
        class="button primary empty-state-action"
        @click="query || appliedRuntimeFilter ? reset() : openEdit()"
      >
        <Icon :name="query || appliedRuntimeFilter ? 'refresh' : 'plus'" :size="16" />
        {{ t(query || appliedRuntimeFilter ? 'common.reset' : 'providers.create') }}
      </button>
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
    v-if="providerInitializeOpen"
    :title="t('providers.initializeTitle')"
    :busy="busy"
    medium
    @close="closeProviderInitialization"
  >
    <p class="muted provider-initialize-hint">{{ t('providers.initializeHint') }}</p>
    <div class="provider-initialize-toolbar">
      <span>
        {{
          t('providers.initializeSelectionCount', {
            count: selectedProviderCodes.length,
            total: providerInitializeOptions.length,
          })
        }}
      </span>
      <button type="button" class="text-button" :disabled="busy" @click="toggleAllProviderOptions">
        {{ t(allProviderOptionsSelected ? 'providers.clearSelection' : 'providers.selectAll') }}
      </button>
    </div>
    <div
      class="provider-initialize-options"
      role="group"
      :aria-label="t('providers.initializeSelection')"
    >
      <label
        v-for="option in providerInitializeOptions"
        :key="option.code"
        class="provider-initialize-option"
        :class="{ 'is-selected': selectedProviderCodes.includes(option.code) }"
      >
        <input
          v-model="selectedProviderCodes"
          type="checkbox"
          :value="option.code"
          :disabled="busy"
          :aria-label="t('providers.initializeProviderSelection', { name: option.name })"
        />
        <span>
          <strong>{{ option.name }}</strong>
          <small>{{ option.website }}</small>
        </span>
      </label>
    </div>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="closeProviderInitialization">
        {{ t('common.cancel') }}</button
      ><button
        type="button"
        class="button primary"
        :disabled="busy || !selectedProviderCodes.length"
        @click="initializeProviders"
      >
        {{ t(busy ? 'common.working' : 'providers.initializeSelected') }}
      </button>
    </template>
  </Modal>

  <Modal
    v-if="editing"
    :title="t(editTarget ? 'providers.editTitle' : 'providers.create')"
    :busy="busy"
    :body-class="activeConfigTab === 'models' ? 'provider-model-modal-body' : undefined"
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
              <div class="endpoint-editor">
                <input
                  id="provider-openai-endpoint-input"
                  v-model="form.openaiBaseUrl"
                  type="url"
                  :placeholder="
                    form.openaiNetworkScope === 'PRIVATE'
                      ? 'http://192.168.1.20:8080/v1'
                      : 'https://api.example.com/v1'
                  "
                  spellcheck="false"
                  :disabled="busy"
                />
                <select
                  v-model="form.openaiNetworkScope"
                  :aria-label="t('providers.networkScopeFor', { protocol: 'OpenAI' })"
                  :disabled="busy"
                >
                  <option value="PUBLIC">{{ t('providers.networkPublic') }}</option>
                  <option value="PRIVATE">{{ t('providers.networkPrivate') }}</option>
                </select>
              </div>
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-anthropic-endpoint-input">{{
              t('providers.anthropicEndpoint')
            }}</label>
            <div class="provider-field-control">
              <div class="endpoint-editor">
                <input
                  id="provider-anthropic-endpoint-input"
                  v-model="form.anthropicBaseUrl"
                  type="url"
                  :placeholder="
                    form.anthropicNetworkScope === 'PRIVATE'
                      ? 'http://192.168.1.20:8080/anthropic'
                      : 'https://api.example.com/anthropic'
                  "
                  spellcheck="false"
                  :disabled="busy"
                />
                <select
                  v-model="form.anthropicNetworkScope"
                  :aria-label="t('providers.networkScopeFor', { protocol: 'Anthropic' })"
                  :disabled="busy"
                >
                  <option value="PUBLIC">{{ t('providers.networkPublic') }}</option>
                  <option value="PRIVATE">{{ t('providers.networkPrivate') }}</option>
                </select>
              </div>
            </div>
          </div>
          <p
            v-if="form.openaiNetworkScope === 'PRIVATE' || form.anthropicNetworkScope === 'PRIVATE'"
            class="private-network-warning"
            role="note"
          >
            {{ t('providers.privateNetworkWarning') }}
          </p>
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
            <p>{{ t('providers.mappingHint') }}</p>
          </header>
          <div v-if="availableMappingModels.length" class="mapping-workspace">
            <section
              class="mapping-pane mapping-catalog-pane"
              aria-labelledby="mapping-catalog-title"
            >
              <header class="mapping-pane-head">
                <span class="mapping-pane-title">
                  <strong id="mapping-catalog-title">{{ t('providers.mappingListTitle') }}</strong>
                  <span class="mapping-count-tag">{{
                    mappingFilterActive
                      ? t('providers.filteredMappingCount', {
                          count: filteredMappingRows.length,
                        })
                      : availableMappingModels.length
                  }}</span>
                </span>
                <span class="mapping-bulk-actions">
                  <button
                    type="button"
                    class="text-button mapping-bulk-button"
                    :disabled="busy || !filteredMappingRows.length"
                    @click="selectAllMappings"
                  >
                    {{ t('providers.selectAll') }}
                  </button>
                  <span class="mapping-action-separator" aria-hidden="true">/</span>
                  <button
                    type="button"
                    class="text-button mapping-bulk-button"
                    :disabled="busy || !filteredMappingRows.length"
                    @click="invertMappingSelection"
                  >
                    {{ t('providers.invertSelection') }}
                  </button>
                </span>
              </header>
              <div class="mapping-toolbar">
                <div class="mapping-search-box">
                  <input
                    id="provider-mapping-search"
                    v-model="mappingQuery"
                    type="search"
                    :aria-label="t('providers.mappingSearch')"
                    :placeholder="t('providers.mappingSearchPlaceholder')"
                    :disabled="busy"
                  />
                  <button
                    v-if="mappingQuery"
                    type="button"
                    class="mapping-search-clear"
                    :aria-label="t('providers.clearMappingSearch')"
                    :disabled="busy"
                    @click="mappingQuery = ''"
                  >
                    <Icon name="close" :size="14" />
                  </button>
                </div>
              </div>
              <div v-if="filteredMappingRows.length" class="mapping-catalog-list">
                <div
                  v-for="row in filteredMappingRows"
                  :key="row.model.id"
                  class="mapping-catalog-grid mapping-catalog-row"
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
                  <span class="mapping-model-summary">
                    <strong class="mapping-model-name model-name-regular">{{
                      row.model.name
                    }}</strong>
                    <span>{{ row.model.code }}</span>
                  </span>
                </div>
              </div>
              <p v-else class="mapping-empty mapping-filter-empty">
                {{ t('providers.noMatchingMappings') }}
                <button type="button" class="text-button" @click="resetMappingFilters">
                  {{ t('providers.clearMappingFilters') }}
                </button>
              </p>
            </section>
            <section
              class="mapping-pane mapping-selected-pane"
              :aria-label="t('providers.selectedMappingsTitle')"
            >
              <header class="mapping-pane-head mapping-selected-head">
                <strong>{{ t('providers.logicalModel') }}</strong>
                <strong>{{ t('providers.upstreamModelCode') }}</strong>
                <span aria-hidden="true"></span>
              </header>
              <div v-if="selectedMappingRows.length" class="mapping-selected-list">
                <div
                  v-for="row in selectedMappingRows"
                  :key="row.model.id"
                  class="mapping-selected-row"
                >
                  <span class="mapping-model-summary mapping-selected-model">
                    <strong class="mapping-model-name model-name-regular">{{
                      row.model.name
                    }}</strong>
                    <span>{{ row.model.code }}</span>
                  </span>
                  <label class="mapping-control">
                    <input
                      :value="row.mapping?.upstreamModelCode ?? ''"
                      spellcheck="false"
                      :disabled="busy"
                      :placeholder="
                        t('providers.upstreamModelPlaceholder', { code: row.model.code })
                      "
                      :aria-label="t('providers.mappingCodeFor', { name: row.model.name })"
                      @input="updateUpstreamModelCode(row.model.id, $event)"
                    />
                  </label>
                  <button
                    type="button"
                    class="mapping-remove"
                    :disabled="busy"
                    :aria-label="t('providers.removeMappingFor', { name: row.model.name })"
                    @click="toggleMapping(row.model)"
                  >
                    <Icon name="close" :size="15" />
                  </button>
                </div>
              </div>
              <p v-else class="mapping-empty mapping-selected-empty">
                {{ t('providers.selectedMappingsEmpty') }}
              </p>
            </section>
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
              <div class="provider-field-control proxy-url-control">
                <select
                  id="provider-proxy-scheme"
                  :value="form.proxyScheme"
                  :aria-label="t('providers.proxyProtocol')"
                  :disabled="busy"
                  @change="changeProxyScheme"
                >
                  <option value="http">HTTP</option>
                  <option value="https">HTTPS</option>
                  <option value="socks5">SOCKS5</option>
                  <option value="socks5h">SOCKS5H</option>
                </select>
                <input
                  id="provider-proxy-url"
                  v-model="proxyAddress"
                  type="text"
                  placeholder="username:password@proxy.example.com:1080"
                  autocomplete="off"
                  spellcheck="false"
                  :disabled="busy"
                />
              </div>
            </div>
            <div
              v-if="canUpdateProxyCredentials"
              class="provider-field-row proxy-switch-row proxy-credential-update-row"
            >
              <label class="provider-field-label" for="provider-proxy-credentials-update">{{
                t('providers.proxyCredentialsUpdate')
              }}</label>
              <div class="provider-field-control proxy-switch-field">
                <span class="proxy-switch-control">
                  <input
                    id="provider-proxy-credentials-update"
                    :checked="form.updateProxyCredentials"
                    type="checkbox"
                    role="switch"
                    :aria-label="t('providers.proxyCredentialsUpdate')"
                    :title="t('providers.proxyCredentialsUpdateHint')"
                    :disabled="busy"
                    @change="toggleProxyCredentialUpdate"
                  />
                  <span class="proxy-switch-track" aria-hidden="true"></span>
                </span>
              </div>
            </div>
            <p v-if="isSocksProxyURL(form.proxyUrl)" class="proxy-socks-hint">
              {{ t('providers.socksProxyHint') }}
            </p>
            <template v-else>
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
            </template>
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
    v-if="testSelectionTarget"
    :title="
      t(
        testCredentialSelectionRequired
          ? 'resources.selectTestCredentialTitle'
          : testProtocolSelectionRequired
            ? 'resources.selectTestProtocolTitle'
            : 'resources.selectTestModelTitle',
        { name: testSelectionTarget.name },
      )
    "
    :busy="busy"
    medium
    @close="closeTestSelection"
  >
    <section v-if="testCredentialSelectionRequired" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestCredentialHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestCredential')"
      >
        <label
          v-for="resource in resourcesFor(testSelectionTarget)"
          :key="resource.id"
          class="credential-test-option"
          :class="{ 'is-selected': selectedTestResourceID === resource.id }"
        >
          <input
            v-model="selectedTestResourceID"
            type="radio"
            name="provider-test-credential"
            :value="resource.id"
            :aria-label="t('resources.selectCredentialForTest', { name: resource.name })"
          />
          <span class="credential-test-option-main">
            <strong>{{ resource.name }}</strong>
            <small>{{ t(`resources.authTypes.${resource.authType || 'API_KEY'}`) }}</small>
          </span>
          <span class="credential-test-option-status">
            <Status :value="resource.runtimeStatus || 'HEALTHY'" />
            <Status
              v-if="resource.authType === 'SUBSCRIPTION'"
              :value="resource.quotaStatus || 'UNKNOWN'"
            />
          </span>
        </label>
      </div>
    </section>
    <section v-if="testProtocolSelectionRequired" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestProtocolHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestProtocol')"
      >
        <label
          v-for="endpoint in testProtocolOptions"
          :key="endpoint.protocolType"
          class="credential-test-option"
          :class="{ 'is-selected': selectedTestProtocol === endpoint.protocolType }"
        >
          <input
            v-model="selectedTestProtocol"
            type="radio"
            name="provider-test-protocol"
            :value="endpoint.protocolType"
            :aria-label="
              t('resources.selectProtocolForTest', {
                protocol: endpoint.protocolType === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI',
              })
            "
          />
          <span class="credential-test-option-main">
            <strong>{{ endpoint.protocolType === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI' }}</strong>
            <TechnicalValue :value="endpoint.baseUrl" :copyable="false" muted />
          </span>
        </label>
      </div>
    </section>
    <section v-if="testModelSelectionRequired" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestModelHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestModel')"
      >
        <label
          v-for="option in testModelOptions"
          :key="option.mapping.id"
          class="credential-test-option"
          :class="{ 'is-selected': selectedTestMappingID === option.mapping.id }"
        >
          <input
            v-model="selectedTestMappingID"
            type="radio"
            name="provider-test-model"
            :value="option.mapping.id"
            :aria-label="
              t('resources.selectModelForTest', {
                name:
                  option.model?.name ||
                  option.mapping.upstreamModelCode ||
                  option.model?.code ||
                  t('common.none'),
              })
            "
          />
          <span class="credential-test-option-main">
            <strong>{{ option.model?.name || option.mapping.upstreamModelCode || '-' }}</strong>
            <TechnicalValue
              :value="option.mapping.upstreamModelCode || option.model?.code || '-'"
              :copyable="false"
              muted
            />
          </span>
        </label>
      </div>
    </section>
    <footer class="form-footer">
      <button type="button" class="button" :disabled="busy" @click="closeTestSelection">
        {{ t('common.cancel') }}
      </button>
      <button
        type="button"
        class="button primary"
        :disabled="busy || !testSelectionReady"
        @click="testSelectedProviderCredential"
      >
        {{ t('resources.startTest') }}
      </button>
    </footer>
  </Modal>

  <Modal
    v-if="credentialTarget && !priceTarget"
    :title="t('resources.configurationTitle')"
    :busy="busy"
    :wide="!credentialCreating"
    @close="closeCredential"
  >
    <template v-if="!credentialCreating">
      <div class="credential-list-head">
        <div class="credential-list-heading">
          <div class="credential-title-line">
            <h3>{{ credentialTarget.name }}</h3>
            <Status :value="credentialTarget.status" />
          </div>
          <p>{{ t('resources.listHint') }}</p>
        </div>
        <button type="button" class="button primary" :disabled="busy" @click="addCredential">
          <Icon name="plus" :size="16" />{{ t('resources.addCredential') }}
        </button>
      </div>
      <dl class="credential-overview">
        <div class="credential-overview-endpoints">
          <dt>{{ t('resources.baseUrl') }}</dt>
          <dd>
            <span v-for="endpoint in credentialTarget.endpoints" :key="endpoint.protocolType">
              <b>{{ endpoint.protocolType === 'OPENAI' ? 'OpenAI' : 'Anthropic' }}</b>
              <span v-if="endpoint.networkScope === 'PRIVATE'" class="endpoint-private-badge">{{
                t('providers.networkPrivate')
              }}</span>
              <TechnicalValue :value="endpoint.baseUrl" />
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
            <col class="credential-operation-column" />
          </colgroup>
          <thead>
            <tr>
              <th>{{ t('resources.credential') }}</th>
              <th>{{ t('resources.authType') }}</th>
              <th>{{ t('resources.runtimeStatus') }}</th>
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
                <div class="credential-auth-content">
                  <span v-if="resource.authType !== 'SUBSCRIPTION'">
                    {{ t(`resources.authTypes.${resource.authType || 'API_KEY'}`) }}
                  </span>
                  <small v-if="resource.planCode" class="credential-detail credential-plan">
                    {{ t('resources.plan') }} · {{ resource.planCode }}
                  </small>
                  <div
                    v-if="resource.authType === 'SUBSCRIPTION'"
                    class="credential-subscription-price"
                  >
                    <small class="credential-detail">
                      {{ t('resources.pricing.currentSubscription') }} ·
                      <strong>{{ subscriptionPriceLabel(resource) }}</strong>
                    </small>
                    <small class="credential-detail">
                      {{ t('resources.pricing.effectiveDate') }} ·
                      {{ subscriptionPriceEffectiveDate(resource) }}
                    </small>
                  </div>
                  <small
                    v-if="resource.authType === 'SUBSCRIPTION' && quotaRefreshing[resource.id]"
                    class="credential-detail credential-quota-feedback"
                  >
                    {{ t('resources.quotaRefreshing') }}
                  </small>
                  <small
                    v-else-if="
                      resource.authType === 'SUBSCRIPTION' && quotaRefreshError[resource.id]
                    "
                    class="credential-detail credential-quota-feedback is-error"
                    :title="quotaRefreshError[resource.id]"
                  >
                    {{ t('resources.quotaRefreshFailed') }} · {{ quotaRefreshError[resource.id] }}
                  </small>
                  <div
                    v-else-if="
                      resource.authType === 'SUBSCRIPTION' && displayableQuotas(resource.id).length
                    "
                    class="credential-quota-windows"
                  >
                    <div
                      v-for="quota in displayableQuotas(resource.id)"
                      :key="quota.code"
                      class="credential-quota-window"
                    >
                      <small class="credential-detail">
                        {{ quotaWindowLabel(quota) }} · {{ quotaRemainingLabel(quota) }}
                      </small>
                      <small class="credential-detail credential-quota-reset">
                        {{ t('resources.quotaResetsAt') }} · {{ date(quota.resetsAt) }}
                      </small>
                    </div>
                  </div>
                  <template v-else-if="resource.authType === 'SUBSCRIPTION'">
                    <small v-if="credentialQuotas[resource.id]" class="credential-detail">
                      {{ t('resources.quotaAmountUnknown') }}
                    </small>
                    <small class="credential-detail credential-quota-reset">
                      {{ t('resources.quotaResetsAt') }} · {{ date(resource.quotaResetsAt) }}
                    </small>
                  </template>
                  <div
                    v-if="
                      resource.authAdapter === 'OPENAI_CODEX' && credentialResetCredits[resource.id]
                    "
                    class="credential-reset-credits"
                  >
                    <small class="credential-detail">
                      {{
                        t('resources.resetCreditsAvailable', {
                          count: credentialResetCredits[resource.id].availableCount,
                        })
                      }}
                    </small>
                    <button
                      v-if="credentialResetCredits[resource.id].availableCount > 0"
                      type="button"
                      class="text-button"
                      :disabled="busy || resetCreditConsuming[resource.id]"
                      @click="consumeResetCredit(resource)"
                    >
                      {{
                        t(
                          resetCreditConsuming[resource.id]
                            ? 'resources.resetCreditConsuming'
                            : 'resources.consumeResetCredit',
                        )
                      }}
                    </button>
                  </div>
                </div>
              </td>
              <td class="credential-runtime" :data-label="t('resources.runtimeStatus')">
                <div class="credential-runtime-content">
                  <div class="credential-runtime-line">
                    <Status :value="resource.runtimeStatus || 'HEALTHY'" />
                    <button
                      v-if="
                        credentialTarget &&
                        (resource.authType === 'SUBSCRIPTION' || credentialTarget.modelCount > 0)
                      "
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
                  <div v-if="resource.runtimeStatus === 'BLOCKED'" class="credential-runtime-error">
                    <span>{{ blockedResourceReason(resource) }}</span>
                    <small
                      v-if="resource.lastErrorCode"
                      class="credential-detail"
                      :title="resource.lastErrorCode"
                    >
                      {{ resource.lastErrorCode }}
                    </small>
                  </div>
                </div>
              </td>
              <td class="credential-action-column" :data-label="t('common.actions')">
                <div class="row-actions">
                  <button
                    type="button"
                    class="text-button"
                    :disabled="busy"
                    @click="priceTarget = resource"
                  >
                    {{ t('resources.pricing.action') }}
                  </button>
                  <button
                    v-if="exportableSubscription(resource)"
                    type="button"
                    class="text-button"
                    :aria-label="t('resources.exportFor', { name: resource.name })"
                    :disabled="busy"
                    @click="exportSubscription(resource)"
                  >
                    {{ t('resources.export') }}
                  </button>
                  <button
                    type="button"
                    class="text-button danger"
                    :disabled="busy"
                    @click="credentialDeleteTarget = resource"
                  >
                    {{ t('resources.delete') }}
                  </button>
                </div>
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
          <select
            id="credential-auth-type"
            v-model="credentialAuthType"
            :disabled="busy"
            @change="resetCredentialInput"
          >
            <option value="API_KEY">{{ t('resources.authTypes.API_KEY') }}</option>
            <option
              v-if="credentialTarget.authAdapters?.some((adapter) => adapter !== 'API_KEY')"
              value="SUBSCRIPTION"
            >
              {{ t('resources.authTypes.SUBSCRIPTION') }}
            </option>
          </select>
        </div>
      </div>
      <div v-if="credentialAuthType === 'API_KEY'" class="credential-form-row">
        <label class="credential-form-label" for="credential-api-key">{{
          t('resources.apiKey')
        }}</label>
        <div class="credential-form-control">
          <input
            id="credential-api-key"
            v-model="credential"
            type="text"
            autocomplete="off"
            required
            autofocus
            :disabled="busy"
            spellcheck="false"
          />
        </div>
      </div>
      <p v-if="credentialAuthType === 'API_KEY'" class="field-hint credential-form-hint">
        {{ t('resources.credentialHint') }}
      </p>
      <section v-else class="subscription-import">
        <div
          v-if="!claudeSubscription"
          class="subscription-input-tabs"
          role="tablist"
          :aria-label="t('resources.subscriptionMethodLabel')"
        >
          <button
            id="subscription-upload-tab"
            ref="subscriptionUploadTab"
            type="button"
            role="tab"
            :aria-selected="subscriptionInputMode === 'UPLOAD'"
            aria-controls="subscription-upload-panel"
            :tabindex="subscriptionInputMode === 'UPLOAD' ? 0 : -1"
            :disabled="busy"
            @click="selectSubscriptionInputMode('UPLOAD')"
            @keydown.right.prevent="focusSubscriptionInputMode('PASTE')"
            @keydown.end.prevent="focusSubscriptionInputMode('PASTE')"
          >
            {{ t('resources.subscriptionMethods.UPLOAD') }}
          </button>
          <button
            id="subscription-paste-tab"
            ref="subscriptionPasteTab"
            type="button"
            role="tab"
            :aria-selected="subscriptionInputMode === 'PASTE'"
            aria-controls="subscription-paste-panel"
            :tabindex="subscriptionInputMode === 'PASTE' ? 0 : -1"
            :disabled="busy"
            @click="selectSubscriptionInputMode('PASTE')"
            @keydown.left.prevent="focusSubscriptionInputMode('UPLOAD')"
            @keydown.home.prevent="focusSubscriptionInputMode('UPLOAD')"
          >
            {{ t('resources.subscriptionMethods.PASTE') }}
          </button>
        </div>

        <component
          :is="claudeSubscription ? 'div' : 'details'"
          :key="subscriptionInputMode"
          class="subscription-source-guide"
          :class="{ 'subscription-source-guide-static': claudeSubscription }"
        >
          <summary v-if="!claudeSubscription">
            <Icon name="arrow" :size="13" />
            <span>{{
              t(
                subscriptionInputMode === 'UPLOAD'
                  ? 'resources.subscriptionFolderCommandTitle'
                  : 'resources.subscriptionCommandTitle',
              )
            }}</span>
          </summary>
          <p v-else class="subscription-command-title">
            {{ t('resources.claudeSubscriptionCommandTitle') }}
          </p>
          <div class="subscription-command-list">
            <div
              v-for="item in subscriptionCommands"
              :key="item.platform"
              :class="{ 'subscription-command-item-direct': !item.platform }"
            >
              <span v-if="item.platform">{{ item.platform }}</span>
              <code :title="item.command">{{ item.command }}</code>
              <button
                type="button"
                class="icon-button subscription-command-copy"
                :aria-label="
                  t(
                    item.platform
                      ? 'resources.copySubscriptionCommand'
                      : 'resources.copyClaudeSubscriptionCommand',
                    { platform: item.platform },
                  )
                "
                :title="
                  t(
                    item.platform
                      ? 'resources.copySubscriptionCommand'
                      : 'resources.copyClaudeSubscriptionCommand',
                    { platform: item.platform },
                  )
                "
                @click="copySubscriptionCommand(item.command)"
              >
                <Icon name="copy" :size="14" />
              </button>
            </div>
          </div>
        </component>

        <div
          v-if="!claudeSubscription && subscriptionInputMode === 'UPLOAD'"
          id="subscription-upload-panel"
          class="subscription-input-panel"
          role="tabpanel"
          aria-labelledby="subscription-upload-tab"
        >
          <input
            id="credential-auth-file"
            class="subscription-file-input"
            type="file"
            accept=".json,application/json"
            required
            :aria-label="t('resources.authFile')"
            :disabled="busy"
            @change="importSubscription"
          />
          <label class="subscription-file-picker" for="credential-auth-file">
            <Icon name="upload" :size="21" />
            <span>
              <strong>{{ subscriptionFileName || t('resources.subscriptionChooseFile') }}</strong>
              <small>{{ t('resources.subscriptionFileLimit') }}</small>
            </span>
          </label>
          <p class="field-hint">
            {{ t('resources.subscriptionUploadHint') }}
          </p>
        </div>

        <div
          v-else
          id="subscription-paste-panel"
          class="subscription-input-panel"
          :role="claudeSubscription ? undefined : 'tabpanel'"
          :aria-labelledby="claudeSubscription ? undefined : 'subscription-paste-tab'"
        >
          <p class="subscription-paste-intro">
            {{
              t(
                claudeSubscription
                  ? 'resources.claudeSubscriptionPasteHint'
                  : 'resources.subscriptionPasteHint',
              )
            }}
          </p>
          <textarea
            id="credential-auth-content"
            v-model="credential"
            rows="8"
            autocomplete="off"
            required
            :aria-label="
              t(
                claudeSubscription
                  ? 'resources.claudeSubscriptionPasteLabel'
                  : 'resources.subscriptionPasteLabel',
              )
            "
            :disabled="busy"
            :placeholder="
              t(
                claudeSubscription
                  ? 'resources.claudeSubscriptionPastePlaceholder'
                  : 'resources.subscriptionPastePlaceholder',
              )
            "
            spellcheck="false"
          ></textarea>
        </div>
      </section>
    </form>
    <template v-if="credentialCreating" #footer>
      <button type="button" class="button" :disabled="busy" @click="cancelCredentialCreation">
        {{ t('common.cancel') }}</button
      ><button type="submit" form="credential-create-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>

  <Modal
    v-if="credentialTarget && priceTarget"
    :title="t('resources.pricing.title')"
    :wide="priceTarget.authType === 'API_KEY'"
    @close="priceTarget = null"
  >
    <CredentialPriceEditor
      :provider="credentialTarget"
      :resource="priceTarget"
      @close="priceTarget = null"
      @changed="loadResources"
    />
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
    <p v-if="busy" role="status">
      {{
        t(
          testTarget.resource.authType === 'SUBSCRIPTION'
            ? 'resources.subscriptionTesting'
            : 'resources.testing',
        )
      }}
    </p>
    <template v-if="testResult">
      <div class="alert" :class="testResult.ok ? 'success' : 'error'" role="status">
        <i18n-t
          v-if="testResult.code === 'CODEX_APP_SERVER_UNAVAILABLE'"
          keypath="errors.CODEX_APP_SERVER_UNAVAILABLE"
          tag="span"
        >
          <template #executable><code>CODEX_EXECUTABLE</code></template>
        </i18n-t>
        <span v-else>{{ resultMessage(testResult, testTarget.resource) }}</span>
      </div>
      <dl class="detail-grid">
        <template v-if="testTarget.protocol">
          <dt>{{ t('resources.protocol') }}</dt>
          <dd>{{ testTarget.protocol === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI' }}</dd>
        </template>
        <template v-if="testResult.testedModelCode">
          <dt>{{ t('resources.testedModel') }}</dt>
          <dd><TechnicalValue :value="testResult.testedModelCode" :copyable="false" /></dd>
        </template>
        <dt>{{ t('common.code') }}</dt>
        <dd>{{ testResult.code }}</dd>
        <dt>HTTP</dt>
        <dd>{{ testResult.httpStatus || t('common.none') }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ testResult.latencyMs }} ms</dd>
      </dl>
    </template>
    <p class="muted">
      {{
        t(
          testTarget.resource.authType === 'SUBSCRIPTION'
            ? testTarget.resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE'
              ? 'resources.claudeSubscriptionTestHint'
              : 'resources.subscriptionTestHint'
            : 'resources.testHint',
        )
      }}
    </p>
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
        <dt>{{ t('resources.modelSyncSource') }}</dt>
        <dd>{{ t(`resources.modelSyncSources.${syncResult.source}`) }}</dd>
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
  <GuideTour
    v-if="providerGuideOpen"
    :steps="providerGuideSteps"
    control-prefix="providers.guide"
    @close="providerGuideOpen = false"
  />
</template>

<style scoped>
.provider-toolbar-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.provider-create-menu {
  position: relative;
  flex: none;
}
.provider-create-trigger {
  min-width: 144px;
}
.provider-create-trigger > span {
  flex: 1;
  text-align: left;
}
.provider-create-chevron {
  margin-left: 2px;
  transform: rotate(90deg);
  transition: transform 150ms;
}
.provider-create-chevron.expanded {
  transform: rotate(-90deg);
}
.provider-create-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 20;
  width: 268px;
  padding: 6px;
  border: 1px solid var(--line);
  border-radius: var(--radius-control);
  background: #fff;
  box-shadow: 0 12px 28px rgb(15 23 42 / 12%);
}
.provider-create-option {
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
  width: 100%;
  padding: 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text);
  text-align: left;
}
.provider-create-option:hover,
.provider-create-option:focus-visible {
  background: var(--color-primary-soft);
  outline: none;
}
.provider-create-option + .provider-create-option {
  margin-top: 2px;
}
.provider-create-option-icon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 7px;
  background: #eff6ff;
  color: var(--blue);
}
.provider-create-option strong,
.provider-create-option small {
  display: block;
}
.provider-create-option strong {
  font-size: 13px;
  font-weight: 600;
}
.provider-create-option small {
  margin-top: 3px;
  color: var(--color-text-muted);
  font-size: 11px;
  line-height: 1.45;
}
.provider-runtime-filter {
  display: flex;
  flex: none;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--color-text-secondary);
  font-weight: 500;
  white-space: nowrap;
}
.provider-runtime-filter select {
  width: 120px;
  padding-top: 9px;
  padding-bottom: 9px;
  background: var(--color-surface);
  border-color: var(--color-border);
  font-size: 12px;
}
.providers-table {
  min-width: 1184px;
  table-layout: fixed;
}
.providers-table th,
.providers-table td {
  padding-left: 14px;
  padding-right: 14px;
}
.providers-table td {
  height: 58px;
  padding-top: 7px;
  padding-bottom: 7px;
}
.providers-table .avatar {
  width: 36px;
  height: 36px;
}
.provider-name-column {
  width: 252px;
}
.provider-id-column {
  width: 192px;
}
.provider-enable-column {
  width: 88px;
}
.provider-runtime-column {
  width: 126px;
}
.provider-proxy-column {
  width: 72px;
}
.provider-endpoint-column {
  width: auto;
}
.provider-action-column {
  width: 176px;
}
.providers-table th:nth-child(5),
.providers-table td:nth-child(5) {
  padding-left: 8px;
  padding-right: 8px;
}
.providers-table th:nth-child(3),
.providers-table td:nth-child(3) {
  padding-left: 12px;
  padding-right: 12px;
}
.providers-table th:nth-child(4),
.providers-table td:nth-child(4) {
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
  box-shadow: 0 0 0 2px #dbeafe;
}
.provider-runtime-state:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.endpoint {
  min-width: 0;
}
.endpoint :deep(.technical-value-copy) {
  width: 22px;
  height: 22px;
}
.provider-name-line {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}
.provider-name-line strong {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.person > div {
  flex: 1;
  min-width: 0;
}
.person > div > .technical-value {
  margin-top: 2px;
}
.provider-name-actions {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 2px;
}
.provider-quick-action {
  display: inline-grid;
  flex: none;
  width: 25px;
  height: 25px;
  place-items: center;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: 50%;
  color: var(--color-text-muted);
  background: #f8fafc;
  opacity: 0.72;
  text-decoration: none;
  transition:
    color 0.15s,
    background-color 0.15s,
    opacity 0.15s;
}
.providers-table tbody tr:hover .provider-quick-action,
.provider-quick-action:focus-visible {
  color: var(--blue);
  opacity: 1;
}
.provider-quick-action:hover:not(:disabled) {
  border-color: var(--blue);
  background: var(--color-primary-soft);
  opacity: 1;
}
.provider-website-action {
  border: 0;
  background: transparent;
}
.provider-website-action:hover:not(:disabled) {
  border: 0;
  background: var(--color-primary-soft);
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
  gap: 2px;
}
.endpoint-stack > span {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr) auto;
  align-items: center;
  gap: 6px;
  min-height: 20px;
}
.endpoint-private-badge {
  display: inline-flex;
  align-items: center;
  width: max-content;
  padding: 2px 5px;
  border: 1px solid var(--color-warning-border);
  border-radius: 999px;
  color: var(--color-warning-text);
  background: var(--color-warning-bg);
  font-size: 10px;
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
}
.endpoint-protocol {
  display: inline-flex;
  min-height: 18px;
  align-items: center;
  justify-content: center;
  padding: 2px 5px;
  border-radius: 4px;
  color: var(--color-text-secondary);
  background: #f1f5f9;
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
  color: var(--color-success);
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
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 14px;
}
.credential-list-heading {
  min-width: 0;
}
.credential-title-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 9px;
}
.credential-list-head h3 {
  margin: 0;
  color: var(--color-text);
  font-size: 17px;
  line-height: 1.35;
}
.credential-title-line .status {
  margin: 0;
}
.credential-list-head p {
  max-width: 520px;
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.credential-overview {
  margin: 0 0 16px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: #f8fafc;
}
.credential-overview dt,
.credential-overview dd {
  min-width: 0;
  margin: 0;
}
.credential-overview dt {
  color: var(--color-text-secondary);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.01em;
}
.credential-overview dd {
  color: var(--color-text);
  font-size: 12px;
}
.credential-overview-endpoints {
  display: grid;
  grid-template-columns: 78px minmax(0, 1fr);
  align-items: stretch;
}
.credential-overview-endpoints dt {
  display: flex;
  align-items: center;
  padding: 12px 14px;
  border-right: 1px solid #e3eaf1;
}
.credential-overview-endpoints dd {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  background: #fff;
}
.credential-overview-endpoints dd > span {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
  padding: 11px 14px;
}
.credential-overview-endpoints dd > span + span {
  border-left: 1px solid #e3eaf1;
}
.credential-overview-endpoints b {
  flex: 0 0 auto;
  color: #73889a;
  font-size: 10px;
  font-weight: 600;
}
.credential-overview-endpoints .technical-value {
  min-width: 0;
}
.credential-list {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius-control);
}
.credential-list table {
  width: 100%;
  margin: 0;
  table-layout: fixed;
  white-space: normal;
}
.credential-name-column {
  width: 31%;
}
.credential-auth-column {
  width: 29%;
}
.credential-runtime-column {
  width: auto;
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
.credential-auth-content {
  min-width: 0;
}
.credential-plan {
  margin: 0 0 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--line);
  color: #536d84;
  font-weight: 600;
}
.credential-subscription-price {
  display: grid;
  gap: 3px;
  margin: 0 0 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--line);
}
.credential-subscription-price strong {
  color: var(--color-text);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.credential-plan + .credential-quota-windows,
.credential-plan + .credential-quota-feedback {
  margin-top: 0;
}
.credential-quota-windows {
  display: grid;
  gap: 7px;
  margin-top: 6px;
}
.credential-quota-window .credential-detail {
  margin-top: 0;
}
.credential-quota-window .credential-detail:first-child {
  color: #536d84;
}
.credential-quota-window .credential-detail + .credential-detail {
  margin-top: 2px;
}
.credential-quota-feedback.is-error {
  color: var(--danger);
  text-overflow: clip;
  white-space: normal;
}
.credential-reset-credits {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--line);
}
.credential-reset-credits .credential-detail {
  margin-top: 0;
}
.credential-reset-credits .text-button {
  flex: 0 0 auto;
  font-size: 11px;
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
.credential-runtime-content {
  min-width: 0;
}
.credential-runtime-error {
  margin-top: 7px;
  color: #566d82;
  font-size: 12px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}
.credential-runtime-error .credential-detail {
  max-width: 100%;
  white-space: normal;
  overflow-wrap: anywhere;
}
.credential-action-column {
  text-align: right;
}
.credential-runtime-line {
  display: flex;
  align-items: center;
  gap: 4px;
}
.credential-test-selection-hint {
  margin-bottom: 14px;
}
.provider-initialize-hint {
  margin-bottom: 14px;
}
.provider-initialize-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
  color: var(--muted);
  font-size: 12px;
}
.provider-initialize-options {
  display: grid;
  max-height: min(52vh, 520px);
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
}
.provider-initialize-option {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  margin: 0;
  padding: 12px 14px;
  cursor: pointer;
  background: #fff;
}
.provider-initialize-option + .provider-initialize-option {
  border-top: 1px solid #e5ebf1;
}
.provider-initialize-option:hover {
  background: #f8fafc;
}
.provider-initialize-option.is-selected {
  background: var(--color-primary-soft);
}
.provider-initialize-option input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--blue);
}
.provider-initialize-option > span {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.provider-initialize-option strong,
.provider-initialize-option small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.provider-initialize-option small {
  color: var(--muted);
  font-size: 11px;
}
.credential-test-section + .credential-test-section {
  margin-top: 18px;
}
.credential-test-options {
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
}
.credential-test-option {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  margin: 0;
  padding: 13px 14px;
  cursor: pointer;
  background: #fff;
}
.credential-test-option + .credential-test-option {
  border-top: 1px solid #e5ebf1;
}
.credential-test-option:hover {
  background: #f8fafc;
}
.credential-test-option.is-selected {
  background: var(--color-primary-soft);
}
.credential-test-option input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--blue);
}
.credential-test-option-main {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.credential-test-option-main strong,
.credential-test-option-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-test-option-main small {
  color: var(--muted);
  font-size: 11px;
}
.credential-test-option-status {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}
.credential-verify-action {
  width: 26px;
  height: 26px;
  color: var(--blue);
}
.credential-verify-action.is-blocked {
  color: var(--color-warning-text);
}
.credential-empty {
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 42px 20px;
  border: 1px dashed #cbd5e1;
  border-radius: var(--radius-control);
  color: var(--color-text-muted);
  text-align: center;
}
.credential-empty strong {
  color: var(--color-text);
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
  color: var(--color-text-secondary);
  line-height: 1.4;
  text-align: right;
}
.credential-form-control {
  min-width: 0;
}
.credential-form-hint {
  margin: -2px 0 0 100px;
}
.subscription-import {
  display: grid;
  min-width: 0;
  gap: 12px;
}
.subscription-input-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid #dce5ee;
}
.subscription-input-tabs button {
  position: relative;
  padding: 9px 14px 10px;
  border: 0;
  color: var(--color-text-secondary);
  cursor: pointer;
  background: transparent;
  font: inherit;
  font-weight: 600;
}
.subscription-input-tabs button::after {
  position: absolute;
  right: 12px;
  bottom: -1px;
  left: 12px;
  height: 2px;
  content: '';
  background: transparent;
}
.subscription-input-tabs button[aria-selected='true'] {
  color: var(--blue);
}
.subscription-input-tabs button[aria-selected='true']::after {
  background: var(--blue);
}
.subscription-input-tabs button:focus-visible {
  border-radius: 4px;
  outline: 2px solid #bfdbfe;
  outline-offset: -2px;
}
.subscription-input-tabs button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.subscription-source-guide {
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 7px;
  color: var(--color-text-secondary);
  background: #f8fafc;
}
.subscription-source-guide summary {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 8px 10px;
  color: #708397;
  cursor: pointer;
  font-size: 11px;
  line-height: 1.4;
  list-style: none;
}
.subscription-source-guide-static {
  padding-top: 10px;
}
.subscription-command-title {
  margin: 0 12px 8px;
  color: var(--muted);
  font-size: 12px;
}
.subscription-source-guide summary::-webkit-details-marker {
  display: none;
}
.subscription-source-guide summary:hover {
  color: #4f6e89;
  background: #f7f9fc;
}
.subscription-source-guide summary:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: -2px;
}
.subscription-source-guide summary svg {
  flex: none;
  transition: transform 0.16s ease;
}
.subscription-source-guide[open] summary {
  border-bottom: 1px solid #e8edf2;
}
.subscription-source-guide[open] summary svg {
  transform: rotate(90deg);
}
.subscription-command-list {
  display: grid;
  padding: 4px 10px 6px;
}
.subscription-command-list > div {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr) 28px;
  align-items: center;
  min-width: 0;
  gap: 7px;
  padding: 7px 0;
}
.subscription-command-list > .subscription-command-item-direct {
  grid-template-columns: minmax(0, 1fr) 28px;
}
.subscription-command-item-direct code {
  overflow: visible;
  text-overflow: clip;
  white-space: normal;
  overflow-wrap: anywhere;
}
.subscription-command-list > div + div {
  border-top: 1px solid #e1e9f2;
}
.subscription-command-list span {
  color: #71869a;
  font-size: 10px;
}
.subscription-command-list code {
  min-width: 0;
  overflow: hidden;
  color: #385a75;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subscription-command-copy {
  width: 28px;
  height: 28px;
  color: #6e91b8;
  opacity: 0.8;
}
.subscription-command-list > div:hover .subscription-command-copy,
.subscription-command-copy:focus-visible {
  opacity: 1;
}
.subscription-input-panel {
  display: grid;
  gap: 8px;
}
.subscription-file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
  white-space: nowrap;
}
.subscription-file-picker {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 64px;
  padding: 11px 14px;
  border: 1px dashed #cbd5e1;
  border-radius: var(--radius-control);
  color: var(--color-text-secondary);
  cursor: pointer;
  background: #f8fafc;
}
.subscription-file-picker:hover {
  border-color: #93c5fd;
  background: var(--color-primary-soft);
}
.subscription-file-input:focus-visible + .subscription-file-picker {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.subscription-file-input:disabled + .subscription-file-picker {
  cursor: not-allowed;
  opacity: 0.6;
}
.subscription-file-picker > svg {
  flex: none;
}
.subscription-file-picker span {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.subscription-file-picker strong {
  overflow: hidden;
  color: #29465f;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subscription-file-picker small {
  color: var(--muted);
  font-size: 10px;
}
.subscription-input-panel > .field-hint {
  margin: 0;
}
.subscription-paste-intro {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.5;
}
.subscription-input-panel textarea {
  width: 100%;
  min-height: 142px;
  resize: vertical;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 11px;
  line-height: 1.55;
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
  color: var(--color-text-muted);
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
  background: var(--color-primary-soft);
}
.provider-more summary:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.provider-more-menu {
  position: absolute;
  z-index: 4;
  top: 50%;
  right: calc(100% + 4px);
  min-width: 80px;
  padding: 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #fff;
  box-shadow: 0 12px 28px rgb(15 23 42 / 12%);
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
:global(.modal-body.provider-model-modal-body) {
  overflow-y: hidden;
  scrollbar-gutter: auto;
}
:global(.modal-body.provider-model-modal-body .provider-form) {
  height: min(640px, calc(100dvh - 200px));
  min-height: 0;
  grid-template-rows: auto minmax(0, 1fr);
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
  border-left: 3px solid var(--color-primary);
  border-radius: 6px;
  color: var(--color-text-secondary);
  background: var(--color-primary-soft);
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
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.provider-field-control {
  min-width: 0;
}
.endpoint-editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 132px;
  gap: 8px;
}
.private-network-warning {
  margin: 0 0 0 calc(var(--provider-field-label-width) + 12px);
  padding: 8px 10px;
  border: 1px solid var(--color-warning-border);
  border-radius: 7px;
  color: var(--color-text-secondary);
  background: var(--color-warning-bg);
  font-size: 12px;
  line-height: 1.55;
}
.required-label::after {
  margin-left: 4px;
  color: var(--danger);
  content: '*';
}
.config-editor {
  display: grid;
  min-height: 0;
  margin-top: 0;
  grid-template-rows: auto minmax(0, 1fr);
}
.config-tabs {
  display: flex;
  gap: 4px;
  overflow-x: auto;
  overflow-y: hidden;
  border-bottom: 1px solid var(--color-border);
}
.config-tab {
  position: relative;
  flex: none;
  min-width: 112px;
  padding: 9px 15px 10px;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
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
  color: var(--color-primary);
}
.config-tab.is-active {
  color: var(--color-primary-hover);
}
.config-tab.is-active::after {
  background: var(--blue);
}
.config-panel {
  min-height: 0;
  padding-top: 14px;
}
.config-panel:focus-visible {
  outline: 2px solid #bfdbfe;
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
  border: 1px solid #cbd5e1;
  border-radius: 999px;
  background: #cbd5e1;
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
  outline: 2px solid #bfdbfe;
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
  border-top: 1px solid var(--color-border);
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
.proxy-url-control {
  display: grid;
  grid-template-columns: 124px minmax(0, 1fr);
  gap: 8px;
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
  background: var(--color-sidebar);
  box-shadow: 0 4px 12px rgb(15 23 42 / 18%);
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
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.proxy-headers-head {
  display: flex;
  grid-column: 1 / -1;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.proxy-socks-hint {
  grid-column: 2;
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
}
.proxy-headers-head > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}
.proxy-headers-head strong {
  color: var(--color-text);
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
  color: var(--color-danger);
}
.proxy-header-remove:hover:not(:disabled) {
  color: #8f2f35;
}
.mapping-workspace {
  display: grid;
  min-width: 0;
  min-height: 0;
  gap: 14px;
  margin-bottom: 16px;
  grid-template-columns: minmax(280px, 0.9fr) minmax(360px, 1.1fr);
}
.mapping-pane {
  display: grid;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #fff;
}
.mapping-catalog-pane {
  grid-template-rows: auto auto minmax(0, 1fr);
}
.mapping-selected-pane {
  grid-template-rows: auto minmax(0, 1fr);
}
.mapping-pane-head {
  display: flex;
  min-width: 0;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 12px;
  border-bottom: 1px solid #e5ebf1;
  background: #f8fafc;
}
.mapping-pane-head strong {
  color: var(--color-text);
  font-size: 12px;
}
.mapping-pane-head .mapping-pane-title {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  align-items: center;
  gap: 8px;
}
.mapping-count-tag {
  display: inline-flex;
  flex: none;
  min-height: 20px;
  align-items: center;
  padding: 1px 7px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
  line-height: 18px;
  white-space: nowrap;
}
.mapping-bulk-actions {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  gap: 2px;
}
.mapping-bulk-button {
  min-height: 24px;
  padding: 2px 4px;
  font-size: 11px;
}
.mapping-action-separator {
  color: #94a3b8;
  font-size: 11px;
}
.mapping-selected-head {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr) 32px;
  gap: 10px;
  padding-right: 22px;
}
.mapping-toolbar {
  min-width: 0;
  padding: 10px 10px 8px;
}
.mapping-search-box {
  position: relative;
  min-width: 0;
  color: #92a0af;
}
.mapping-search-box input[type='search'] {
  min-height: 36px;
  padding: 7px 36px 7px 11px;
  font-size: 12px;
  background: #f8fafc;
}
.mapping-search-box input[type='search']::-webkit-search-cancel-button {
  display: none;
}
.mapping-search-clear {
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
.mapping-search-clear:hover {
  color: var(--color-text);
  background: #e9eff6;
}
.mapping-catalog-list,
.mapping-selected-list {
  min-width: 0;
  min-height: 0;
  overflow-x: clip;
  overflow-y: scroll;
  overscroll-behavior: contain;
  scrollbar-color: #94a3b8 #f1f5f9;
  scrollbar-gutter: stable;
  scrollbar-width: auto;
}
.mapping-catalog-list {
  grid-row: 3;
}
.mapping-selected-list {
  grid-row: 2;
}
.mapping-catalog-list::-webkit-scrollbar,
.mapping-selected-list::-webkit-scrollbar {
  width: 12px;
}
.mapping-catalog-list::-webkit-scrollbar-track,
.mapping-selected-list::-webkit-scrollbar-track {
  background: #f1f5f9;
}
.mapping-catalog-list::-webkit-scrollbar-thumb,
.mapping-selected-list::-webkit-scrollbar-thumb {
  min-height: 40px;
  border: 3px solid #f1f5f9;
  border-radius: 999px;
  background: #94a3b8;
}
.mapping-catalog-list::-webkit-scrollbar-thumb:hover,
.mapping-selected-list::-webkit-scrollbar-thumb:hover {
  background: #64748b;
}
.mapping-editor {
  display: grid;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
}
.mapping-catalog-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: 42px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 5px 10px;
  border-top: 1px solid #f1f5f9;
}
.mapping-catalog-row {
  min-height: 52px;
  background: #fff;
}
.mapping-catalog-row.is-selected {
  background: var(--color-primary-soft);
  box-shadow: inset 3px 0 var(--color-primary);
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
  display: block;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mapping-model-summary {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.mapping-model-summary > span {
  min-width: 0;
  overflow: hidden;
  color: var(--muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mapping-selected-row {
  display: grid;
  min-height: 52px;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr) 32px;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 12px;
  border-top: 1px solid #f1f5f9;
  background: #fff;
}
.mapping-selected-row:first-child {
  border-top: 0;
}
.mapping-control {
  display: block;
  min-width: 0;
  margin: 0;
}
.mapping-control input {
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 5px 10px;
}
.mapping-remove {
  display: grid;
  width: 30px;
  height: 30px;
  padding: 0;
  place-items: center;
  color: var(--muted);
  border: 0;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
}
.mapping-remove:hover:not(:disabled) {
  color: var(--danger);
  background: #fef2f2;
}
.mapping-empty {
  margin: 0;
  padding: 18px 14px;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}
.mapping-filter-empty {
  grid-row: 4;
  align-self: start;
  border-top: 1px solid #e5ebf1;
}
.mapping-filter-empty .text-button {
  margin-left: 6px;
}
.mapping-selected-empty {
  display: grid;
  min-height: 0;
  place-items: center;
  line-height: 1.6;
}
@media (max-width: 760px) {
  .credential-test-option {
    grid-template-columns: 20px minmax(0, 1fr);
  }
  .credential-test-option-status {
    grid-column: 2;
    justify-content: flex-start;
  }
  .credential-overview {
    background: #fff;
  }
  .credential-overview-endpoints {
    grid-template-columns: minmax(0, 1fr);
  }
  .credential-overview-endpoints dt {
    padding: 9px 12px 7px;
    border-right: 0;
    background: #f8fbfe;
  }
  .credential-overview-endpoints dd {
    grid-template-columns: minmax(0, 1fr);
  }
  .credential-overview-endpoints dd > span {
    padding: 10px 12px;
  }
  .credential-overview-endpoints dd > span + span {
    border-top: 1px solid #e3eaf1;
    border-left: 0;
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
  .subscription-input-tabs button {
    flex: 1;
  }
  .subscription-source-guide {
    border-radius: 6px;
  }
  .subscription-command-list > div {
    grid-template-columns: 86px minmax(0, 1fr) 28px;
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
  .credential-list .row-actions {
    justify-content: flex-start;
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
  .provider-create-menu,
  .provider-create-trigger {
    width: 100%;
  }
  .provider-create-dropdown {
    right: auto;
    left: 0;
    width: min(100%, 320px);
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
  .endpoint-editor {
    grid-template-columns: minmax(0, 1fr);
  }
  .private-network-warning {
    margin-left: 0;
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
  .proxy-socks-hint {
    grid-column: 1;
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
  .mapping-editor {
    overflow-y: auto;
    padding-right: 4px;
    grid-template-rows: auto auto;
  }
  .mapping-workspace {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: repeat(2, minmax(260px, 38vh));
  }
  .mapping-pane {
    min-height: 260px;
  }
}
</style>
