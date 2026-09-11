<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { all, api, errorText, gatewayBaseUrl } from '../api'
import { compactCount, count } from '../composables'
import { activeLocale, i18n, t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import type { Dashboard, Provider, Resource } from '../types'
import Icon from '../components/Icon.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'

type AccessProtocol = 'openai' | 'anthropic'
type SetupMethod = 'script' | 'ccswitch'
type SetupPlatform = 'unix' | 'windows'

const summary = ref<Dashboard | null>(null)
const healthProviders = ref<Provider[]>([])
const healthResources = ref<Resource[]>([])
const loading = ref(false)
const error = ref('')
const providerHealthLoading = ref(false)
const providerHealthError = ref('')
const copied = ref('')
const setupProtocol = ref<AccessProtocol | null>(null)
const setupMethod = ref<SetupMethod>('script')
const scriptCopied = ref('')
let revision = 0

const codexUrl = `${gatewayBaseUrl}/v1`
const claudeUrl = `${gatewayBaseUrl}/anthropic`
const currentMonth = ref(new Date())
const monthLabel = computed(() =>
  new Intl.DateTimeFormat(activeLocale.value, { year: 'numeric', month: 'long' }).format(
    currentMonth.value,
  ),
)
const maxTokens = computed(() =>
  Math.max(1, ...(summary.value?.tokenRanking.map((item) => item.tokens) ?? [])),
)
const maxClientRequests = computed(() =>
  Math.max(1, ...(summary.value?.clientModelRanking.map((item) => item.requests) ?? [])),
)
const maxProviderCalls = computed(() =>
  Math.max(1, ...(summary.value?.providerRanking.map((item) => item.calls) ?? [])),
)
const enabledHealthProviders = computed(() =>
  healthProviders.value.filter((provider) => provider.status === 'ACTIVE'),
)
const providerHealthIssues = computed(() =>
  enabledHealthProviders.value.flatMap((provider) => {
    const configured = healthResources.value.filter(
      (resource) => resource.providerId === provider.id,
    )
    if (!configured.length)
      return [{ provider, reason: t('providers.runtimeReasons.UNCONFIGURED') }]

    if (configured.some((resource) => resource.runtimeStatus !== 'BLOCKED')) return []

    const blocked = [...configured]
      .filter((resource) => resource.runtimeStatus === 'BLOCKED')
      .sort((left, right) => (right.lastErrorAt || '').localeCompare(left.lastErrorAt || ''))[0]
    if (!blocked) return [{ provider, reason: t('providers.runtimeReasons.UNKNOWN') }]
    const key = `resources.blockReasons.${blocked.blockedReason || 'UNKNOWN_PERMANENT'}`
    const reason = t(i18n.global.te(key) ? key : 'resources.blockReasons.UNKNOWN_PERMANENT')
    return [
      {
        provider,
        reason: blocked.lastHttpStatus ? `${reason} · HTTP ${blocked.lastHttpStatus}` : reason,
      },
    ]
  }),
)
const availableProviderCount = computed(
  () => enabledHealthProviders.value.length - providerHealthIssues.value.length,
)
const providerHealthState = computed(() => {
  if (providerHealthLoading.value) return 'loading'
  if (providerHealthError.value) return 'unknown'
  if (!enabledHealthProviders.value.length) return 'empty'
  if (!providerHealthIssues.value.length) return 'healthy'
  return availableProviderCount.value > 0 ? 'degraded' : 'unavailable'
})
const providerHealthSummary = computed(() => {
  switch (providerHealthState.value) {
    case 'loading':
      return t('home.providerHealthChecking')
    case 'unknown':
      return t('home.providerHealthUnknown')
    case 'empty':
      return t('home.providerHealthEmpty')
    case 'healthy':
      return t('home.providerHealthHealthy')
    case 'unavailable':
      return t('home.providerHealthUnavailable')
    default:
      return t('home.providerHealthPartial', {
        available: count(availableProviderCount.value),
        total: count(enabledHealthProviders.value.length),
      })
  }
})
const providerHealthAlertTitle = computed(() =>
  providerHealthIssues.value.length === 1
    ? t('home.providerHealthSingleIssue', {
        name: providerHealthIssues.value[0]?.provider.name || '',
      })
    : t('home.providerHealthMultipleIssues', {
        count: count(providerHealthIssues.value.length),
      }),
)
const providerHealthAlertDetail = computed(() =>
  providerHealthIssues.value.length === 1
    ? providerHealthIssues.value[0]?.reason || ''
    : t('home.providerHealthAffected', {
        names: providerHealthIssues.value.map((issue) => issue.provider.name).join('、'),
      }),
)
const setupUrl = computed(() => (setupProtocol.value === 'anthropic' ? claudeUrl : codexUrl))
const setupScripts = computed<Record<SetupPlatform, string>>(() => {
  const baseVariable =
    setupProtocol.value === 'anthropic' ? 'ANTHROPIC_BASE_URL' : 'OPENAI_BASE_URL'
  const keyVariable = setupProtocol.value === 'anthropic' ? 'ANTHROPIC_API_KEY' : 'OPENAI_API_KEY'
  return {
    unix: `printf 'Zentrola Access Key: '\nread -r -s ZENTROLA_KEY\nprintf '\\n'\nexport ${baseVariable}='${setupUrl.value}'\nexport ${keyVariable}="$ZENTROLA_KEY"\nunset ZENTROLA_KEY`,
    windows: `$secureKey = Read-Host 'Zentrola Access Key' -AsSecureString\n$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password\n$env:${baseVariable} = '${setupUrl.value}'\n$env:${keyVariable} = $zentrolaKey\nRemove-Variable secureKey, zentrolaKey`,
  }
})

function monthQuery() {
  const now = new Date()
  currentMonth.value = now
  const from = new Date(now.getFullYear(), now.getMonth(), 1)
  const to = new Date(now.getFullYear(), now.getMonth() + 1, 1)
  return new URLSearchParams({ from: from.toISOString(), to: to.toISOString() }).toString()
}

async function loadDashboard(current: number) {
  error.value = ''
  try {
    const result = await api<Dashboard>(`/usage/dashboard?${monthQuery()}`)
    if (current === revision)
      summary.value = {
        activeMemberCount: Number.isFinite(result.activeMemberCount) ? result.activeMemberCount : 0,
        modelCount: Number.isFinite(result.modelCount) ? result.modelCount : 0,
        providerCount: Number.isFinite(result.providerCount) ? result.providerCount : 0,
        totalTokens: Number.isFinite(result.totalTokens) ? result.totalTokens : 0,
        tokenRanking: Array.isArray(result.tokenRanking) ? result.tokenRanking : [],
        clientModelRanking: Array.isArray(result.clientModelRanking)
          ? result.clientModelRanking
          : [],
        providerRanking: Array.isArray(result.providerRanking) ? result.providerRanking : [],
      }
  } catch (e) {
    if (current === revision) error.value = errorText(e)
  }
}

async function loadProviderHealth(current: number) {
  providerHealthLoading.value = true
  providerHealthError.value = ''
  try {
    const [providers, resources] = await Promise.all([
      all<Provider>('/providers'),
      all<Resource>('/resources'),
    ])
    if (current === revision) {
      healthProviders.value = providers
      healthResources.value = resources
    }
  } catch (e) {
    if (current === revision) providerHealthError.value = errorText(e)
  } finally {
    if (current === revision) providerHealthLoading.value = false
  }
}

async function load() {
  const current = ++revision
  loading.value = true
  await Promise.all([loadDashboard(current), loadProviderHealth(current)])
  if (current === revision) loading.value = false
}

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

function openSetup() {
  setupProtocol.value = 'openai'
  setupMethod.value = 'script'
  scriptCopied.value = ''
}

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

function closeSetup() {
  setupProtocol.value = null
  scriptCopied.value = ''
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
    <section class="dashboard-metrics" :aria-label="t('home.monthOverview')">
      <div class="dashboard-section-head">
        <div>
          <h2>{{ t('home.monthOverview') }}</h2>
        </div>
        <span class="dashboard-period">{{ monthLabel }}</span>
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
        <div class="provider-total">
          <div class="provider-metric-label">
            <dt>{{ t('home.providers') }}</dt>
            <span
              class="provider-health-badge"
              :class="`is-${providerHealthState}`"
              role="img"
              :aria-label="providerHealthSummary"
              :title="providerHealthSummary"
            >
              <i></i>
            </span>
          </div>
          <dd>{{ summary ? count(summary.providerCount) : '—' }}</dd>
        </div>
        <div class="token-total">
          <dt>{{ t('home.monthTokens') }}</dt>
          <dd
            :title="summary ? count(summary.totalTokens) : undefined"
            :aria-label="summary ? count(summary.totalTokens) : undefined"
          >
            {{ summary ? compactCount(summary.totalTokens) : '—' }}
          </dd>
        </div>
      </dl>
      <div
        v-if="providerHealthIssues.length && !providerHealthLoading && !providerHealthError"
        class="provider-health-alert"
        :class="{ critical: availableProviderCount === 0 }"
        role="alert"
      >
        <Icon name="alert" :size="17" />
        <div>
          <strong>{{ providerHealthAlertTitle }}</strong>
          <span>{{ providerHealthAlertDetail }}</span>
        </div>
        <RouterLink :to="{ path: '/providers', query: { runtimeStatus: 'ABNORMAL' } }">
          {{ t('home.providerHealthViewProviders') }}
        </RouterLink>
      </div>
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
    </section>
  </div>

  <div class="ranking-grid">
    <section class="ranking-panel">
      <div class="ranking-head">
        <div>
          <h2>{{ t('home.tokenRanking') }}</h2>
        </div>
        <span>Top 10</span>
      </div>
      <ol v-if="summary?.tokenRanking.length" class="ranking-list token-ranking">
        <li v-for="(item, index) in summary.tokenRanking" :key="item.principalId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong>{{ item.name }}</strong>
              <span
                :title="`${count(item.tokens)} Token`"
                :aria-label="`${count(item.tokens)} Token`"
              >
                {{ compactCount(item.tokens) }} Token
              </span>
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
          <h2>{{ t('home.clientModelRanking') }}</h2>
        </div>
        <span>Top 10</span>
      </div>
      <ol v-if="summary?.clientModelRanking.length" class="ranking-list model-ranking">
        <li v-for="(item, index) in summary.clientModelRanking" :key="item.modelId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong :title="item.modelName">{{ item.modelName }}</strong>
              <span>{{ t('home.requestCount', { count: count(item.requests) }) }}</span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.requests / maxClientRequests) * 100}%` }"></i>
            </div>
            <small
              :title="`${count(item.tokens)} Token`"
              :aria-label="`${count(item.tokens)} Token`"
            >
              {{ item.modelCode }} · {{ compactCount(item.tokens) }} Token
            </small>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="models" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>

    <section class="ranking-panel">
      <div class="ranking-head">
        <div>
          <h2>{{ t('home.providerRanking') }}</h2>
        </div>
        <span>Top 10</span>
      </div>
      <ol v-if="summary?.providerRanking.length" class="ranking-list provider-ranking">
        <li v-for="(item, index) in summary.providerRanking" :key="item.providerId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong :title="item.providerName">{{ item.providerName }}</strong>
              <span>{{ t('home.callCount', { count: count(item.calls) }) }}</span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.calls / maxProviderCalls) * 100}%` }"></i>
            </div>
            <small
              :title="`${count(item.tokens)} Token`"
              :aria-label="`${count(item.tokens)} Token`"
            >
              {{ compactCount(item.tokens) }} Token
            </small>
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
