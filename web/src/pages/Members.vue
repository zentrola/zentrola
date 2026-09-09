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
import ConfirmDialog from '../components/ConfirmDialog.vue'
const { items, cursor, page, pageSize, total, loading, error, load, previous, retry, setPageSize } =
  useCollection<Member>(() => '/members')
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
  if (!name.value) {
    validation.value = t('common.required')
    return
  }
  if (creating.value && !selectedGroupIDs.value.length) {
    validation.value = t('members.groupRequired')
    return
  }
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
  <PageHeader name="members" />
  <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading"
      :label="t('members.member')"
      :placeholder="t('members.searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #actions>
        <div class="list-toolbar-actions">
          <button type="button" class="button primary" @click="newMember">
            <Icon name="plus" :size="18" />{{ t('members.create') }}
          </button>
        </div>
      </template>
    </ListSearch>
    <div v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </div>
    <div class="table-scroll">
      <table class="members-table">
        <thead>
          <tr>
            <th>{{ t('members.member') }}</th>
            <th>{{ t('common.status') }}</th>
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
                  <div class="member-name-row">
                    <strong>{{ member.name }}</strong>
                    <button
                      type="button"
                      class="icon-button view-keys"
                      :aria-label="t('members.viewKeys')"
                      :title="t('members.viewKeys')"
                      :disabled="keysLoading[member.id]"
                      @click="viewingKeys = member"
                    >
                      <Icon name="eye" :size="16" />
                    </button>
                  </div>
                  <small>{{ member.id }}</small>
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
                </div>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="member.status"
                :name="member.name"
                :aria-label="t('members.statusFor', { name: member.name })"
                :active-label="t('members.active')"
                :inactive-label="t('members.inactive')"
                :disabled="statusDisabled(member)"
                :busy="busy && statusTarget?.id === member.id"
                :title="statusTitle(member)"
                @change="openStatus(member)"
              />
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
    <ListFooter
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
  </section>
  <MemberKeys v-if="viewingKeys" :member="viewingKeys" @close="viewingKeys = null" />
  <Modal
    v-if="creating || editing"
    :title="editing ? t('members.editTitle', { name: editing.name }) : t('members.create')"
    :busy="busy"
    medium
    @close="closeMemberForm"
    ><form class="member-form" @submit.prevent="saveMember">
      <div class="member-form-fields">
        <div class="member-form-row">
          <label class="member-form-label required-label" for="member-name-input">{{
            t('members.member')
          }}</label>
          <div class="member-form-control">
            <input id="member-name-input" v-model="name" required autofocus :disabled="busy" />
          </div>
        </div>
        <div class="member-form-row">
          <div
            id="member-groups-title"
            class="member-form-label"
            :class="{ 'required-label': creating }"
          >
            {{ t('members.groups') }}
          </div>
          <section
            class="member-form-control member-group-field"
            aria-labelledby="member-groups-title"
            :aria-required="creating"
          >
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
        </div>
        <div class="member-form-row">
          <label class="member-form-label" for="member-remark-input">{{
            t('common.remark')
          }}</label>
          <div class="member-form-control">
            <textarea
              id="member-remark-input"
              v-model="remark"
              rows="4"
              :disabled="busy"
            ></textarea>
          </div>
        </div>
      </div>
      <div v-if="validation" class="alert error" role="alert">{{ validation }}</div>
      <div v-if="actionError && !groupsReady" class="form-retry">
        <button type="button" class="text-button" :disabled="busy" @click="run(loadMemberGroups)">
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
  <ConfirmDialog
    v-if="statusTarget"
    :title="
      t(statusTarget.status === 'ACTIVE' ? 'members.deactivateTitle' : 'members.activateTitle')
    "
    :message="
      t('common.confirmStatus', {
        name: statusTarget.name,
        status: t(statusTarget.status === 'ACTIVE' ? 'members.deactivate' : 'members.activate'),
      })
    "
    :hint="statusTarget.status === 'ACTIVE' ? t('members.deactivateHint') : undefined"
    :confirm-label="t(statusTarget.status === 'ACTIVE' ? 'members.deactivate' : 'members.activate')"
    :busy="busy"
    :tone="statusTarget.status === 'ACTIVE' ? 'warning' : 'success'"
    @close="statusTarget = null"
    @confirm="changeStatus"
  />
  <ConfirmDialog
    v-if="deleteTarget"
    :title="t('members.deleteTitle')"
    :message="t('members.deleteQuestion', { name: deleteTarget.name })"
    :hint="t('members.deleteConsequence')"
    :confirm-label="t('members.delete')"
    :busy="busy"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteMember"
  />
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
      <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
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
  min-width: 930px;
  table-layout: fixed;
}
.members-table th:nth-child(1) {
  width: 260px;
}
.members-table th:nth-child(2) {
  width: 100px;
}
.members-table th:nth-child(3) {
  width: 110px;
}
.members-table th:nth-child(4) {
  width: 164px;
}
.members-table th:nth-child(5) {
  width: 140px;
}
.members-table th:nth-child(6) {
  width: 176px;
}
.members-table th,
.members-table td {
  padding-left: 12px;
  padding-right: 12px;
}
.members-table .person > div {
  min-width: 0;
}
.members-table .person strong {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.member-name-row {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
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
.key-expiry {
  align-items: flex-start;
}
.view-keys {
  flex: 0 0 26px;
  width: 26px;
  height: 26px;
  color: var(--blue);
}
.key-error {
  max-width: 210px;
  margin-top: 5px;
  font-size: 11px;
  white-space: normal;
  color: var(--danger);
}
.member-form-fields {
  display: grid;
  gap: 18px;
}
.member-form-row {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.member-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: #485e72;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.member-form-control {
  min-width: 0;
}
.required-label::after {
  content: '*';
  margin-left: 4px;
  color: var(--danger);
}
.member-group-field {
  min-width: 0;
}
.member-group-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  max-height: 240px;
  overflow: auto;
  padding: 0;
}
.member-group-option {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  margin: 0;
  min-width: 0;
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
@media (max-width: 640px) {
  .member-form-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .member-form-label {
    padding-top: 0;
    text-align: left;
  }
}
</style>
