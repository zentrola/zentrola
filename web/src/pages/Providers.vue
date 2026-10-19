<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { all, api, download } from '../api'
import { useAction, useCollection, useListSearch } from '../composables'
import { useDropdownMenu } from '../dropdownMenu'
import { saveBlob } from '../files'
import { i18n, t } from '../i18n'
import { useProviderConnectionTest } from '../providerConnectionTest'
import { providerGuideSteps as providerGuideAllSteps } from '../providerGuide'
import { useCredentialQuotas } from '../providerQuotas'
import { useProviderResources } from '../providerResources'
import { useProviderRuntime } from '../providerRuntime'
import { showSuccessToast } from '../toast'
import type {
  Model,
  Provider,
  ProviderDetail,
  ProviderInitializeOption,
  ProviderInitializeResult,
  Resource,
} from '../types'
import Icon from '../components/Icon.vue'
import GuideTour from '../components/InitializationGuide.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import CredentialPriceEditor from '../components/CredentialPriceEditor.vue'
import ProviderConnectionSelection from '../components/ProviderConnectionSelection.vue'
import ConnectionResultModal from '../components/providers/ConnectionResultModal.vue'
import ModelSyncResultModal from '../components/providers/ModelSyncResultModal.vue'
import ProviderCredentialsModal, {
  type CredentialInput,
} from '../components/providers/ProviderCredentialsModal.vue'
import ProviderEditorModal, {
  type ProviderInput,
} from '../components/providers/ProviderEditorModal.vue'
import ProviderInitializeModal from '../components/providers/ProviderInitializeModal.vue'
import ProviderTable from '../components/providers/ProviderTable.vue'

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
const { busy, run } = useAction()
const {
  open: createMenuOpen,
  root: createMenuRoot,
  trigger: createMenuTrigger,
  menu: createMenu,
  menuId: createMenuId,
  close: closeCreateMenu,
  show: showCreateMenu,
  onKeydown: onCreateMenuKeydown,
  onFocusOut: onCreateMenuFocusOut,
} = useDropdownMenu()
const providerGuideOpen = ref(false)
const providerInitializeOpen = ref(false)
const providerInitializeOptions = ref<ProviderInitializeOption[]>([])
const selectedProviderCodes = ref<string[]>([])
const editing = ref(false)
const editTarget = ref<ProviderDetail | null>(null)
const editModels = ref<Model[]>([])
const statusTarget = ref<Provider | null>(null)
const deleteTarget = ref<Provider | null>(null)
const credentialTarget = ref<Provider | null>(null)
const priceTarget = ref<Resource | null>(null)
const priceSaving = ref(false)
const credentialDeleteTarget = ref<Resource | null>(null)
const { resources, error: resourceError, load: loadResources } = useProviderResources()

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
// 运行状态按搜索后实际展示的行集合计算，运行状态筛选在此基础上进行。
const {
  resourcesFor,
  resourceFor,
  runtimeFor: providerRuntime,
  abnormal: providerRuntimeAbnormal,
} = useProviderRuntime(searchVisible, resources)
const quotas = useCredentialQuotas({ loadResources, resourcesFor })
const {
  selectionTarget: testSelectionTarget,
  testTarget,
  testResult,
  syncTarget,
  syncResult,
  verifiedAt: credentialVerifiedAt,
  prepareTestConnection,
  openActivation,
  closeSelection: closeTestSelection,
  submitSelection: testSelectedProviderCredential,
  syncModels,
} = useProviderConnectionTest({
  run,
  loadResources,
  refresh,
  resourcesFor,
  resourceFor,
  resetCredits: quotas.resetCredits,
})

const visible = computed(() =>
  searchVisible.value.filter((provider) => {
    if (!appliedRuntimeFilter.value) return true
    const abnormal = providerRuntimeAbnormal(provider)
    return appliedRuntimeFilter.value === 'ABNORMAL'
      ? abnormal
      : provider.status === 'ACTIVE' && !abnormal
  }),
)
const filtered = computed(() => Boolean(query.value || appliedRuntimeFilter.value))
const guideTargets = computed(() => ({
  credential: visible.value[0]?.id ?? '',
  website: visible.value.find((provider) => provider.website)?.id ?? '',
  sync: visible.value.find((provider) => provider.modelSyncSupported)?.id ?? '',
  test:
    visible.value.find((provider) => resourceFor(provider) && provider.modelCount > 0)?.id ?? '',
}))
const providerGuideSteps = computed(() =>
  providerGuideAllSteps.filter((step) => {
    if (step.key === 'create') return true
    if (!guideTargets.value.credential) return false
    if (step.key === 'website') return Boolean(guideTargets.value.website)
    if (step.key === 'sync') return Boolean(guideTargets.value.sync)
    if (step.key === 'test') return Boolean(guideTargets.value.test)
    return true
  }),
)

function hasCredential(provider: Provider) {
  return Boolean(resourceFor(provider))
}

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

function reload() {
  void retry()
  void loadResources()
}

function openEdit(provider: Provider | null = null) {
  void run(async () => {
    const detail = provider ? await api<ProviderDetail>(`/providers/${provider.id}`) : null
    editModels.value = detail ? detail.models : await all<Model>('/models?status=ACTIVE')
    editTarget.value = detail
    editing.value = true
  })
}

function saveProvider(input: ProviderInput) {
  const target = editTarget.value
  void run(async () => {
    await api(target ? `/providers/${target.id}` : '/providers', target ? 'PUT' : 'POST', input)
    editing.value = false
    await load()
  })
}

function openProviderInitialization() {
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

function initializeProviders() {
  if (!selectedProviderCodes.value.length) return
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

function selectCreateAction(action: 'thirdParty' | 'modelVendor') {
  closeCreateMenu(true)
  if (action === 'thirdParty') openEdit()
  else openProviderInitialization()
}

function configureCredential(provider: Provider) {
  credentialTarget.value = provider
  quotas.openSession(provider)
}

function closePriceEditor() {
  priceTarget.value = null
  priceSaving.value = false
}

function closeCredential() {
  quotas.closeSession()
  credentialTarget.value = null
  closePriceEditor()
}

async function createCredential(input: CredentialInput) {
  let saved = false
  await run(async () => {
    await api('/resources', 'POST', input)
    await loadResources()
    saved = true
    showSuccessToast(t('common.saved'))
  })
  return saved
}

function exportSubscription(resource: Resource) {
  void run(async () => {
    saveBlob(await download(`/resources/${resource.id}/credential/export`), 'auth.json')
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

function testCredentialFromModal(resource: Resource) {
  const provider = credentialTarget.value
  if (!provider) return
  closeCredential()
  prepareTestConnection(provider, resource)
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

function changeStatus(provider: Provider) {
  if (provider.status !== 'ACTIVE') {
    openActivation(provider)
    return
  }
  statusTarget.value = provider
  void run(async () => {
    try {
      await api(`/providers/${provider.id}/status`, 'PATCH', { status: 'DISABLED' })
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
})
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
    <ProviderTable
      :providers="visible"
      :busy="busy"
      :loading="loading"
      :status-target-id="statusTarget?.id"
      :guide="guideTargets"
      :runtime-for="providerRuntime"
      :has-credential="hasCredential"
      @test="prepareTestConnection($event)"
      @sync="syncModels"
      @change-status="changeStatus"
      @edit="openEdit"
      @credentials="configureCredential"
      @delete="deleteTarget = $event"
    />
    <div v-if="!visible.length" class="empty-state">
      <Icon name="providers" :size="32" />
      <p>
        {{ t(loading ? 'common.loading' : filtered ? 'common.noResults' : 'providers.empty') }}
      </p>
      <button
        v-if="!loading"
        type="button"
        class="button primary empty-state-action"
        @click="filtered ? reset() : openEdit()"
      >
        <Icon :name="filtered ? 'refresh' : 'plus'" :size="16" />
        {{ t(filtered ? 'common.reset' : 'providers.create') }}
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

  <ProviderInitializeModal
    v-if="providerInitializeOpen"
    v-model="selectedProviderCodes"
    :options="providerInitializeOptions"
    :busy="busy"
    @close="closeProviderInitialization"
    @submit="initializeProviders"
  />

  <ProviderEditorModal
    v-if="editing"
    :provider="editTarget"
    :models="editModels"
    :busy="busy"
    @close="editing = false"
    @submit="saveProvider"
  />

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

  <ProviderConnectionSelection
    v-if="testSelectionTarget"
    :provider="testSelectionTarget.provider"
    :detail="testSelectionTarget.detail"
    :resources="resourcesFor(testSelectionTarget.provider)"
    :preferred-resource-id="testSelectionTarget.resourceId"
    :mode="testSelectionTarget.mode"
    :busy="busy"
    @close="closeTestSelection"
    @submit="testSelectedProviderCredential"
  />

  <ProviderCredentialsModal
    v-if="credentialTarget && !priceTarget"
    :provider="credentialTarget"
    :resources="resourcesFor(credentialTarget)"
    :busy="busy"
    :quotas="quotas"
    :verified-at="credentialVerifiedAt"
    :create-credential="createCredential"
    @close="closeCredential"
    @test="testCredentialFromModal"
    @price="priceTarget = $event"
    @export="exportSubscription"
    @delete="credentialDeleteTarget = $event"
  />

  <Modal
    v-if="credentialTarget && priceTarget"
    :title="t('resources.pricing.title')"
    :wide="priceTarget.authType === 'API_KEY'"
    :busy="priceSaving"
    @close="closePriceEditor"
  >
    <CredentialPriceEditor
      :provider="credentialTarget"
      :resource="priceTarget"
      @close="closePriceEditor"
      @changed="loadResources"
      @busy="priceSaving = $event"
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

  <ConnectionResultModal
    v-if="testTarget"
    :target="testTarget"
    :result="testResult"
    :busy="busy"
    @close="testTarget = null"
  />

  <ModelSyncResultModal
    v-if="syncTarget"
    :provider="syncTarget.provider"
    :result="syncResult"
    :busy="busy"
    @close="syncTarget = null"
  />
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
@media (max-width: 760px) {
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
}
</style>
