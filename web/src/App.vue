<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { identity, sessionExpired, login, logout, errorText, api, ApiError } from './api'
import { t } from './i18n'
import Icon from './components/Icon.vue'
const route = useRoute()
const username = ref(''),
  password = ref(''),
  busy = ref(false),
  error = ref(''),
  mobile = ref(false)
const setupRequired = ref(false),
  setupReady = ref(false),
  setupLoading = ref(false),
  setupError = ref(''),
  confirmPassword = ref(''),
  notice = ref('')
// 只展示后端确认过的锁定，按提交时的账号保存，避免影响其他账号。
const loginLocks = ref(new Map<string, number>())
const lockNow = ref(Date.now())
let lockTimer: ReturnType<typeof setInterval> | undefined
const lockSeconds = computed(() =>
  setupRequired.value
    ? 0
    : Math.max(0, Math.ceil(((loginLocks.value.get(username.value) ?? 0) - lockNow.value) / 1000)),
)
const lockTime = computed(
  () =>
    `${Math.floor(lockSeconds.value / 60)} 分 ${String(lockSeconds.value % 60).padStart(2, '0')} 秒`,
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
  const passwordBytes = new TextEncoder().encode(password.value).length
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
      : password.value.includes('\0')
        ? t('setup.passwordControl')
        : passwordBytes < 12
          ? t('setup.passwordShort')
          : passwordBytes > 72
            ? t('setup.passwordLong')
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
onMounted(loadSetup)
watch(identity, (value) => {
  if (!value) {
    notice.value = ''
    void loadSetup()
  }
})
const navigation = ['members', 'groups', 'models', 'resources', 'usage', 'operations']
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
      <div class="brand"><span class="brand-mark">z</span><span>zentrola</span></div>
      <div class="story-copy">
        <div class="story-emblem"><Icon name="shield" :size="38" /></div>
        <h1>{{ t('login.heading') }}</h1>
        <p>{{ t('login.description') }}</p>
        <div class="governance-flow">
          <div v-for="(item, index) in ['member', 'group', 'model']" :key="item">
            <span class="flow-node"
              ><Icon :name="['members', 'groups', 'models'][index]" />{{ t(`login.${item}`) }}</span
            ><Icon v-if="index < 2" name="arrow" :size="14" />
          </div>
        </div>
        <div class="flow-result"><Icon name="usage" :size="17" />{{ t('login.usage') }}</div>
      </div>
      <div class="story-footer">OpenAI <span>/</span> Anthropic</div>
    </aside>
    <main class="login-form-area">
      <form ref="form" class="login-form" :novalidate="setupRequired" @submit.prevent="signIn">
        <p class="login-label">{{ t('console') }}</p>
        <h2>{{ t(setupRequired ? 'setup.title' : 'login.title') }}</h2>
        <p>{{ t(setupRequired ? 'setup.subtitle' : 'login.subtitle') }}</p>
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
        <template v-if="setupReady"
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
  <div v-else class="app-shell">
    <button
      v-if="mobile"
      class="nav-overlay"
      :aria-label="t('close')"
      @click="mobile = false"
    ></button>
    <aside class="sidebar" :class="{ open: mobile }">
      <RouterLink to="/members" class="brand"
        ><span class="brand-mark">z</span><span>zentrola</span></RouterLink
      >
      <div class="workspace-label">
        <span class="workspace-square"><Icon name="shield" :size="18" /></span>
        <div>
          {{ t('console') }}<small>{{ t('governance') }}</small>
        </div>
      </div>
      <nav :aria-label="t('console')">
        <template v-for="(item, index) in navigation" :key="item"
          ><p v-if="index === 0 || index === 4" class="nav-section">
            {{ t(index === 0 ? 'governance' : 'records') }}
          </p>
          <RouterLink :to="`/${item}`"
            ><Icon :name="item" /><span>{{ t(`nav.${item}`) }}</span></RouterLink
          ></template
        >
      </nav>
      <div class="sidebar-bottom">
        <span class="avatar admin-avatar">{{
          (identity.displayName || identity.username).slice(0, 1)
        }}</span>
        <div class="admin-info">
          <strong>{{ identity.displayName || identity.username }}</strong
          ><small>{{ t('admin') }}</small>
        </div>
        <button class="icon-button" :aria-label="t('logout')" @click="signOut">
          <Icon name="logout" :size="19" />
        </button>
      </div>
    </aside>
    <div class="workspace">
      <header class="topbar">
        <button class="icon-button mobile-toggle" :aria-label="t('menu')" @click="mobile = !mobile">
          <Icon name="menu" />
        </button>
        <div class="breadcrumb">
          <span>{{ t('console') }}</span
          ><Icon name="arrow" :size="13" /><strong>{{
            t(`nav.${String(route.name || 'members')}`)
          }}</strong>
        </div>
        <div class="topbar-account"><span class="online-dot"></span>{{ identity.username }}</div>
      </header>
      <main id="main" class="main-content"><RouterView /></main>
    </div>
  </div>
</template>
