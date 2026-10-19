// 服务商页操作引导的完整步骤；页面按当前列表可用的入口过滤后展示。
export const providerGuideSteps = [
  {
    key: 'create',
    target: '#provider-guide-create',
    icon: 'providers',
    titleKey: 'providers.guideSteps.create.title',
    descriptionKey: 'providers.guideSteps.create.description',
  },
  {
    key: 'website',
    target: '#provider-guide-website',
    icon: 'website',
    titleKey: 'providers.guideSteps.website.title',
    descriptionKey: 'providers.guideSteps.website.description',
  },
  {
    key: 'credential',
    target: '#provider-guide-credential',
    icon: 'key',
    titleKey: 'providers.guideSteps.credential.title',
    descriptionKey: 'providers.guideSteps.credential.description',
  },
  {
    key: 'sync',
    target: '#provider-guide-sync',
    icon: 'refresh',
    titleKey: 'providers.guideSteps.sync.title',
    descriptionKey: 'providers.guideSteps.sync.description',
  },
  {
    key: 'test',
    target: '#provider-guide-test',
    icon: 'activity',
    titleKey: 'providers.guideSteps.test.title',
    descriptionKey: 'providers.guideSteps.test.description',
  },
  {
    key: 'edit',
    target: '#provider-guide-edit',
    icon: 'edit',
    titleKey: 'providers.guideSteps.edit.title',
    descriptionKey: 'providers.guideSteps.edit.description',
  },
] as const

export type ProviderGuideStepKey = (typeof providerGuideSteps)[number]['key']
