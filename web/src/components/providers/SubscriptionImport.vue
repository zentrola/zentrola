<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { t } from '../../i18n'
import { showErrorToast, showSuccessToast } from '../../toast'
import Icon from '../Icon.vue'

export type SubscriptionInputMode = 'UPLOAD' | 'PASTE'

// 订阅凭据文件上限，与后端校验保持一致。
const maxSubscriptionBytes = 65536
const codexUploadCommands = [
  { platform: 'macOS', command: 'open ~/.codex' },
  {
    platform: 'Windows PowerShell',
    command: 'explorer.exe "$env:USERPROFILE\\.codex"',
  },
  { platform: 'Linux', command: 'xdg-open ~/.codex' },
] as const
const codexPasteCommands = [
  { platform: 'macOS', command: 'pbcopy < ~/.codex/auth.json' },
  {
    platform: 'Windows PowerShell',
    command: 'Get-Content -Raw "$env:USERPROFILE\\.codex\\auth.json" | Set-Clipboard',
  },
  { platform: 'Linux', command: 'cat ~/.codex/auth.json' },
] as const
const claudeCommands = [{ platform: '', command: 'claude setup-token' }] as const

const props = defineProps<{ claudeSubscription: boolean; busy: boolean; invalidText: string }>()
const mode = defineModel<SubscriptionInputMode>('mode', { required: true })
const credential = defineModel<string>('credential', { required: true })
const fileName = defineModel<string>('fileName', { required: true })
const uploadTab = ref<HTMLButtonElement | null>(null)
const pasteTab = ref<HTMLButtonElement | null>(null)

const commands = computed(() => {
  if (props.claudeSubscription) return claudeCommands
  return mode.value === 'UPLOAD' ? codexUploadCommands : codexPasteCommands
})

function selectMode(value: SubscriptionInputMode) {
  if (mode.value === value) return
  mode.value = value
  fileName.value = ''
  credential.value = ''
}

async function focusMode(value: SubscriptionInputMode) {
  selectMode(value)
  await nextTick()
  const tab = value === 'UPLOAD' ? uploadTab.value : pasteTab.value
  tab?.focus()
}

async function copyCommand(command: string) {
  try {
    await navigator.clipboard.writeText(command)
    showSuccessToast(t('common.copied'))
  } catch {
    showErrorToast(t('common.copyFailed'))
  }
}

async function importFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (file.size > maxSubscriptionBytes) {
    showErrorToast(props.invalidText)
    input.value = ''
    fileName.value = ''
    credential.value = ''
    return
  }
  credential.value = await file.text()
  fileName.value = file.name
}
</script>

<template>
  <section class="subscription-import">
    <div
      v-if="!claudeSubscription"
      class="subscription-input-tabs"
      role="tablist"
      :aria-label="t('resources.subscriptionMethodLabel')"
    >
      <button
        id="subscription-upload-tab"
        ref="uploadTab"
        type="button"
        role="tab"
        :aria-selected="mode === 'UPLOAD'"
        aria-controls="subscription-upload-panel"
        :tabindex="mode === 'UPLOAD' ? 0 : -1"
        :disabled="busy"
        @click="selectMode('UPLOAD')"
        @keydown.right.prevent="focusMode('PASTE')"
        @keydown.end.prevent="focusMode('PASTE')"
      >
        {{ t('resources.subscriptionMethods.UPLOAD') }}
      </button>
      <button
        id="subscription-paste-tab"
        ref="pasteTab"
        type="button"
        role="tab"
        :aria-selected="mode === 'PASTE'"
        aria-controls="subscription-paste-panel"
        :tabindex="mode === 'PASTE' ? 0 : -1"
        :disabled="busy"
        @click="selectMode('PASTE')"
        @keydown.left.prevent="focusMode('UPLOAD')"
        @keydown.home.prevent="focusMode('UPLOAD')"
      >
        {{ t('resources.subscriptionMethods.PASTE') }}
      </button>
    </div>

    <component
      :is="claudeSubscription ? 'div' : 'details'"
      :key="mode"
      class="subscription-source-guide"
      :class="{ 'subscription-source-guide-static': claudeSubscription }"
    >
      <summary v-if="!claudeSubscription">
        <Icon name="arrow" :size="13" />
        <span>{{
          t(
            mode === 'UPLOAD'
              ? 'resources.subscriptionFolderCommandTitle'
              : 'resources.subscriptionCommandTitle',
          )
        }}</span>
      </summary>
      <p v-else class="subscription-command-title">
        {{ t('resources.claudeSubscriptionCommandTitle') }}
      </p>
      <div class="subscription-command-list">
        <div
          v-for="item in commands"
          :key="item.platform"
          :class="{ 'subscription-command-item-direct': !item.platform }"
        >
          <span v-if="item.platform">{{ item.platform }}</span>
          <code :title="item.command">{{ item.command }}</code>
          <button
            type="button"
            class="icon-button subscription-command-copy"
            :aria-label="
              t(
                item.platform
                  ? 'resources.copySubscriptionCommand'
                  : 'resources.copyClaudeSubscriptionCommand',
                { platform: item.platform },
              )
            "
            :title="
              t(
                item.platform
                  ? 'resources.copySubscriptionCommand'
                  : 'resources.copyClaudeSubscriptionCommand',
                { platform: item.platform },
              )
            "
            @click="copyCommand(item.command)"
          >
            <Icon name="copy" :size="14" />
          </button>
        </div>
      </div>
    </component>

    <div
      v-if="!claudeSubscription && mode === 'UPLOAD'"
      id="subscription-upload-panel"
      class="subscription-input-panel"
      role="tabpanel"
      aria-labelledby="subscription-upload-tab"
    >
      <input
        id="credential-auth-file"
        class="subscription-file-input"
        type="file"
        accept=".json,application/json"
        required
        :aria-label="t('resources.authFile')"
        :disabled="busy"
        @change="importFile"
      />
      <label class="subscription-file-picker" for="credential-auth-file">
        <Icon name="upload" :size="21" />
        <span>
          <strong>{{ fileName || t('resources.subscriptionChooseFile') }}</strong>
          <small>{{ t('resources.subscriptionFileLimit') }}</small>
        </span>
      </label>
      <p class="field-hint">
        {{ t('resources.subscriptionUploadHint') }}
      </p>
    </div>

    <div
      v-else
      id="subscription-paste-panel"
      class="subscription-input-panel"
      :role="claudeSubscription ? undefined : 'tabpanel'"
      :aria-labelledby="claudeSubscription ? undefined : 'subscription-paste-tab'"
    >
      <p class="subscription-paste-intro">
        {{
          t(
            claudeSubscription
              ? 'resources.claudeSubscriptionPasteHint'
              : 'resources.subscriptionPasteHint',
          )
        }}
      </p>
      <textarea
        id="credential-auth-content"
        v-model="credential"
        rows="8"
        autocomplete="off"
        required
        :aria-label="
          t(
            claudeSubscription
              ? 'resources.claudeSubscriptionPasteLabel'
              : 'resources.subscriptionPasteLabel',
          )
        "
        :disabled="busy"
        :placeholder="
          t(
            claudeSubscription
              ? 'resources.claudeSubscriptionPastePlaceholder'
              : 'resources.subscriptionPastePlaceholder',
          )
        "
        spellcheck="false"
      ></textarea>
    </div>
  </section>
</template>

<style scoped>
.subscription-import {
  display: grid;
  min-width: 0;
  gap: 12px;
}
.subscription-input-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid #dce5ee;
}
.subscription-input-tabs button {
  position: relative;
  padding: 9px 14px 10px;
  border: 0;
  color: var(--color-text-secondary);
  cursor: pointer;
  background: transparent;
  font: inherit;
  font-weight: 600;
}
.subscription-input-tabs button::after {
  position: absolute;
  right: 12px;
  bottom: -1px;
  left: 12px;
  height: 2px;
  content: '';
  background: transparent;
}
.subscription-input-tabs button[aria-selected='true'] {
  color: var(--blue);
}
.subscription-input-tabs button[aria-selected='true']::after {
  background: var(--blue);
}
.subscription-input-tabs button:focus-visible {
  border-radius: 4px;
  outline: 2px solid #bfdbfe;
  outline-offset: -2px;
}
.subscription-input-tabs button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.subscription-source-guide {
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 7px;
  color: var(--color-text-secondary);
  background: #f8fafc;
}
.subscription-source-guide summary {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 8px 10px;
  color: #708397;
  cursor: pointer;
  font-size: 11px;
  line-height: 1.4;
  list-style: none;
}
.subscription-source-guide-static {
  padding-top: 10px;
}
.subscription-command-title {
  margin: 0 12px 8px;
  color: var(--muted);
  font-size: 12px;
}
.subscription-source-guide summary::-webkit-details-marker {
  display: none;
}
.subscription-source-guide summary:hover {
  color: #4f6e89;
  background: #f7f9fc;
}
.subscription-source-guide summary:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: -2px;
}
.subscription-source-guide summary svg {
  flex: none;
  transition: transform 0.16s ease;
}
.subscription-source-guide[open] summary {
  border-bottom: 1px solid #e8edf2;
}
.subscription-source-guide[open] summary svg {
  transform: rotate(90deg);
}
.subscription-command-list {
  display: grid;
  padding: 4px 10px 6px;
}
.subscription-command-list > div {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr) 28px;
  align-items: center;
  min-width: 0;
  gap: 7px;
  padding: 7px 0;
}
.subscription-command-list > .subscription-command-item-direct {
  grid-template-columns: minmax(0, 1fr) 28px;
}
.subscription-command-item-direct code {
  overflow: visible;
  text-overflow: clip;
  white-space: normal;
  overflow-wrap: anywhere;
}
.subscription-command-list > div + div {
  border-top: 1px solid #e1e9f2;
}
.subscription-command-list span {
  color: #71869a;
  font-size: 10px;
}
.subscription-command-list code {
  min-width: 0;
  overflow: hidden;
  color: #385a75;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subscription-command-copy {
  width: 28px;
  height: 28px;
  color: #6e91b8;
  opacity: 0.8;
}
.subscription-command-list > div:hover .subscription-command-copy,
.subscription-command-copy:focus-visible {
  opacity: 1;
}
.subscription-input-panel {
  display: grid;
  gap: 8px;
}
.subscription-file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
  white-space: nowrap;
}
.subscription-file-picker {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 64px;
  padding: 11px 14px;
  border: 1px dashed #cbd5e1;
  border-radius: var(--radius-control);
  color: var(--color-text-secondary);
  cursor: pointer;
  background: #f8fafc;
}
.subscription-file-picker:hover {
  border-color: #93c5fd;
  background: var(--color-primary-soft);
}
.subscription-file-input:focus-visible + .subscription-file-picker {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
.subscription-file-input:disabled + .subscription-file-picker {
  cursor: not-allowed;
  opacity: 0.6;
}
.subscription-file-picker > svg {
  flex: none;
}
.subscription-file-picker span {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.subscription-file-picker strong {
  overflow: hidden;
  color: #29465f;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subscription-file-picker small {
  color: var(--muted);
  font-size: 10px;
}
.subscription-input-panel > .field-hint {
  margin: 0;
}
.subscription-paste-intro {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  line-height: 1.5;
}
.subscription-input-panel textarea {
  width: 100%;
  min-height: 142px;
  resize: vertical;
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 11px;
  line-height: 1.55;
}
@media (max-width: 760px) {
  .subscription-input-tabs button {
    flex: 1;
  }
  .subscription-source-guide {
    border-radius: 6px;
  }
  .subscription-command-list > div {
    grid-template-columns: 86px minmax(0, 1fr) 28px;
  }
}
</style>
