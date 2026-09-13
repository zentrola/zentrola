export interface Page<T> {
  items: T[]
  nextCursor: string | null
  total: number
}
export interface Identity {
  id: string
  username: string
  displayName: string
}
export interface Member {
  id: string
  name: string
  remark: string | null
  status: string
  createdAt: string
}
export interface Group extends Member {
  code: string
}
export interface Model {
  id: string
  code: string
  name: string
  status: string
  inputModalities: Modality[]
  outputModalities: Modality[]
  remark: string
  publisherProviderId: string | null
  publisherProviderName: string | null
  createdAt: string
  updatedAt: string
}
export type Modality = 'TEXT' | 'IMAGE' | 'AUDIO' | 'VIDEO'
export interface Provider {
  id: string
  code: string
  name: string
  type: 'OFFICIAL' | 'PLATFORM' | 'PARTNER' | 'CUSTOM'
  status: string
  website: string | null
  endpoints: ProviderEndpoint[]
  modelSyncSupported: boolean
  authAdapters: string[]
  proxyEnabled: boolean
  proxyUrl: string | null
  proxyHeaders: ProviderProxyHeader[]
  createdAt: string
  updatedAt: string
}
export type ProviderProtocol = 'OPENAI' | 'ANTHROPIC'
export interface ProviderEndpoint {
  protocolType: ProviderProtocol
  baseUrl: string
}
export interface ProviderProxyHeader {
  key: string
  configured: boolean
}
export interface ProviderMapping {
  id: string
  providerId: string
  modelId: string
  upstreamModelCode: string
  priority: number
  createdAt: string
  updatedAt: string
}
export interface ProviderDetail extends Provider {
  mappings: ProviderMapping[]
}
export interface ProviderInitializeResult {
  total: number
  created: number
  updated: number
  existing: number
}
export interface Resource {
  id: string
  name: string
  providerId: string
  authType: 'API_KEY' | 'SUBSCRIPTION'
  authAdapter: 'API_KEY' | 'OPENAI_CODEX'
  subscriptionType: 'PERSONAL' | 'SEAT' | null
  planCode: string | null
  externalAccountRef: string | null
  priority: number
  effectiveAt: string | null
  expiresAt: string | null
  quotaStatus: 'AVAILABLE' | 'NEAR_LIMIT' | 'EXHAUSTED' | 'UNKNOWN'
  quotaCheckedAt: string | null
  quotaResetsAt: string | null
  runtimeStatus: 'HEALTHY' | 'BLOCKED'
  blockedReason: string | null
  blockedAt: string | null
  lastErrorAt: string | null
  lastHttpStatus: number | null
  lastErrorCode: string | null
  credentialConfigured: boolean
  createdAt: string
  updatedAt: string
}
export interface ResourceQuota {
  code: string
  name: string | null
  status: 'AVAILABLE' | 'NEAR_LIMIT' | 'EXHAUSTED' | 'UNKNOWN'
  unit: string | null
  limitValue: string | null
  usedValue: string | null
  remainingValue: string | null
  usedPercent: number | null
  windowDurationSeconds: number | null
  resetsAt: string | null
  reachedType: string | null
  observedAt: string
}
export interface AccessKey {
  id: string
  name: string
  maskedKey: string
  status: string
  expiresAt: string | null
  revokedAt: string | null
  createdAt: string
}
export interface CreatedKey {
  id: string
  key: string
  maskedKey: string
  name: string
  expiresAt: string | null
}
export interface ConnectionResult {
  ok: boolean
  code: string
  httpStatus?: number
  latencyMs: number
}
export interface ModelSyncResult extends ConnectionResult {
  discovered: number
  created: number
  updated: number
  mapped: number
}
export interface Usage {
  id: string
  requestId: string
  clientProtocol: string
  principalId: string
  principalName: string
  modelId: string
  resourceId: string
  providerId: string
  providerModelId: string
  requestAt: string
  completedAt: string
  status: string
  errorType: string | null
  inputTokens: number | null
  outputTokens: number | null
  cachedInputTokens: number | null
  latencyMs: number
  attemptNo: number
}
export interface UsageStatistic {
  entityId: string
  name: string
  code: string
  count: number
  successful: number
  inputTokens: number
  outputTokens: number
  cachedInputTokens: number
  tokens: number
  overallTokens: number
  averageLatencyMs: number
}
export interface TokenRank {
  principalId: string
  name: string
  tokens: number
}
export interface ClientModelRank {
  modelId: string
  modelCode: string
  modelName: string
  requests: number
  tokens: number
}
export interface ProviderRank {
  providerId: string
  providerName: string
  calls: number
  tokens: number
}
export interface Dashboard {
  activeMemberCount: number
  modelCount: number
  providerCount: number
  totalTokens: number
  tokenRanking: TokenRank[]
  clientModelRanking: ClientModelRank[]
  providerRanking: ProviderRank[]
}
export interface Operation {
  id: string
  operatorName: string
  type: string
  targetType: string
  targetId: string | null
  requestId: string | null
  result: string
  errorCode: string | null
  createdAt: string
  before: unknown
  after: unknown
}
