<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { all, api, errorText } from '../api'
import { useCollection, useAction, useListSearch, date, dateOnly, validText } from '../composables'
import { t } from '../i18n'
import type { Member, Group, AccessKey, CreatedKey, Page } from '../types'
import Icon from '../components/Icon.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import MemberKeys from '../components/MemberKeys.vue'
const { items, cursor, loading, error, load } = useCollection<Member>(() => '/members')
const { busy, error: actionError, run } = useAction()
const notice = ref(''),
  creating = ref(false),
  editing = ref<Member | null>(null),
  name = ref(''),
  remark = ref(''),
  validation = ref(''),
  groupCandidates = ref<Group[]>([]),
  selectedGroupIDs = ref<string[]>([]),
  originalGroupIDs = ref<string[]>([]),
  groupsReady = ref(false)
const statusTarget = ref<Member | null>(null),
  selected = ref<Member | null>(null),
  viewingKeys = ref<Member | null>(null),
  deleteTarget = ref<Member | null>(null),
  memberKeys = ref<Record<string, AccessKey[]>>({}),
  keyErrors = ref<Record<string, string>>({}),
  keysLoading = ref<Record<string, boolean>>({})
const keyName = ref(''),
  expires = ref(''),
  createdKey = ref<CreatedKey | null>(null),
  copied = ref('')
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (m) => `${m.name} ${m.id} ${m.remark || ''}`,
  load,
)
const originalGroupIDSet = computed(() => new Set(originalGroupIDs.value))
watch(items, (members) => {
  for (const member of members) void refreshKeys(member.id)
})
onMounted(() => load())
function newMember() {
  editing.value = null
  name.value = ''
  remark.value = ''
  validation.value = ''
  groupCandidates.value = []
  selectedGroupIDs.value = []
  originalGroupIDs.value = []
  groupsReady.value = false
  actionError.value = ''
  creating.value = true
  void run(loadMemberGroups)
}
async function loadMemberGroups() {
  groupsReady.value = false
  const member = editing.value
  const [groups, current] = await Promise.all([
    all<Group>('/groups?status=ACTIVE'),
    member ? all<Group>(`/members/${member.id}/groups`) : Promise.resolve([]),
  ])
  groupCandidates.value = groups
  const activeGroupIDs = new Set(groups.map((group) => group.id))
  originalGroupIDs.value = current
    .filter((group) => activeGroupIDs.has(group.id))
    .map((group) => group.id)
  selectedGroupIDs.value = [...originalGroupIDs.value]
  groupsReady.value = true
}
function openEdit(member: Member) {
  creating.value = false
  editing.value = member
  name.value = member.name
  remark.value = member.remark || ''
  validation.value = ''
  groupCandidates.value = []
  selectedGroupIDs.value = []
  originalGroupIDs.value = []
  groupsReady.value = false
  actionError.value = ''
  void run(loadMemberGroups)
}
function closeMemberForm() {
  creating.value = false
  editing.value = null
}
function saveMember() {
  validation.value = ''
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    validation.value = t('common.byteLimit')
    return
  }
  void run(async () => {
    const member = editing.value
    await api(member ? `/members/${member.id}` : '/members', member ? 'PUT' : 'POST', {
      name: name.value,
      remark: remark.value,
      groupIds: selectedGroupIDs.value,
    })
    closeMemberForm()
    notice.value = t('common.saved')
    await load()
  })
}
function openStatus(member: Member) {
  actionError.value = ''
  statusTarget.value = member
}
function statusDisabled(member: Member) {
  if (busy.value || loading.value) return true
  if (member.status === 'ACTIVE') return false
  return (
    !!keysLoading.value[member.id] ||
    !!keyErrors.value[member.id] ||
    !memberKeys.value[member.id]?.length
  )
}
function statusTitle(member: Member) {
  if (
    member.status !== 'ACTIVE' &&
    !keysLoading.value[member.id] &&
    !keyErrors.value[member.id] &&
    !memberKeys.value[member.id]?.length
  ) {
    return t('members.keyRequired')
  }
  return undefined
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
    const result = await api<Page<AccessKey>>(`/members/${memberID}/keys?limit=1`)
    memberKeys.value[memberID] = result.items
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
  // 日期按浏览器本地时区解释，选中当天仍可使用至当天结束。
  const expiresAt = expires.value ? new Date(`${expires.value}T23:59:59.999`) : null
  if (expiresAt && (!Number.isFinite(expiresAt.getTime()) || expiresAt.getTime() <= Date.now())) {
    validation.value = t('members.future')
    return
  }
  void run(async () => {
    const memberID = selected.value!.id
    createdKey.value = await api<CreatedKey>(`/members/${memberID}/keys`, 'POST', {
      name: keyName.value,
      ...(expiresAt ? { expiresAt: expiresAt.toISOString() } : {}),
    })
    copied.value = ''
    keyName.value = ''
    expires.value = ''
    selected.value = null
    notice.value = t('members.keyCreated')
    await refreshKeys(memberID)
  })
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
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :placeholder="t('members.searchPlaceholder')"
      @search="search"
      @reset="reset"
    />
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
                :disabled="statusDisabled(member)"
                :busy="busy && statusTarget?.id === member.id"
                :title="statusTitle(member)"
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
                  <div class="key-details">
                    <code :title="key.name">{{ key.prefix }}</code>
                    <button
                      class="icon-button view-keys"
                      :aria-label="t('members.viewKeys')"
                      :title="t('members.viewKeys')"
                      @click="viewingKeys = member"
                    >
                      <Icon name="eye" :size="17" />
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
                  {{ key.expiresAt ? dateOnly(key.expiresAt) : t('members.noExpiry') }}
                </div>
              </template>
              <span
                v-if="
                  keyErrors[member.id] || keysLoading[member.id] || !memberKeys[member.id]?.length
                "
                class="muted"
                >{{ t('members.none') }}</span
              >
            </td>
            <td class="remark-cell" :title="member.remark || ''">
              {{ member.remark || t('members.none') }}
            </td>
            <td>{{ date(member.createdAt) }}</td>
            <td>
              <div class="row-actions">
                <button class="text-button" :disabled="busy || loading" @click="openEdit(member)">
                  {{ t('members.edit') }}
                </button>
                <button
                  class="text-button danger"
                  :disabled="busy || loading"
                  @click="openDelete(member)"
                >
                  {{ t('members.delete') }}
                </button>
                <button
                  class="text-button"
                  :disabled="busy || loading || keysLoading[member.id]"
                  @click="openKeys(member)"
                >
                  {{ t('members.assignKey') }}
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
  <MemberKeys v-if="viewingKeys" :member="viewingKeys" @close="viewingKeys = null" />
  <Modal
    v-if="creating || editing"
    :title="editing ? t('members.editTitle', { name: editing.name }) : t('members.create')"
    :busy="busy"
    medium
    @close="closeMemberForm"
    ><form @submit.prevent="saveMember">
      <label
        >{{ t('common.name') }}<input v-model="name" required autofocus :disabled="busy" /></label
      ><label
        >{{ t('common.remark') }}<textarea v-model="remark" rows="3" :disabled="busy"></textarea>
      </label>
      <section class="member-group-field" aria-labelledby="member-groups-title">
        <div class="member-group-field-head">
          <h3 id="member-groups-title">{{ t('members.groups') }}</h3>
          <span v-if="groupsReady">{{
            t('members.groupSelectionCount', {
              count: selectedGroupIDs.length,
              total: groupCandidates.length,
            })
          }}</span>
        </div>
        <div class="member-group-list">
          <label v-for="group in groupCandidates" :key="group.id" class="member-group-option">
            <input
              v-model="selectedGroupIDs"
              type="checkbox"
              :value="group.id"
              :aria-label="t('members.groupSelection', { name: group.name })"
              :disabled="
                busy ||
                !groupsReady ||
                (!originalGroupIDSet.has(group.id) &&
                  (group.status !== 'ACTIVE' || editing?.status === 'DISABLED'))
              "
            />
            <strong>{{ group.name }}</strong>
          </label>
          <p v-if="busy && !groupsReady" class="empty-compact">{{ t('common.loading') }}</p>
          <p v-else-if="groupsReady && !groupCandidates.length" class="empty-compact">
            {{ t('members.noGroups') }}
          </p>
        </div>
      </section>
      <div v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError
        }}<button
          v-if="actionError && !groupsReady"
          type="button"
          class="text-button"
          :disabled="busy"
          @click="run(loadMemberGroups)"
        >
          {{ t('common.retry') }}
        </button>
      </div>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="closeMemberForm">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy || !groupsReady">
          {{ t(busy ? 'common.working' : editing ? 'common.save' : 'common.create') }}
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
        >{{ t('members.expires') }}<input v-model="expires" type="date" :disabled="busy"
      /></label>
      <p class="muted">{{ t('members.expiresHint') }}</p>
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
.key-details {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}
.key-expiry {
  align-items: flex-start;
}
.view-keys {
  width: 28px;
  height: 28px;
  color: var(--blue);
}
.key-error {
  max-width: 260px;
  white-space: normal;
  color: var(--danger);
}
.member-group-field {
  margin-top: 20px;
}
.member-group-field-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 7px;
}
.member-group-field-head h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}
.member-group-field-head span {
  color: var(--muted);
  font-size: 12px;
}
.member-group-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  max-height: 240px;
  overflow: auto;
  padding: 8px;
  border: 1px solid var(--line);
  border-radius: 12px;
}
.member-group-option {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 12px;
  padding: 11px 14px;
  margin: 0;
  min-width: 0;
  border: 1px solid var(--line);
  border-radius: 8px;
  cursor: pointer;
}
.member-group-option input {
  width: 18px;
  height: 18px;
  padding: 0;
  margin: 0;
  accent-color: var(--blue);
}
.member-group-option strong {
  min-width: 0;
  overflow-wrap: anywhere;
}
.member-group-list > .empty-compact {
  grid-column: 1 / -1;
}
</style>
