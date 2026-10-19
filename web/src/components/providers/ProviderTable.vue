<script setup lang="ts">
import { t } from '../../i18n'
import { providerRuntimeLabel, type ProviderRuntime } from '../../providerRuntime'
import type { Provider, ProviderProtocol } from '../../types'
import Icon from '../Icon.vue'
import RowActionMenu from '../RowActionMenu.vue'
import Status from '../Status.vue'
import StatusSwitch from '../StatusSwitch.vue'
import TableScroll from '../TableScroll.vue'
import TechnicalValue from '../TechnicalValue.vue'

export type ProviderGuideTargets = {
  credential: string
  website: string
  sync: string
  test: string
}

const props = defineProps<{
  providers: Provider[]
  busy: boolean
  loading: boolean
  statusTargetId?: string
  guide: ProviderGuideTargets
  runtimeFor: (provider: Provider) => ProviderRuntime
  hasCredential: (provider: Provider) => boolean
}>()
const emit = defineEmits<{
  test: [provider: Provider]
  sync: [provider: Provider]
  changeStatus: [provider: Provider]
  edit: [provider: Provider]
  credentials: [provider: Provider]
  delete: [provider: Provider]
}>()

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

function enableBlockReason(provider: Provider) {
  if (provider.status === 'ACTIVE') return undefined
  const credentialConfigured = props.hasCredential(provider)
  const modelConfigured = provider.modelCount > 0
  if (!credentialConfigured && !modelConfigured)
    return t('providers.configurationRequiredBeforeEnable')
  if (!modelConfigured) return t('providers.modelRequiredBeforeEnable')
  if (!credentialConfigured) return t('providers.credentialRequiredBeforeEnable')
  return undefined
}

function runtimeActionLabel(provider: Provider) {
  const runtime = props.runtimeFor(provider)
  if (runtime.status === 'MISSING_MODEL') {
    return t('providers.configureModelsFromRuntime', { name: provider.name })
  }
  return t('providers.openCredentialsFromRuntime', {
    name: provider.name,
    status: providerRuntimeLabel(runtime),
  })
}

// 缺少模型时直接打开编辑弹窗配置映射，其余状态查看凭据。
function openRuntime(provider: Provider) {
  if (props.runtimeFor(provider).status === 'MISSING_MODEL') emit('edit', provider)
  else emit('credentials', provider)
}
</script>

<template>
  <TableScroll has-actions>
    <table class="providers-table">
      <colgroup>
        <col class="record-id-column" />
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
        <tr v-for="provider in providers" :key="provider.id">
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
                  :id="provider.id === guide.website ? 'provider-guide-website' : undefined"
                  class="provider-quick-action provider-website-action"
                  :href="provider.website"
                  target="_blank"
                  rel="noopener noreferrer"
                  :aria-label="t('providers.openWebsiteFor', { name: provider.name })"
                  :title="t('providers.visitWebsite')"
                >
                  <Icon name="website" :size="15" /></a
                ><button
                  v-if="hasCredential(provider) && provider.modelCount > 0"
                  type="button"
                  class="provider-quick-action provider-direct-action"
                  :id="provider.id === guide.test ? 'provider-guide-test' : undefined"
                  :aria-label="t('providers.testConnectionFor', { name: provider.name })"
                  :title="t('resources.test')"
                  :disabled="busy"
                  @click="emit('test', provider)"
                >
                  <Icon name="activity" :size="16" /></button
                ><button
                  v-if="provider.modelSyncSupported"
                  type="button"
                  class="provider-quick-action provider-direct-action"
                  :id="provider.id === guide.sync ? 'provider-guide-sync' : undefined"
                  :aria-label="t('providers.syncModelsFor', { name: provider.name })"
                  :title="t('resources.syncModels')"
                  :disabled="busy"
                  @click="emit('sync', provider)"
                >
                  <Icon name="refresh" :size="16" /></button
              ></span>
            </div>
          </td>
          <td>
            <StatusSwitch
              :value="provider.status"
              :name="provider.name"
              :disabled="busy || loading || Boolean(enableBlockReason(provider))"
              :busy="busy && statusTargetId === provider.id"
              :title="enableBlockReason(provider)"
              @change="emit('changeStatus', provider)"
            />
          </td>
          <td class="provider-runtime">
            <button
              type="button"
              class="provider-runtime-state"
              :title="runtimeActionLabel(provider)"
              :aria-label="runtimeActionLabel(provider)"
              :disabled="busy"
              @click="openRuntime(provider)"
            >
              <Status :value="runtimeFor(provider).status" />
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
                :id="provider.id === guide.credential ? 'provider-guide-edit' : undefined"
                class="text-button"
                :disabled="busy"
                @click="emit('edit', provider)"
              >
                {{ t('providers.edit') }}
              </button>
              <button
                type="button"
                class="text-button"
                :disabled="busy"
                :id="provider.id === guide.credential ? 'provider-guide-credential' : undefined"
                :aria-label="t('providers.editCredentialFor', { name: provider.name })"
                :title="t('providers.editCredentialFor', { name: provider.name })"
                @click="emit('credentials', provider)"
              >
                {{ t('providers.credentialAction') }}
              </button>
              <RowActionMenu
                :label="t('providers.moreActionsFor', { name: provider.name })"
                :title="t('providers.moreActions')"
              >
                <button
                  type="button"
                  class="text-button danger"
                  :disabled="busy"
                  @click="emit('delete', provider)"
                >
                  {{ t('providers.delete') }}
                </button>
              </RowActionMenu>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </TableScroll>
</template>

<style scoped>
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
.provider-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  white-space: nowrap;
}
</style>
