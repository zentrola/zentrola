<script setup lang="ts">
import { computed } from 'vue'
import { t } from '../../i18n'
import {
  isSocksProxyURL,
  maxProxyHeaders,
  proxyURLWithOriginalCredentials,
  proxyURLWithoutCredentials,
  splitProxyAddress,
  type ProxyHeaderDraft,
  type ProxyScheme,
} from '../../providerProxy'
import Icon from '../Icon.vue'
import FieldHelp from '../FieldHelp.vue'
import ToggleSwitch from '../ToggleSwitch.vue'

export type ProxyDraft = {
  proxyEnabled: boolean
  proxyScheme: ProxyScheme
  proxyUrl: string
  updateProxyCredentials: boolean
  proxyHeaders: ProxyHeaderDraft[]
}

// draft 由父组件持有的响应式对象，编辑器直接修改其字段。
const props = defineProps<{
  draft: ProxyDraft
  busy: boolean
  // 编辑已启用代理的服务商时，可显式选择是否更新代理认证信息。
  canUpdateCredentials: boolean
  originalProxyUrl: string | null
}>()
const draft = computed(() => props.draft)

const proxyAddress = computed({
  get: () => splitProxyAddress(draft.value.proxyUrl, draft.value.proxyScheme).address,
  set: (value: string) => {
    const parsed = splitProxyAddress(value, draft.value.proxyScheme)
    draft.value.proxyScheme = parsed.scheme
    draft.value.proxyUrl = `${parsed.scheme}://${parsed.address}`
  },
})

function changeScheme(event: Event) {
  const scheme = (event.target as HTMLSelectElement).value as ProxyScheme
  const address = proxyAddress.value
  draft.value.proxyScheme = scheme
  draft.value.proxyUrl = `${scheme}://${address}`
}

function toggleCredentialUpdate(checked: boolean) {
  draft.value.updateProxyCredentials = checked
  draft.value.proxyUrl = checked
    ? proxyURLWithoutCredentials(draft.value.proxyUrl)
    : proxyURLWithOriginalCredentials(draft.value.proxyUrl, props.originalProxyUrl)
}

function addHeader() {
  if (draft.value.proxyHeaders.length >= maxProxyHeaders) return
  draft.value.proxyHeaders.push({ key: '', value: '', configured: false, originalKey: '' })
}

function removeHeader(index: number) {
  draft.value.proxyHeaders.splice(index, 1)
}
</script>

<template>
  <div class="provider-field-row proxy-switch-row">
    <label class="provider-field-label" for="provider-proxy-enabled">{{
      t('providers.proxyEnabled')
    }}</label>
    <div class="provider-field-control proxy-switch-field">
      <ToggleSwitch
        id="provider-proxy-enabled"
        v-model="draft.proxyEnabled"
        :aria-label="t('providers.proxyEnabled')"
        :title="t('providers.proxyHint')"
        :disabled="busy"
      />
    </div>
  </div>
  <div v-if="draft.proxyEnabled" class="proxy-fields">
    <div class="provider-field-row proxy-url-row">
      <div class="provider-field-label proxy-url-label">
        <label for="provider-proxy-url">{{ t('providers.proxyUrl') }}</label>
        <FieldHelp
          :text="
            isSocksProxyURL(draft.proxyUrl)
              ? `${t('providers.proxyUrlCredentialHint')} · ${t('providers.socksProxyHint')}`
              : t('providers.proxyUrlCredentialHint')
          "
        />
      </div>
      <div class="provider-field-control proxy-url-control">
        <select
          id="provider-proxy-scheme"
          :value="draft.proxyScheme"
          :aria-label="t('providers.proxyProtocol')"
          :disabled="busy"
          @change="changeScheme"
        >
          <option value="http">HTTP</option>
          <option value="https">HTTPS</option>
          <option value="socks5">SOCKS5</option>
          <option value="socks5h">SOCKS5H</option>
        </select>
        <input
          id="provider-proxy-url"
          v-model="proxyAddress"
          type="text"
          placeholder="username:password@proxy.example.com:1080"
          autocomplete="off"
          spellcheck="false"
          :disabled="busy"
        />
      </div>
    </div>
    <div
      v-if="canUpdateCredentials"
      class="provider-field-row proxy-switch-row proxy-credential-update-row"
    >
      <label class="provider-field-label" for="provider-proxy-credentials-update">{{
        t('providers.proxyCredentialsUpdate')
      }}</label>
      <div class="provider-field-control proxy-switch-field">
        <ToggleSwitch
          id="provider-proxy-credentials-update"
          :model-value="draft.updateProxyCredentials"
          :aria-label="t('providers.proxyCredentialsUpdate')"
          :title="t('providers.proxyCredentialsUpdateHint')"
          :disabled="busy"
          @update:model-value="toggleCredentialUpdate"
        />
      </div>
    </div>
    <template v-if="!isSocksProxyURL(draft.proxyUrl)">
      <div class="proxy-headers-head">
        <div>
          <strong>{{ t('providers.proxyHeaders') }}</strong>
          <span>{{ t('providers.proxyHeadersHint') }}</span>
        </div>
        <button
          type="button"
          class="button compact"
          :disabled="busy || draft.proxyHeaders.length >= maxProxyHeaders"
          @click="addHeader"
        >
          <Icon name="plus" :size="15" />{{ t('providers.addProxyHeader') }}
        </button>
      </div>
      <div v-if="draft.proxyHeaders.length" class="proxy-header-list">
        <div v-for="(header, index) in draft.proxyHeaders" :key="index" class="proxy-header-row">
          <label class="proxy-header-field">
            <span>{{ t('providers.proxyHeaderKey') }}</span>
            <input
              v-model="header.key"
              spellcheck="false"
              maxlength="128"
              :disabled="busy"
              placeholder="Proxy-Authorization"
            />
          </label>
          <label class="proxy-header-field">
            <span>{{ t('providers.proxyHeaderValue') }}</span>
            <input
              v-model="header.value"
              type="text"
              autocomplete="off"
              spellcheck="false"
              :disabled="busy"
              :placeholder="
                header.configured
                  ? t('providers.proxyHeaderValueConfigured')
                  : t('providers.proxyHeaderValuePlaceholder')
              "
            />
          </label>
          <button
            type="button"
            class="icon-button proxy-header-remove"
            :disabled="busy"
            :aria-label="t('providers.removeProxyHeader', { index: index + 1 })"
            :title="t('common.remove')"
            @click="removeHeader(index)"
          >
            <Icon name="trash" :size="17" />
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
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
.proxy-switch-row {
  align-items: center;
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
}
.proxy-switch-row .provider-field-label {
  padding-top: 0;
  text-align: left;
}
.proxy-switch-field {
  display: flex;
  align-items: center;
  min-height: 40px;
}
.proxy-fields {
  display: grid;
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
  gap: 12px 0;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--color-border);
}
.proxy-fields > label {
  margin: 0;
}
.proxy-url-row {
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
}
.proxy-url-row .provider-field-label {
  text-align: left;
}
.proxy-url-label {
  display: flex;
  align-items: center;
  gap: 5px;
}
.proxy-url-label > label {
  margin: 0;
  font: inherit;
}
.proxy-url-control {
  display: grid;
  grid-template-columns: 124px minmax(0, 1fr);
  gap: 8px;
}
.proxy-headers-head {
  display: flex;
  grid-column: 1 / -1;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.proxy-headers-head > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}
.proxy-headers-head strong {
  color: var(--color-text);
  font-size: 13px;
}
.proxy-headers-head span {
  color: var(--muted);
  font-size: 11px;
}
.button.compact {
  min-height: 34px;
  padding: 6px 11px;
  font-size: 12px;
}
.proxy-header-list {
  display: grid;
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
  column-gap: 12px;
  row-gap: 9px;
}
.proxy-header-row {
  display: grid;
  grid-column: 2;
  grid-template-columns: minmax(100px, 0.7fr) minmax(160px, 1.3fr) 36px;
  align-items: end;
  gap: 10px;
}
.proxy-header-row label:nth-child(2) {
  grid-column: 2;
}
.proxy-header-remove {
  grid-column: 3;
}
.proxy-header-row label {
  min-width: 0;
  margin: 0;
}
.proxy-header-field {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
}
.proxy-header-field > span {
  display: block;
  margin: 0;
  color: #60788d;
  font-size: 11px;
  font-weight: 600;
}
.proxy-header-remove {
  width: 36px;
  height: 40px;
  color: var(--color-danger);
}
.proxy-header-remove:hover:not(:disabled) {
  color: #8f2f35;
}
@media (max-width: 760px) {
  .provider-toolbar-actions .button {
    flex: 1;
  }
  .provider-field-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }
  .provider-field-label {
    padding-top: 0;
    text-align: left;
  }
  .proxy-editor,
  .proxy-fields,
  .proxy-header-list {
    grid-template-columns: minmax(0, 1fr);
  }
  .proxy-switch-row {
    grid-column: 1;
    grid-template-columns: auto 40px;
    gap: 12px;
  }
  .proxy-url-row {
    grid-column: 1;
    grid-template-columns: minmax(0, 1fr);
  }
  .proxy-headers-head {
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }
  .proxy-headers-head > div {
    flex-wrap: wrap;
  }
  .proxy-headers-head .button {
    align-self: flex-start;
  }
  .proxy-header-row {
    grid-column: 1;
    grid-template-columns: minmax(0, 1fr) 36px;
  }
  .proxy-header-row label:nth-child(2) {
    grid-column: 1;
  }
  .proxy-header-remove {
    grid-column: 2;
    grid-row: 1 / span 2;
    align-self: center;
  }
}
</style>
