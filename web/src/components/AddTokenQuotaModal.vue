<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import { useAction, validText } from '../composables'
import { t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import Modal from './Modal.vue'

const props = defineProps<{ name: string; path: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const amount = ref('')
const reason = ref('')
const { busy, run } = useAction()

function validAmount(value: string) {
  if (!/^[1-9][0-9]*$/.test(value)) return false
  try {
    return BigInt(value) <= 9223372036854775807n
  } catch {
    return false
  }
}

function submit() {
  if (!validAmount(amount.value) || !validText(reason.value, 500)) {
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
        {{ t('tokenQuota.amount') }}
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
      <p class="muted">{{ t('tokenQuota.amountHint') }}</p>
      <label>
        {{ t('tokenQuota.reason') }}
        <textarea v-model.trim="reason" rows="3" required :disabled="busy"></textarea>
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
