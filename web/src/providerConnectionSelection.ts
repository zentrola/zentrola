import type { Provider, ProviderDetail, ProviderProtocol, Resource } from './types'

export function usesInferenceModel(resource: Resource): boolean {
  return resource.authType === 'API_KEY' || resource.authAdapter === 'ANTHROPIC_CLAUDE_CODE'
}

export function preferredTestProtocol(provider: Provider): ProviderProtocol | undefined {
  return (
    provider.endpoints.find((endpoint) => endpoint.protocolType === 'ANTHROPIC') ??
    provider.endpoints.find((endpoint) => endpoint.protocolType === 'OPENAI')
  )?.protocolType
}

export function testModelOptionsFor(detail: ProviderDetail) {
  return detail.mappings
    .map((mapping) => ({
      mapping,
      model: detail.models.find((model) => model.id === mapping.modelId),
    }))
    .filter((option) => option.model?.status === 'ACTIVE')
}

export function preferredTestMappingID(detail: ProviderDetail): string {
  const options = testModelOptionsFor(detail)
  const preferred =
    detail.code === 'google-gemini-official'
      ? options.find(({ mapping, model }) => {
          return (
            (mapping.upstreamModelCode || model?.code || '').toLowerCase() === 'gemini-3.6-flash'
          )
        })
      : undefined
  return preferred?.mapping.id ?? options[0]?.mapping.id ?? ''
}
