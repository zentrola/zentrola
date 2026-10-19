import { gatewayBaseUrl } from './api'

// 首页接入地址与客户端环境变量脚本。
export type AccessProtocol = 'openai' | 'anthropic'
export type SetupMethod = 'script' | 'ccswitch'
export type SetupPlatform = 'unix' | 'windows'

export const codexUrl = `${gatewayBaseUrl}/v1`
export const claudeUrl = `${gatewayBaseUrl}/anthropic`

// 脚本通过隐藏输入读取访问密钥，避免密钥出现在命令历史中。
export function setupScriptsFor(protocol: AccessProtocol): Record<SetupPlatform, string> {
  const setupUrl = protocol === 'anthropic' ? claudeUrl : codexUrl
  const baseVariable = protocol === 'anthropic' ? 'ANTHROPIC_BASE_URL' : 'OPENAI_BASE_URL'
  const keyVariable = protocol === 'anthropic' ? 'ANTHROPIC_API_KEY' : 'OPENAI_API_KEY'
  return {
    unix: `printf 'Zentrola Access Key: '\nread -r -s ZENTROLA_KEY\nprintf '\\n'\nexport ${baseVariable}='${setupUrl}'\nexport ${keyVariable}="$ZENTROLA_KEY"\nunset ZENTROLA_KEY`,
    windows: `$secureKey = Read-Host 'Zentrola Access Key' -AsSecureString\n$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password\n$env:${baseVariable} = '${setupUrl}'\n$env:${keyVariable} = $zentrolaKey\nRemove-Variable secureKey, zentrolaKey`,
  }
}
