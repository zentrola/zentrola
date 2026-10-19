import type { Request, Route } from '@playwright/test'
import type { MockState } from './state'

export type MockContext = {
  route: Route
  req: Request
  url: URL
  path: string
  method: string
  body: any
  segments: string[]
  reply: (data: unknown, status?: number, code?: string) => Promise<void>
  pageReply: (items: any[]) => Promise<void>
  state: MockState
}

// 处理器返回 undefined 表示未命中，交给下一个处理器；顺序与原单体路由保持一致。
export type MockResult = Promise<void> | undefined
export type MockHandler = (ctx: MockContext) => MockResult
