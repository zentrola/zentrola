<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import { useAction, validText } from '../composables'
import { activeLocale, t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import Modal from './Modal.vue'

const props = defineProps<{ name: string; path: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const amount = ref('')
const reason = ref('')
const { busy, run } = useAction()
const presets = [
  { value: '1000000', label: 'tokenQuota.presets.oneMillion' },
  { value: '10000000', label: 'tokenQuota.presets.tenMillion' },
  { value: '100000000', label: 'tokenQuota.presets.oneHundredMillion' },
]

function validAmount(value: string) {
  if (!/^[1-9][0-9]*$/.test(value)) return false
  try {
    return BigInt(value) <= 9223372036854775807n
  } catch {
    return false
  }
}

const formattedAmount = computed(() => {
  if (!validAmount(amount.value)) return ''
  const value = new Intl.NumberFormat(activeLocale.value, {
    notation: 'compact',
    maximumFractionDigits: 2,
  }).format(BigInt(amount.value))
  return t('tokenQuota.formattedAmount', { value })
})

function submit() {
  if (!validAmount(amount.value) || (reason.value !== '' && !validText(reason.value, 500))) {
    showErrorToast(t('tokenQuota.invalid'))
    return
  }
  void run(async () => {
    await api(`${props.path}/token-quota/add`, 'POST', {
      amount: amount.value,
      reason: reason.value,
    })
    showSuccessToast(t('tokenQuota.added'))
    emit('saved')
  })
}
</script>

<template>
  <Modal :title="t('tokenQuota.addTitle', { name })" :busy="busy" @close="emit('close')">
    <form @submit.prevent="submit">
      <label>
        <span class="quota-amount-label">
          <span>{{ t('tokenQuota.amount') }}</span>
          <strong v-if="formattedAmount">{{ formattedAmount }}</strong>
        </span>
        <input
          v-model.trim="amount"
          inputmode="numeric"
          autocomplete="off"
          required
          autofocus
          :placeholder="t('tokenQuota.amountPlaceholder')"
          :disabled="busy"
        />
      </label>
      <div class="quota-presets" role="group" :aria-label="t('tokenQuota.commonAmounts')">
        <button
          v-for="preset in presets"
          :key="preset.value"
          type="button"
          class="quota-preset"
          :class="{ selected: amount === preset.value }"
          :aria-pressed="amount === preset.value"
          :disabled="busy"
          @click="amount = preset.value"
        >
          {{ t(preset.label) }}
        </button>
      </div>
      <p class="quota-rule-hint muted">{{ t('tokenQuota.amountHint') }}</p>
      <label>
        {{ t('tokenQuota.reason') }}
        <textarea
          v-model.trim="reason"
          rows="2"
          :placeholder="t('tokenQuota.reasonPlaceholder')"
          :disabled="busy"
        ></textarea>
      </label>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'tokenQuota.addAction') }}
        </button>
      </footer>
    </form>
  </Modal>
</template>

<style scoped>
.quota-presets {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: -6px;
}
.quota-amount-label {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}
.quota-amount-label strong {
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.quota-preset {
  min-height: 34px;
  padding: 5px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #f8fafc;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  transition:
    border-color 0.15s,
    background-color 0.15s,
    color 0.15s;
}
.quota-preset:hover:not(:disabled) {
  border-color: #9fb8dc;
  background: #f3f7fd;
  color: var(--color-primary);
}
.quota-preset.selected {
  border-color: #8eacd5;
  background: #edf4fd;
  color: var(--color-primary);
}
.quota-preset:focus-visible {
  outline: 2px solid rgb(37 99 235 / 22%);
  outline-offset: 2px;
}
.quota-rule-hint {
  margin: 8px 0 16px;
  font-size: 12px;
  line-height: 1.5;
}
</style>
