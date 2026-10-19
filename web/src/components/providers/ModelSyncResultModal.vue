<script setup lang="ts">
import { t } from '../../i18n'
import { errorCodeText } from '../../providerQuotas'
import type { ModelSyncResult, Provider } from '../../types'
import Modal from '../Modal.vue'

defineProps<{ provider: Provider; result: ModelSyncResult | null; busy: boolean }>()
defineEmits<{ close: [] }>()
</script>

<template>
  <Modal
    :title="`${provider.name} / ${t('resources.syncResult')}`"
    :busy="busy"
    @close="$emit('close')"
  >
    <p v-if="busy && !result" role="status">{{ t('resources.syncingModels') }}</p>
    <template v-if="result">
      <div class="alert" :class="result.ok ? 'success' : 'error'" role="status">
        {{ result.ok ? t('resources.syncPassed') : errorCodeText(result.code) }}
      </div>
      <dl class="detail-grid">
        <dt>{{ t('resources.modelSyncSource') }}</dt>
        <dd>{{ t(`resources.modelSyncSources.${result.source}`) }}</dd>
        <dt>{{ t('resources.discoveredModels') }}</dt>
        <dd>{{ result.discovered }}</dd>
        <dt>{{ t('resources.createdModels') }}</dt>
        <dd>{{ result.created }}</dd>
        <dt>{{ t('resources.updatedModels') }}</dt>
        <dd>{{ result.updated }}</dd>
        <dt>{{ t('resources.createdMappings') }}</dt>
        <dd>{{ result.mapped }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ result.latencyMs }} ms</dd>
      </dl>
    </template>
    <p class="muted">{{ t('resources.syncHint') }}</p>
    <template #footer>
      <button class="button" :disabled="busy" @click="$emit('close')">{{ t('close') }}</button>
    </template>
  </Modal>
</template>

<style scoped>
@media (max-width: 760px) {
  .provider-toolbar-actions .button {
    flex: 1;
  }
}
</style>
