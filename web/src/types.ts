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
  modelCount: number
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
export type ProviderNetworkScope = 'PUBLIC' | 'PRIVATE'
export interface ProviderEndpoint {
  protocolType: ProviderProtocol
  baseUrl: string
  networkScope: ProviderNetworkScope
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
  models: Model[]
}
export interface ProviderInitializeOption {
  code: string
  name: string
  website: string
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
  authAdapter: 'API_KEY' | 'OPENAI_CODEX' | 'ANTHROPIC_CLAUDE_CODE'
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
  subscriptionPrice: SubscriptionPriceSummary | null
  createdAt: string
  updatedAt: string
}
export interface SubscriptionPriceSummary {
  currency: 'CNY' | 'USD'
  periodAmount: string
  billingPeriod: 'MONTH' | 'YEAR'
  effectiveAt: string
}
export interface ModelPrice {
  id: string
  providerCredentialId: string
  providerModelId: string
  currency: 'CNY' | 'USD'
  inputPrice: string
  outputPrice: string
  cachedInputPrice: string
  effectiveAt: string
  createdAt: string
}
export interface SubscriptionPrice {
  id: string
  providerCredentialId: string
  currency: 'CNY' | 'USD'
  periodAmount: string
  billingPeriod: 'MONTH' | 'YEAR'
  effectiveAt: string
  createdAt: string
}
export interface CredentialPrices {
  modelPrices: ModelPrice[]
  subscriptionPrices: SubscriptionPrice[]
  subscriptionPrice: SubscriptionPrice | null
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
export interface RateLimitResetCredit {
  id: string
  resetType: string
  status: string
  grantedAt: string
  expiresAt: string | null
  title: string | null
  description: string | null
}
export interface RateLimitResetCredits {
  availableCount: number
  credits: RateLimitResetCredit[]
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
  providerModelMappingId?: string
  testedModelId?: string
  testedModelCode?: string
  resetCredits?: RateLimitResetCredits
}
export interface ResetCreditConsumeResult {
  outcome: 'reset' | 'alreadyRedeemed' | 'nothingToReset' | 'noCredit'
  resetCredits?: RateLimitResetCredits
}
export interface ModelSyncResult extends ConnectionResult {
  discovered: number
  created: number
  updated: number
  mapped: number
  source: 'PROVIDER'
}
export interface Usage {
  id: string
  requestId: string
  clientProtocol: string
  principalId: string
  principalName: string
  principalType: 'MEMBER' | 'APPLICATION'
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
export interface ActiveModel {
  modelName: string
  modelCode: string
  providerName: string
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
  targetName: string | null
  requestId: string | null
  result: string
  errorCode: string | null
  createdAt: string
  before: unknown
  after: unknown
}

export type BillingType = 'SUBSCRIPTION' | 'API_KEY'
export type BillingCurrency = 'CNY' | 'USD'
export type BillingDocumentType = 'CHARGE' | 'ADJUSTMENT'

export interface BillingCurrencyTotal {
  billingType: BillingType
  currency: BillingCurrency
  totalAmount: string
  allocatedAmount: string
  unallocatedAmount: string
  totalTokens: number
}
export interface BillingPrincipalTotal {
  principalId: string
  principalName: string
  principalType: 'MEMBER' | 'APPLICATION'
  billingType: BillingType
  currency: BillingCurrency
  amount: string
  tokens: number
}
export interface BillingStatistics {
  from: string
  to: string
  totals: BillingCurrencyTotal[]
  items: BillingPrincipalTotal[]
  total: number
}
export interface BillingDocument {
  id: string
  billingType: BillingType
  documentType: BillingDocumentType
  status: string
  credentialId: string
  credentialName: string
  originalDocumentId: string | null
  periodStart: string
  periodEnd: string
  totalTokens: number
  totalAmount: string
  currency: BillingCurrency
  createdAt: string
}
export interface BillingDocumentItem {
  id: string
  principalId: string
  principalName: string
  principalType: 'MEMBER' | 'APPLICATION'
  usageTokens: number
  allocationRatio: string | null
  amount: string
}
export interface BillingDocumentDetail extends BillingDocument {
  items: BillingDocumentItem[]
  original: BillingDocument | null
  adjustments: BillingDocument[]
  ratingCount: number
}
export type BillingIssueReason = 'INCOMPLETE_TOKENS' | 'MISSING_PRICE' | 'PENDING_RATING'
export interface UnratedUsage {
  usageRecordId: string
  principalId: string
  principalName: string
  principalType: 'MEMBER' | 'APPLICATION'
  credentialId: string
  credentialName: string
  providerModelId: string
  startedAt: string
  inputTokens: number | null
  cachedInputTokens: number | null
  outputTokens: number | null
  reason: BillingIssueReason
  waitingSince: string
}

export type UsageCostRatingStatus =
  | 'RATED'
  | 'SUBSCRIPTION_SHARED'
  | 'INCOMPLETE_TOKENS'
  | 'MISSING_PRICE'
  | 'PENDING_RATING'
  | 'NOT_BILLABLE'
export interface UsageCostTotal {
  currency: BillingCurrency
  amount: string
  rated: number
}
export interface UsageCostSummary {
  from: string
  to: string
  requests: number
  attempts: number
  successful: number
  inputTokens: number
  cachedInputTokens: number
  outputTokens: number
  rated: number
  shared: number
  unrated: number
  totals: UsageCostTotal[]
}
export interface UsageCost {
  id: string
  requestId: string
  attemptNo: number
  principalId: string
  principalName: string
  principalType: 'MEMBER' | 'APPLICATION'
  modelId: string
  modelName: string
  providerId: string
  providerName: string
  providerModelId: string
  resourceId: string
  resourceName: string
  clientProtocol: string
  status: string
  errorType: string | null
  startedAt: string
  completedAt: string
  latencyMs: number
  inputTokens: number | null
  cachedInputTokens: number | null
  outputTokens: number | null
  billingType: BillingType
  ratingStatus: UsageCostRatingStatus
  ratingId: string | null
  ratingRevision: number | null
  currency: BillingCurrency | null
  totalCost: string | null
  ratedAt: string | null
}
export interface UsageCostRating {
  id: string
  revision: number
  modelPriceId: string
  priceEffectiveAt: string
  inputPrice: string
  cachedInputPrice: string
  outputPrice: string
  inputCost: string
  cachedInputCost: string
  outputCost: string
  totalCost: string
  currency: BillingCurrency
  createdAt: string
}
export interface SubscriptionAllocation {
  priceId: string
  currency: BillingCurrency
  periodAmount: string
  billingPeriod: 'MONTH' | 'YEAR'
  effectiveAt: string
  documentId: string | null
  periodStart: string | null
  periodEnd: string | null
  principalAmount: string | null
}
export interface UsageCostDetail extends UsageCost {
  ratings: UsageCostRating[]
  subscription: SubscriptionAllocation | null
}
