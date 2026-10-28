<script setup lang="ts">
import { computed } from 'vue'
import { date } from '../../composables'
import { t } from '../../i18n'
import { blockedResourceReason } from '../../providerRuntime'
import { connectionResultMessage } from '../../providerQuotas'
import type { ConnectionResult, Provider, Resource } from '../../types'

const props = defineProps<{
  providers: Provider[]
  resources: Resource[]
  loading: boolean
  error: string
  checks: Record<string, { result: ConnectionResult; checkedAt: string }>
  checkErrors: Record<string, string>
  checkingResourceId: string | null
}>()
const emit = defineEmits<{ retry: []; check: [resource: Resource] }>()

type HealthState = 'available' | 'unavailable' | 'unknown'

const groups = computed(() =>
  props.providers
    .filter((provider) => provider.status === 'ACTIVE')
    .map((provider) => ({
      provider,
      resources: props.resources
        .filter((resource) => resource.providerId === provider.id)
        .sort((a, b) => a.priority - b.priority || a.name.localeCompare(b.name)),
    })),
)
function health(resource: Resource): {
  state: HealthState
  reason: string
  checkedAt: string | null
} {
  const check = props.checks[resource.id]
  if (!resource.isEnabled)
    return { state: 'unavailable', reason: t('home.credentialDisabled'), checkedAt: null }
  if (!resource.credentialConfigured)
    return { state: 'unavailable', reason: t('home.credentialNotConfigured'), checkedAt: null }
  if (resource.effectiveAt && Date.parse(resource.effectiveAt) > Date.now())
    return { state: 'unavailable', reason: t('home.credentialNotEffective'), checkedAt: null }
  if (resource.expiresAt && Date.parse(resource.expiresAt) <= Date.now())
    return { state: 'unavailable', reason: t('home.credentialExpired'), checkedAt: null }
  if (check)
    return {
      state: check.result.ok ? 'available' : 'unavailable',
      reason: connectionResultMessage(check.result, resource),
      checkedAt: check.checkedAt,
    }
  if (resource.runtimeStatus === 'BLOCKED')
    return {
      state: 'unavailable',
      reason: blockedResourceReason(resource),
      checkedAt: resource.lastErrorAt,
    }
  if (resource.quotaStatus === 'EXHAUSTED')
    return {
      state: 'unavailable',
      reason: t('home.credentialQuotaExhausted'),
      checkedAt: resource.quotaCheckedAt,
    }
  if (
    resource.authType === 'SUBSCRIPTION' &&
    resource.quotaCheckedAt &&
    (resource.quotaStatus === 'AVAILABLE' || resource.quotaStatus === 'NEAR_LIMIT')
  ) {
    const age = Date.now() - Date.parse(resource.quotaCheckedAt)
    if (age >= 0 && age < 24 * 60 * 60 * 1000)
      return { state: 'available', reason: '', checkedAt: resource.quotaCheckedAt }
    return {
      state: 'unknown',
      reason: t('home.credentialCheckExpired'),
      checkedAt: resource.quotaCheckedAt,
    }
  }
  return { state: 'unknown', reason: t('home.credentialUnverifiedHint'), checkedAt: null }
}

function canCheck(provider: Provider, resource: Resource) {
  return resource.authType === 'SUBSCRIPTION' || provider.modelCount > 0
}

function checkLabel(resource: Resource) {
  return t('home.checkCredentialFor', { name: resource.name })
}
</script>

<template>
  <section class="credential-health-panel" :aria-label="t('home.credentialHealth')">
    <div class="credential-health-head">
      <h2>{{ t('home.credentialHealth') }}</h2>
      <div class="credential-health-legend" :aria-label="t('home.credentialLegend')">
        <span><i class="is-available"></i>{{ t('home.credentialStatus.available') }}</span>
        <span><i class="is-unavailable"></i>{{ t('home.credentialStatus.unavailable') }}</span>
        <span><i class="is-unknown"></i>{{ t('home.credentialStatus.unknown') }}</span>
      </div>
    </div>

    <p v-if="loading" class="credential-health-message" role="status">
      {{ t('home.credentialHealthLoading') }}
    </p>
    <div v-else-if="error" class="alert error credential-health-message" role="alert">
      {{ error }}
      <button type="button" class="text-button" @click="emit('retry')">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-else-if="!groups.length" class="credential-health-message">
      {{ t('home.credentialHealthEmpty') }}
    </p>
    <div v-else class="credential-health-list">
      <div v-for="group in groups" :key="group.provider.id" class="credential-health-group">
        <h3 :title="group.provider.name">{{ group.provider.name }}</h3>
        <div v-if="group.resources.length" class="credential-health-items">
          <div
            v-for="resource in group.resources"
            :key="resource.id"
            class="credential-health-item"
          >
            <span class="credential-health-tag">
              <span class="credential-health-name" :title="resource.name">{{ resource.name }}</span>
              <span class="credential-health-marker">
                <button
                  v-if="canCheck(group.provider, resource)"
                  type="button"
                  class="credential-health-dot-button"
                  :class="`is-${health(resource).state}`"
                  :disabled="checkingResourceId !== null"
                  :aria-label="checkLabel(resource)"
                  :aria-describedby="`credential-health-tip-${resource.id}`"
                  @click="emit('check', resource)"
                >
                  <i></i>
                </button>
                <span
                  v-else
                  class="credential-health-dot-button"
                  :class="`is-${health(resource).state}`"
                  tabindex="0"
                  :aria-label="`${resource.name}：${t(`home.credentialStatus.${health(resource).state}`)}`"
                  :aria-describedby="`credential-health-tip-${resource.id}`"
                  ><i></i
                ></span>
                <span
                  :id="`credential-health-tip-${resource.id}`"
                  class="credential-health-tooltip"
                  role="tooltip"
                >
                  <strong :class="`is-${health(resource).state}`">
                    {{ t(`home.credentialStatus.${health(resource).state}`) }}
                  </strong>
                  <span v-if="health(resource).reason">{{ health(resource).reason }}</span>
                  <code
                    v-if="
                      !checks[resource.id] &&
                      resource.runtimeStatus === 'BLOCKED' &&
                      resource.lastErrorCode
                    "
                  >
                    {{ resource.lastErrorCode }}
                  </code>
                  <small
                    >{{ t('home.credentialLastChecked') }}
                    {{ date(health(resource).checkedAt) }}</small
                  >
                  <small
                    v-if="checkErrors[resource.id]"
                    class="credential-check-error"
                    role="alert"
                  >
                    {{ checkErrors[resource.id] }}
                  </small>
                  <small v-if="canCheck(group.provider, resource)">
                    {{
                      checkingResourceId === resource.id
                        ? t('home.checkingCredential')
                        : t('home.credentialClickToCheck')
                    }}
                  </small>
                </span>
              </span>
            </span>
          </div>
        </div>
        <span v-else class="credential-health-empty" :title="t('home.credentialMissing')">-</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.credential-health-panel {
  margin-bottom: 18px;
  padding: 20px 24px;
  border: 1px solid rgb(226 232 240 / 80%);
  border-radius: 12px;
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}
.credential-health-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.credential-health-head h2 {
  margin: 0;
}
.credential-health-legend,
.credential-health-empty {
  color: var(--muted);
  font-size: 12px;
}
.credential-health-legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px 18px;
}
.credential-health-legend > span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.credential-health-legend i,
.credential-health-dot-button i {
  display: block;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: currentColor;
}
.is-available {
  color: #16834a;
}
.is-unavailable {
  color: #c43b3b;
}
.is-unknown {
  color: #788598;
}
.credential-health-list {
  margin-top: 14px;
  border-top: 1px solid var(--line);
}
.credential-health-group {
  display: grid;
  grid-template-columns: minmax(0, 96px) minmax(0, 1fr);
  align-items: start;
  gap: 8px;
  padding: 10px 0;
  border-bottom: 1px solid var(--line);
}
.credential-health-group:last-child {
  border-bottom: 0;
}
.credential-health-group h3 {
  min-width: 0;
  overflow: hidden;
  margin: 6px 0 0;
  font-size: 13px;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-health-items {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.credential-health-item {
  min-width: 0;
  width: fit-content;
  max-width: min(180px, 100%);
}
.credential-health-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  width: fit-content;
  max-width: 100%;
  min-height: 32px;
  padding: 3px 5px 3px 10px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--color-canvas);
}
.credential-health-name {
  min-width: 0;
  overflow: hidden;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-health-marker {
  position: relative;
  display: inline-flex;
  flex: none;
}
.credential-health-dot-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 21px;
  height: 21px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  cursor: pointer;
}
.credential-health-dot-button:disabled {
  cursor: wait;
  opacity: 0.6;
}
.credential-health-dot-button:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}
.credential-health-tooltip {
  position: absolute;
  z-index: 5;
  top: calc(100% + 5px);
  right: 0;
  display: flex;
  width: 240px;
  max-width: calc(100vw - 48px);
  flex-direction: column;
  gap: 5px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
  color: var(--color-text);
  font-size: 12px;
  line-height: 1.4;
  opacity: 0;
  pointer-events: none;
  visibility: hidden;
}
.credential-health-marker:hover .credential-health-tooltip,
.credential-health-marker:focus-within .credential-health-tooltip {
  opacity: 1;
  visibility: visible;
}
.credential-health-tooltip span,
.credential-health-tooltip code {
  overflow-wrap: anywhere;
}
.credential-health-tooltip small {
  color: var(--muted);
}
.credential-health-tooltip .credential-check-error {
  color: #c43b3b;
}
.credential-health-message {
  margin: 16px 0 0;
}
@media (max-width: 700px) {
  .credential-health-panel {
    padding: 18px;
  }
  .credential-health-head {
    flex-wrap: wrap;
  }
  .credential-health-group {
    grid-template-columns: 80px minmax(0, 1fr);
    gap: 8px;
  }
  .credential-health-items {
    gap: 7px;
  }
  .credential-health-tooltip {
    width: 190px;
  }
}
</style>
