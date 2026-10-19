import type { Dashboard } from './types'

// 首页看板的统计区间与数据整形，均为无副作用的纯函数。
export type StatisticsDimension = 'member' | 'model' | 'provider'

// 当前 UTC 自然月的起止时间，to 为下月首日零点（不含）。
export function utcMonthRange(now: Date) {
  const year = now.getUTCFullYear()
  const month = now.getUTCMonth()
  return { from: new Date(Date.UTC(year, month, 1)), to: new Date(Date.UTC(year, month + 1, 1)) }
}

// 跳转到用量统计页时按 UTC 日期传入本月首日与末日。
export function statisticsRoute(month: Date, dimension: StatisticsDimension) {
  const year = month.getUTCFullYear()
  const monthIndex = month.getUTCMonth()
  const dateValue = (value: Date) => value.toISOString().slice(0, 10)
  return {
    name: 'usage',
    query: {
      view: 'statistics',
      dimension,
      from: dateValue(new Date(Date.UTC(year, monthIndex, 1))),
      to: dateValue(new Date(Date.UTC(year, monthIndex + 1, 0))),
    },
  }
}

// 兼容后端缺省字段，保证看板渲染时数值与排行列表始终可用。
export function normalizeDashboard(result: Dashboard): Dashboard {
  return {
    activeMemberCount: Number.isFinite(result.activeMemberCount) ? result.activeMemberCount : 0,
    modelCount: Number.isFinite(result.modelCount) ? result.modelCount : 0,
    providerCount: Number.isFinite(result.providerCount) ? result.providerCount : 0,
    totalTokens: Number.isFinite(result.totalTokens) ? result.totalTokens : 0,
    tokenRanking: Array.isArray(result.tokenRanking) ? result.tokenRanking : [],
    clientModelRanking: Array.isArray(result.clientModelRanking) ? result.clientModelRanking : [],
    providerRanking: Array.isArray(result.providerRanking) ? result.providerRanking : [],
  }
}
