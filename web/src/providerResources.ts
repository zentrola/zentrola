import { onUnmounted, ref } from 'vue'
import { all, errorText, isAbortError } from './api'
import type { Resource } from './types'

// 服务商凭据列表：只接受最新一次请求的结果，避免晚到的旧响应让已删除凭据重新出现；
// 被新请求取代的调用会继续等待最新请求完成，保证调用方随后读取到最新列表。
export function useProviderResources() {
  const resources = ref<Resource[]>([])
  const error = ref('')
  let revision = 0
  let controller: AbortController | null = null
  let latest: Promise<void> = Promise.resolve()

  async function fetchResources(current: number, signal: AbortSignal) {
    error.value = ''
    try {
      const result = await all<Resource>('/resources', { signal })
      if (current === revision) resources.value = result
    } catch (reason) {
      if (current === revision && !isAbortError(reason)) error.value = errorText(reason)
    } finally {
      if (current === revision) controller = null
    }
  }
  async function load() {
    const current = ++revision
    controller?.abort()
    const next = (controller = new AbortController())
    let waiting = fetchResources(current, next.signal)
    latest = waiting
    await waiting
    while (latest !== waiting) {
      waiting = latest
      await waiting
    }
  }
  onUnmounted(() => {
    revision++
    controller?.abort()
  })
  return { resources, error, load }
}
