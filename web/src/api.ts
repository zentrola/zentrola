import { ref } from 'vue'
import type { Identity, Page } from './types'
import { t, i18n } from './i18n'

declare global {
  interface Window {
    __ZENTROLA_CONFIG__?: { apiBaseUrl?: string; gatewayBaseUrl?: string }
  }
}

// 只保存 Token 和服务端到期时间；身份信息每次启动都通过 /me 校验。
export const sessionKey = 'zentrola.admin.session'
type Session = { token: string; expiresAt: string }
const token = ref('')
export const identity = ref<Identity | null>(null)
export const sessionExpired = ref(false)
let generation = 0
let expiry: ReturnType<typeof setTimeout> | undefined
const pending = new Set<AbortController>()
const apiBaseUrl = (
  window.__ZENTROLA_CONFIG__?.apiBaseUrl ||
  import.meta.env.VITE_API_BASE_URL ||
  ''
)
  .trim()
  .replace(/\/+$/, '')
export const gatewayBaseUrl = (
  window.__ZENTROLA_CONFIG__?.gatewayBaseUrl ||
  import.meta.env.VITE_GATEWAY_BASE_URL ||
  apiBaseUrl ||
  (import.meta.env.DEV ? 'http://127.0.0.1:9527' : window.location.origin)
)
  .trim()
  .replace(/\/+$/, '')
export function clearSession(expired = false, removeStored = true) {
  generation++
  token.value = ''
  identity.value = null
  sessionExpired.value = expired
  clearTimeout(expiry)
  for (const controller of pending) controller.abort()
  pending.clear()
  if (removeStored) {
    try {
      localStorage.removeItem(sessionKey)
    } catch {
      // 浏览器禁用存储时仍可清理当前页面会话。
    }
  }
}

function validSession(value: unknown): value is Session {
  if (!value || typeof value !== 'object') return false
  const session = value as Partial<Session>
  return (
    typeof session.token === 'string' &&
    session.token.length > 0 &&
    typeof session.expiresAt === 'string' &&
    Number.isFinite(Date.parse(session.expiresAt))
  )
}

function scheduleExpiry(session: Session) {
  clearTimeout(expiry)
  const remaining = Date.parse(session.expiresAt) - Date.now()
  if (remaining <= 0) {
    clearSession(true)
    return false
  }
  expiry = setTimeout(() => scheduleExpiry(session), Math.min(remaining, 2147483647))
  return true
}

export async function restoreSession() {
  let stored: string | null
  try {
    stored = localStorage.getItem(sessionKey)
  } catch {
    return
  }
  if (!stored) return
  let session: unknown
  try {
    session = JSON.parse(stored)
  } catch {
    clearSession()
    return
  }
  if (!validSession(session)) {
    clearSession()
    return
  }
  if (!scheduleExpiry(session)) return
  token.value = session.token
  try {
    identity.value = await api<Identity>('/me')
    sessionExpired.value = false
  } catch (error) {
    // 401 已由 api 清理；网络错误保留凭证，交给页面显示重试。
    if (error instanceof ApiError && error.code === 'UNAUTHENTICATED') return
    throw error
  }
}
export class ApiError extends Error {
  constructor(
    public code: string,
    public requestId = '',
    public retryAfterSeconds = 0,
    public field = '',
  ) {
    super(code)
  }
}
export function errorText(error: unknown) {
  const code = error instanceof ApiError ? error.code : 'UNKNOWN'
  const key = i18n.global.te(`errors.${code}`) ? `errors.${code}` : 'errors.UNKNOWN'
  const text = t(key, {
    field: error instanceof ApiError && error.field ? error.field : '-',
  })
  return error instanceof ApiError && error.requestId ? `${text} (${error.requestId})` : text
}
export async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const controller = new AbortController()
  pending.add(controller)
  const epoch = generation
  const timeout = setTimeout(() => controller.abort('timeout'), 25000)
  try {
    const response = await fetch(`${apiBaseUrl}/api/v1${path}`, {
      method,
      signal: controller.signal,
      cache: 'no-store',
      credentials: 'omit',
      redirect: 'error',
      headers: {
        ...(token.value ? { Authorization: `Bearer ${token.value}` } : {}),
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const data = await response.json()
    if (epoch !== generation) throw new ApiError('UNAUTHENTICATED')
    if (!response.ok || data.code !== 'OK') {
      if (response.status === 401 && token.value) clearSession(true)
      throw new ApiError(
        data.code || 'UNKNOWN',
        data.requestId || response.headers.get('X-Request-ID') || '',
        typeof data.data?.retryAfterSeconds === 'number' &&
        Number.isFinite(data.data.retryAfterSeconds) &&
        data.data.retryAfterSeconds > 0
          ? Math.ceil(data.data.retryAfterSeconds)
          : 0,
        typeof data.data?.field === 'string' ? data.data.field : '',
      )
    }
    return data.data as T
  } catch (error) {
    if (epoch !== generation) throw new ApiError('UNAUTHENTICATED')
    if (error instanceof ApiError) throw error
    throw new ApiError(controller.signal.reason === 'timeout' ? 'TIMEOUT' : 'NETWORK')
  } finally {
    clearTimeout(timeout)
    pending.delete(controller)
  }
}
export async function login(username: string, password: string) {
  clearSession()
  const result = await api<Session>('/auth/login', 'POST', {
    username,
    password,
  })
  if (!validSession(result)) throw new ApiError('UNKNOWN')
  if (!scheduleExpiry(result)) throw new ApiError('UNAUTHENTICATED')
  token.value = result.token
  try {
    identity.value = await api<Identity>('/me')
  } catch (error) {
    clearSession()
    throw error
  }
  sessionExpired.value = false
  try {
    localStorage.setItem(
      sessionKey,
      JSON.stringify({ token: result.token, expiresAt: result.expiresAt }),
    )
  } catch {
    // 存储不可用时降级为当前页面会话，不阻断登录。
  }
}
export async function logout() {
  try {
    await api('/auth/logout', 'POST')
  } finally {
    clearSession()
  }
}
export async function all<T>(path: string): Promise<T[]> {
  const items: T[] = []
  let after: string | null = null
  do {
    const url = new URL(path, 'http://zentrola.local')
    url.searchParams.set('limit', '100')
    if (after) url.searchParams.set('after', after)
    else url.searchParams.delete('after')
    const page: Page<T> = await api(`${url.pathname}?${url.searchParams}`)
    items.push(...page.items)
    if (page.nextCursor === after && after !== null) throw new ApiError('UNKNOWN')
    after = page.nextCursor
  } while (after)
  return items
}
