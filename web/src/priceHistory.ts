import { computed } from 'vue'

type PriceVersion = { id: string; effectiveAt: string }

// 折叠时默认展示的最近价格条数。
export const collapsedPriceCount = 3

function newestFirst(left: PriceVersion, right: PriceVersion) {
  return right.effectiveAt.localeCompare(left.effectiveAt)
}

// 价格历史按生效时间倒序；折叠时只展示最近几条，并保证当前生效的价格始终可见。
export function priceHistoryView<T extends PriceVersion>(prices: T[], expanded: boolean) {
  const history = [...prices].sort(newestFirst)
  const now = new Date().toISOString()
  const current = history.find((price) => price.effectiveAt <= now)
  if (expanded || history.length <= collapsedPriceCount)
    return { history, current, visible: history }
  const visible = history.slice(0, collapsedPriceCount)
  if (current && !visible.some((price) => price.id === current.id)) visible.push(current)
  return { history, current, visible: visible.sort(newestFirst) }
}

export function usePriceHistory<T extends PriceVersion>(
  prices: () => T[],
  expanded: () => boolean,
) {
  return computed(() => priceHistoryView(prices(), expanded()))
}
