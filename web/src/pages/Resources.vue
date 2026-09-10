<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, all, errorText } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t, i18n } from '../i18n'
import { showErrorToast } from '../toast'
import type { Resource, Provider, ConnectionResult } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
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
} = useCollection<Resource>(() => '/resources')
const { busy, error: actionError, run } = useAction()
const providers = ref<Provider[]>([]),
  providerError = ref(''),
  creating = ref(false),
  replaceTarget = ref<Resource | null>(null),
  statusTarget = ref<Resource | null>(null),
  testTarget = ref<Resource | null>(null),
  testResult = ref<ConnectionResult | null>(null)
const name = ref(''),
  providerID = ref(''),
  credential = ref('')
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (r) => `${r.name} ${r.id} ${providerName(r.providerId)}`,
  load,
)
function providerName(id: string) {
  return providers.value.find((p) => p.id === id)?.name || id
}
async function loadProviders() {
  providerError.value = ''
  try {
    providers.value = await all<Provider>('/providers')
  } catch (e) {
    providerError.value = errorText(e)
  }
}
onMounted(() => {
  void load()
  void loadProviders()
})
function newResource() {
  creating.value = true
  name.value = ''
  providerID.value = ''
  credential.value = ''
  actionError.value = ''
}
function replace(resource: Resource) {
  replaceTarget.value = resource
  credential.value = ''
  actionError.value = ''
}
function closeEdit() {
  creating.value = false
  replaceTarget.value = null
  credential.value = ''
}
function save() {
  if (!/^[\x21-\x7e]{1,4096}$/.test(credential.value)) {
    showErrorToast(t('resources.credentialRequired'))
    return
  }
  if (creating.value && (!validText(name.value, 128) || !providerID.value)) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  void run(async () => {
    if (creating.value)
      await api('/resources', 'POST', {
        name: name.value,
        providerId: providerID.value,
        credential: credential.value,
      })
    else
      await api(`/resources/${replaceTarget.value!.id}/credential`, 'PUT', {
        credential: credential.value,
      })
    closeEdit()
    await load()
  })
}
function changeStatus(resource: Resource) {
  actionError.value = ''
  statusTarget.value = resource
  void run(async () => {
    try {
      await api(`/resources/${resource.id}/status`, 'PATCH', {
        status: resource.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
      })
      await refresh()
    } finally {
      statusTarget.value = null
    }
  })
}
function test(resource: Resource) {
  testTarget.value = resource
  testResult.value = null
  actionError.value = ''
  void run(async () => {
    testResult.value = await api(`/resources/${resource.id}/test-connection`, 'POST')
  })
}
function resultMessage(result: ConnectionResult) {
  return result.ok
    ? t('resources.testPassed')
    : t(i18n.global.te(`errors.${result.code}`) ? `errors.${result.code}` : 'errors.UNKNOWN')
}
function runtimeReason(resource: Resource) {
  if (!resource.blockedReason) return ''
  const key = `resources.blockReasons.${resource.blockedReason}`
  const reason = t(i18n.global.te(key) ? key : 'resources.blockReasons.UNKNOWN_PERMANENT')
  return resource.lastHttpStatus ? `${reason} · HTTP ${resource.lastHttpStatus}` : reason
}
</script>
<template>
  <PageHeader name="resources"
    ><button class="button primary" @click="newResource">
      <Icon name="plus" :size="18" />{{ t('resources.create') }}
    </button></PageHeader
  >
  <div class="context-note">
    <Icon name="resources" :size="19" /><span>{{ t('resources.limit') }}</span>
  </div>
  <p v-if="providerError" class="alert error" role="alert">
    {{ providerError
    }}<button class="text-button" @click="loadProviders">{{ t('common.retry') }}</button>
  </p>
  <section class="panel">
    <ListSearch v-model="keyword" :loading="loading" @search="search" @reset="reset" />
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </p>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('resources.provider') }}</th>
            <th>{{ t('resources.credential') }}</th>
            <th>{{ t('resources.runtimeStatus') }}</th>
            <th>{{ t('common.status') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="resource in visible" :key="resource.id">
            <td>
              <strong>{{ resource.name }}</strong
              ><small class="subline">{{ date(resource.updatedAt) }}</small>
            </td>
            <td>{{ providerName(resource.providerId) }}</td>
            <td>
              <span class="credential-state"
                ><Icon name="shield" :size="15" />{{
                  t(resource.credentialConfigured ? 'resources.configured' : 'resources.missing')
                }}</span
              >
            </td>
            <td>
              <Status :value="resource.runtimeStatus" />
              <small
                v-if="resource.runtimeStatus === 'BLOCKED'"
                class="subline"
                :title="resource.lastErrorCode || undefined"
                >{{ runtimeReason(resource) }}</small
              >
            </td>
            <td>
              <StatusSwitch
                :value="resource.status"
                :name="resource.name"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === resource.id"
                @change="changeStatus(resource)"
              />
            </td>
            <td>
              <div class="row-actions">
                <button class="text-button" :disabled="busy" @click="test(resource)">
                  {{ t('resources.test') }}</button
                ><button class="text-button" @click="replace(resource)">
                  {{ t('resources.replace') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="resources" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'common.empty') }}</h3>
      <p v-if="!loading && !query">{{ t('resources.empty') }}</p>
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
    v-if="creating || replaceTarget"
    :title="t(creating ? 'resources.create' : 'resources.replace')"
    :busy="busy"
    @close="closeEdit"
    ><p class="muted">{{ t(creating ? 'resources.createHint' : 'resources.replaceHint') }}</p>
    <form @submit.prevent="save">
      <template v-if="creating"
        ><label
          >{{ t('common.name') }}<input v-model="name" required :disabled="busy" autofocus /></label
        ><label
          >{{ t('resources.provider')
          }}<select v-model="providerID" required :disabled="busy">
            <option value="">{{ t('common.select') }}</option>
            <option
              v-for="provider in providers.filter((p) => p.status === 'ACTIVE')"
              :key="provider.id"
              :value="provider.id"
            >
              {{ provider.name }}
            </option>
          </select></label
        >
        <p v-if="providerError" class="alert error">
          {{ providerError
          }}<button type="button" class="text-button" @click="loadProviders">
            {{ t('common.retry') }}
          </button>
        </p></template
      ><label
        >{{ t('resources.credential')
        }}<input
          v-model="credential"
          type="password"
          autocomplete="new-password"
          required
          :disabled="busy"
          spellcheck="false"
      /></label>
      <p class="field-hint">{{ t('resources.credentialHint') }}</p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="closeEdit">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'common.save') }}
        </button>
      </footer>
    </form></Modal
  >
  <Modal
    v-if="testTarget"
    :title="`${testTarget.name} / ${t('resources.result')}`"
    :busy="busy"
    @close="testTarget = null"
    ><p v-if="busy" role="status">{{ t('resources.testing') }}</p>
    <template v-if="testResult"
      ><div class="alert" :class="testResult.ok ? 'success' : 'error'" role="status">
        {{ resultMessage(testResult) }}
      </div>
      <dl class="detail-grid">
        <dt>{{ t('common.code') }}</dt>
        <dd>{{ testResult.code }}</dd>
        <dt>HTTP</dt>
        <dd>{{ testResult.httpStatus || t('common.none') }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ testResult.latencyMs }} ms</dd>
      </dl></template
    >
    <p class="muted">{{ t('resources.testHint') }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="testTarget = null">{{ t('close') }}</button>
    </footer></Modal
  >
</template>
