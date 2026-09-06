export interface Page<T> {
  items: T[]
  nextCursor: string | null
}
export interface Identity {
  id: string
  organizationId: string
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
  createdAt: string
  updatedAt: string
}
export type Modality = 'TEXT' | 'IMAGE' | 'AUDIO' | 'VIDEO'
export interface Provider {
  id: string
  code: string
  name: string
  status: string
  baseUrl: string
  openaiBaseUrl: string | null
}
export interface Resource {
  id: string
  name: string
  providerId: string
  status: string
  credentialConfigured: boolean
  createdAt: string
  updatedAt: string
}
export interface AccessKey {
  id: string
  name: string
  prefix: string
  status: string
  expiresAt: string | null
  revokedAt: string | null
  createdAt: string
}
export interface CreatedKey {
  id: string
  key: string
  prefix: string
  name: string
  expiresAt: string | null
}
export interface ConnectionResult {
  ok: boolean
  code: string
  httpStatus?: number
  latencyMs: number
}
export interface Usage {
  id: string
  requestId: string
  clientProtocol: string
  principalId: string
  modelId: string | null
  resourceId: string | null
  providerId: string | null
  providerModelId: string | null
  requestAt: string
  completedAt: string
  status: string
  errorType: string | null
  inputTokens: number | null
  outputTokens: number | null
  cachedInputTokens: number | null
  latencyMs: number
  usageId: string | null
  attemptNo: number | null
  attemptStatus: string | null
  attemptErrorType: string | null
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
