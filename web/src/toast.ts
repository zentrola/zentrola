import { readonly, ref } from 'vue'

export type ToastMessage = {
  id: number
  message: string
  tone: 'error' | 'success'
}

const currentToast = ref<ToastMessage | null>(null)
let nextToastID = 0
let dismissTimer: ReturnType<typeof setTimeout> | undefined

function showToast(message: string, tone: ToastMessage['tone']) {
  if (!message) return
  currentToast.value = { id: ++nextToastID, message, tone }
  clearTimeout(dismissTimer)
  dismissTimer = setTimeout(() => dismissToast(), 6000)
}

export function showErrorToast(message: string) {
  showToast(message, 'error')
}

export function showSuccessToast(message: string) {
  showToast(message, 'success')
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
