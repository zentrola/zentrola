const dayMs = 86400000

// 日期控件的值都是 UTC 自然日（YYYY-MM-DD）。
export function utcDateValue(value: Date) {
  return value.toISOString().slice(0, 10)
}

export function utcDayRange(from: string, to: string, maxDays = 366, allowSameDay = true) {
  const start = Date.parse(`${from}T00:00:00Z`)
  const lastDay = Date.parse(`${to}T00:00:00Z`)
  if (!Number.isFinite(start) || !Number.isFinite(lastDay)) return null
  // 结束日期包含当天，因此接口的 to 取结束日期次日 00:00Z（开区间）。
  const end = lastDay + dayMs
  if (end <= start || (!allowSameDay && from === to) || end - start > maxDays * dayMs) return null
  return { from: new Date(start).toISOString(), to: new Date(end).toISOString() }
}

export function utcDayRangeParams(from: string, to: string, maxDays = 366, allowSameDay = true) {
  const range = utcDayRange(from, to, maxDays, allowSameDay)
  return range ? new URLSearchParams(range) : null
}
