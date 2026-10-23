<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { all, api, errorText } from '../api'
import { normalizeDashboard, utcMonthRange } from '../dashboard'
import { t } from '../i18n'
import type { ConnectionResult, Dashboard, Provider, Resource } from '../types'
import Icon from '../components/Icon.vue'
import InitializationGuide from '../components/InitializationGuide.vue'
import PageHeader from '../components/PageHeader.vue'
import AccessPanel from '../components/home/AccessPanel.vue'
import AccessSetupModal from '../components/home/AccessSetupModal.vue'
import CredentialHealthPanel from '../components/home/CredentialHealthPanel.vue'
import HomeMetrics from '../components/home/HomeMetrics.vue'
import HomeRankings from '../components/home/HomeRankings.vue'

const summary = ref<Dashboard | null>(null)
const healthProviders = ref<Provider[]>([])
const healthResources = ref<Resource[]>([])
const loading = ref(false)
const error = ref('')
const credentialChecks = ref<Record<string, { result: ConnectionResult; checkedAt: string }>>({})
const checkingResourceId = ref<string | null>(null)
const credentialCheckError = ref<Record<string, string>>({})
const providerHealthLoading = ref(false)
const providerHealthError = ref('')
const initializationGuideOpen = ref(false)
const setupOpen = ref(false)
let revision = 0

const currentMonth = ref(new Date())

function monthQuery() {
  const { from, to } = utcMonthRange(new Date())
  currentMonth.value = from
  return new URLSearchParams({ from: from.toISOString(), to: to.toISOString() }).toString()
}

async function loadDashboard(current: number) {
  error.value = ''
  try {
    const result = await api<Dashboard>(`/usage/dashboard?${monthQuery()}`)
    if (current === revision) summary.value = normalizeDashboard(result)
  } catch (e) {
    if (current === revision) error.value = errorText(e)
  }
}

async function loadProviderHealth(current: number) {
  providerHealthLoading.value = true
  providerHealthError.value = ''
  try {
    const [providers, resources] = await Promise.all([
      all<Provider>('/providers'),
      all<Resource>('/resources'),
    ])
    if (current === revision) {
      healthProviders.value = providers
      healthResources.value = resources
    }
  } catch (e) {
    if (current === revision) providerHealthError.value = errorText(e)
  } finally {
    if (current === revision) providerHealthLoading.value = false
  }
}

async function checkCredential(resource: Resource) {
  if (checkingResourceId.value) return
  checkingResourceId.value = resource.id
  delete credentialCheckError.value[resource.id]
  try {
    const result = await api<ConnectionResult>(`/resources/${resource.id}/test-connection`, 'POST')
    credentialChecks.value[resource.id] = { result, checkedAt: new Date().toISOString() }
    await loadProviderHealth(revision)
  } catch (reason) {
    credentialCheckError.value[resource.id] = errorText(reason)
  } finally {
    checkingResourceId.value = null
  }
}

async function load() {
  const current = ++revision
  loading.value = true
  await Promise.all([loadDashboard(current), loadProviderHealth(current)])
  if (current === revision) loading.value = false
}

onMounted(load)
</script>

<template>
  <PageHeader name="home" :show-description="false">
    <div class="dashboard-heading-actions">
      <button
        type="button"
        class="button"
        aria-haspopup="dialog"
        @click="initializationGuideOpen = true"
      >
        <Icon name="guide" :size="17" />{{ t('home.initializationAction') }}
      </button>
      <button class="button" :disabled="loading" @click="load">
        <Icon name="refresh" :size="16" />{{ t('home.refresh') }}
      </button>
    </div>
  </PageHeader>

  <p v-if="error" class="alert error dashboard-alert" role="alert">
    {{ error }}<button class="text-button" @click="load">{{ t('common.retry') }}</button>
  </p>

  <div class="dashboard-overview" :aria-busy="loading">
    <HomeMetrics
      :summary="summary"
      :month="currentMonth"
      :providers="healthProviders"
      :resources="healthResources"
      :health-loading="providerHealthLoading"
      :health-error="providerHealthError"
    />
    <AccessPanel @setup="setupOpen = true" />
  </div>

  <CredentialHealthPanel
    :providers="healthProviders"
    :resources="healthResources"
    :loading="providerHealthLoading"
    :error="providerHealthError"
    :checks="credentialChecks"
    :check-errors="credentialCheckError"
    :checking-resource-id="checkingResourceId"
    @retry="loadProviderHealth(revision)"
    @check="checkCredential"
  />

  <HomeRankings :summary="summary" :loading="loading" :month="currentMonth" />

  <AccessSetupModal v-if="setupOpen" @close="setupOpen = false" />
  <InitializationGuide v-if="initializationGuideOpen" @close="initializationGuideOpen = false" />
</template>

<style scoped>
.dashboard-heading-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

@media (max-width: 560px) {
  .dashboard-heading-actions {
    width: 100%;
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
