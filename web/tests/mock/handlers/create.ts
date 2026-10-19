import { stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 一级资源创建
export function handleCreate(ctx: MockContext): MockResult {
  const { path, method, body, segments, reply } = ctx
  const {
    next,
    members,
    models,
    providers,
    providerMappings,
    groups,
    resources,
    providerInputs,
    safeProviderProxy,
    relationships,
    flags,
  } = ctx.state
  if (method === 'POST' && segments.length === 1) {
    const id = next()
    const { modelIds = [], groupIds = [], mappings = [], ...fields } = body
    const subscriptionAccountRef =
      path === '/resources' &&
      body.authType === 'SUBSCRIPTION' &&
      body.authAdapter === 'OPENAI_CODEX'
        ? JSON.parse(body.credential).tokens.account_id
        : null
    const row = {
      ...fields,
      id,
      name: subscriptionAccountRef
        ? `${fields.name} · ${subscriptionAccountRef.slice(-6)}`
        : fields.name,
      ...(path === '/groups' ? { code: body.code || `group-${id}` } : {}),
      status:
        path === '/members' || path === '/models' || path === '/providers' ? 'DISABLED' : 'ACTIVE',
      createdAt: stamp,
      updatedAt: stamp,
      credentialConfigured: path === '/resources',
      runtimeStatus: path === '/resources' ? 'HEALTHY' : undefined,
      blockedReason: null,
      blockedAt: null,
      lastErrorAt: null,
      lastHttpStatus: null,
      lastErrorCode: null,
      modelSyncSupported: path === '/providers' ? false : undefined,
      authAdapters: path === '/providers' ? ['API_KEY'] : undefined,
      publisherProviderName:
        path === '/models'
          ? (providers.find((provider) => provider.id === body.publisherProviderId)?.name ?? null)
          : undefined,
      ...(path === '/resources'
        ? {
            authType: body.authType || 'API_KEY',
            authAdapter: body.authAdapter || 'API_KEY',
            subscriptionType: body.authType === 'SUBSCRIPTION' ? 'PERSONAL' : null,
            planCode: body.authType === 'SUBSCRIPTION' ? 'plus' : null,
            externalAccountRef: subscriptionAccountRef,
            priority: body.priority ?? 100,
            effectiveAt: body.effectiveAt ?? null,
            expiresAt: body.expiresAt ?? null,
            quotaStatus: 'UNKNOWN',
            quotaCheckedAt: null,
            quotaResetsAt: null,
          }
        : {}),
    }
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const target =
      path === '/members'
        ? members
        : path === '/groups'
          ? groups
          : path === '/models'
            ? models
            : path === '/providers'
              ? providers
              : resources
    if (
      path === '/resources' &&
      row.authType === 'SUBSCRIPTION' &&
      row.externalAccountRef &&
      resources.some(
        (resource) =>
          resource.providerId === row.providerId &&
          resource.authAdapter === row.authAdapter &&
          resource.externalAccountRef === row.externalAccountRef,
      )
    )
      return reply(null, 409, 'SUBSCRIPTION_ACCOUNT_ALREADY_EXISTS')
    target.push(row)
    if (path === '/groups') {
      relationships.set(`groups/${id}/models`, new Set(modelIds))
    }
    if (path === '/providers') {
      providerInputs.push(structuredClone(body))
      Object.assign(row, safeProviderProxy(body))
      providerMappings.set(
        id,
        mappings.map((mapping: any) => ({
          ...mapping,
          priority: 100,
          id: next(),
          providerId: id,
          createdAt: stamp,
          updatedAt: stamp,
        })),
      )
    }
    if (path === '/members') {
      for (const group of groups) {
        const relation = relationships.get(`groups/${group.id}/members`) || new Set<string>()
        relationships.set(`groups/${group.id}/members`, relation)
        if (groupIds.includes(group.id)) relation.add(id)
      }
    }
    const { credential: _, ...safe } = row
    return reply(safe, 201)
  }
  return undefined
}
