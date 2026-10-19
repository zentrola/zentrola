<script setup lang="ts">
import { computed, ref } from 'vue'
import { date } from '../../composables'
import { t } from '../../i18n'
import { formatMoney } from '../../money'
import { quotaRemainingLabel, quotaWindowLabel, type CredentialQuotas } from '../../providerQuotas'
import { blockedResourceReason } from '../../providerRuntime'
import { showErrorToast } from '../../toast'
import type { Provider, Resource } from '../../types'
import Icon from '../Icon.vue'
import Modal from '../Modal.vue'
import Status from '../Status.vue'
import TechnicalValue from '../TechnicalValue.vue'
import SubscriptionImport, { type SubscriptionInputMode } from './SubscriptionImport.vue'

type CredentialAuthType = 'API_KEY' | 'SUBSCRIPTION'
export type CredentialInput = {
  providerId: string
  name: string
  credential: string
  authType: CredentialAuthType
  authAdapter: string
}

const props = defineProps<{
  provider: Provider
  resources: Resource[]
  busy: boolean
  quotas: CredentialQuotas
  verifiedAt: Record<string, string>
  // 保存成功返回 true，弹窗随后回到凭据列表。
  createCredential: (input: CredentialInput) => Promise<boolean>
}>()
const emit = defineEmits<{
  close: []
  test: [resource: Resource]
  price: [resource: Resource]
  export: [resource: Resource]
  delete: [resource: Resource]
}>()

const creating = ref(false)
const authType = ref<CredentialAuthType>('API_KEY')
const inputMode = ref<SubscriptionInputMode>('UPLOAD')
const fileName = ref('')
const credential = ref('')
const subscriptionAdapter = computed(
  () => props.provider.authAdapters?.find((adapter) => adapter !== 'API_KEY') ?? '',
)
const claudeSubscription = computed(() => subscriptionAdapter.value === 'ANTHROPIC_CLAUDE_CODE')
const subscriptionRequiredText = computed(() =>
  t(
    claudeSubscription.value
      ? 'resources.claudeSubscriptionRequired'
      : 'resources.subscriptionRequired',
  ),
)

function resetInput() {
  inputMode.value = 'UPLOAD'
  fileName.value = ''
  credential.value = ''
}

function startCreation() {
  creating.value = true
  authType.value = 'API_KEY'
  resetInput()
}

function cancelCreation() {
  creating.value = false
  resetInput()
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

async function save() {
  const valid =
    authType.value === 'API_KEY'
      ? /^[\x21-\x7e]{1,4096}$/.test(credential.value)
      : validSubscriptionCredential(credential.value)
  if (!valid) {
    showErrorToast(
      authType.value === 'API_KEY'
        ? t('resources.credentialRequired')
        : subscriptionRequiredText.value,
    )
    return
  }
  const saved = await props.createCredential({
    providerId: props.provider.id,
    name: `${props.provider.name} ${t(`resources.authTypes.${authType.value}`)}`,
    credential: credential.value,
    authType: authType.value,
    authAdapter: authType.value === 'SUBSCRIPTION' ? subscriptionAdapter.value : 'API_KEY',
  })
  if (saved) cancelCreation()
}

function lastVerification(resource: Resource) {
  return props.verifiedAt[resource.id] || resource.quotaCheckedAt || resource.lastErrorAt
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
  return `${formatMoney(price.periodAmount, price.currency)} / ${t(`resources.pricing.periods.${price.billingPeriod}`)}`
}

// 生效日期为 UTC 自然日，直接取 ISO 日期部分展示。
function subscriptionPriceEffectiveDate(resource: Resource) {
  return resource.subscriptionPrice?.effectiveAt.slice(0, 10) ?? '-'
}

function exportableSubscription(resource: Resource) {
  return (
    resource.authType === 'SUBSCRIPTION' &&
    resource.authAdapter === 'OPENAI_CODEX' &&
    resource.subscriptionType === 'PERSONAL'
  )
}
</script>

<template>
  <Modal
    :title="t('resources.configurationTitle')"
    :busy="busy"
    :wide="!creating"
    @close="emit('close')"
  >
    <template v-if="!creating">
      <div class="credential-list-head">
        <div class="credential-list-heading">
          <div class="credential-title-line">
            <h3>{{ provider.name }}</h3>
            <Status :value="provider.status" />
          </div>
          <p>{{ t('resources.listHint') }}</p>
        </div>
        <button type="button" class="button primary" :disabled="busy" @click="startCreation">
          <Icon name="plus" :size="16" />{{ t('resources.addCredential') }}
        </button>
      </div>
      <dl class="credential-overview">
        <div class="credential-overview-endpoints">
          <dt>{{ t('resources.baseUrl') }}</dt>
          <dd>
            <span v-for="endpoint in provider.endpoints" :key="endpoint.protocolType">
              <b>{{ endpoint.protocolType === 'OPENAI' ? 'OpenAI' : 'Anthropic' }}</b>
              <span v-if="endpoint.networkScope === 'PRIVATE'" class="endpoint-private-badge">{{
                t('providers.networkPrivate')
              }}</span>
              <TechnicalValue :value="endpoint.baseUrl" />
            </span>
            <span v-if="!provider.endpoints.length">{{ t('common.none') }}</span>
          </dd>
        </div>
      </dl>
      <div v-if="resources.length" class="credential-list">
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
                    v-if="
                      resource.authType === 'SUBSCRIPTION' &&
                      quotas.refreshing[resource.id] &&
                      !quotas.displayableQuotas(resource.id).length
                    "
                    class="credential-detail credential-quota-feedback"
                  >
                    {{ t('resources.quotaRefreshing') }}
                  </small>
                  <small
                    v-else-if="
                      resource.authType === 'SUBSCRIPTION' && quotas.refreshErrors[resource.id]
                    "
                    class="credential-detail credential-quota-feedback is-error"
                    :title="quotas.refreshErrors[resource.id]"
                  >
                    {{ t('resources.quotaRefreshFailed') }} ·
                    {{ quotas.refreshErrors[resource.id] }}
                  </small>
                  <div
                    v-else-if="
                      resource.authType === 'SUBSCRIPTION' &&
                      quotas.displayableQuotas(resource.id).length
                    "
                    class="credential-quota-windows"
                  >
                    <small
                      v-if="quotas.refreshing[resource.id]"
                      class="credential-detail credential-quota-feedback"
                    >
                      {{ t('resources.quotaRefreshing') }}
                    </small>
                    <div
                      v-for="quota in quotas.displayableQuotas(resource.id)"
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
                    <small v-if="quotas.quotas[resource.id]" class="credential-detail">
                      {{ t('resources.quotaAmountUnknown') }}
                    </small>
                    <small class="credential-detail credential-quota-reset">
                      {{ t('resources.quotaResetsAt') }} · {{ date(resource.quotaResetsAt) }}
                    </small>
                  </template>
                  <div
                    v-if="resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE'"
                    class="credential-quota-refresh"
                  >
                    <small class="credential-detail">
                      {{ t('resources.quotaCheckedAt') }} · {{ date(resource.quotaCheckedAt) }}
                    </small>
                    <button
                      type="button"
                      class="text-button"
                      :aria-label="t('resources.refreshQuotaFor', { name: resource.name })"
                      :title="t('resources.refreshQuotaHint')"
                      :disabled="busy || quotas.refreshing[resource.id]"
                      @click="quotas.refreshClaudeQuota(resource)"
                    >
                      {{ t('resources.refreshQuota') }}
                    </button>
                  </div>
                  <div
                    v-if="
                      resource.authAdapter === 'OPENAI_CODEX' && quotas.resetCredits[resource.id]
                    "
                    class="credential-reset-credits"
                  >
                    <small class="credential-detail">
                      {{
                        t('resources.resetCreditsAvailable', {
                          count: quotas.resetCredits[resource.id].availableCount,
                        })
                      }}
                    </small>
                    <button
                      v-if="quotas.resetCredits[resource.id].availableCount > 0"
                      type="button"
                      class="text-button"
                      :disabled="busy || quotas.consuming[resource.id]"
                      @click="quotas.consumeResetCredit(resource)"
                    >
                      {{
                        t(
                          quotas.consuming[resource.id]
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
      <div v-else class="credential-empty">
        <Icon name="lock-open" :size="28" />
        <strong>{{ t('resources.emptyCredentials') }}</strong>
        <p>{{ t('resources.emptyCredentialsHint') }}</p>
      </div>
    </template>
    <form v-else id="credential-create-form" class="credential-create-form" @submit.prevent="save">
      <p class="muted">{{ t('resources.createHint') }}</p>
      <div class="credential-form-row">
        <label class="credential-form-label" for="credential-auth-type">{{
          t('resources.authType')
        }}</label>
        <div class="credential-form-control">
          <select
            id="credential-auth-type"
            v-model="authType"
            :disabled="busy"
            @change="resetInput"
          >
            <option value="API_KEY">{{ t('resources.authTypes.API_KEY') }}</option>
            <option
              v-if="provider.authAdapters?.some((adapter) => adapter !== 'API_KEY')"
              value="SUBSCRIPTION"
            >
              {{ t('resources.authTypes.SUBSCRIPTION') }}
            </option>
          </select>
        </div>
      </div>
      <div v-if="authType === 'API_KEY'" class="credential-form-row">
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
      <p v-if="authType === 'API_KEY'" class="field-hint credential-form-hint">
        {{ t('resources.credentialHint') }}
      </p>
      <SubscriptionImport
        v-else
        v-model:mode="inputMode"
        v-model:credential="credential"
        v-model:file-name="fileName"
        :claude-subscription="claudeSubscription"
        :busy="busy"
        :invalid-text="subscriptionRequiredText"
      />
    </form>
    <template v-if="creating" #footer>
      <button type="button" class="button" :disabled="busy" @click="cancelCreation">
        {{ t('common.cancel') }}</button
      ><button type="submit" form="credential-create-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
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
.credential-reset-credits,
.credential-quota-refresh {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--line);
}
.credential-reset-credits .credential-detail,
.credential-quota-refresh .credential-detail {
  margin-top: 0;
}
.credential-reset-credits .text-button,
.credential-quota-refresh .text-button {
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
@media (max-width: 760px) {
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
  .provider-toolbar-actions .button {
    flex: 1;
  }
}
</style>
