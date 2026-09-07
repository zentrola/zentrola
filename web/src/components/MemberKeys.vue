<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import type { AccessKey, Member } from '../types'
import { useCollection, useAction, dateOnly } from '../composables'
import { t } from '../i18n'
import Modal from './Modal.vue'
import ListFooter from './ListFooter.vue'
import Icon from './Icon.vue'

const props = defineProps<{ member: Member }>()
defineEmits<{ close: [] }>()
const { items, cursor, loading, error, load } = useCollection<AccessKey>(
  () => `/members/${props.member.id}/keys`,
)
const { busy, error: actionError, run } = useAction()
const revokeTarget = ref<AccessKey | null>(null),
  notice = ref(''),
  now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
})
onUnmounted(() => clearInterval(timer))
function canRevoke(key: AccessKey) {
  return (
    key.status === 'ACTIVE' &&
    !key.revokedAt &&
    (!key.expiresAt || Date.parse(key.expiresAt) > now.value)
  )
}
function openRevoke(key: AccessKey) {
  now.value = Date.now()
  if (!canRevoke(key)) return
  actionError.value = ''
  revokeTarget.value = key
}
function revoke() {
  const key = revokeTarget.value
  now.value = Date.now()
  if (!key || !canRevoke(key)) {
    revokeTarget.value = null
    return
  }
  void run(async () => {
    await api(`/access-keys/${key.id}/revoke`, 'POST')
    key.status = 'REVOKED'
    key.revokedAt = new Date().toISOString()
    revokeTarget.value = null
    notice.value = t('members.revoked')
  })
}
</script>

<template>
  <Modal
    :title="t('members.keyListTitle', { name: member.name })"
    :busy="busy"
    medium
    @close="$emit('close')"
  >
    <div class="context-note key-prefix-note" role="note">
      <Icon name="shield" :size="18" /><span>{{ t('members.prefixHint') }}</span>
    </div>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <div v-if="error" class="alert error" role="alert">
      {{ error }}
      <button class="text-button" :disabled="loading" @click="load(!!cursor)">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-if="loading && !items.length" class="muted" role="status">{{ t('common.loading') }}</p>
    <div v-if="items.length" class="table-scroll">
      <table class="key-list">
        <thead>
          <tr>
            <th>{{ t('members.keyDisplayName') }}</th>
            <th>{{ t('members.assignedKeys') }}</th>
            <th>{{ t('members.expiryDate') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="key in items" :key="key.id">
            <td class="key-display-name">{{ key.name }}</td>
            <td>
              <code>{{ key.prefix }}</code>
            </td>
            <td>{{ key.expiresAt ? dateOnly(key.expiresAt) : t('members.noExpiry') }}</td>
            <td class="align-right">
              <button
                v-if="canRevoke(key)"
                class="text-button danger"
                :disabled="busy || loading"
                @click="openRevoke(key)"
              >
                {{ t('members.revoke') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else-if="!loading && !error" class="muted">{{ t('members.noKeys') }}</p>
    <ListFooter
      v-if="!error && items.length"
      :count="items.length"
      :cursor="cursor"
      :loading="loading"
      @more="load(true)"
    />
  </Modal>
  <Modal
    v-if="revokeTarget"
    :title="t('members.revokeTitle')"
    :busy="busy"
    @close="revokeTarget = null"
  >
    <p>{{ t('members.revokeHint', { name: revokeTarget.name }) }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="revokeTarget = null">
        {{ t('common.cancel') }}
      </button>
      <button class="button danger-fill" :disabled="busy" @click="revoke">
        {{ t('members.revoke') }}
      </button>
    </footer>
  </Modal>
</template>

<style scoped>
.key-list {
  min-width: 520px;
}
.key-display-name {
  white-space: normal;
  overflow-wrap: anywhere;
  max-width: 240px;
}
.key-prefix-note {
  margin-bottom: 16px;
}
</style>
