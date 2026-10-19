<script setup lang="ts">
import { t } from '../../i18n'
import { connectionResultMessage } from '../../providerQuotas'
import type { ConnectionTestTarget } from '../../providerConnectionTest'
import type { ConnectionResult } from '../../types'
import Modal from '../Modal.vue'
import TechnicalValue from '../TechnicalValue.vue'

defineProps<{
  target: ConnectionTestTarget
  result: ConnectionResult | null
  busy: boolean
}>()
defineEmits<{ close: [] }>()
</script>

<template>
  <Modal
    :title="`${target.provider.name} / ${t(target.activation ? 'providers.activationResult' : 'resources.result')}`"
    :busy="busy"
    @close="$emit('close')"
  >
    <p v-if="busy" role="status">
      {{
        t(
          target.activation
            ? 'providers.testingAndEnabling'
            : target.resource.authType === 'SUBSCRIPTION' &&
                target.resource.authAdapter !== 'ANTHROPIC_CLAUDE_CODE'
              ? 'resources.subscriptionTesting'
              : 'resources.testing',
        )
      }}
    </p>
    <template v-if="result">
      <div class="alert" :class="result.ok ? 'success' : 'error'" role="status">
        <i18n-t
          v-if="result.code === 'CODEX_APP_SERVER_UNAVAILABLE'"
          keypath="errors.CODEX_APP_SERVER_UNAVAILABLE"
          tag="span"
        >
          <template #executable><code>CODEX_EXECUTABLE</code></template>
        </i18n-t>
        <span v-else>{{
          connectionResultMessage(result, target.resource, target.activation)
        }}</span>
      </div>
      <dl class="detail-grid">
        <template v-if="target.protocol">
          <dt>{{ t('resources.protocol') }}</dt>
          <dd>{{ target.protocol === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI' }}</dd>
        </template>
        <template v-if="result.testedModelCode">
          <dt>{{ t('resources.testedModel') }}</dt>
          <dd><TechnicalValue :value="result.testedModelCode" :copyable="false" /></dd>
        </template>
        <dt>{{ t('common.code') }}</dt>
        <dd>{{ result.code }}</dd>
        <dt>HTTP</dt>
        <dd>{{ result.httpStatus || t('common.none') }}</dd>
        <dt>{{ t('resources.latency') }}</dt>
        <dd>{{ result.latencyMs }} ms</dd>
      </dl>
    </template>
    <p class="muted">
      {{
        t(
          target.resource.authType === 'SUBSCRIPTION'
            ? target.resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE'
              ? 'resources.claudeSubscriptionTestHint'
              : 'resources.subscriptionTestHint'
            : 'resources.testHint',
        )
      }}
    </p>
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
