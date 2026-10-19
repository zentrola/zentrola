<script setup lang="ts">
import { ref } from 'vue'
import { claudeUrl, codexUrl } from '../../accessSetup'
import { t } from '../../i18n'
import { showErrorToast, showSuccessToast } from '../../toast'
import Icon from '../Icon.vue'

const emit = defineEmits<{ setup: [] }>()
const copied = ref('')

async function copyAddress(name: string, value: string) {
  try {
    await navigator.clipboard.writeText(value)
    copied.value = name
    showSuccessToast(t('common.copied'))
    window.setTimeout(() => {
      if (copied.value === name) copied.value = ''
    }, 1800)
  } catch {
    copied.value = ''
    showErrorToast(t('common.copyFailed'))
  }
}
</script>

<template>
  <section class="access-panel" :aria-label="t('home.accessTitle')">
    <div class="access-panel-head">
      <div>
        <h2>{{ t('home.accessTitle') }}</h2>
      </div>
      <button type="button" class="access-guide" aria-haspopup="dialog" @click="emit('setup')">
        {{ t('home.setupAction') }}
      </button>
    </div>
    <div class="access-addresses">
      <div>
        <div class="access-address-name">
          <div class="access-address-title">
            <span>OpenAI</span>
            <small>{{ t('home.openaiClients') }}</small>
          </div>
        </div>
        <code>{{ codexUrl }}</code>
        <button
          class="copy-address"
          :aria-label="t('home.copyAddress', { name: 'OpenAI' })"
          @click="copyAddress('codex', codexUrl)"
        >
          <Icon :name="copied === 'codex' ? 'check' : 'copy'" :size="16" />
        </button>
      </div>
      <div>
        <div class="access-address-name">
          <div class="access-address-title">
            <span>Anthropic</span>
            <small>{{ t('home.anthropicClients') }}</small>
          </div>
        </div>
        <code>{{ claudeUrl }}</code>
        <button
          class="copy-address"
          :aria-label="t('home.copyAddress', { name: 'Anthropic' })"
          @click="copyAddress('claude', claudeUrl)"
        >
          <Icon :name="copied === 'claude' ? 'check' : 'copy'" :size="16" />
        </button>
      </div>
    </div>
  </section>
</template>
