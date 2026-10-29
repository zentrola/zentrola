<script setup lang="ts">
import { date } from '../../composables'
import { t } from '../../i18n'
import { exportableSubscription } from '../../providerCredentials'
import type { CredentialQuotas } from '../../providerQuotas'
import { blockedResourceReason } from '../../providerRuntime'
import type { Provider, Resource } from '../../types'
import Icon from '../Icon.vue'
import Status from '../Status.vue'
import StatusSwitch from '../StatusSwitch.vue'
import CredentialAuthDetails from './CredentialAuthDetails.vue'

const props = defineProps<{
  provider: Provider
  resources: Resource[]
  busy: boolean
  quotas: CredentialQuotas
  verifiedAt: Record<string, string>
}>()
const emit = defineEmits<{
  test: [resource: Resource]
  price: [resource: Resource]
  export: [resource: Resource]
  status: [resource: Resource]
  delete: [resource: Resource]
}>()

function lastVerification(resource: Resource) {
  return props.verifiedAt[resource.id] || resource.quotaCheckedAt || resource.lastErrorAt
}

function verificationLabel(resource: Resource) {
  return t(
    resource.runtimeStatus === 'BLOCKED' ? 'resources.verifyAndRestoreFor' : 'resources.verifyFor',
    { name: resource.name },
  )
}
</script>

<template>
  <div class="credential-list">
    <table>
      <colgroup>
        <col class="credential-name-column" />
        <col class="credential-auth-column" />
        <col class="credential-status-column" />
        <col class="credential-runtime-column" />
        <col class="credential-operation-column" />
      </colgroup>
      <thead>
        <tr>
          <th>{{ t('resources.credential') }}</th>
          <th>{{ t('resources.authType') }}</th>
          <th>{{ t('resources.routingStatus') }}</th>
          <th>{{ t('resources.runtimeStatus') }}</th>
          <th class="credential-action-column">{{ t('common.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="resource in resources" :key="resource.id">
          <td :data-label="t('resources.credential')">
            <strong>{{ resource.name }}</strong>
            <small v-if="resource.externalAccountRef" class="credential-detail">{{
              resource.externalAccountRef
            }}</small>
            <small class="credential-detail">
              {{ t('resources.lastVerifiedAt') }} ·
              {{ date(lastVerification(resource)) }}
            </small>
          </td>
          <td class="credential-auth" :data-label="t('resources.authType')">
            <CredentialAuthDetails :resource="resource" :busy="busy" :quotas="quotas" />
          </td>
          <td :data-label="t('resources.routingStatus')">
            <StatusSwitch
              :value="resource.isEnabled ? 'ACTIVE' : 'DISABLED'"
              :name="resource.name"
              :aria-label="t('resources.routingStatusFor', { name: resource.name })"
              :disabled="busy"
              @change="emit('status', resource)"
            />
          </td>
          <td class="credential-runtime" :data-label="t('resources.runtimeStatus')">
            <div class="credential-runtime-content">
              <div class="credential-runtime-line">
                <Status :value="resource.runtimeStatus || 'HEALTHY'" />
                <button
                  v-if="resource.authType === 'SUBSCRIPTION' || provider.modelCount > 0"
                  type="button"
                  class="icon-button credential-verify-action"
                  :class="{ 'is-blocked': resource.runtimeStatus === 'BLOCKED' }"
                  :aria-label="verificationLabel(resource)"
                  :title="verificationLabel(resource)"
                  :disabled="busy"
                  @click="emit('test', resource)"
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
                @click="emit('price', resource)"
              >
                {{ t('resources.pricing.action') }}
              </button>
              <button
                v-if="exportableSubscription(resource)"
                type="button"
                class="text-button"
                :aria-label="t('resources.exportFor', { name: resource.name })"
                :disabled="busy"
                @click="emit('export', resource)"
              >
                {{ t('resources.export') }}
              </button>
              <button
                type="button"
                class="text-button danger"
                :disabled="busy"
                @click="emit('delete', resource)"
              >
                {{ t('resources.delete') }}
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
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
  width: 29%;
}
.credential-auth-column {
  width: 24%;
}
.credential-status-column {
  width: 84px;
}
.credential-runtime-column {
  width: auto;
}
.credential-operation-column {
  width: 90px;
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
.credential-action-column .row-actions {
  flex-direction: column;
  align-items: flex-end;
  gap: 0;
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
  color: var(--color-warning-text);
}
@media (max-width: 760px) {
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
  /* 额度刷新等按钮位于 CredentialAuthDetails 内，需穿透 scoped 样式。 */
  .credential-list :deep(.text-button) {
    justify-self: start;
  }
  .credential-list .row-actions {
    flex-direction: row;
    flex-wrap: wrap;
    justify-content: flex-start;
  }
}
</style>
