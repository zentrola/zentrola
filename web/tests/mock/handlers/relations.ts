import { expect } from '@playwright/test'
import { stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 启用、状态切换、分组关系与访问密钥
export function handleRelations(ctx: MockContext): MockResult {
  const { url, method, body, segments, reply, pageReply } = ctx
  const {
    next,
    members,
    models,
    providers,
    providerMappings,
    groups,
    resources,
    keys,
    providerActivationInputs,
    relationships,
    flags,
  } = ctx.state
  if (segments[0] === 'providers' && segments[2] === 'activate' && method === 'POST') {
    providerActivationInputs.push(structuredClone(body))
    const provider = providers.find((candidate) => candidate.id === segments[1])
    const resource = resources.find((candidate) => candidate.id === body.resourceId)
    const mapping = (providerMappings.get(segments[1]) || []).find(
      (candidate) => candidate.id === body.providerModelMappingId,
    )
    const model = models.find((candidate) => candidate.id === mapping?.modelId)
    if (
      !provider ||
      !resource ||
      resource.providerId !== provider.id ||
      (resource.authType === 'API_KEY' && (!body.protocol || !mapping || !model)) ||
      (resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE' && (body.protocol || !mapping || !model))
    )
      return reply(null, 400, 'INVALID_ARGUMENT')
    if (!flags.failedTest) provider.status = 'ACTIVE'
    return reply({
      ok: !flags.failedTest,
      code: flags.failedTest ? 'UPSTREAM_AUTH_FAILED' : 'OK',
      latencyMs: 12,
      providerModelMappingId: mapping?.id,
      testedModelId: model?.id,
      testedModelCode: mapping?.upstreamModelCode || model?.code,
    })
  }
  if (segments[2] === 'status') {
    const list =
      segments[0] === 'members'
        ? members
        : segments[0] === 'groups'
          ? groups
          : segments[0] === 'models'
            ? models
            : segments[0] === 'providers'
              ? providers
              : resources
    const row = list.find((x) => x.id === segments[1])
    if (
      segments[0] === 'members' &&
      body.status === 'ACTIVE' &&
      !keys.some((key) => key.memberID === segments[1])
    )
      return reply(null, 409, 'MEMBER_ACCESS_KEY_REQUIRED')
    row.status = body.status
    return reply(row)
  }
  if (segments[0] === 'members' && segments[2] === 'groups' && method === 'GET') {
    return pageReply(
      groups.filter((group) => relationships.get(`groups/${group.id}/members`)?.has(segments[1])),
    )
  }
  if (segments[0] === 'members' && segments[2] === 'keys') {
    if (method === 'GET') {
      const limit = Number(url.searchParams.get('limit') || '50')
      const after = url.searchParams.get('after')
      const matching = keys.filter(
        (key) =>
          key.memberID === segments[1] &&
          (url.searchParams.get('expiry') !== 'unexpired' ||
            (key.status === 'ACTIVE' &&
              !key.revokedAt &&
              (!key.expiresAt || Date.parse(key.expiresAt) > Date.now()))),
      )
      const items = matching
        .filter((key) => !after || BigInt(key.id) < BigInt(after))
        .sort((a, b) => (BigInt(a.id) > BigInt(b.id) ? -1 : 1))
        .slice(0, limit)
        .map(({ key: _, memberID: __, ...safe }) => safe)
      return reply({
        items,
        nextCursor: items.length === limit ? items.at(-1)!.id : null,
        total: matching.length,
      })
    }
    expect(members.some((member) => member.id === segments[1])).toBe(true)
    const row = {
      ...body,
      id: next(),
      memberID: segments[1],
      key: 'vk-fixture-key-shown-once',
      maskedKey: 'vk-fixture1********ture',
      status: 'ACTIVE',
      createdAt: stamp,
      revokedAt: null,
    }
    keys.push(row)
    return reply(row, 201)
  }
  if (segments[0] === 'access-keys') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const row = keys.find((k) => k.id === segments[1])
    if (segments[2] === 'management-permission' && method === 'PUT') {
      row.canManage = body.canManage
      return reply({ canManage: row.canManage })
    }
    row.status = 'REVOKED'
    row.revokedAt = stamp
    return reply({ revoked: true })
  }
  if (segments[0] === 'groups' && segments.length >= 3) {
    if (flags.conflict && (method === 'PUT' || method === 'DELETE'))
      return reply(null, 409, 'CONFLICT')
    const key = segments.slice(0, 3).join('/'),
      ids = relationships.get(key) || new Set<string>()
    relationships.set(key, ids)
    if (method === 'PUT') ids.add(segments[3])
    if (method === 'DELETE') ids.delete(segments[3])
    if (method === 'GET')
      return pageReply((segments[2] === 'members' ? members : models).filter((x) => ids.has(x.id)))
    return reply({ updated: true })
  }
  return undefined
}
