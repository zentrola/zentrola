<script setup lang="ts">
import { ref } from 'vue'
import { t } from '../../i18n'
import type { CredentialInput } from '../../providerCredentials'
import type { CredentialQuotas } from '../../providerQuotas'
import type { Provider, Resource } from '../../types'
import Icon from '../Icon.vue'
import Modal from '../Modal.vue'
import Status from '../Status.vue'
import TechnicalValue from '../TechnicalValue.vue'
import CredentialCreateForm from './CredentialCreateForm.vue'
import CredentialTable from './CredentialTable.vue'

export type { CredentialInput } from '../../providerCredentials'

defineProps<{
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
  status: [resource: Resource]
  delete: [resource: Resource]
}>()

const creating = ref(false)
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
        <button type="button" class="button primary" :disabled="busy" @click="creating = true">
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
      <CredentialTable
        v-if="resources.length"
        :provider="provider"
        :resources="resources"
        :busy="busy"
        :quotas="quotas"
        :verified-at="verifiedAt"
        @test="emit('test', $event)"
        @price="emit('price', $event)"
        @export="emit('export', $event)"
        @status="emit('status', $event)"
        @delete="emit('delete', $event)"
      />
      <div v-else class="credential-empty">
        <Icon name="lock-open" :size="28" />
        <strong>{{ t('resources.emptyCredentials') }}</strong>
        <p>{{ t('resources.emptyCredentialsHint') }}</p>
      </div>
    </template>
    <CredentialCreateForm
      v-else
      :provider="provider"
      :busy="busy"
      :create-credential="createCredential"
      @saved="creating = false"
    />
    <template v-if="creating" #footer>
      <button type="button" class="button" :disabled="busy" @click="creating = false">
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
  .credential-list-head {
    align-items: stretch;
    flex-direction: column;
  }
  .credential-list-head .button {
    align-self: flex-start;
  }
  .provider-toolbar-actions .button {
    flex: 1;
  }
}
</style>
