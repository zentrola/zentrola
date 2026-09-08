<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, errorText, gatewayBaseUrl } from '../api'
import { count, dateOnly } from '../composables'
import { t } from '../i18n'
import type { Dashboard } from '../types'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'

type AccessProtocol = 'openai' | 'anthropic'
type SetupPlatform = 'unix' | 'windows'

const summary = ref<Dashboard | null>(null)
const loading = ref(false)
const error = ref('')
const copied = ref('')
const setupProtocol = ref<AccessProtocol | null>(null)
const setupPlatform = ref<SetupPlatform>('unix')
const scriptCopied = ref('')
let revision = 0

const codexUrl = `${gatewayBaseUrl}/v1`
const claudeUrl = `${gatewayBaseUrl}/anthropic`
const today = new Date()
const todayLabel = dateOnly(today.toISOString())
const maxTokens = computed(() =>
  Math.max(1, ...(summary.value?.tokenRanking.map((item) => item.tokens) ?? [])),
)
const maxRequests = computed(() =>
  Math.max(1, ...(summary.value?.modelRanking.map((item) => item.requests) ?? [])),
)
const setupProtocolName = computed(() =>
  setupProtocol.value === 'anthropic' ? 'Anthropic' : 'OpenAI',
)
const setupUrl = computed(() => (setupProtocol.value === 'anthropic' ? claudeUrl : codexUrl))
const setupScript = computed(() => {
  const baseVariable =
    setupProtocol.value === 'anthropic' ? 'ANTHROPIC_BASE_URL' : 'OPENAI_BASE_URL'
  const keyVariable = setupProtocol.value === 'anthropic' ? 'ANTHROPIC_API_KEY' : 'OPENAI_API_KEY'
  return setupPlatform.value === 'windows'
    ? `$secureKey = Read-Host 'Zentrola Access Key' -AsSecureString\n$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password\n$env:${baseVariable} = '${setupUrl.value}'\n$env:${keyVariable} = $zentrolaKey\nRemove-Variable secureKey, zentrolaKey`
    : `printf 'Zentrola Access Key: '\nread -r -s ZENTROLA_KEY\nprintf '\\n'\nexport ${baseVariable}='${setupUrl.value}'\nexport ${keyVariable}="$ZENTROLA_KEY"\nunset ZENTROLA_KEY`
})

function todayQuery() {
  const from = new Date()
  from.setHours(0, 0, 0, 0)
  const to = new Date(from)
  to.setDate(to.getDate() + 1)
  return new URLSearchParams({ from: from.toISOString(), to: to.toISOString() }).toString()
}

async function load() {
  const current = ++revision
  loading.value = true
  error.value = ''
  try {
    const result = await api<Dashboard>(`/usage/dashboard?${todayQuery()}`)
    if (current === revision)
      summary.value = {
        activeMemberCount: Number.isFinite(result.activeMemberCount) ? result.activeMemberCount : 0,
        modelCount: Number.isFinite(result.modelCount) ? result.modelCount : 0,
        providerCount: Number.isFinite(result.providerCount) ? result.providerCount : 0,
        totalTokens: Number.isFinite(result.totalTokens) ? result.totalTokens : 0,
        tokenRanking: Array.isArray(result.tokenRanking) ? result.tokenRanking : [],
        modelRanking: Array.isArray(result.modelRanking) ? result.modelRanking : [],
      }
  } catch (e) {
    if (current === revision) error.value = errorText(e)
  } finally {
    if (current === revision) loading.value = false
  }
}

async function copyAddress(name: string, value: string) {
  try {
    await navigator.clipboard.writeText(value)
    copied.value = name
    window.setTimeout(() => {
      if (copied.value === name) copied.value = ''
    }, 1800)
  } catch {
    copied.value = 'error'
  }
}

function openSetup() {
  setupProtocol.value = 'openai'
  setupPlatform.value = 'unix'
  scriptCopied.value = ''
}

function selectSetupProtocol(protocol: AccessProtocol) {
  setupProtocol.value = protocol
  scriptCopied.value = ''
}

function selectSetupPlatform(platform: SetupPlatform) {
  setupPlatform.value = platform
  scriptCopied.value = ''
}

function closeSetup() {
  setupProtocol.value = null
  scriptCopied.value = ''
}

async function copySetupScript() {
  try {
    await navigator.clipboard.writeText(setupScript.value)
    scriptCopied.value = 'success'
    window.setTimeout(() => {
      if (scriptCopied.value === 'success') scriptCopied.value = ''
    }, 1800)
  } catch {
    scriptCopied.value = 'error'
  }
}

onMounted(load)
</script>

<template>
  <PageHeader name="home" :show-description="false">
    <button class="button" :disabled="loading" @click="load">
      <Icon name="refresh" :size="16" />{{ t('home.refresh') }}
    </button>
  </PageHeader>

  <p v-if="error" class="alert error dashboard-alert" role="alert">
    {{ error }}<button class="text-button" @click="load">{{ t('common.retry') }}</button>
  </p>

  <div class="dashboard-overview" :aria-busy="loading">
    <section class="dashboard-metrics" :aria-label="t('home.todayOverview')">
      <div class="dashboard-section-head">
        <div>
          <h2>{{ t('home.todayOverview') }}</h2>
          <p>{{ todayLabel }}</p>
        </div>
        <span class="live-indicator"><i></i>{{ t('home.liveData') }}</span>
      </div>
      <dl class="metric-strip">
        <div>
          <dt>{{ t('home.activeMembers') }}</dt>
          <dd>{{ summary ? count(summary.activeMemberCount) : '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('home.supportedModels') }}</dt>
          <dd>{{ summary ? count(summary.modelCount) : '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('home.providers') }}</dt>
          <dd>{{ summary ? count(summary.providerCount) : '—' }}</dd>
        </div>
        <div class="token-total">
          <dt>{{ t('home.todayTokens') }}</dt>
          <dd>{{ summary ? count(summary.totalTokens) : '—' }}</dd>
        </div>
      </dl>
    </section>

    <section class="access-panel" :aria-label="t('home.accessTitle')">
      <div class="access-panel-head">
        <div>
          <h2>{{ t('home.accessTitle') }}</h2>
        </div>
        <button type="button" class="access-guide" aria-haspopup="dialog" @click="openSetup">
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
      <p v-if="copied" class="copy-feedback" role="status">
        {{ t(copied === 'error' ? 'common.copyFailed' : 'common.copied') }}
      </p>
    </section>
  </div>

  <div class="ranking-grid">
    <section class="ranking-panel">
      <div class="ranking-head">
        <div>
          <h2>{{ t('home.tokenRanking') }}</h2>
          <p>{{ t('home.tokenRankingHint') }}</p>
        </div>
        <span>Top 10</span>
      </div>
      <ol v-if="summary?.tokenRanking.length" class="ranking-list">
        <li v-for="(item, index) in summary.tokenRanking" :key="item.principalId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong>{{ item.name }}</strong
              ><span>{{ count(item.tokens) }} Token</span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.tokens / maxTokens) * 100}%` }"></i>
            </div>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="usage" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>

    <section class="ranking-panel">
      <div class="ranking-head">
        <div>
          <h2>{{ t('home.modelRanking') }}</h2>
          <p>{{ t('home.modelRankingHint') }}</p>
        </div>
        <span>Top 10</span>
      </div>
      <ol v-if="summary?.modelRanking.length" class="ranking-list model-ranking">
        <li v-for="(item, index) in summary.modelRanking" :key="item.modelId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong>{{ item.name }}</strong>
              <span>{{ t('home.requestCount', { count: count(item.requests) }) }}</span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.requests / maxRequests) * 100}%` }"></i>
            </div>
            <small>{{ count(item.tokens) }} Token</small>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="models" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>
  </div>

  <Modal
    v-if="setupProtocol"
    medium
    :title="t('home.setupTitle')"
    description-id="access-setup-description"
    @close="closeSetup"
  >
    <div class="access-setup-dialog">
      <p id="access-setup-description" class="setup-intro">
        {{ t('home.setupDescription') }}
      </p>
      <section class="setup-step">
        <div class="setup-step-head">
          <span class="setup-step-number">1</span>
          <div>
            <h3 id="setup-protocol-label">{{ t('home.protocol') }}</h3>
            <p>{{ t('home.protocolHint') }}</p>
          </div>
        </div>
        <div class="setup-protocols" role="radiogroup" aria-labelledby="setup-protocol-label">
          <button
            type="button"
            role="radio"
            :class="{ active: setupProtocol === 'openai' }"
            :aria-checked="setupProtocol === 'openai'"
            @click="selectSetupProtocol('openai')"
          >
            <span>
              <strong>OpenAI</strong>
              <small>{{ t('home.openaiSetupHint') }}</small>
            </span>
            <code>/v1</code>
            <Icon v-if="setupProtocol === 'openai'" name="check" :size="16" />
          </button>
          <button
            type="button"
            role="radio"
            :class="{ active: setupProtocol === 'anthropic' }"
            :aria-checked="setupProtocol === 'anthropic'"
            @click="selectSetupProtocol('anthropic')"
          >
            <span>
              <strong>Anthropic</strong>
              <small>{{ t('home.anthropicSetupHint') }}</small>
            </span>
            <code>/anthropic</code>
            <Icon v-if="setupProtocol === 'anthropic'" name="check" :size="16" />
          </button>
        </div>
      </section>

      <section class="setup-step setup-script-step">
        <div class="setup-step-head setup-script-step-head">
          <span class="setup-step-number">2</span>
          <div class="setup-step-title">
            <h3 id="setup-platform-label">{{ t('home.setupCode') }}</h3>
            <p>{{ t('home.platform') }}</p>
          </div>
          <div class="setup-platform" role="tablist" aria-labelledby="setup-platform-label">
            <button
              id="setup-unix-tab"
              type="button"
              role="tab"
              :class="{ active: setupPlatform === 'unix' }"
              :aria-selected="setupPlatform === 'unix'"
              aria-controls="access-setup-script"
              @click="selectSetupPlatform('unix')"
            >
              macOS / Linux
            </button>
            <button
              id="setup-windows-tab"
              type="button"
              role="tab"
              :class="{ active: setupPlatform === 'windows' }"
              :aria-selected="setupPlatform === 'windows'"
              aria-controls="access-setup-script"
              @click="selectSetupPlatform('windows')"
            >
              Windows
            </button>
          </div>
        </div>
        <div
          id="access-setup-script"
          class="setup-script"
          role="tabpanel"
          :aria-labelledby="setupPlatform === 'unix' ? 'setup-unix-tab' : 'setup-windows-tab'"
        >
          <div class="setup-script-head">
            <span class="setup-script-context">
              <strong>{{ setupProtocolName }}</strong>
              <span>{{ t(setupPlatform === 'windows' ? 'home.powershell' : 'home.shell') }}</span>
            </span>
            <button type="button" @click="copySetupScript">
              <Icon :name="scriptCopied === 'success' ? 'check' : 'copy'" :size="15" />
              {{ t(scriptCopied === 'success' ? 'common.copied' : 'home.copyScript') }}
            </button>
          </div>
          <pre><code>{{ setupScript }}</code></pre>
        </div>
      </section>
      <p v-if="scriptCopied === 'error'" class="setup-copy-error" role="alert">
        {{ t('common.copyFailed') }}
      </p>

      <section class="setup-step setup-run-step">
        <div class="setup-step-head">
          <span class="setup-step-number">3</span>
          <div>
            <h3>{{ t('home.runClient') }}</h3>
            <p class="setup-note">{{ t('home.setupNote') }}</p>
          </div>
        </div>
      </section>

      <aside class="cc-switch-help">
        <div class="cc-switch-copy">
          <span class="cc-switch-mark">CC</span>
          <div>
            <strong>{{ t('home.ccSwitchTitle') }}</strong>
            <p>
              {{ t(setupPlatform === 'windows' ? 'home.ccSwitchWindows' : 'home.ccSwitchUnix') }}
            </p>
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
  </Modal>
</template>
