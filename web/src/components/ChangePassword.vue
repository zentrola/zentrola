<script setup lang="ts">
import { computed, ref, nextTick } from 'vue'
import { api, ApiError, errorText } from '../api'
import { t } from '../i18n'
import { showErrorToast } from '../toast'
import Modal from './Modal.vue'

defineProps<{ username: string }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const currentPassword = ref(''),
  newPassword = ref(''),
  confirmation = ref('')
const busy = ref(false),
  submitted = ref(false)
const form = ref<HTMLFormElement>()
const errors = computed(() => {
  const characters = Array.from(newPassword.value).length
  return {
    currentPassword: currentPassword.value ? '' : t('passwordChange.currentRequired'),
    newPassword: !newPassword.value
      ? t('setup.passwordRequired')
      : characters < 6
        ? t('setup.passwordShort')
        : characters > 30
          ? t('setup.passwordLong')
          : !/^[\x21-\x7e]+$/.test(newPassword.value)
            ? t('setup.passwordCharacters')
            : newPassword.value === currentPassword.value
              ? t('passwordChange.same')
              : '',
    confirmation: !confirmation.value
      ? t('setup.confirmRequired')
      : confirmation.value !== newPassword.value
        ? t('setup.mismatch')
        : '',
  }
})
async function submit() {
  if (busy.value) return
  submitted.value = true
  const invalid = (Object.keys(errors.value) as (keyof typeof errors.value)[]).find(
    (key) => errors.value[key],
  )
  if (invalid) {
    form.value?.querySelector<HTMLInputElement>(`[name="${invalid}"]`)?.focus()
    return
  }
  busy.value = true
  try {
    await api('/auth/password', 'POST', {
      currentPassword: currentPassword.value,
      newPassword: newPassword.value,
    })
    currentPassword.value = newPassword.value = confirmation.value = ''
    emit('changed')
  } catch (e) {
    showErrorToast(errorText(e))
    if (e instanceof ApiError && e.code === 'CURRENT_PASSWORD_INCORRECT') {
      currentPassword.value = ''
      busy.value = false
      await nextTick()
      form.value?.querySelector<HTMLInputElement>('[name="currentPassword"]')?.focus()
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Modal :title="t('passwordChange.title')" :busy="busy" @close="emit('close')">
    <p class="password-hint" id="password-change-hint">{{ t('passwordChange.hint') }}</p>
    <form
      id="password-change-form"
      ref="form"
      novalidate
      aria-describedby="password-change-hint"
      @submit.prevent="submit"
    >
      <input type="text" name="username" autocomplete="username" :value="username" hidden />
      <label for="current-password"
        >{{ t('passwordChange.current') }}
        <input
          id="current-password"
          v-model="currentPassword"
          name="currentPassword"
          type="password"
          autocomplete="current-password"
          required
          autofocus
          :disabled="busy"
          :aria-invalid="submitted && !!errors.currentPassword"
          :aria-describedby="
            submitted && errors.currentPassword ? 'current-password-error' : undefined
          "
        />
      </label>
      <p
        v-if="submitted && errors.currentPassword"
        id="current-password-error"
        class="auth-field-error"
        role="alert"
      >
        {{ errors.currentPassword }}
      </p>
      <label for="new-password"
        >{{ t('passwordChange.new') }}
        <input
          id="new-password"
          v-model="newPassword"
          name="newPassword"
          type="password"
          autocomplete="new-password"
          required
          :disabled="busy"
          :aria-invalid="submitted && !!errors.newPassword"
          :aria-describedby="submitted && errors.newPassword ? 'new-password-error' : undefined"
        />
      </label>
      <p
        v-if="submitted && errors.newPassword"
        id="new-password-error"
        class="auth-field-error"
        role="alert"
      >
        {{ errors.newPassword }}
      </p>
      <label for="confirm-new-password"
        >{{ t('passwordChange.confirm') }}
        <input
          id="confirm-new-password"
          v-model="confirmation"
          name="confirmation"
          type="password"
          autocomplete="new-password"
          required
          :disabled="busy"
          :aria-invalid="submitted && !!errors.confirmation"
          :aria-describedby="
            submitted && errors.confirmation ? 'confirm-new-password-error' : undefined
          "
        />
      </label>
      <p
        v-if="submitted && errors.confirmation"
        id="confirm-new-password-error"
        class="auth-field-error"
        role="alert"
      >
        {{ errors.confirmation }}
      </p>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" form="password-change-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'passwordChange.submit') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.password-hint {
  color: var(--muted);
  margin: 0 0 20px;
  line-height: 1.7;
}
</style>
