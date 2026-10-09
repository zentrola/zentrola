<script setup lang="ts">
import { computed } from 'vue'
import { activeLocale, t } from '../i18n'
import type { TokenQuotaStatus } from '../types'

const props = defineProps<{
  limit: string | null
  status?: TokenQuotaStatus
}>()

function tokens(value: string) {
  try {
    return new Intl.NumberFormat(activeLocale.value).format(BigInt(value))
  } catch {
    return value
  }
}

const progress = computed(() => Math.max(0, Math.min(100, props.status?.usedPercent ?? 0)))
</script>

<template>
  <span v-if="!limit" class="quota-empty">-</span>
  <div v-else class="quota-usage" :class="status?.level.toLowerCase()">
    <div class="quota-numbers">
      <strong>{{ status ? tokens(status.usedTokens) : '-' }}</strong>
      <span>/ {{ tokens(limit) }}</span>
    </div>
    <div class="quota-progress" aria-hidden="true">
      <span :style="{ width: `${progress}%` }"></span>
    </div>
    <small v-if="status">{{
      t('tokenQuota.remaining', { value: tokens(status.remainingTokens) })
    }}</small>
  </div>
</template>

<style scoped>
.quota-empty {
  color: var(--color-text-secondary);
}
.quota-usage {
  width: min(100%, 180px);
}
.quota-numbers {
  display: flex;
  align-items: baseline;
  gap: 4px;
  white-space: nowrap;
}
.quota-numbers strong {
  color: var(--color-text);
  font-size: 13px;
}
.quota-numbers span,
.quota-usage small {
  color: var(--color-text-secondary);
  font-size: 11px;
}
.quota-progress {
  height: 3px;
  margin: 5px 0 3px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--line);
}
.quota-progress span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--blue);
}
.quota-usage.notice .quota-progress span {
  background: var(--color-warning-text);
}
.quota-usage.warning .quota-progress span,
.quota-usage.exhausted .quota-progress span {
  background: var(--danger);
}
</style>
