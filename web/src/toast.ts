import { readonly, ref } from 'vue'

export type ToastMessage = {
  id: number
  message: string
  tone: 'error'
}

const currentToast = ref<ToastMessage | null>(null)
let nextToastID = 0
let dismissTimer: ReturnType<typeof setTimeout> | undefined

export function showErrorToast(message: string) {
  if (!message) return
  currentToast.value = { id: ++nextToastID, message, tone: 'error' }
  clearTimeout(dismissTimer)
  dismissTimer = setTimeout(() => dismissToast(), 6000)
}

export function dismissToast(id?: number) {
  if (id !== undefined && currentToast.value?.id !== id) return
  currentToast.value = null
  clearTimeout(dismissTimer)
  dismissTimer = undefined
}

export function useToast() {
  return { toast: readonly(currentToast), dismissToast }
}
