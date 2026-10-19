import type { ProviderNetworkScope } from './types'

// 服务商编辑表单的地址、代理与代理请求头校验，均为无副作用的纯函数。
export const proxySchemes = ['http', 'https', 'socks5', 'socks5h'] as const
export type ProxyScheme = (typeof proxySchemes)[number]

export type ProxyHeaderDraft = {
  key: string
  value: string
  configured: boolean
  originalKey: string
}

export const maxProxyHeaders = 32

const deniedProxyHeaders = new Set([
  'host',
  'connection',
  'content-length',
  'proxy-connection',
  'transfer-encoding',
  'upgrade',
])

export function validURL(
  value: string,
  endpoint = false,
  networkScope: ProviderNetworkScope = 'PUBLIC',
) {
  if (!value) return true
  try {
    const parsed = new URL(value)
    return (
      parsed.username === '' &&
      parsed.password === '' &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (endpoint
        ? networkScope === 'PRIVATE'
          ? ['http:', 'https:'].includes(parsed.protocol)
          : parsed.protocol === 'https:'
        : ['http:', 'https:'].includes(parsed.protocol))
    )
  } catch {
    return false
  }
}

export function normalizeURL(value: string) {
  return value.trim().replace(/\/+$/, '')
}

export function validProxyURL(value: string) {
  try {
    const parsed = new URL(value.trim())
    return (
      ['http:', 'https:', 'socks5:', 'socks5h:'].includes(parsed.protocol) &&
      Boolean(parsed.hostname) &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (parsed.pathname === '' || parsed.pathname === '/')
    )
  } catch {
    return false
  }
}

// 拆出地址中的协议前缀；重复粘贴的前缀会被逐个剥离，最后一个合法协议生效。
export function splitProxyAddress(value: string, fallback: ProxyScheme) {
  let address = value.trimStart()
  let scheme = fallback
  let match = /^(https?|socks5h?):\/\//i.exec(address)
  while (match) {
    const candidate = match[1].toLowerCase()
    if (proxySchemes.includes(candidate as ProxyScheme)) scheme = candidate as ProxyScheme
    address = address.slice(match[0].length)
    match = /^(https?|socks5h?):\/\//i.exec(address)
  }
  return { scheme, address }
}

export function proxySchemeFromURL(value: string): ProxyScheme {
  return splitProxyAddress(value, 'http').scheme
}

export function proxyCredentialSignature(value: string | null) {
  if (!value) return ''
  try {
    const parsed = new URL(value.trim())
    if (!parsed.username && !parsed.password) return ''
    return `${parsed.username}\u0000${parsed.password}`
  } catch {
    return ''
  }
}

export function proxyURLWithoutCredentials(value: string) {
  try {
    const parsed = new URL(value.trim())
    parsed.username = ''
    parsed.password = ''
    return parsed.toString().replace(/\/$/, '')
  } catch {
    return value
  }
}

export function proxyURLWithOriginalCredentials(value: string, original: string | null) {
  if (!original) return value
  try {
    const parsed = new URL(value.trim())
    const originalParsed = new URL(original)
    parsed.username = originalParsed.username
    parsed.password = originalParsed.password
    return parsed.toString().replace(/\/$/, '')
  } catch {
    return value
  }
}

export function isSocksProxyURL(value: string) {
  try {
    return ['socks5:', 'socks5h:'].includes(new URL(value.trim()).protocol)
  } catch {
    return false
  }
}

export function validProxyHeaders(headers: ProxyHeaderDraft[]) {
  if (headers.length > maxProxyHeaders) return false
  const names = new Set<string>()
  for (const header of headers) {
    const key = header.key.trim()
    const normalized = key.toLowerCase()
    const unchanged = header.configured && normalized === header.originalKey.toLowerCase()
    if (
      !/^[!#$%&'*+\-.^_`|~0-9A-Za-z]{1,128}$/.test(key) ||
      deniedProxyHeaders.has(normalized) ||
      names.has(normalized) ||
      (!header.value && !unchanged) ||
      header.value.length > 4096 ||
      /[\r\n\0]/.test(header.value)
    )
      return false
    names.add(normalized)
  }
  return true
}
