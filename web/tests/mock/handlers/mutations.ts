import { stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 编辑、删除与配额调整
export function handleMutations(ctx: MockContext): MockResult {
  const { method, body, segments, reply } = ctx
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
  if (segments[0] === 'models' && segments.length === 2 && method === 'PUT') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const row = models.find((model) => model.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    Object.assign(row, body, {
      publisherProviderName:
        providers.find((provider) => provider.id === body.publisherProviderId)?.name ?? null,
    })
    return reply(row)
  }
  if (segments[0] === 'models' && segments[2] === 'provider-priorities') {
    const model = models.find((item) => item.id === segments[1])
    if (!model) return reply(null, 404, 'NOT_FOUND')
    const rows = [...providerMappings.values()]
      .flat()
      .filter((mapping) => mapping.modelId === model.id)
    if (method === 'PUT') {
      if (flags.conflict || body.mappings.length !== rows.length)
        return reply(null, 409, 'CONFLICT')
      for (const entry of body.mappings) {
        const row = rows.find((mapping) => mapping.id === entry.id)
        if (!row || row.updatedAt !== entry.updatedAt || entry.priority < 1 || entry.priority > 100)
          return reply(null, 409, 'CONFLICT')
      }
      for (const entry of body.mappings) {
        const row = rows.find((mapping) => mapping.id === entry.id)
        row.priority = entry.priority
        row.updatedAt = '2026-09-06T07:31:00Z'
      }
    }
    return reply(
      rows
        .map((mapping) => {
          const provider = providers.find((item) => item.id === mapping.providerId)
          return {
            ...mapping,
            providerName: provider?.name ?? '',
            providerStatus: provider?.status ?? 'DISABLED',
            endpointConfigured: Boolean(provider?.endpoints?.length),
            resourceAvailable: resources.some((resource) => resource.providerId === provider?.id),
          }
        })
        .sort((a, b) => a.priority - b.priority || Number(a.providerId) - Number(b.providerId)),
    )
  }
  if (segments[0] === 'providers' && segments.length === 2 && method === 'PUT') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const row = providers.find((provider) => provider.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    const { mappings = [], ...fields } = body
    providerInputs.push(structuredClone(body))
    const { proxyUrl: _, proxyHeaders: __, ...safeFields } = fields
    Object.assign(row, safeFields, safeProviderProxy(body))
    providerMappings.set(
      row.id,
      mappings.map((mapping: any) => ({
        ...mapping,
        id:
          (providerMappings.get(row.id) || []).find((old) => old.modelId === mapping.modelId)?.id ??
          next(),
        priority:
          (providerMappings.get(row.id) || []).find((old) => old.modelId === mapping.modelId)
            ?.priority ?? 100,
        providerId: row.id,
        createdAt: stamp,
        updatedAt: stamp,
      })),
    )
    return reply(row)
  }
  if (segments[0] === 'groups' && segments.length === 2 && method === 'PUT') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const row = groups.find((group) => group.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    const { modelIds = [], ...fields } = body
    Object.assign(row, fields)
    relationships.set(`groups/${row.id}/models`, new Set(modelIds))
    return reply(row)
  }
  if (
    (segments[0] === 'members' || segments[0] === 'groups') &&
    segments[2] === 'token-quota' &&
    segments[3] === 'add' &&
    method === 'POST'
  ) {
    const source = segments[0] === 'groups' ? groups : members
    const row = source.find((entry) => entry.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    row.monthlyTokenLimit = String(BigInt(row.monthlyTokenLimit || '0') + BigInt(body.amount))
    return reply(row)
  }
  if (
    (segments[0] === 'members' || segments[0] === 'groups') &&
    segments[2] === 'token-quota' &&
    segments.length === 3 &&
    method === 'DELETE'
  ) {
    const source = segments[0] === 'groups' ? groups : members
    const row = source.find((entry) => entry.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    row.monthlyTokenLimit = null
    return reply(row)
  }
  if (segments[0] === 'members' && segments.length === 2 && method === 'PUT') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const row = members.find((member) => member.id === segments[1])
    if (!row) return reply(null, 404, 'NOT_FOUND')
    const { groupIds = [], ...fields } = body
    Object.assign(row, fields)
    for (const group of groups) {
      const relation = relationships.get(`groups/${group.id}/members`) || new Set<string>()
      relationships.set(`groups/${group.id}/members`, relation)
      if (groupIds.includes(group.id)) relation.add(row.id)
      else relation.delete(row.id)
    }
    return reply(row)
  }
  if (segments[0] === 'members' && segments.length === 2 && method === 'DELETE') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const index = members.findIndex((member) => member.id === segments[1])
    if (index < 0) return reply(null, 404, 'NOT_FOUND')
    members.splice(index, 1)
    return reply({ deleted: true })
  }
  if (segments[0] === 'groups' && segments.length === 2 && method === 'DELETE') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const index = groups.findIndex((group) => group.id === segments[1])
    if (index < 0) return reply(null, 404, 'NOT_FOUND')
    groups.splice(index, 1)
    relationships.delete(`groups/${segments[1]}/members`)
    relationships.delete(`groups/${segments[1]}/models`)
    return reply({ deleted: true })
  }
  if (segments[0] === 'models' && segments.length === 2 && method === 'DELETE') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const index = models.findIndex((model) => model.id === segments[1])
    if (index < 0) return reply(null, 404, 'NOT_FOUND')
    models.splice(index, 1)
    for (const [key, ids] of relationships) {
      if (key.endsWith('/models')) ids.delete(segments[1])
    }
    for (const mappings of providerMappings.values()) {
      for (let mappingIndex = mappings.length - 1; mappingIndex >= 0; mappingIndex--) {
        if (mappings[mappingIndex].modelId === segments[1]) mappings.splice(mappingIndex, 1)
      }
    }
    return reply({ deleted: true })
  }
  if (segments[0] === 'providers' && segments.length === 2 && method === 'DELETE') {
    if (flags.conflict) return reply(null, 409, 'CONFLICT')
    const index = providers.findIndex((provider) => provider.id === segments[1])
    if (index < 0) return reply(null, 404, 'NOT_FOUND')
    providers.splice(index, 1)
    providerMappings.delete(segments[1])
    for (let resourceIndex = resources.length - 1; resourceIndex >= 0; resourceIndex--) {
      if (resources[resourceIndex].providerId === segments[1]) resources.splice(resourceIndex, 1)
    }
    return reply({ deleted: true })
  }
  if (segments[0] === 'resources' && segments.length === 2 && method === 'DELETE') {
    const index = resources.findIndex((resource) => resource.id === segments[1])
    if (index < 0) return reply(null, 404, 'NOT_FOUND')
    resources.splice(index, 1)
    return reply({ deleted: true })
  }
  if (segments[0] === 'resources' && segments.length === 2 && method === 'GET') {
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    const { credential: _, quotaDetails: __, ...safe } = resource
    return reply(safe)
  }
  return undefined
}
