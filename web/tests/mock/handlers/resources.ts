import { expect } from '@playwright/test'
import { stamp } from '../constants'
import { recordClaudeQuota } from '../helpers'
import type { MockContext, MockResult } from '../context'

// 凭证导出、额度与连接测试
export function handleResources(ctx: MockContext): MockResult {
  const { route, req, url, method, body, segments, reply } = ctx
  const { models, providerMappings, resources, flags } = ctx.state
  if (
    segments[0] === 'resources' &&
    segments[2] === 'credential' &&
    segments[3] === 'export' &&
    method === 'GET'
  ) {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    if (
      resource.authType !== 'SUBSCRIPTION' ||
      resource.authAdapter !== 'OPENAI_CODEX' ||
      resource.subscriptionType !== 'PERSONAL'
    )
      return reply(null, 409, 'CREDENTIAL_EXPORT_UNSUPPORTED')
    expect(req.headers().authorization).toBe('Bearer fixture-admin-token')
    return route.fulfill({
      status: 200,
      contentType: 'application/octet-stream',
      headers: { 'Content-Disposition': 'attachment; filename="auth.json"' },
      body: resource.credential,
    })
  }
  if (segments[2] === 'credential') {
    resources.find((r) => r.id === segments[1]).credential = body.credential
    return reply({ updated: true })
  }
  if (segments[2] === 'quotas' && method === 'GET') {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    return reply(resource.quotaDetails || [])
  }
  if (segments[2] === 'rate-limit-reset-credit' && segments[3] === 'consume' && method === 'POST') {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    expect(body.idempotencyKey).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    )
    expect(body.creditId).toBe('credit-1')
    resource.quotaStatus = 'AVAILABLE'
    resource.quotaDetails = (resource.quotaDetails || []).map((quota: any) => ({
      ...quota,
      usedPercent: 0,
      status: 'AVAILABLE',
    }))
    return reply({
      outcome: 'reset',
      resetCredits: { availableCount: 0, credits: [] },
    })
  }
  if (segments[2] === 'refresh-quota' && method === 'POST') {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    if (resource.authAdapter !== 'ANTHROPIC_CLAUDE_CODE')
      return reply(null, 409, 'QUOTA_REFRESH_UNSUPPORTED')
    recordClaudeQuota(resource, 1, 7)
    return reply({
      ok: true,
      code: 'OK',
      httpStatus: 200,
      latencyMs: 90,
      testedModelCode: 'claude-haiku',
      quotas: resource.quotaDetails,
    })
  }
  if (segments[2] === 'test-connection') {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    const providerModelMappingId = url.searchParams.get('providerModelMappingId')
    const testedMapping = [...providerMappings.values()]
      .flat()
      .find((mapping) => mapping.id === providerModelMappingId)
    const testedModel = models.find((model) => model.id === testedMapping?.modelId)
    if (resource?.authAdapter === 'ANTHROPIC_CLAUDE_CODE' && !flags.failedTest)
      recordClaudeQuota(resource, 50, 56)
    if (resource?.authAdapter === 'OPENAI_CODEX' && !flags.failedTest) {
      resource.quotaStatus = 'AVAILABLE'
      resource.quotaCheckedAt = stamp
      resource.quotaResetsAt = stamp
      resource.quotaDetails = [
        {
          code: 'codex.primary',
          name: null,
          status: 'AVAILABLE',
          unit: 'PERCENT',
          limitValue: null,
          usedValue: null,
          remainingValue: null,
          usedPercent: 25,
          windowDurationSeconds: 18000,
          resetsAt: stamp,
          reachedType: null,
          observedAt: stamp,
        },
        {
          code: 'codex.secondary',
          name: null,
          status: 'AVAILABLE',
          unit: 'PERCENT',
          limitValue: null,
          usedValue: null,
          remainingValue: null,
          usedPercent: 6,
          windowDurationSeconds: 604800,
          resetsAt: stamp,
          reachedType: null,
          observedAt: stamp,
        },
        {
          code: 'codex.spark.primary',
          name: 'Codex Spark',
          status: 'AVAILABLE',
          unit: 'PERCENT',
          limitValue: null,
          usedValue: null,
          remainingValue: null,
          usedPercent: 0,
          windowDurationSeconds: 604800,
          resetsAt: stamp,
          reachedType: null,
          observedAt: stamp,
        },
      ]
    }
    return reply({
      ok: !flags.failedTest,
      code: flags.failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
      httpStatus: flags.failedTest ? 401 : 200,
      latencyMs: 140,
      providerModelMappingId: testedMapping?.id,
      testedModelId: testedModel?.id,
      testedModelCode: testedMapping?.upstreamModelCode || testedModel?.code,
      resetCredits:
        resource?.authAdapter === 'OPENAI_CODEX'
          ? {
              availableCount: 1,
              credits: [
                {
                  id: 'credit-1',
                  resetType: 'codexRateLimits',
                  status: 'available',
                  grantedAt: stamp,
                  expiresAt: stamp,
                  title: 'Rate-limit reset',
                  description: 'Reset an eligible Codex rate-limit window.',
                },
              ],
            }
          : undefined,
    })
  }
  return undefined
}
