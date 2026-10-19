<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import { validText } from '../../composables'
import { t } from '../../i18n'
import {
  isSocksProxyURL,
  normalizeURL,
  proxyCredentialSignature,
  proxySchemeFromURL,
  validProxyHeaders,
  validProxyURL,
  validURL,
} from '../../providerProxy'
import { showErrorToast } from '../../toast'
import type { Model, ProviderDetail, ProviderNetworkScope, ProviderProtocol } from '../../types'
import Modal from '../Modal.vue'
import ProviderMappingEditor, { type MappingDraft } from './ProviderMappingEditor.vue'
import ProviderProxyEditor, { type ProxyDraft } from './ProviderProxyEditor.vue'

export type ProviderInput = {
  name: string
  website: string
  endpoints: Array<{
    protocolType: ProviderProtocol
    baseUrl: string
    networkScope: ProviderNetworkScope
  }>
  proxyEnabled: boolean
  proxyUrl: string
  updateProxyCredentials: boolean
  proxyHeaders: Array<{ key: string; value: string }>
  mappings: MappingDraft[]
}

const props = defineProps<{
  // 为空表示新增第三方服务商。
  provider: ProviderDetail | null
  models: Model[]
  busy: boolean
}>()
const emit = defineEmits<{ close: []; submit: [input: ProviderInput] }>()

type ConfigTab = 'models' | 'proxy'
const activeConfigTab = ref<ConfigTab>('models')
const modelConfigTab = ref<HTMLButtonElement | null>(null)
const proxyConfigTab = ref<HTMLButtonElement | null>(null)
const availableModels = computed(() => props.models.filter((model) => model.status === 'ACTIVE'))

function endpoint(protocolType: ProviderProtocol) {
  return props.provider?.endpoints.find((item) => item.protocolType === protocolType)
}

// 只保留当前仍可选的启用模型映射，并按模型去重。
function initialMappings(): MappingDraft[] {
  const modelIDs = new Set(availableModels.value.map((model) => model.id))
  return (props.provider?.mappings ?? [])
    .map((mapping) => ({
      modelId: mapping.modelId,
      upstreamModelCode: mapping.upstreamModelCode,
    }))
    .filter((mapping) => modelIDs.has(mapping.modelId))
    .filter(
      (mapping, index, mappings) =>
        mappings.findIndex((candidate) => candidate.modelId === mapping.modelId) === index,
    )
}

const form = reactive({
  name: props.provider?.name ?? '',
  website: props.provider?.website ?? '',
  anthropicBaseUrl: endpoint('ANTHROPIC')?.baseUrl ?? '',
  anthropicNetworkScope: (endpoint('ANTHROPIC')?.networkScope ?? 'PUBLIC') as ProviderNetworkScope,
  openaiBaseUrl: endpoint('OPENAI')?.baseUrl ?? '',
  openaiNetworkScope: (endpoint('OPENAI')?.networkScope ?? 'PUBLIC') as ProviderNetworkScope,
  mappings: initialMappings(),
})
const proxy = reactive<ProxyDraft>({
  proxyEnabled: props.provider?.proxyEnabled ?? false,
  proxyScheme: proxySchemeFromURL(props.provider?.proxyUrl ?? ''),
  proxyUrl: props.provider?.proxyUrl ?? '',
  updateProxyCredentials: false,
  proxyHeaders: (props.provider?.proxyHeaders ?? []).map((header) => ({
    key: header.key,
    value: '',
    configured: header.configured,
    originalKey: header.key,
  })),
})

function activateConfigTab(tab: ConfigTab, focus = false) {
  activeConfigTab.value = tab
  if (!focus) return
  void nextTick(() => {
    const button = tab === 'models' ? modelConfigTab.value : proxyConfigTab.value
    button?.focus()
  })
}

function invalidMapping(mapping: MappingDraft) {
  return (
    !availableModels.value.some((model) => model.id === mapping.modelId) ||
    (mapping.upstreamModelCode.trim() !== '' && !validText(mapping.upstreamModelCode.trim(), 128))
  )
}

function save() {
  let validation = ''
  const endpointDrafts: ProviderInput['endpoints'] = [
    {
      protocolType: 'OPENAI',
      baseUrl: normalizeURL(form.openaiBaseUrl),
      networkScope: form.openaiNetworkScope,
    },
    {
      protocolType: 'ANTHROPIC',
      baseUrl: normalizeURL(form.anthropicBaseUrl),
      networkScope: form.anthropicNetworkScope,
    },
  ]
  const socksProxy = proxy.proxyEnabled && isSocksProxyURL(proxy.proxyUrl)
  const original = props.provider
  const input: ProviderInput = {
    name: form.name.trim(),
    website: normalizeURL(form.website),
    endpoints: endpointDrafts.filter((item) => item.baseUrl),
    proxyEnabled: proxy.proxyEnabled,
    proxyUrl: proxy.proxyEnabled ? proxy.proxyUrl.trim() : '',
    updateProxyCredentials: proxy.proxyEnabled && (!original || proxy.updateProxyCredentials),
    proxyHeaders:
      proxy.proxyEnabled && !socksProxy
        ? proxy.proxyHeaders.map((header) => ({ key: header.key.trim(), value: header.value }))
        : [],
    mappings: form.mappings.map((mapping) => ({
      modelId: mapping.modelId,
      upstreamModelCode: mapping.upstreamModelCode.trim(),
    })),
  }
  if (!validText(input.name, 128)) validation = t('common.byteLimit')
  else if (
    !validURL(input.website) ||
    input.endpoints.some((item) => !validURL(item.baseUrl, true, item.networkScope))
  )
    validation = t('providers.urlInvalid')
  else if (!input.endpoints.length) validation = t('providers.endpointRequired')
  else if (input.proxyEnabled && !validProxyURL(input.proxyUrl))
    validation = t('providers.proxyUrlInvalid')
  else if (
    input.proxyEnabled &&
    original?.proxyEnabled &&
    !proxy.updateProxyCredentials &&
    proxyCredentialSignature(input.proxyUrl) !== proxyCredentialSignature(original.proxyUrl)
  )
    validation = t('providers.proxyCredentialsUpdateRequired')
  else if (
    input.proxyEnabled &&
    proxy.updateProxyCredentials &&
    !proxyCredentialSignature(input.proxyUrl)
  )
    validation = t('providers.proxyCredentialsRequired')
  else if (input.proxyEnabled && !socksProxy && !validProxyHeaders(proxy.proxyHeaders))
    validation = t('providers.proxyHeadersInvalid')
  else if (form.mappings.some(invalidMapping)) validation = t('providers.mappingInvalid')
  else if (new Set(form.mappings.map((mapping) => mapping.modelId)).size !== form.mappings.length)
    validation = t('providers.mappingDuplicate')
  if (validation) {
    if (
      input.proxyEnabled &&
      (!validProxyURL(input.proxyUrl) || (!socksProxy && !validProxyHeaders(proxy.proxyHeaders)))
    )
      activeConfigTab.value = 'proxy'
    else if (form.mappings.some(invalidMapping)) activeConfigTab.value = 'models'
    showErrorToast(validation)
    return
  }
  emit('submit', input)
}
</script>

<template>
  <Modal
    :title="t(provider ? 'providers.editTitle' : 'providers.create')"
    :busy="busy"
    :body-class="activeConfigTab === 'models' ? 'provider-model-modal-body' : undefined"
    wide
    @close="emit('close')"
  >
    <form id="provider-form" class="provider-form" @submit.prevent="save">
      <section class="connection-editor" :aria-label="t('providers.connectionTitle')">
        <p class="provider-connection-hint">{{ t('providers.endpointHint') }}</p>
        <div class="connection-fields">
          <div class="provider-field-row">
            <label class="provider-field-label required-label" for="provider-name-input">{{
              t('providers.name')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-name-input"
                v-model="form.name"
                required
                autofocus
                :disabled="busy"
              />
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-website-input">{{
              t('providers.website')
            }}</label>
            <div class="provider-field-control">
              <input
                id="provider-website-input"
                v-model="form.website"
                type="url"
                placeholder="https://example.com"
                :disabled="busy"
              />
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-openai-endpoint-input">{{
              t('providers.openaiEndpoint')
            }}</label>
            <div class="provider-field-control">
              <div class="endpoint-editor">
                <input
                  id="provider-openai-endpoint-input"
                  v-model="form.openaiBaseUrl"
                  type="url"
                  :placeholder="
                    form.openaiNetworkScope === 'PRIVATE'
                      ? 'http://192.168.1.20:8080/v1'
                      : 'https://api.example.com/v1'
                  "
                  spellcheck="false"
                  :disabled="busy"
                />
                <select
                  v-model="form.openaiNetworkScope"
                  :aria-label="t('providers.networkScopeFor', { protocol: 'OpenAI' })"
                  :disabled="busy"
                >
                  <option value="PUBLIC">{{ t('providers.networkPublic') }}</option>
                  <option value="PRIVATE">{{ t('providers.networkPrivate') }}</option>
                </select>
              </div>
            </div>
          </div>
          <div class="provider-field-row">
            <label class="provider-field-label" for="provider-anthropic-endpoint-input">{{
              t('providers.anthropicEndpoint')
            }}</label>
            <div class="provider-field-control">
              <div class="endpoint-editor">
                <input
                  id="provider-anthropic-endpoint-input"
                  v-model="form.anthropicBaseUrl"
                  type="url"
                  :placeholder="
                    form.anthropicNetworkScope === 'PRIVATE'
                      ? 'http://192.168.1.20:8080/anthropic'
                      : 'https://api.example.com/anthropic'
                  "
                  spellcheck="false"
                  :disabled="busy"
                />
                <select
                  v-model="form.anthropicNetworkScope"
                  :aria-label="t('providers.networkScopeFor', { protocol: 'Anthropic' })"
                  :disabled="busy"
                >
                  <option value="PUBLIC">{{ t('providers.networkPublic') }}</option>
                  <option value="PRIVATE">{{ t('providers.networkPrivate') }}</option>
                </select>
              </div>
            </div>
          </div>
          <p
            v-if="form.openaiNetworkScope === 'PRIVATE' || form.anthropicNetworkScope === 'PRIVATE'"
            class="private-network-warning"
            role="note"
          >
            {{ t('providers.privateNetworkWarning') }}
          </p>
        </div>
      </section>
      <section class="config-editor">
        <div class="config-tabs" role="tablist" :aria-label="t('providers.configTabs')">
          <button
            id="provider-models-tab"
            ref="modelConfigTab"
            type="button"
            role="tab"
            class="config-tab"
            :class="{ 'is-active': activeConfigTab === 'models' }"
            :aria-selected="activeConfigTab === 'models'"
            aria-controls="provider-models-panel"
            :tabindex="activeConfigTab === 'models' ? 0 : -1"
            @click="activateConfigTab('models')"
            @keydown.left.prevent="activateConfigTab('proxy', true)"
            @keydown.right.prevent="activateConfigTab('proxy', true)"
            @keydown.home.prevent="activateConfigTab('models', true)"
            @keydown.end.prevent="activateConfigTab('proxy', true)"
          >
            {{ t('providers.modelConfig') }}
          </button>
          <button
            id="provider-proxy-tab"
            ref="proxyConfigTab"
            type="button"
            role="tab"
            class="config-tab"
            :class="{ 'is-active': activeConfigTab === 'proxy' }"
            :aria-selected="activeConfigTab === 'proxy'"
            aria-controls="provider-proxy-panel"
            :tabindex="activeConfigTab === 'proxy' ? 0 : -1"
            @click="activateConfigTab('proxy')"
            @keydown.left.prevent="activateConfigTab('models', true)"
            @keydown.right.prevent="activateConfigTab('models', true)"
            @keydown.home.prevent="activateConfigTab('models', true)"
            @keydown.end.prevent="activateConfigTab('proxy', true)"
          >
            {{ t('providers.proxyConfig') }}
          </button>
        </div>
        <section
          v-show="activeConfigTab === 'models'"
          id="provider-models-panel"
          class="config-panel mapping-editor"
          role="tabpanel"
          aria-labelledby="provider-models-tab"
          tabindex="0"
        >
          <ProviderMappingEditor v-model="form.mappings" :models="availableModels" :busy="busy" />
        </section>
        <section
          v-show="activeConfigTab === 'proxy'"
          id="provider-proxy-panel"
          class="config-panel proxy-editor"
          role="tabpanel"
          aria-labelledby="provider-proxy-tab"
          tabindex="0"
        >
          <ProviderProxyEditor
            :draft="proxy"
            :busy="busy"
            :can-update-credentials="Boolean(provider?.proxyEnabled)"
            :original-proxy-url="provider?.proxyUrl ?? null"
          />
        </section>
      </section>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="emit('close')">
        {{ t('common.cancel') }}</button
      ><button type="submit" form="provider-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.provider-form {
  --provider-field-label-width: 160px;

  display: grid;
  gap: 14px;
}
:global(.modal-body.provider-model-modal-body) {
  overflow-y: hidden;
  scrollbar-gutter: auto;
}
:global(.modal-body.provider-model-modal-body .provider-form) {
  height: min(640px, calc(100dvh - 200px));
  min-height: 0;
  grid-template-rows: auto minmax(0, 1fr);
}
.provider-form label {
  margin: 0;
}
.connection-editor,
.config-editor,
.config-panel {
  min-width: 0;
}
.provider-connection-hint {
  margin: 0 0 10px;
  padding: 7px 10px;
  border-left: 3px solid var(--color-primary);
  border-radius: 6px;
  color: var(--color-text-secondary);
  background: var(--color-primary-soft);
  font-size: 12px;
  line-height: 1.55;
}
.connection-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
}
.provider-field-row {
  display: grid;
  grid-template-columns: var(--provider-field-label-width) minmax(0, 1fr);
  align-items: start;
  gap: 12px;
}
.provider-field-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.provider-field-control {
  min-width: 0;
}
.endpoint-editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 132px;
  gap: 8px;
}
.private-network-warning {
  margin: 0 0 0 calc(var(--provider-field-label-width) + 12px);
  padding: 8px 10px;
  border: 1px solid var(--color-warning-border);
  border-radius: 7px;
  color: var(--color-text-secondary);
  background: var(--color-warning-bg);
  font-size: 12px;
  line-height: 1.55;
}
.required-label::after {
  margin-left: 4px;
  color: var(--danger);
  content: '*';
}
.config-editor {
  display: grid;
  min-height: 0;
  margin-top: 0;
  grid-template-rows: auto minmax(0, 1fr);
}
.config-tabs {
  display: flex;
  gap: 4px;
  overflow-x: auto;
  overflow-y: hidden;
  border-bottom: 1px solid var(--color-border);
}
.config-tab {
  position: relative;
  flex: none;
  min-width: 112px;
  padding: 9px 15px 10px;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
}
.config-tab::after {
  position: absolute;
  right: 12px;
  bottom: -1px;
  left: 12px;
  height: 2px;
  background: transparent;
  content: '';
}
.config-tab:hover {
  color: var(--color-primary);
}
.config-tab.is-active {
  color: var(--color-primary-hover);
}
.config-tab.is-active::after {
  background: var(--blue);
}
.config-panel {
  min-height: 0;
  padding-top: 14px;
}
.config-panel:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 4px;
}
.proxy-editor {
  display: grid;
  grid-template-columns: var(--provider-field-label-width) minmax(0, 1fr);
  column-gap: 12px;
}
.mapping-editor {
  display: grid;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
}
@media (max-width: 760px) {
  .provider-toolbar-actions .button {
    flex: 1;
  }
  .connection-fields {
    grid-template-columns: 1fr;
  }
  .provider-field-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }
  .provider-field-label {
    padding-top: 0;
    text-align: left;
  }
  .endpoint-editor {
    grid-template-columns: minmax(0, 1fr);
  }
  .private-network-warning {
    margin-left: 0;
  }
  .proxy-editor,
  .proxy-fields,
  .proxy-header-list {
    grid-template-columns: minmax(0, 1fr);
  }
  .mapping-editor {
    overflow-y: auto;
    padding-right: 4px;
    grid-template-rows: auto auto;
  }
}
</style>
