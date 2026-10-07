<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  identity,
  sessionExpired,
  login,
  logout,
  restoreSession,
  clearSession,
  sessionKey,
  errorText,
  api,
  ApiError,
} from './api'
import { t } from './i18n'
import Icon from './components/Icon.vue'
import ChangePassword from './components/ChangePassword.vue'
import AccountMenu from './components/AccountMenu.vue'
import LanguageSwitch from './components/LanguageSwitch.vue'
import ToastHost from './components/ToastHost.vue'
const changingPassword = ref(false)
async function passwordChanged() {
  changingPassword.value = false
  clearSession()
  await nextTick()
  notice.value = t('passwordChange.success')
}
const route = useRoute()
const username = ref(''),
  password = ref(''),
  busy = ref(false),
  error = ref(''),
  mobile = ref(false)
const compactSidebarQuery = '(min-width: 801px) and (max-width: 1024px)'
const sidebarCollapsed = ref(
  typeof window !== 'undefined' && window.matchMedia(compactSidebarQuery).matches,
)
let sidebarMedia: MediaQueryList | undefined
function syncSidebarBreakpoint(event: MediaQueryListEvent | MediaQueryList) {
  sidebarCollapsed.value = event.matches
}
const setupRequired = ref(false),
  setupReady = ref(false),
  setupLoading = ref(false),
  setupError = ref(''),
  confirmPassword = ref(''),
  notice = ref('')
const restoring = ref(true),
  restoreError = ref('')
let restoreRevision = 0
async function initializeSession() {
  const revision = ++restoreRevision
  restoring.value = true
  restoreError.value = ''
  setupReady.value = false
  try {
    await restoreSession()
    if (revision === restoreRevision && !identity.value) await loadSetup()
  } catch (e) {
    if (revision === restoreRevision) restoreError.value = errorText(e)
  } finally {
    if (revision === restoreRevision) restoring.value = false
  }
}
function syncSession(event: StorageEvent) {
  if (event.storageArea !== localStorage || (event.key !== sessionKey && event.key !== null)) return
  clearSession(false, false)
  void initializeSession()
}
// 只展示后端确认过的锁定，按提交时的账号保存，避免影响其他账号。
const loginLocks = ref(new Map<string, number>())
const lockNow = ref(Date.now())
let lockTimer: ReturnType<typeof setInterval> | undefined
const lockSeconds = computed(() =>
  setupRequired.value
    ? 0
    : Math.max(0, Math.ceil(((loginLocks.value.get(username.value) ?? 0) - lockNow.value) / 1000)),
)
const lockTime = computed(() =>
  t('login.lockDuration', {
    minutes: Math.floor(lockSeconds.value / 60),
    seconds: String(lockSeconds.value % 60).padStart(2, '0'),
  }),
)
function rememberLock(account: string, seconds: number) {
  lockNow.value = Date.now()
  loginLocks.value.set(account, lockNow.value + seconds * 1000)
  if (lockTimer !== undefined) return
  lockTimer = setInterval(() => {
    lockNow.value = Date.now()
    for (const [account, until] of loginLocks.value) {
      if (until <= lockNow.value) loginLocks.value.delete(account)
    }
    if (!loginLocks.value.size) {
      clearInterval(lockTimer)
      lockTimer = undefined
    }
  }, 1000)
}
onUnmounted(() => clearInterval(lockTimer))
watch(username, () => {
  error.value = ''
})
type SetupField = 'username' | 'password' | 'confirmPassword'
const form = ref<HTMLFormElement | null>(null)
const touched = ref<Record<SetupField, boolean>>({
  username: false,
  password: false,
  confirmPassword: false,
})
const setupErrors = computed(() => {
  const usernameBytes = new TextEncoder().encode(username.value).length
  const passwordCharacters = Array.from(password.value).length
  return {
    username: !username.value
      ? t('setup.usernameRequired')
      : /[\u0000-\u001f\u007f-\u009f]/.test(username.value)
        ? t('setup.usernameControl')
        : /^\p{White_Space}|\p{White_Space}$/u.test(username.value)
          ? t('setup.usernameWhitespace')
          : usernameBytes > 64
            ? t('setup.usernameLength')
            : '',
    password: !password.value
      ? t('setup.passwordRequired')
      : passwordCharacters < 6
        ? t('setup.passwordShort')
        : passwordCharacters > 30
          ? t('setup.passwordLong')
          : !/^[\x21-\x7e]+$/.test(password.value)
            ? t('setup.passwordCharacters')
            : '',
    confirmPassword: !confirmPassword.value
      ? t('setup.confirmRequired')
      : password.value !== confirmPassword.value
        ? t('setup.mismatch')
        : '',
  }
})
function fieldError(field: SetupField) {
  return setupRequired.value && touched.value[field] ? setupErrors.value[field] : ''
}
function resetValidation() {
  touched.value = { username: false, password: false, confirmPassword: false }
}
watch(setupRequired, resetValidation)
let setupRevision = 0
async function loadSetup() {
  const revision = ++setupRevision
  setupLoading.value = true
  setupReady.value = false
  setupError.value = ''
  try {
    const status = await api<{ required: boolean }>('/auth/setup')
    if (revision !== setupRevision) return
    setupRequired.value = status.required
    setupReady.value = true
  } catch (e) {
    if (revision === setupRevision) setupError.value = errorText(e)
  } finally {
    if (revision === setupRevision) setupLoading.value = false
  }
}
onMounted(() => {
  window.addEventListener('storage', syncSession)
  sidebarMedia = window.matchMedia(compactSidebarQuery)
  sidebarCollapsed.value = sidebarMedia.matches
  sidebarMedia.addEventListener('change', syncSidebarBreakpoint)
  void initializeSession()
})
onUnmounted(() => {
  window.removeEventListener('storage', syncSession)
  sidebarMedia?.removeEventListener('change', syncSidebarBreakpoint)
})
watch(identity, (value) => {
  changingPassword.value = false
  if (!value && !restoring.value) {
    notice.value = ''
    void loadSetup()
  }
})
const navigation = [
  'home',
  'members',
  'applications',
  'groups',
  'models',
  'providers',
  'usage',
  'billing',
  'operations',
]
watch(
  () => route.path,
  () => {
    mobile.value = false
  },
)
async function signIn() {
  if (busy.value || !setupReady.value || lockSeconds.value > 0) return
  if (setupRequired.value) {
    touched.value = { username: true, password: true, confirmPassword: true }
    const firstInvalid = (Object.keys(setupErrors.value) as SetupField[]).find(
      (field) => setupErrors.value[field],
    )
    if (firstInvalid) {
      form.value?.querySelector<HTMLInputElement>(`[name="${firstInvalid}"]`)?.focus()
      return
    }
  }
  busy.value = true
  error.value = ''
  const submittedUsername = username.value
  try {
    if (setupRequired.value) {
      await api('/auth/setup', 'POST', { username: username.value, password: password.value })
      setupRequired.value = false
      notice.value = t('setup.created')
    } else {
      await login(submittedUsername, password.value)
      loginLocks.value.delete(submittedUsername)
    }
  } catch (e) {
    if (e instanceof ApiError && e.code === 'ACCOUNT_LOCKED' && e.retryAfterSeconds > 0) {
      rememberLock(submittedUsername, e.retryAfterSeconds)
    } else {
      error.value = errorText(e)
    }
    if (e instanceof ApiError && e.code === 'ALREADY_INITIALIZED') await loadSetup()
  } finally {
    password.value = ''
    confirmPassword.value = ''
    resetValidation()
    busy.value = false
  }
}
async function signOut() {
  try {
    await logout()
  } catch {
    /* 客户端会话已清理。 */
  }
}
</script>
<template>
  <div v-if="!identity" class="login-shell">
    <aside class="login-story">
      <div class="brand">
        <span class="brand-mark">Z</span><span>{{ t('brand') }}</span>
      </div>
      <div class="story-copy">
        <h1>{{ t('login.heading') }}</h1>
        <p>{{ t('login.description') }}</p>
        <div class="protocol-promise">
          <Icon name="check" :size="18" />
          <div>
            <strong>{{ t('login.protocols') }}</strong>
            <span>{{ t('login.protocolHabit') }}</span>
          </div>
        </div>
      </div>
    </aside>
    <main class="login-form-area">
      <LanguageSwitch class="login-language" />
      <form ref="form" class="login-form" :novalidate="setupRequired" @submit.prevent="signIn">
        <h2>
          {{
            t(
              restoring || restoreError
                ? 'login.restoring'
                : setupRequired
                  ? 'setup.title'
                  : 'login.title',
            )
          }}
        </h2>
        <p>{{ t(setupRequired ? 'setup.subtitle' : 'login.subtitle') }}</p>
        <p v-if="restoring" role="status">{{ t('login.restoring') }}</p>
        <div v-if="restoreError" class="alert error" role="alert">
          {{ restoreError
          }}<button type="button" class="text-button" @click="initializeSession">
            {{ t('common.retry') }}
          </button>
        </div>
        <p v-if="setupLoading" role="status">{{ t('setup.checking') }}</p>
        <div v-if="setupError" class="alert error" role="alert">
          {{ setupError
          }}<button type="button" class="text-button" @click="loadSetup">
            {{ t('common.retry') }}
          </button>
        </div>
        <div v-if="notice" class="alert success" role="status">{{ notice }}</div>
        <div v-if="lockSeconds > 0 || error || sessionExpired" class="alert error" role="alert">
          {{
            lockSeconds > 0 ? t('login.locked', { time: lockTime }) : error || t('login.expired')
          }}
        </div>
        <template v-if="setupReady && !restoring && !restoreError"
          ><label for="auth-username"
            >{{ t('login.username')
            }}<input
              v-model="username"
              id="auth-username"
              name="username"
              autocomplete="username"
              required
              :maxlength="setupRequired ? undefined : 64"
              :aria-invalid="!!fieldError('username')"
              :aria-describedby="fieldError('username') ? 'username-error' : undefined"
              @blur="touched.username = true"
              :disabled="busy"
              autofocus
          /></label>
          <p
            v-if="fieldError('username')"
            id="username-error"
            class="auth-field-error"
            role="alert"
          >
            {{ fieldError('username') }}
          </p>
          <label for="auth-password"
            >{{ t('login.password')
            }}<input
              v-model="password"
              id="auth-password"
              name="password"
              type="password"
              :autocomplete="setupRequired ? 'new-password' : 'current-password'"
              required
              :aria-invalid="!!fieldError('password')"
              :aria-describedby="fieldError('password') ? 'password-error' : undefined"
              @blur="touched.password = true"
              :disabled="busy"
          /></label>
          <p
            v-if="fieldError('password')"
            id="password-error"
            class="auth-field-error"
            role="alert"
          >
            {{ fieldError('password') }}
          </p>
          <template v-if="setupRequired"
            ><label for="auth-confirm-password"
              >{{ t('setup.confirmPassword')
              }}<input
                v-model="confirmPassword"
                id="auth-confirm-password"
                name="confirmPassword"
                type="password"
                autocomplete="new-password"
                required
                :aria-invalid="!!fieldError('confirmPassword')"
                :aria-describedby="
                  fieldError('confirmPassword') ? 'confirm-password-error' : undefined
                "
                @blur="touched.confirmPassword = true"
                :disabled="busy"
            /></label>
            <p
              v-if="fieldError('confirmPassword')"
              id="confirm-password-error"
              class="auth-field-error"
              role="alert"
            >
              {{ fieldError('confirmPassword') }}
            </p></template
          >
          <button class="button primary login-submit" :disabled="busy || lockSeconds > 0">
            {{
              lockSeconds > 0
                ? t('login.retryIn', { time: lockTime })
                : t(busy ? 'common.working' : setupRequired ? 'setup.submit' : 'login.submit')
            }}<Icon name="arrow" :size="17" />
          </button>
        </template>
        <p class="session-note"><Icon name="shield" :size="16" />{{ t('login.session') }}</p>
      </form>
    </main>
  </div>
  <div v-else class="app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
    <button
      v-if="mobile"
      class="nav-overlay"
      :aria-label="t('close')"
      @click="mobile = false"
    ></button>
    <aside class="sidebar" :class="{ open: mobile, collapsed: sidebarCollapsed }">
      <RouterLink to="/" class="brand" :aria-label="t('console')" :title="t('console')"
        ><span class="brand-mark">Z</span><span>{{ t('brand') }}</span></RouterLink
      >
      <button
        type="button"
        class="sidebar-collapse"
        :aria-label="t(sidebarCollapsed ? 'expandNavigation' : 'collapseNavigation')"
        :title="t(sidebarCollapsed ? 'expandNavigation' : 'collapseNavigation')"
        :aria-expanded="!sidebarCollapsed"
        aria-controls="primary-navigation"
        @click="sidebarCollapsed = !sidebarCollapsed"
      >
        <Icon name="arrow" :size="15" />
      </button>
      <nav id="primary-navigation" :aria-label="t('console')">
        <template v-for="item in navigation" :key="item"
          ><p v-if="item === 'home' || item === 'members' || item === 'usage'" class="nav-section">
            {{ t(item === 'home' ? 'overview' : item === 'members' ? 'governance' : 'records') }}
            <a
              v-if="item === 'home'"
              class="community-source-link"
              href="https://github.com/zentrola/zentrola"
              target="_blank"
              rel="noopener noreferrer"
              :aria-label="t('communityRepository')"
              :title="t('communityRepository')"
            >
              <Icon name="external" :size="12" />
            </a>
          </p>
          <RouterLink
            :to="`/${item}`"
            :aria-label="t(`nav.${item}`)"
            :data-label="t(`nav.${item}`)"
            :id="'nav-' + item"
            ><Icon :name="item" /><span>{{ t(`nav.${item}`) }}</span></RouterLink
          ></template
        >
      </nav>
    </aside>
    <div class="workspace">
      <header class="topbar">
        <button class="icon-button mobile-toggle" :aria-label="t('menu')" @click="mobile = !mobile">
          <Icon name="menu" />
        </button>
        <div class="breadcrumb">
          <span>{{ t('console') }}</span
          ><Icon name="arrow" :size="13" /><strong>{{
            t(`nav.${String(route.name || 'home')}`)
          }}</strong>
        </div>
        <div class="topbar-actions">
          <LanguageSwitch />
          <AccountMenu
            :username="identity.username"
            :display-name="identity.displayName"
            @change-password="changingPassword = true"
            @logout="signOut"
          />
        </div>
      </header>
      <main id="main" class="main-content"><RouterView /></main>
    </div>
    <ChangePassword
      v-if="changingPassword"
      :username="identity.username"
      @close="changingPassword = false"
      @changed="passwordChanged"
    />
  </div>
  <ToastHost />
</template>
