<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { all, api, errorText } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'
import type { Member, Group, CreatedKey, TokenQuotaStatus } from '../types'
import Icon from '../components/Icon.vue'
import StatusSwitch from '../components/StatusSwitch.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
import MemberKeys from '../components/MemberKeys.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import TableScroll from '../components/TableScroll.vue'
import TechnicalValue from '../components/TechnicalValue.vue'
import TokenQuotaUsage from '../components/TokenQuotaUsage.vue'
import AddTokenQuotaModal from '../components/AddTokenQuotaModal.vue'
import RowActionMenu from '../components/RowActionMenu.vue'
const { kind = 'member' } = defineProps<{ kind?: 'member' | 'application' }>()
const basePath = computed(() => (kind === 'application' ? '/applications' : '/members'))
const local = (key: string, values: Record<string, string | number> = {}) =>
  t(`${kind === 'application' ? 'applications' : 'members'}.${key}`, values)
const {
  items,
  cursor,
  page,
  pageSize,
  total,
  loading,
  error,
  load,
  refresh,
  previous,
  retry,
  setPageSize,
} = useCollection<Member>(() => basePath.value)
const { busy, error: actionError, run } = useAction()
const creating = ref(false),
  editing = ref<Member | null>(null),
  name = ref(''),
  remark = ref(''),
  groupCandidates = ref<Group[]>([]),
  selectedGroupIDs = ref<string[]>([]),
  originalGroupIDs = ref<string[]>([]),
  groupsReady = ref(false)
const statusTarget = ref<Member | null>(null),
  selected = ref<Member | null>(null),
  viewingKeys = ref<Member | null>(null),
  deleteTarget = ref<Member | null>(null),
  quotaTarget = ref<Member | null>(null),
  quotaRemovalTarget = ref<Member | null>(null)
const quotaStatuses = ref<Record<string, TokenQuotaStatus>>({})
let quotaRevision = 0
const keyName = ref(''),
  expires = ref(''),
  createdKey = ref<CreatedKey | null>(null)
const { keyword, query, visible, search, reset, searching, searchingAll } = useListSearch(
  items,
  (m) => `${m.name} ${m.id} ${m.remark || ''}`,
  () => basePath.value,
  loading,
)
const originalGroupIDSet = computed(() => new Set(originalGroupIDs.value))
onMounted(() => load())
watch(visible, (rows) => void loadQuotaStatuses(rows), { immediate: true })

async function loadQuotaStatuses(rows: Member[]) {
  const revision = ++quotaRevision
  const ids = rows.filter((member) => member.monthlyTokenLimit).map((member) => member.id)
  if (!ids.length) {
    quotaStatuses.value = {}
    return
  }
  try {
    const params = new URLSearchParams({ scopeType: 'PRINCIPAL', scopeIds: ids.join(',') })
    const statuses = await api<TokenQuotaStatus[]>(`/token-quotas?${params}`)
    if (revision !== quotaRevision) return
    quotaStatuses.value = Object.fromEntries(statuses.map((status) => [status.scopeId, status]))
  } catch (error) {
    if (revision === quotaRevision) showErrorToast(errorText(error))
  }
}

async function quotaSaved() {
  quotaTarget.value = null
  await refresh()
}
function removeTokenQuota() {
  if (!quotaRemovalTarget.value) return
  const memberID = quotaRemovalTarget.value.id
  void run(async () => {
    await api(`${basePath.value}/${memberID}/token-quota`, 'DELETE')
    quotaRemovalTarget.value = null
    await refresh()
    showSuccessToast(t('tokenQuota.removed'))
  })
}
function newMember() {
  editing.value = null
  name.value = ''
  remark.value = ''
  groupCandidates.value = []
  selectedGroupIDs.value = []
  originalGroupIDs.value = []
  groupsReady.value = false
  actionError.value = ''
  creating.value = false
  void run(async () => {
    await loadMemberGroups()
    if (!groupCandidates.value.length) {
      showErrorToast(local('addGroupFirst'))
      return
    }
    creating.value = true
  })
}
async function loadMemberGroups() {
  groupsReady.value = false
  const member = editing.value
  const [groups, current] = await Promise.all([
    all<Group>('/groups?status=ACTIVE'),
    member ? all<Group>(`${basePath.value}/${member.id}/groups`) : Promise.resolve([]),
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
  if (!name.value) {
    showErrorToast(t('common.required'))
    return
  }
  if (creating.value && !selectedGroupIDs.value.length) {
    showErrorToast(local('groupRequired'))
    return
  }
  if (!validText(name.value, 128) || !validText(remark.value, 2000, false)) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  void run(async () => {
    const member = editing.value
    await api(member ? `${basePath.value}/${member.id}` : basePath.value, member ? 'PUT' : 'POST', {
      name: name.value,
      remark: remark.value,
      groupIds: selectedGroupIDs.value,
    })
    closeMemberForm()
    await load()
    showSuccessToast(t('common.saved'))
  })
}
function changeStatus(member: Member) {
  actionError.value = ''
  statusTarget.value = member
  void run(async () => {
    try {
      await api(`${basePath.value}/${member.id}/status`, 'PATCH', {
        status: member.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE',
      })
      await refresh()
      showSuccessToast(t('common.updatedOK'))
    } finally {
      statusTarget.value = null
    }
  })
}
function openKeys(member: Member) {
  selected.value = member
  keyName.value = ''
  expires.value = ''
  actionError.value = ''
}
function openDelete(member: Member) {
  actionError.value = ''
  deleteTarget.value = member
}
function deleteMember() {
  void run(async () => {
    await api(`${basePath.value}/${deleteTarget.value!.id}`, 'DELETE')
    deleteTarget.value = null
    await load()
    showSuccessToast(local('deleted'))
  })
}
function issueKey() {
  if (!validText(keyName.value, 128)) {
    showErrorToast(t('common.byteLimit'))
    return
  }
  // 日期按浏览器本地时区解释，选中当天仍可使用至当天结束。
  const expiresAt = expires.value ? new Date(`${expires.value}T23:59:59.999`) : null
  if (expiresAt && (!Number.isFinite(expiresAt.getTime()) || expiresAt.getTime() <= Date.now())) {
    showErrorToast(local('future'))
    return
  }
  void run(async () => {
    const memberID = selected.value!.id
    createdKey.value = await api<CreatedKey>(`${basePath.value}/${memberID}/keys`, 'POST', {
      name: keyName.value,
      ...(expiresAt ? { expiresAt: expiresAt.toISOString() } : {}),
    })
    keyName.value = ''
    expires.value = ''
    selected.value = null
    showSuccessToast(local('keyCreated'))
  })
}
async function copyKey() {
  try {
    await navigator.clipboard.writeText(createdKey.value!.key)
    showSuccessToast(t('common.copied'))
  } catch {
    showErrorToast(t('common.copyFailed'))
  }
}
</script>
<template>
  <PageHeader :name="kind === 'application' ? 'applications' : 'members'" />
  <section class="panel">
    <ListSearch
      v-model="keyword"
      :loading="loading || searching"
      :label="local('member')"
      :placeholder="local('searchPlaceholder')"
      @search="search"
      @reset="reset"
    >
      <template #actions>
        <div class="list-toolbar-actions">
          <button type="button" class="button primary" @click="newMember">
            <Icon name="plus" :size="18" />{{ local('create') }}
          </button>
        </div>
      </template>
    </ListSearch>
    <div v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="retry">{{ t('common.retry') }}</button>
    </div>
    <TableScroll has-actions>
      <table class="members-table">
        <colgroup>
          <col class="record-id-column" />
          <col class="member-name-column" />
          <col class="member-status-column" />
          <col class="member-quota-column" />
          <col class="member-remark-column" />
          <col class="member-created-column" />
          <col class="member-action-column" />
        </colgroup>
        <thead>
          <tr>
            <th>{{ t('common.id') }}</th>
            <th>{{ local('member') }}</th>
            <th>{{ local('activationStatus') }}</th>
            <th>{{ t('tokenQuota.column') }}</th>
            <th>{{ t('common.remark') }}</th>
            <th>{{ t('common.created') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="member in visible" :key="member.id">
            <td class="record-id-cell"><TechnicalValue :value="member.id" :copyable="false" /></td>
            <td>
              <div class="person">
                <span class="avatar">{{ member.name.slice(0, 1) }}</span>
                <div>
                  <div class="member-name-row">
                    <strong>{{ member.name }}</strong>
                    <button
                      type="button"
                      class="icon-button view-keys"
                      :aria-label="local('viewKeys')"
                      :title="local('viewKeys')"
                      @click="viewingKeys = member"
                    >
                      <Icon name="shield" :size="16" />
                    </button>
                  </div>
                </div>
              </div>
            </td>
            <td>
              <StatusSwitch
                :value="member.status"
                :name="member.name"
                :aria-label="local('statusFor', { name: member.name })"
                :disabled="busy || loading"
                :busy="busy && statusTarget?.id === member.id"
                @change="changeStatus(member)"
              />
            </td>
            <td>
              <TokenQuotaUsage
                :limit="member.monthlyTokenLimit"
                :status="quotaStatuses[member.id]"
              />
            </td>
            <td class="remark-cell" :title="member.remark || ''">
              {{ member.remark || local('none') }}
            </td>
            <td>{{ date(member.createdAt) }}</td>
            <td>
              <div class="row-actions">
                <button class="text-button" :disabled="busy || loading" @click="openEdit(member)">
                  {{ local('edit') }}
                </button>
                <button class="text-button" :disabled="busy || loading" @click="openKeys(member)">
                  {{ local('assignKey') }}
                </button>
                <RowActionMenu
                  :label="t('common.moreActionsFor', { name: member.name })"
                  :title="t('common.moreActions')"
                >
                  <button
                    class="text-button"
                    :disabled="busy || loading"
                    @click="quotaTarget = member"
                  >
                    {{ t('tokenQuota.addAction') }}
                  </button>
                  <button
                    v-if="member.monthlyTokenLimit"
                    class="text-button"
                    :disabled="busy || loading"
                    @click="quotaRemovalTarget = member"
                  >
                    {{ t('tokenQuota.removeAction') }}
                  </button>
                  <button
                    class="text-button danger"
                    :disabled="busy || loading"
                    @click="openDelete(member)"
                  >
                    {{ local('delete') }}
                  </button>
                </RowActionMenu>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </TableScroll>
    <div v-if="!visible.length" class="empty-state">
      <Icon :name="kind === 'application' ? 'applications' : 'members'" :size="32" />
      <h3>
        {{
          t(loading || searching ? 'common.loading' : query ? 'common.noResults' : 'common.empty')
        }}
      </h3>
      <p v-if="!loading && !searching && !query">{{ local('empty') }}</p>
    </div>
    <ListFooter
      v-if="!searchingAll"
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
  <MemberKeys v-if="viewingKeys" :member="viewingKeys" :kind="kind" @close="viewingKeys = null" />
  <AddTokenQuotaModal
    v-if="quotaTarget"
    :name="quotaTarget.name"
    :path="`${basePath}/${quotaTarget.id}`"
    @close="quotaTarget = null"
    @saved="quotaSaved"
  />
  <Modal
    v-if="creating || editing"
    :title="editing ? local('editTitle', { name: editing.name }) : local('create')"
    :busy="busy"
    medium
    @close="closeMemberForm"
    ><form class="member-form" @submit.prevent="saveMember">
      <div class="member-form-fields">
        <div class="member-form-row">
          <label class="member-form-label required-label" for="member-name-input">{{
            local('member')
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
            {{ local('groups') }}
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
                  :aria-label="local('groupSelection', { name: group.name })"
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
                {{ local('noGroups') }}
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
              rows="3"
              :disabled="busy"
            ></textarea>
          </div>
        </div>
      </div>
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
    v-if="quotaRemovalTarget"
    :title="t('tokenQuota.removeTitle')"
    :message="t('tokenQuota.removeQuestion', { name: quotaRemovalTarget.name })"
    :hint="t('tokenQuota.removeHint')"
    :confirm-label="t('tokenQuota.removeAction')"
    :busy="busy"
    tone="warning"
    @close="quotaRemovalTarget = null"
    @confirm="removeTokenQuota"
  />
  <ConfirmDialog
    v-if="deleteTarget"
    :title="local('deleteTitle')"
    :message="local('deleteQuestion', { name: deleteTarget.name })"
    :hint="local('deleteConsequence')"
    :confirm-label="local('delete')"
    :busy="busy"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteMember"
  />
  <Modal
    v-if="selected"
    :title="local('keyTitle', { name: selected.name })"
    :busy="busy"
    @close="selected = null"
  >
    <p class="muted">{{ local('keyHint') }}</p>
    <form @submit.prevent="issueKey">
      <label
        >{{ local('keyName') }}<input v-model="keyName" required autofocus :disabled="busy"
      /></label>
      <label>{{ local('expires') }}<input v-model="expires" type="date" :disabled="busy" /></label>
      <p class="muted">{{ local('expiresHint') }}</p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="selected = null">
          {{ t('common.cancel') }}
        </button>
        <button class="button primary" :disabled="busy">
          {{
            busy ? t('common.working') : local(kind === 'application' ? 'issueKey' : 'assignKey')
          }}
        </button>
      </footer>
    </form>
  </Modal>
  <Modal v-if="createdKey" :title="local('oneTime')" locked
    ><p>{{ local('oneTimeHint') }}</p>
    <textarea
      class="secret-output"
      :value="createdKey.key"
      :aria-label="local('oneTime')"
      readonly
      spellcheck="false"
      rows="3"
    ></textarea>
    <footer class="form-footer">
      <button class="button" @click="copyKey">{{ t('common.copy') }}</button
      ><button class="button primary" @click="createdKey = null">
        {{ local('acknowledged') }}
      </button>
    </footer></Modal
  >
</template>

<style scoped>
.members-table {
  min-width: 1146px;
  table-layout: fixed;
}
.member-name-column {
  width: 24%;
}
.member-quota-column {
  width: 190px;
}
.member-status-column {
  width: 100px;
}
.member-remark-column {
  width: calc(76% - 686px);
}
.member-created-column {
  width: 140px;
}
.member-action-column {
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
.view-keys {
  flex: 0 0 26px;
  width: 26px;
  height: 26px;
  color: var(--blue);
}
.member-form-fields {
  display: grid;
  gap: 12px;
}
.member-form-row {
  display: grid;
  grid-template-columns: 80px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.member-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.member-form-control {
  min-width: 0;
}
.member-form-control > input,
.member-form-control > textarea {
  padding: 8px 10px;
}
.member-form-control > textarea {
  min-height: 76px;
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
  display: flex;
  flex-wrap: wrap;
  gap: 4px 24px;
  max-height: 176px;
  overflow: auto;
  padding: 0;
}
.member-group-option {
  display: flex;
  flex-direction: row;
  align-items: center;
  flex: 0 1 150px;
  gap: 8px;
  padding: 5px 0;
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
  flex-basis: 100%;
}
.member-form .form-footer {
  margin-top: 16px;
  padding-top: 14px;
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
  .member-group-option {
    flex-basis: calc(50% - 12px);
  }
}
</style>
