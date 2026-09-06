import { ref } from 'vue'
import type { Identity, Page } from './types'
import { t, i18n } from './i18n'

// 管理员 JWT 只存在内存中，不进入 Web Storage、URL 或日志。
const token = ref('')
export const identity = ref<Identity | null>(null)
export const sessionExpired = ref(false)
let generation = 0
let expiry: ReturnType<typeof setTimeout> | undefined
const pending = new Set<AbortController>()
export function clearSession(expired = false) {
  generation++
  token.value = ''
  identity.value = null
  sessionExpired.value = expired
  clearTimeout(expiry)
  for (const controller of pending) controller.abort()
  pending.clear()
}
export class ApiError extends Error {
  constructor(
    public code: string,
    public requestId = '',
    public retryAfterSeconds = 0,
  ) {
    super(code)
  }
}
export function errorText(error: unknown) {
  const code = error instanceof ApiError ? error.code : 'UNKNOWN'
  const text = t(i18n.global.te(`errors.${code}`) ? `errors.${code}` : 'errors.UNKNOWN')
  return error instanceof ApiError && error.requestId ? `${text} (${error.requestId})` : text
}
export async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const controller = new AbortController()
  pending.add(controller)
  const epoch = generation
  const timeout = setTimeout(() => controller.abort('timeout'), 25000)
  try {
    const response = await fetch(`/api/v1${path}`, {
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
      )
    }
    return data.data as T
  } catch (error) {
    if (error instanceof ApiError) throw error
    throw new ApiError(controller.signal.reason === 'timeout' ? 'TIMEOUT' : 'NETWORK')
  } finally {
    clearTimeout(timeout)
    pending.delete(controller)
  }
}
export async function login(username: string, password: string) {
  const result = await api<{ token: string; expiresAt: string }>('/auth/login', 'POST', {
    username,
    password,
  })
  token.value = result.token
  try {
    identity.value = await api<Identity>('/me')
  } catch (error) {
    clearSession()
    throw error
  }
  sessionExpired.value = false
  expiry = setTimeout(
    () => clearSession(true),
    Math.max(0, Math.min(Date.parse(result.expiresAt) - Date.now(), 2147483647)),
  )
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
    const query = new URLSearchParams({ limit: '100', ...(after ? { after } : {}) })
    const page: Page<T> = await api(`${path}?${query}`)
    items.push(...page.items)
    if (page.nextCursor === after && after !== null) throw new ApiError('UNKNOWN')
    after = page.nextCursor
  } while (after)
  return items
}
