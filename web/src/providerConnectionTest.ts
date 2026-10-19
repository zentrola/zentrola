import { reactive, ref } from 'vue'
import { api } from './api'
import {
  preferredTestMappingID,
  preferredTestProtocol,
  testModelOptionsFor,
  usesInferenceModel,
} from './providerConnectionSelection'
import type {
  ConnectionResult,
  ModelSyncResult,
  Provider,
  ProviderDetail,
  ProviderProtocol,
  RateLimitResetCredits,
  Resource,
} from './types'

export type ConnectionTestTarget = {
  provider: Provider
  resource: Resource
  protocol?: ProviderProtocol
  activation?: boolean
}

export type ConnectionSelectionTarget = {
  provider: Provider
  detail: ProviderDetail
  resourceId?: string
  mode: 'TEST' | 'ACTIVATE'
}

export type ConnectionSelection = {
  resource: Resource
  protocol?: ProviderProtocol
  providerModelMappingID?: string
}

// 服务商连接测试、启用前测试与模型同步：负责选择测试凭据 / 协议 / 模型并记录结果。
export function useProviderConnectionTest(options: {
  run: (work: () => Promise<void>) => Promise<void>
  loadResources: () => Promise<void>
  refresh: () => Promise<void>
  resourcesFor: (provider: Provider) => Resource[]
  resourceFor: (provider: Provider) => Resource | undefined
  resetCredits: Record<string, RateLimitResetCredits>
}) {
  const selectionTarget = ref<ConnectionSelectionTarget | null>(null)
  const testTarget = ref<ConnectionTestTarget | null>(null)
  const testResult = ref<ConnectionResult | null>(null)
  const syncTarget = ref<{ provider: Provider } | null>(null)
  const syncResult = ref<ModelSyncResult | null>(null)
  const verifiedAt = reactive<Record<string, string>>({})

  function recordResetCredits(resource: Resource, result: ConnectionResult) {
    if (result.resetCredits) options.resetCredits[resource.id] = result.resetCredits
  }

  async function submitConnectionTest(
    provider: Provider,
    resource: Resource,
    protocol?: ProviderProtocol,
    providerModelMappingID?: string,
  ) {
    testTarget.value = { provider, resource, protocol }
    testResult.value = null
    try {
      const query = new URLSearchParams()
      if (protocol) query.set('protocol', protocol)
      if (providerModelMappingID) query.set('providerModelMappingId', providerModelMappingID)
      const suffix = query.size ? `?${query.toString()}` : ''
      const result = await api<ConnectionResult>(
        `/resources/${resource.id}/test-connection${suffix}`,
        'POST',
      )
      recordResetCredits(resource, result)
      verifiedAt[resource.id] = new Date().toISOString()
      await options.loadResources()
      testResult.value = result
    } catch (error) {
      testTarget.value = null
      throw error
    }
  }

  async function submitProviderActivation(
    provider: Provider,
    resource: Resource,
    protocol?: ProviderProtocol,
    providerModelMappingID?: string,
  ) {
    testTarget.value = { provider, resource, protocol, activation: true }
    testResult.value = null
    let result: ConnectionResult
    try {
      result = await api<ConnectionResult>(`/providers/${provider.id}/activate`, 'POST', {
        resourceId: resource.id,
        protocol: protocol ?? '',
        providerModelMappingId: providerModelMappingID ?? '',
      })
    } catch (error) {
      testTarget.value = null
      throw error
    }
    testResult.value = result
    recordResetCredits(resource, result)
    if (result.ok) verifiedAt[resource.id] = new Date().toISOString()
    await options.loadResources()
    if (result.ok) await options.refresh()
  }

  function testConnection(
    provider: Provider,
    resource: Resource,
    protocol = resource.authType === 'API_KEY' ? preferredTestProtocol(provider) : undefined,
    providerModelMappingID?: string,
  ) {
    void options.run(() =>
      submitConnectionTest(provider, resource, protocol, providerModelMappingID),
    )
  }

  function openSelection(
    provider: Provider,
    detail: ProviderDetail,
    resource?: Resource,
    mode: 'TEST' | 'ACTIVATE' = 'TEST',
  ) {
    selectionTarget.value = {
      provider,
      detail,
      resourceId: resource?.id ?? options.resourceFor(provider)?.id,
      mode,
    }
  }

  function closeSelection() {
    selectionTarget.value = null
  }

  // 只有一个可选组合时直接测试；存在多个凭据、协议或测试模型时先让管理员选择。
  function prepareTestConnection(provider: Provider, resource?: Resource) {
    const candidates = options.resourcesFor(provider)
    if (!candidates.length) return
    void options.run(async () => {
      const detail = await api<ProviderDetail>(`/providers/${provider.id}`)
      const selectedResource = resource ?? candidates[0]
      const mappingID = usesInferenceModel(selectedResource)
        ? preferredTestMappingID(detail)
        : undefined
      const selectionRequired =
        (!resource && candidates.length > 1) ||
        (selectedResource.authType === 'API_KEY' && provider.endpoints.length > 1) ||
        (usesInferenceModel(selectedResource) && testModelOptionsFor(detail).length > 1)
      if (selectionRequired) {
        openSelection(provider, detail, resource)
        return
      }
      await submitConnectionTest(
        provider,
        selectedResource,
        selectedResource.authType === 'API_KEY' ? preferredTestProtocol(provider) : undefined,
        mappingID,
      )
    })
  }

  function openActivation(provider: Provider) {
    void options.run(async () => {
      const detail = await api<ProviderDetail>(`/providers/${provider.id}`)
      openSelection(provider, detail, undefined, 'ACTIVATE')
    })
  }

  function submitSelection(selection: ConnectionSelection) {
    if (!selectionTarget.value) return
    const { provider, mode } = selectionTarget.value
    closeSelection()
    if (mode === 'ACTIVATE')
      void options.run(() =>
        submitProviderActivation(
          provider,
          selection.resource,
          selection.protocol,
          selection.providerModelMappingID,
        ),
      )
    else
      testConnection(
        provider,
        selection.resource,
        selection.protocol,
        selection.providerModelMappingID,
      )
  }

  function syncModels(provider: Provider) {
    syncTarget.value = { provider }
    syncResult.value = null
    void options.run(async () => {
      try {
        syncResult.value = await api<ModelSyncResult>(
          `/providers/${provider.id}/sync-models`,
          'POST',
        )
      } catch (error) {
        syncTarget.value = null
        throw error
      }
    })
  }

  return {
    selectionTarget,
    testTarget,
    testResult,
    syncTarget,
    syncResult,
    verifiedAt,
    prepareTestConnection,
    openActivation,
    closeSelection,
    submitSelection,
    syncModels,
  }
}
