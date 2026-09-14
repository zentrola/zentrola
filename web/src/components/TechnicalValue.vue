<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import Icon from './Icon.vue'

const props = withDefaults(
  defineProps<{
    value: string | number | null | undefined
    copyValue?: string | number | null
    copyable?: boolean
    muted?: boolean
  }>(),
  {
    copyValue: null,
    copyable: true,
    muted: false,
  },
)

const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | undefined
const text = computed(() =>
  props.value === null || props.value === undefined || props.value === ''
    ? '-'
    : String(props.value),
)
const clipboardValue = computed(() =>
  props.copyValue === null || props.copyValue === undefined
    ? props.value === null || props.value === undefined
      ? ''
      : String(props.value)
    : String(props.copyValue),
)

async function copy() {
  if (!clipboardValue.value) return
  try {
    await navigator.clipboard.writeText(clipboardValue.value)
    copied.value = true
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => (copied.value = false), 1500)
    showSuccessToast(t('common.copied'))
  } catch {
    showErrorToast(t('common.copyFailed'))
  }
}

onBeforeUnmount(() => clearTimeout(copiedTimer))
</script>

<template>
  <span class="technical-value" :class="{ 'technical-value-muted': muted }" :title="text">
    <code class="technical-value-text">{{ text }}</code>
    <button
      v-if="copyable && clipboardValue"
      type="button"
      class="technical-value-copy"
      :aria-label="t(copied ? 'common.copied' : 'common.copy')"
      :title="t(copied ? 'common.copied' : 'common.copy')"
      @click.stop="copy"
    >
      <Icon :name="copied ? 'check' : 'copy'" :size="14" />
    </button>
  </span>
</template>

<style scoped>
.technical-value {
  display: flex;
  min-width: 0;
  max-width: 100%;
  align-items: center;
  gap: 4px;
}
.technical-value-text {
  display: block;
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: var(--color-text-secondary);
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 12px;
  line-height: 1.5;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.technical-value-muted .technical-value-text {
  color: var(--color-text-muted);
}
.technical-value-copy {
  display: inline-grid;
  width: 28px;
  height: 28px;
  flex: none;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: var(--color-text-muted);
  background: transparent;
  opacity: 0.62;
  transition:
    color 0.15s,
    background-color 0.15s,
    opacity 0.15s;
}
.technical-value:hover .technical-value-copy,
.technical-value-copy:focus-visible {
  color: var(--color-text-secondary);
  background: #f1f5f9;
  opacity: 1;
}
@media (hover: none) {
  .technical-value-copy {
    opacity: 0.72;
  }
}
</style>
