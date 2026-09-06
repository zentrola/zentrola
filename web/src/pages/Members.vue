<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api, all, errorText } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t } from '../i18n'
import type { Member, AccessKey, CreatedKey } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
const { items, cursor, loading, error, load } = useCollection<Member>(() => '/members')
const { busy, error: actionError, run } = useAction()
const notice = ref(''),
  creating = ref(false),
  name = ref(''),
  remark = ref(''),
  validation = ref('')
const statusTarget = ref<Member | null>(null),
  selected = ref<Member | null>(null),
  deleteTarget = ref<Member | null>(null),
  memberKeys = ref<Record<string, AccessKey[]>>({}),
  keyErrors = ref<Record<string, string>>({}),
  keysLoading = ref<Record<string, boolean>>({})
const keyName = ref(''),
  expires = ref(''),
  createdKey = ref<CreatedKey | null>(null),
  revokeTarget = ref<{ member: Member; key: AccessKey } | null>(null),
  copied = ref('')
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (m) => `${m.name} ${m.id} ${m.remark || ''}`,
  load,
)
watch(items, (members) => {
  for (const member of members) void refreshKeys(member.id)
})
onMounted(() => load())
function newMember() {
  name.value = ''
  remark.value = ''
  validation.value = ''
  actionError.value = ''
  creating.value = true
}
function create() {
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    validation.value = t('common.byteLimit')
    return
  }
  void run(async () => {
    await api('/members', 'POST', {
      name: name.value,
      ...(remark.value ? { remark: remark.value } : {}),
    })
    creating.value = false
    notice.value = t('common.saved')
    await load()
  })
}
function openStatus(member: Member) {
  actionError.value = ''
  statusTarget.value = member
}
function changeStatus() {
  void run(async () => {
    const m = statusTarget.value!
    await api(`/members/${m.id}/status`, 'PATCH', {
      status: m.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
    })
    statusTarget.value = null
    notice.value = t('common.updatedOK')
    await load()
  })
}
async function refreshKeys(memberID: string) {
  if (keysLoading.value[memberID]) return
  keysLoading.value[memberID] = true
  delete keyErrors.value[memberID]
  try {
    memberKeys.value[memberID] = await all<AccessKey>(`/members/${memberID}/keys`)
  } catch (error) {
    keyErrors.value[memberID] = errorText(error)
  } finally {
    keysLoading.value[memberID] = false
  }
}
function openKeys(member: Member) {
  selected.value = member
  keyName.value = ''
  expires.value = ''
  actionError.value = ''
  validation.value = ''
}
function openDelete(member: Member) {
  actionError.value = ''
  deleteTarget.value = member
}
function deleteMember() {
  void run(async () => {
    await api(`/members/${deleteTarget.value!.id}`, 'DELETE')
    deleteTarget.value = null
    notice.value = t('members.deleted')
    await load()
  })
}
function issueKey() {
  validation.value = ''
  if (!validText(keyName.value, 128)) {
    validation.value = t('common.byteLimit')
    return
  }
  if (
    expires.value &&
    (!Number.isFinite(Date.parse(expires.value)) || Date.parse(expires.value) <= Date.now())
  ) {
    validation.value = t('members.future')
    return
  }
  void run(async () => {
    const memberID = selected.value!.id
    createdKey.value = await api<CreatedKey>(`/members/${memberID}/keys`, 'POST', {
      name: keyName.value,
      ...(expires.value ? { expiresAt: new Date(expires.value).toISOString() } : {}),
    })
    copied.value = ''
    keyName.value = ''
    expires.value = ''
    selected.value = null
    notice.value = t('members.keyCreated')
    await refreshKeys(memberID)
  })
}
function revoke() {
  void run(async () => {
    const { member, key } = revokeTarget.value!
    await api(`/access-keys/${key.id}/revoke`, 'POST')
    revokeTarget.value = null
    notice.value = t('members.revoked')
    await refreshKeys(member.id)
  })
}
function openRevoke(member: Member, key: AccessKey) {
  actionError.value = ''
  revokeTarget.value = { member, key }
}
function keyState(key: AccessKey) {
  if (key.revokedAt || key.status === 'REVOKED') return 'REVOKED'
  if (key.expiresAt && Date.parse(key.expiresAt) <= Date.now()) return 'EXPIRED'
  return key.status
}
async function copyKey() {
  try {
    await navigator.clipboard.writeText(createdKey.value!.key)
    copied.value = t('common.copied')
  } catch {
    copied.value = t('common.copyFailed')
  }
}
</script>
<template>
  <PageHeader name="members"
    ><button class="button primary" @click="newMember">
      <Icon name="plus" :size="18" />{{ t('members.create') }}
    </button></PageHeader
  >
  <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  <section class="panel">
    <ListSearch v-model="keyword" :loading="loading" @search="search" @reset="reset" />
    <div v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="load()">{{ t('common.retry') }}</button>
    </div>
    <div class="table-scroll">
      <table class="members-table">
        <thead>
          <tr>
            <th>{{ t('members.member') }}</th>
            <th>{{ t('common.status') }}</th>
            <th>{{ t('members.assignedKeys') }}</th>
            <th>{{ t('members.expiryDate') }}</th>
            <th>{{ t('common.remark') }}</th>
            <th>{{ t('common.created') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="member in visible" :key="member.id">
            <td>
              <div class="person">
                <span class="avatar">{{ member.name.slice(0, 1) }}</span>
                <div>
                  <strong>{{ member.name }}</strong
                  ><small>{{ member.id }}</small>
                </div>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="member.status"
                :name="member.name"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === member.id"
                @change="openStatus(member)"
              />
            </td>
            <td>
              <div v-if="keyErrors[member.id]" class="key-error" role="alert">
                {{ keyErrors[member.id] }}
                <button
                  class="text-button"
                  :disabled="keysLoading[member.id]"
                  @click="refreshKeys(member.id)"
                >
                  {{ t('common.retry') }}
                </button>
              </div>
              <span v-else-if="keysLoading[member.id]" class="muted">{{
                t('common.loading')
              }}</span>
              <template v-else>
                <div v-for="key in memberKeys[member.id]" :key="key.id" class="member-key">
                  <strong class="key-name" :title="key.name">{{ key.name }}</strong>
                  <div class="key-details">
                    <code>{{ key.prefix }}…</code>
                    <Status v-if="keyState(key) !== 'ACTIVE'" :value="keyState(key)" />
                    <button
                      v-if="keyState(key) !== 'REVOKED'"
                      class="text-button danger"
                      :disabled="busy"
                      @click="openRevoke(member, key)"
                    >
                      {{ t('members.revoke') }}
                    </button>
                  </div>
                </div>
                <span v-if="!memberKeys[member.id]?.length" class="muted">{{
                  t('members.noKeys')
                }}</span>
              </template>
            </td>
            <td>
              <template v-if="!keyErrors[member.id] && !keysLoading[member.id]">
                <div
                  v-for="key in memberKeys[member.id]"
                  :key="key.id"
                  class="member-key key-expiry"
                >
                  {{ key.expiresAt ? date(key.expiresAt) : t('members.noExpiry') }}
                </div>
              </template>
              <span
                v-if="
                  keyErrors[member.id] || keysLoading[member.id] || !memberKeys[member.id]?.length
                "
                class="muted"
                >{{ t('common.none') }}</span
              >
            </td>
            <td class="remark-cell" :title="member.remark || ''">
              {{ member.remark || t('common.none') }}
            </td>
            <td>{{ date(member.createdAt) }}</td>
            <td>
              <div class="row-actions">
                <button
                  class="text-button danger"
                  :disabled="busy || loading"
                  @click="openDelete(member)"
                >
                  {{ t('members.delete') }}
                </button>
                <button
                  class="text-button"
                  :disabled="
                    busy || loading || keysLoading[member.id] || member.status !== 'ACTIVE'
                  "
                  :title="member.status !== 'ACTIVE' ? t('members.disabled') : undefined"
                  @click="openKeys(member)"
                >
                  <Icon name="key" :size="15" />{{ t('members.assignKey') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="members" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'common.empty') }}</h3>
      <p v-if="!loading && !query">{{ t('members.empty') }}</p>
    </div>
    <ListFooter :count="items.length" :cursor="cursor" :loading="loading" @more="load(true)" />
  </section>
  <Modal v-if="creating" :title="t('members.create')" :busy="busy" @close="creating = false"
    ><form @submit.prevent="create">
      <label
        >{{ t('common.name') }}<input v-model="name" required autofocus :disabled="busy" /></label
      ><label
        >{{ t('common.remark') }}<textarea v-model="remark" rows="3" :disabled="busy"></textarea>
      </label>
      <div v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError }}
      </div>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="creating = false">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'common.create') }}
        </button>
      </footer>
    </form></Modal
  >
  <Modal v-if="statusTarget" :title="t('common.status')" :busy="busy" @close="statusTarget = null"
    ><p>
      {{
        t('common.confirmStatus', {
          name: statusTarget.name,
          status: t(statusTarget.status === 'ACTIVE' ? 'common.disable' : 'common.enable'),
        })
      }}
    </p>
    <p v-if="statusTarget.status === 'ACTIVE'" class="muted">{{ t('common.disableHint') }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="statusTarget = null">
        {{ t('common.cancel') }}</button
      ><button class="button primary" :disabled="busy" @click="changeStatus">
        {{ t('common.confirm') }}
      </button>
    </footer></Modal
  >
  <Modal
    v-if="deleteTarget"
    :title="t('members.deleteTitle')"
    :busy="busy"
    @close="deleteTarget = null"
  >
    <p>{{ t('members.deleteHint', { name: deleteTarget.name }) }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="deleteTarget = null">
        {{ t('common.cancel') }}
      </button>
      <button class="button danger-fill" :disabled="busy" @click="deleteMember">
        {{ t(busy ? 'common.working' : 'members.delete') }}
      </button>
    </footer>
  </Modal>
  <Modal
    v-if="selected"
    :title="t('members.keyTitle', { name: selected.name })"
    :busy="busy"
    @close="selected = null"
  >
    <p class="muted">{{ t('members.keyHint') }}</p>
    <form @submit.prevent="issueKey">
      <label
        >{{ t('members.keyName') }}<input v-model="keyName" required autofocus :disabled="busy"
      /></label>
      <label
        >{{ t('members.expires') }}<input v-model="expires" type="datetime-local" :disabled="busy"
      /></label>
      <p v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError }}
      </p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="selected = null">
          {{ t('common.cancel') }}
        </button>
        <button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'members.assignKey') }}
        </button>
      </footer>
    </form>
  </Modal>
  <Modal
    v-if="revokeTarget"
    :title="t('members.revokeTitle')"
    :busy="busy"
    @close="revokeTarget = null"
    ><p>{{ t('members.revokeHint', { name: revokeTarget.key.name }) }}</p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="revokeTarget = null">
        {{ t('common.cancel') }}</button
      ><button class="button danger-fill" :disabled="busy" @click="revoke">
        {{ t('members.revoke') }}
      </button>
    </footer></Modal
  >
  <Modal v-if="createdKey" :title="t('members.oneTime')" locked
    ><p>{{ t('members.oneTimeHint') }}</p>
    <textarea
      class="secret-output"
      :value="createdKey.key"
      :aria-label="t('members.oneTime')"
      readonly
      spellcheck="false"
      rows="3"
    ></textarea>
    <p role="status">{{ copied }}</p>
    <footer class="form-footer">
      <button class="button" @click="copyKey">{{ t('common.copy') }}</button
      ><button class="button primary" @click="createdKey = null">
        {{ t('members.acknowledged') }}
      </button>
    </footer></Modal
  >
</template>

<style scoped>
.members-table {
  min-width: 1080px;
}
.members-table th,
.members-table td {
  padding-left: 12px;
  padding-right: 12px;
}
.member-key {
  height: 64px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
}
.member-key + .member-key {
  border-top: 1px solid var(--line);
}
.key-name {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.key-details {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 11px;
}
.key-expiry {
  align-items: flex-start;
}
.key-error {
  max-width: 260px;
  white-space: normal;
  color: var(--danger);
}
</style>
