import type { Page } from '@playwright/test'
import type { MockContext, MockHandler } from './context'
import { handleAuth } from './handlers/auth'
import { handleBilling } from './handlers/billing'
import { handleUsage } from './handlers/usage'
import { handlePrincipals } from './handlers/principals'
import { handleCollections } from './handlers/collections'
import { handleProviders } from './handlers/providers'
import { handleCreate } from './handlers/create'
import { handleMutations } from './handlers/mutations'
import { handlePrices } from './handlers/prices'
import { handleRelations } from './handlers/relations'
import { handleResources } from './handlers/resources'
import { createMockState } from './state'

const handlers: MockHandler[] = [
  handleAuth,
  handleBilling,
  handleUsage,
  handlePrincipals,
  handleCollections,
  handleProviders,
  handleCreate,
  handleMutations,
  handlePrices,
  handleRelations,
  handleResources,
]

// 内存模拟后端：按原有顺序依次交给各资源处理器，未命中时返回 404。
export async function fixture(page: Page) {
  const state = createMockState()
  await page.route('**/api/v1/**', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      path = url.pathname.replace('/api/v1', ''),
      method = req.method()
    const body = req.postDataJSON(),
      segments = path.split('/').filter(Boolean)
    const reply = (data: unknown, status = 200, code = 'OK') =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify({ code, data, requestId: 'req_browser_fixture' }),
      })
    const pageReply = (items: any[]) => reply({ items, nextCursor: null, total: items.length })
    const ctx: MockContext = {
      route,
      req,
      url,
      path,
      method,
      body,
      segments,
      reply,
      pageReply,
      state,
    }
    for (const handle of handlers) {
      const result = handle(ctx)
      if (result) return result
    }
    return reply(null, 404, 'NOT_FOUND')
  })
  return {
    members: state.members,
    models: state.models,
    providers: state.providers,
    providerMappings: state.providerMappings,
    credentialPrices: state.credentialPrices,
    setPriceEndpointUnavailable(value: boolean) {
      state.flags.priceEndpointUnavailable = value
    },
    providerInputs: state.providerInputs,
    providerActivationInputs: state.providerActivationInputs,
    groups: state.groups,
    resources: state.resources,
    keys: state.keys,
    relationships: state.relationships,
    groupQueries: state.groupQueries,
    modelQueries: state.modelQueries,
    providerQueries: state.providerQueries,
    memberListQueries: state.memberListQueries,
    memberSuggestionQueries: state.memberSuggestionQueries,
    applicationSuggestionQueries: state.applicationSuggestionQueries,
    usageQueries: state.usageQueries,
    statisticQueries: state.statisticQueries,
    operations: state.operations,
    operationQueries: state.operationQueries,
    tokenQuotaQueries: state.tokenQuotaQueries,
    modelSyncRequests: state.modelSyncRequests,
    providerInitializeRequests: state.providerInitializeRequests,
    activeModelRows: state.activeModelRows,
    expire: () => {
      state.flags.unauthorized = true
    },
    failTest: () => {
      state.flags.failedTest = true
    },
    conflict: (on: boolean) => {
      state.flags.conflict = on
    },
  }
}

export type MockFixture = Awaited<ReturnType<typeof fixture>>
