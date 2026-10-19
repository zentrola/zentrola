import { stamp } from '../constants'
import type { MockContext, MockResult } from '../context'

// 凭证价格
export function handlePrices(ctx: MockContext): MockResult {
  const { method, body, segments, reply } = ctx
  const { next, credentialPrices, resources, flags } = ctx.state
  if (segments[0] === 'resources' && segments[2] === 'prices') {
    if (flags.priceEndpointUnavailable) return reply(null, 404, 'NOT_FOUND')
    const resource = resources.find((candidate) => candidate.id === segments[1])
    if (!resource) return reply(null, 404, 'NOT_FOUND')
    const prices = credentialPrices.get(resource.id) ?? {
      modelPrices: [],
      subscriptionPrices: [],
      subscriptionPrice: null,
    }
    if (segments.length === 3 && method === 'GET') return reply(prices)
    if (segments[3] === 'models' && segments.length === 5 && method === 'PUT') {
      const saved = {
        ...body,
        id: next(),
        providerCredentialId: resource.id,
        providerModelId: segments[4],
        effectiveAt: body.effectiveAt,
        createdAt: stamp,
      }
      prices.modelPrices = [saved, ...prices.modelPrices.filter((price) => price.id !== saved.id)]
      credentialPrices.set(resource.id, prices)
      return reply(saved)
    }
    if (segments[3] === 'models' && segments.length === 6 && method === 'PUT') {
      const index = prices.modelPrices.findIndex((price) => price.id === segments[5])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      const saved = { ...prices.modelPrices[index], ...body }
      prices.modelPrices[index] = saved
      credentialPrices.set(resource.id, prices)
      return reply(saved)
    }
    if (segments[3] === 'models' && segments.length === 6 && method === 'DELETE') {
      const before = prices.modelPrices.length
      prices.modelPrices = prices.modelPrices.filter((price) => price.id !== segments[5])
      if (prices.modelPrices.length === before) return reply(null, 404, 'NOT_FOUND')
      credentialPrices.set(resource.id, prices)
      return reply({ deleted: true })
    }
    if (segments[3] === 'subscription' && segments.length === 4 && method === 'PUT') {
      const saved = {
        ...body,
        id: next(),
        providerCredentialId: resource.id,
        createdAt: stamp,
      }
      prices.subscriptionPrices = [saved, ...prices.subscriptionPrices]
      prices.subscriptionPrice = [...prices.subscriptionPrices]
        .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
        .find((price) => price.effectiveAt <= new Date().toISOString())
      credentialPrices.set(resource.id, prices)
      return reply(saved)
    }
    if (segments[3] === 'subscription' && segments.length === 5 && method === 'PUT') {
      const index = prices.subscriptionPrices.findIndex((price) => price.id === segments[4])
      if (index < 0) return reply(null, 404, 'NOT_FOUND')
      const saved = { ...prices.subscriptionPrices[index], ...body }
      prices.subscriptionPrices[index] = saved
      prices.subscriptionPrice = [...prices.subscriptionPrices]
        .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
        .find((price) => price.effectiveAt <= new Date().toISOString())
      credentialPrices.set(resource.id, prices)
      return reply(saved)
    }
    if (segments[3] === 'subscription' && segments.length === 5 && method === 'DELETE') {
      const before = prices.subscriptionPrices.length
      prices.subscriptionPrices = prices.subscriptionPrices.filter(
        (price) => price.id !== segments[4],
      )
      if (prices.subscriptionPrices.length === before) return reply(null, 404, 'NOT_FOUND')
      prices.subscriptionPrice = [...prices.subscriptionPrices]
        .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
        .find((price) => price.effectiveAt <= new Date().toISOString())
      credentialPrices.set(resource.id, prices)
      return reply({ deleted: true })
    }
  }
  return undefined
}
