<script setup lang="ts">
import { date } from '../../composables'
import { t } from '../../i18n'
import { subscriptionPriceEffectiveDate, subscriptionPriceLabel } from '../../providerCredentials'
import { quotaRemainingLabel, quotaWindowLabel, type CredentialQuotas } from '../../providerQuotas'
import type { Resource } from '../../types'

defineProps<{ resource: Resource; busy: boolean; quotas: CredentialQuotas }>()
</script>

<template>
  <div class="credential-auth-content">
    <span v-if="resource.authType !== 'SUBSCRIPTION'">
      {{ t(`resources.authTypes.${resource.authType || 'API_KEY'}`) }}
    </span>
    <small v-if="resource.planCode" class="credential-detail credential-plan">
      {{ t('resources.plan') }} · {{ resource.planCode }}
    </small>
    <div v-if="resource.authType === 'SUBSCRIPTION'" class="credential-subscription-price">
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
      v-else-if="resource.authType === 'SUBSCRIPTION' && quotas.refreshErrors[resource.id]"
      class="credential-detail credential-quota-feedback is-error"
      :title="quotas.refreshErrors[resource.id]"
    >
      {{ t('resources.quotaRefreshFailed') }} ·
      {{ quotas.refreshErrors[resource.id] }}
    </small>
    <div
      v-else-if="
        resource.authType === 'SUBSCRIPTION' && quotas.displayableQuotas(resource.id).length
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
    <div v-if="resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE'" class="credential-quota-refresh">
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
      v-if="resource.authAdapter === 'OPENAI_CODEX' && quotas.resetCredits[resource.id]"
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
</template>

<style scoped>
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
/* 与凭据列表的 .credential-detail 相同；需排在 .credential-plan 之后以保持原有层叠顺序。 */
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
</style>
