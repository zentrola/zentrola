<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api'
import type { AccessKey, Member } from '../types'
import { useCollection, useAction, dateOnly } from '../composables'
import { t } from '../i18n'
import { showSuccessToast } from '../toast'
import Modal from './Modal.vue'
import ListFooter from './ListFooter.vue'
import Icon from './Icon.vue'
import ConfirmDialog from './ConfirmDialog.vue'
import TableScroll from './TableScroll.vue'
import TechnicalValue from './TechnicalValue.vue'

const props = defineProps<{ member: Member }>()
defineEmits<{ close: [] }>()
const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<AccessKey>(() => `/members/${props.member.id}/keys`)
const { busy, error: actionError, run } = useAction()
const revokeTarget = ref<AccessKey | null>(null),
  now = ref(Date.now())
let timer: ReturnType<typeof setTimeout> | undefined
function scheduleExpiryUpdate() {
  clearTimeout(timer)
  now.value = Date.now()
  const nextExpiry = items.value.reduce((nearest, key) => {
    if (key.status !== 'ACTIVE' || key.revokedAt || !key.expiresAt) return nearest
    const expiresAt = Date.parse(key.expiresAt)
    return expiresAt > now.value && expiresAt < nearest ? expiresAt : nearest
  }, Number.POSITIVE_INFINITY)
  if (!Number.isFinite(nextExpiry)) {
    timer = undefined
    return
  }
  timer = setTimeout(scheduleExpiryUpdate, Math.min(nextExpiry - now.value + 1, 2147483647))
}
onMounted(() => {
  void load()
})
watch(items, scheduleExpiryUpdate, { deep: true })
onUnmounted(() => clearTimeout(timer))
function canRevoke(key: AccessKey) {
  return (
    key.status === 'ACTIVE' &&
    !key.revokedAt &&
    (!key.expiresAt || Date.parse(key.expiresAt) > now.value)
  )
}
function expiryDate(key: AccessKey) {
  const expiredAt = key.revokedAt ?? key.expiresAt
  return expiredAt ? dateOnly(expiredAt) : t('members.noExpiry')
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
    showSuccessToast(t('members.revoked'))
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
    <div class="context-note masked-key-note" role="note">
      <Icon name="shield" :size="18" /><span>{{ t('members.maskedKeyHint') }}</span>
    </div>
    <div v-if="error" class="alert error" role="alert">
      {{ error }}
      <button class="text-button" :disabled="loading" @click="retry">
        {{ t('common.retry') }}
      </button>
    </div>
    <p v-if="loading && !items.length" class="muted" role="status">{{ t('common.loading') }}</p>
    <TableScroll v-if="items.length" has-actions>
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
              <TechnicalValue :value="key.maskedKey" :copyable="false" />
            </td>
            <td>{{ expiryDate(key) }}</td>
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
    </TableScroll>
    <p v-else-if="!loading && !error" class="muted">{{ t('members.noKeys') }}</p>
    <ListFooter
      v-if="!error && items.length"
      :cursor="cursor"
      :page="page"
      :page-size="pageSize"
      :total="total"
      :loading="loading"
      @first="load()"
      @previous="previous"
      @more="load(true)"
      @page-size="setPageSize"
    />
  </Modal>
  <ConfirmDialog
    v-if="revokeTarget"
    :title="t('members.revokeTitle')"
    :message="t('members.revokeQuestion', { name: revokeTarget.name })"
    :hint="t('members.revokeConsequence')"
    :confirm-label="t('members.revoke')"
    :busy="busy"
    tone="danger"
    @close="revokeTarget = null"
    @confirm="revoke"
  />
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
.masked-key-note {
  margin-bottom: 16px;
}
</style>
