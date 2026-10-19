import type { MockContext, MockResult } from '../context'

// 登录、会话与网关活跃模型
export function handleAuth(ctx: MockContext): MockResult {
  const { path, reply } = ctx
  const { models, activeModelRows, flags } = ctx.state
  if (path === '/auth/setup') return reply({ required: false })
  if (path === '/auth/login')
    return reply({
      token: 'fixture-admin-token',
      expiresAt: new Date(Date.now() + 3600000).toISOString(),
    })
  if (flags.unauthorized) return reply(null, 401, 'UNAUTHENTICATED')
  if (path === '/me') return reply({ id: '1', username: 'admin', displayName: '管理员' })
  if (path === '/auth/logout') return reply({ clearToken: true })
  if (path === '/gateway/active-models') return reply(activeModelRows)
  return undefined
}
