<script setup lang="ts">
import { computed, ref } from 'vue'
import { t } from '../i18n'
import {
  preferredTestMappingID,
  preferredTestProtocol,
  testModelOptionsFor,
} from '../providerConnectionSelection'
import type { Provider, ProviderDetail, ProviderProtocol, Resource } from '../types'
import Modal from './Modal.vue'
import Status from './Status.vue'
import TechnicalValue from './TechnicalValue.vue'

const props = defineProps<{
  provider: Provider
  detail: ProviderDetail
  resources: Resource[]
  preferredResourceId?: string
  mode: 'TEST' | 'ACTIVATE'
  busy: boolean
}>()
const emit = defineEmits<{
  close: []
  submit: [
    selection: {
      resource: Resource
      protocol?: ProviderProtocol
      providerModelMappingID?: string
    },
  ]
}>()

const selectedResourceID = ref(props.preferredResourceId ?? props.resources[0]?.id ?? '')
const selectedProtocol = ref<ProviderProtocol | ''>(preferredTestProtocol(props.provider) ?? '')
const selectedMappingID = ref(preferredTestMappingID(props.detail))
const selectedResource = computed(() =>
  props.resources.find((resource) => resource.id === selectedResourceID.value),
)
const protocolOptions = computed(() =>
  [...props.provider.endpoints].sort((left, right) => {
    if (left.protocolType === right.protocolType) return 0
    return left.protocolType === 'ANTHROPIC' ? -1 : 1
  }),
)
const modelOptions = computed(() => testModelOptionsFor(props.detail))
const showCredentialSelection = computed(
  () => props.mode === 'ACTIVATE' || props.resources.length > 1,
)
const showProtocolSelection = computed(
  () =>
    selectedResource.value?.authType === 'API_KEY' &&
    (props.mode === 'ACTIVATE' || protocolOptions.value.length > 1),
)
const showModelSelection = computed(
  () =>
    selectedResource.value?.authType === 'API_KEY' &&
    (props.mode === 'ACTIVATE' || modelOptions.value.length > 1),
)
const ready = computed(
  () =>
    !!selectedResource.value &&
    (!showProtocolSelection.value || !!selectedProtocol.value) &&
    (!showModelSelection.value || !!selectedMappingID.value),
)
const title = computed(() => {
  if (props.mode === 'ACTIVATE')
    return t('providers.selectActivationTitle', { name: props.provider.name })
  const key = showCredentialSelection.value
    ? 'resources.selectTestCredentialTitle'
    : showProtocolSelection.value
      ? 'resources.selectTestProtocolTitle'
      : 'resources.selectTestModelTitle'
  return t(key, { name: props.provider.name })
})

function submit() {
  const resource = selectedResource.value
  if (!ready.value || !resource) return
  emit('submit', {
    resource,
    protocol: resource.authType === 'API_KEY' ? selectedProtocol.value || undefined : undefined,
    providerModelMappingID:
      resource.authType === 'API_KEY' ? selectedMappingID.value || undefined : undefined,
  })
}
</script>

<template>
  <Modal :title="title" :busy="busy" medium @close="emit('close')">
    <section v-if="showCredentialSelection" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestCredentialHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestCredential')"
      >
        <label
          v-for="resource in resources"
          :key="resource.id"
          class="credential-test-option"
          :class="{ 'is-selected': selectedResourceID === resource.id }"
        >
          <input
            v-model="selectedResourceID"
            type="radio"
            name="provider-test-credential"
            :value="resource.id"
            :aria-label="t('resources.selectCredentialForTest', { name: resource.name })"
          />
          <span class="credential-test-option-main">
            <strong>{{ resource.name }}</strong>
            <small>{{ t(`resources.authTypes.${resource.authType || 'API_KEY'}`) }}</small>
          </span>
          <span class="credential-test-option-status">
            <Status :value="resource.runtimeStatus || 'HEALTHY'" />
            <Status
              v-if="resource.authType === 'SUBSCRIPTION'"
              :value="resource.quotaStatus || 'UNKNOWN'"
            />
          </span>
        </label>
      </div>
    </section>
    <section v-if="showProtocolSelection" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestProtocolHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestProtocol')"
      >
        <label
          v-for="endpoint in protocolOptions"
          :key="endpoint.protocolType"
          class="credential-test-option"
          :class="{ 'is-selected': selectedProtocol === endpoint.protocolType }"
        >
          <input
            v-model="selectedProtocol"
            type="radio"
            name="provider-test-protocol"
            :value="endpoint.protocolType"
            :aria-label="
              t('resources.selectProtocolForTest', {
                protocol: endpoint.protocolType === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI',
              })
            "
          />
          <span class="credential-test-option-main">
            <strong>{{ endpoint.protocolType === 'ANTHROPIC' ? 'Anthropic' : 'OpenAI' }}</strong>
            <TechnicalValue :value="endpoint.baseUrl" :copyable="false" muted />
          </span>
        </label>
      </div>
    </section>
    <section v-if="showModelSelection" class="credential-test-section">
      <p class="muted credential-test-selection-hint">
        {{ t('resources.selectTestModelHint') }}
      </p>
      <div
        class="credential-test-options"
        role="radiogroup"
        :aria-label="t('resources.selectTestModel')"
      >
        <label
          v-for="option in modelOptions"
          :key="option.mapping.id"
          class="credential-test-option"
          :class="{ 'is-selected': selectedMappingID === option.mapping.id }"
        >
          <input
            v-model="selectedMappingID"
            type="radio"
            name="provider-test-model"
            :value="option.mapping.id"
            :aria-label="
              t('resources.selectModelForTest', {
                name:
                  option.model?.name ||
                  option.mapping.upstreamModelCode ||
                  option.model?.code ||
                  t('common.none'),
              })
            "
          />
          <span class="credential-test-option-main">
            <strong>{{ option.model?.name || option.mapping.upstreamModelCode || '-' }}</strong>
            <TechnicalValue
              :value="option.mapping.upstreamModelCode || option.model?.code || '-'"
              :copyable="false"
              muted
            />
          </span>
        </label>
      </div>
    </section>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="button" class="button primary" :disabled="busy || !ready" @click="submit">
        {{ t(mode === 'ACTIVATE' ? 'providers.testAndEnable' : 'resources.startTest') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.credential-test-selection-hint {
  margin-bottom: 14px;
}
.credential-test-section + .credential-test-section {
  margin-top: 18px;
}
.credential-test-options {
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
}
.credential-test-option {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  margin: 0;
  padding: 13px 14px;
  cursor: pointer;
  background: #fff;
}
.credential-test-option + .credential-test-option {
  border-top: 1px solid #e5ebf1;
}
.credential-test-option:hover {
  background: #f8fafc;
}
.credential-test-option.is-selected {
  background: var(--color-primary-soft);
}
.credential-test-option input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--blue);
}
.credential-test-option-main {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.credential-test-option-main strong,
.credential-test-option-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.credential-test-option-main small {
  color: var(--muted);
  font-size: 11px;
}
.credential-test-option-status {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
}
@media (max-width: 760px) {
  .credential-test-option {
    grid-template-columns: 20px minmax(0, 1fr);
  }
  .credential-test-option-status {
    grid-column: 2;
    justify-content: flex-start;
  }
}
</style>
