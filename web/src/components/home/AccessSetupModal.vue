<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  setupScriptsFor,
  type AccessProtocol,
  type SetupMethod,
  type SetupPlatform,
} from '../../accessSetup'
import { t } from '../../i18n'
import { showErrorToast, showSuccessToast } from '../../toast'
import Icon from '../Icon.vue'
import Modal from '../Modal.vue'

const emit = defineEmits<{ close: [] }>()
// 每次打开弹窗都从脚本方式与 OpenAI 协议开始。
const setupProtocol = ref<AccessProtocol>('openai')
const setupMethod = ref<SetupMethod>('script')
const scriptCopied = ref('')
const setupScripts = computed(() => setupScriptsFor(setupProtocol.value))

function selectSetupMethod(method: SetupMethod) {
  setupMethod.value = method
  scriptCopied.value = ''
}

function focusSetupMethod(method: SetupMethod) {
  selectSetupMethod(method)
  window.requestAnimationFrame(() =>
    document.querySelector<HTMLElement>(`#setup-method-${method}-tab`)?.focus(),
  )
}

function selectSetupProtocol(protocol: AccessProtocol) {
  setupProtocol.value = protocol
  scriptCopied.value = ''
}

function focusSetupProtocol(protocol: AccessProtocol) {
  selectSetupProtocol(protocol)
  window.requestAnimationFrame(() =>
    document.querySelector<HTMLElement>(`#setup-${protocol}-tab`)?.focus(),
  )
}

async function copySetupScript(platform: SetupPlatform) {
  try {
    await navigator.clipboard.writeText(setupScripts.value[platform])
    scriptCopied.value = platform
    showSuccessToast(t('common.copied'))
    window.setTimeout(() => {
      if (scriptCopied.value === platform) scriptCopied.value = ''
    }, 1800)
  } catch {
    scriptCopied.value = ''
    showErrorToast(t('common.copyFailed'))
  }
}
</script>

<template>
  <Modal
    medium
    :title="t('home.setupTitle')"
    description-id="access-setup-description"
    @close="emit('close')"
  >
    <div class="access-setup-dialog">
      <p id="access-setup-description" class="setup-intro">
        {{ t('home.setupDescription') }}
      </p>
      <div class="setup-method-tabs" role="tablist" :aria-label="t('home.setupMethod')">
        <button
          id="setup-method-script-tab"
          type="button"
          role="tab"
          :class="{ active: setupMethod === 'script' }"
          :aria-selected="setupMethod === 'script'"
          :tabindex="setupMethod === 'script' ? 0 : -1"
          aria-controls="setup-method-script-panel"
          @click="selectSetupMethod('script')"
          @keydown.right.prevent="focusSetupMethod('ccswitch')"
          @keydown.end.prevent="focusSetupMethod('ccswitch')"
        >
          <strong>{{ t('home.scriptMethod') }}</strong>
          <small>{{ t('home.scriptMethodHint') }}</small>
        </button>
        <button
          id="setup-method-ccswitch-tab"
          type="button"
          role="tab"
          :class="{ active: setupMethod === 'ccswitch' }"
          :aria-selected="setupMethod === 'ccswitch'"
          :tabindex="setupMethod === 'ccswitch' ? 0 : -1"
          aria-controls="setup-method-ccswitch-panel"
          @click="selectSetupMethod('ccswitch')"
          @keydown.left.prevent="focusSetupMethod('script')"
          @keydown.home.prevent="focusSetupMethod('script')"
        >
          <strong>{{ t('home.ccSwitchMethod') }}</strong>
          <small>{{ t('home.ccSwitchMethodHint') }}</small>
        </button>
      </div>

      <div
        v-show="setupMethod === 'script'"
        id="setup-method-script-panel"
        class="setup-method-panel"
        role="tabpanel"
        aria-labelledby="setup-method-script-tab"
      >
        <section class="setup-step">
          <div class="setup-step-head">
            <span class="setup-step-number">1</span>
            <div>
              <h3 id="setup-protocol-label">{{ t('home.protocol') }}</h3>
              <p>{{ t('home.protocolHint') }}</p>
            </div>
          </div>
          <div class="setup-protocols" role="tablist" aria-labelledby="setup-protocol-label">
            <button
              id="setup-openai-tab"
              type="button"
              role="tab"
              :class="{ active: setupProtocol === 'openai' }"
              :aria-selected="setupProtocol === 'openai'"
              :tabindex="setupProtocol === 'openai' ? 0 : -1"
              aria-controls="setup-protocol-panel"
              @click="selectSetupProtocol('openai')"
              @keydown.right.prevent="focusSetupProtocol('anthropic')"
              @keydown.end.prevent="focusSetupProtocol('anthropic')"
            >
              <span>
                <strong>OpenAI</strong>
                <small>{{ t('home.openaiSetupHint') }}</small>
              </span>
              <code>/v1</code>
            </button>
            <button
              id="setup-anthropic-tab"
              type="button"
              role="tab"
              :class="{ active: setupProtocol === 'anthropic' }"
              :aria-selected="setupProtocol === 'anthropic'"
              :tabindex="setupProtocol === 'anthropic' ? 0 : -1"
              aria-controls="setup-protocol-panel"
              @click="selectSetupProtocol('anthropic')"
              @keydown.left.prevent="focusSetupProtocol('openai')"
              @keydown.home.prevent="focusSetupProtocol('openai')"
            >
              <span>
                <strong>Anthropic</strong>
                <small>{{ t('home.anthropicSetupHint') }}</small>
              </span>
              <code>/anthropic</code>
            </button>
          </div>
          <div
            id="setup-protocol-panel"
            class="setup-protocol-panel"
            role="tabpanel"
            :aria-labelledby="
              setupProtocol === 'anthropic' ? 'setup-anthropic-tab' : 'setup-openai-tab'
            "
          >
            <div class="setup-scripts">
              <div class="setup-script">
                <div class="setup-script-head">
                  <span class="setup-script-context">
                    <strong>macOS / Linux</strong>
                    <span>{{ t('home.shell') }}</span>
                  </span>
                  <button
                    type="button"
                    :aria-label="t('home.copyPlatformScript', { platform: 'macOS / Linux' })"
                    @click="copySetupScript('unix')"
                  >
                    <Icon :name="scriptCopied === 'unix' ? 'check' : 'copy'" :size="15" />
                    {{ t(scriptCopied === 'unix' ? 'common.copied' : 'home.copyScript') }}
                  </button>
                </div>
                <pre><code>{{ setupScripts.unix }}</code></pre>
              </div>
              <div class="setup-script">
                <div class="setup-script-head">
                  <span class="setup-script-context">
                    <strong>Windows</strong>
                    <span>{{ t('home.powershell') }}</span>
                  </span>
                  <button
                    type="button"
                    :aria-label="t('home.copyPlatformScript', { platform: 'Windows' })"
                    @click="copySetupScript('windows')"
                  >
                    <Icon :name="scriptCopied === 'windows' ? 'check' : 'copy'" :size="15" />
                    {{ t(scriptCopied === 'windows' ? 'common.copied' : 'home.copyScript') }}
                  </button>
                </div>
                <pre><code>{{ setupScripts.windows }}</code></pre>
              </div>
            </div>
          </div>
        </section>
        <section class="setup-step setup-run-step">
          <div class="setup-step-head">
            <span class="setup-step-number">2</span>
            <div>
              <h3>{{ t('home.runClient') }}</h3>
              <p class="setup-note">{{ t('home.setupNote') }}</p>
            </div>
          </div>
        </section>
      </div>

      <div
        v-show="setupMethod === 'ccswitch'"
        id="setup-method-ccswitch-panel"
        class="setup-method-panel setup-cc-method-panel"
        role="tabpanel"
        aria-labelledby="setup-method-ccswitch-tab"
      >
        <aside class="cc-switch-help">
          <div class="cc-switch-copy">
            <span class="cc-switch-mark">CC</span>
            <div>
              <strong>{{ t('home.ccSwitchTitle') }}</strong>
              <p>{{ t('home.ccSwitchAll') }}</p>
            </div>
          </div>
          <a
            class="button subtle cc-switch-link"
            href="https://ccswitch.io/"
            target="_blank"
            rel="noopener noreferrer"
          >
            {{ t('home.ccSwitchWebsite') }}<Icon name="external" :size="15" />
          </a>
        </aside>
      </div>
    </div>
  </Modal>
</template>
