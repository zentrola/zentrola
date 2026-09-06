<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, all } from '../api'
import { useCollection, useAction, useListSearch, date, validText } from '../composables'
import { t } from '../i18n'
import type { Group, Member, Model } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
import ListSearch from '../components/ListSearch.vue'
const { items, cursor, loading, error, load } = useCollection<Group>(() => '/groups')
const { busy, error: actionError, run } = useAction()
const creating = ref(false),
  code = ref(''),
  name = ref(''),
  remark = ref(''),
  validation = ref('')
const selected = ref<Group | null>(null),
  tab = ref<'members' | 'models'>('members'),
  relations = ref<(Member | Model)[]>([]),
  candidates = ref<(Member | Model)[]>([]),
  addID = ref(''),
  relationReady = ref(false),
  removeTarget = ref<Member | Model | null>(null)
const { keyword, query, visible, search, reset } = useListSearch(
  items,
  (g) => `${g.name} ${g.code} ${g.id} ${g.remark || ''}`,
  load,
)
const relationSearch = useListSearch(relations, (item) => `${item.name} ${item.id}`)
const modelCandidates = computed(() =>
  candidates.value.filter((item): item is Model => 'code' in item),
)
const modelSearch = useListSearch(
  modelCandidates,
  (model) => `${model.name} ${model.code} ${model.id}`,
)
const grantedModelIDs = computed(() => new Set(relations.value.map((model) => model.id)))
const eligible = computed(() =>
  candidates.value.filter(
    (x) => x.status === 'ACTIVE' && !relations.value.some((r) => r.id === x.id),
  ),
)
onMounted(() => load())
function newGroup() {
  code.value = ''
  name.value = ''
  remark.value = ''
  validation.value = ''
  actionError.value = ''
  creating.value = true
}
function create() {
  validation.value = ''
  if (
    !validText(code.value, 64) ||
    !validText(name.value, 128) ||
    !validText(remark.value, 2000, false)
  ) {
    validation.value = t('common.byteLimit')
    return
  }
  void run(async () => {
    await api('/groups', 'POST', {
      code: code.value,
      name: name.value,
      ...(remark.value ? { remark: remark.value } : {}),
    })
    creating.value = false
    await load()
  })
}
async function refreshRelations() {
  relationReady.value = false
  const [current, available] = await Promise.all([
    all<Member | Model>(`/groups/${selected.value!.id}/${tab.value}`),
    all<Member | Model>(`/${tab.value}`),
  ])
  relations.value = current
  candidates.value = available
  relationReady.value = true
  addID.value = ''
}
function manage(group: Group, kind: 'members' | 'models') {
  relationSearch.reset()
  modelSearch.reset()
  selected.value = group
  tab.value = kind
  relations.value = []
  candidates.value = []
  actionError.value = ''
  void run(refreshRelations)
}
function switchTab(kind: 'members' | 'models') {
  if (busy.value || kind === tab.value) return
  relationSearch.reset()
  modelSearch.reset()
  tab.value = kind
  relations.value = []
  candidates.value = []
  void run(refreshRelations)
}
function toggleModel(model: Model, event: Event) {
  const checkbox = event.target as HTMLInputElement
  const allow = checkbox.checked
  const granted = grantedModelIDs.value.has(model.id)
  checkbox.checked = granted
  if (busy.value || !relationReady.value || !selected.value || allow === granted) return
  if (allow && (model.status !== 'ACTIVE' || selected.value.status !== 'ACTIVE')) return
  const groupID = selected.value.id
  void run(async () => {
    try {
      await api(`/groups/${groupID}/models/${model.id}`, allow ? 'PUT' : 'DELETE')
    } finally {
      // 包括响应丢失等失败场景，都重新读取服务器上的实际授权。
      await refreshRelations()
    }
  })
}
function add() {
  if (!addID.value) return
  void run(async () => {
    await api(`/groups/${selected.value!.id}/${tab.value}/${addID.value}`, 'PUT')
    await refreshRelations()
  })
}
function remove() {
  void run(async () => {
    await api(`/groups/${selected.value!.id}/${tab.value}/${removeTarget.value!.id}`, 'DELETE')
    removeTarget.value = null
    await refreshRelations()
  })
}
</script>
<template>
  <PageHeader name="groups"
    ><button class="button primary" @click="newGroup">
      <Icon name="plus" :size="18" />{{ t('groups.create') }}
    </button></PageHeader
  >
  <section class="panel">
    <ListSearch v-model="keyword" :loading="loading" @search="search" @reset="reset" />
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="load()">{{ t('common.retry') }}</button>
    </p>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('common.code') }}</th>
            <th>{{ t('common.status') }}</th>
            <th>{{ t('common.created') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="group in visible" :key="group.id">
            <td>
              <div class="person">
                <span class="avatar group-avatar"><Icon name="groups" :size="19" /></span>
                <div>
                  <strong>{{ group.name }}</strong
                  ><small>{{ group.remark || t('common.noRemark') }}</small>
                </div>
              </div>
            </td>
            <td>
              <code>{{ group.code }}</code>
            </td>
            <td><Status :value="group.status" /></td>
            <td>{{ date(group.createdAt) }}</td>
            <td>
              <div class="row-actions">
                <button class="text-button" @click="manage(group, 'members')">
                  {{ t('groups.members') }}</button
                ><button class="text-button" @click="manage(group, 'models')">
                  {{ t('groups.models') }}
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!visible.length" class="empty-state">
      <Icon name="groups" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : query ? 'common.noResults' : 'common.empty') }}</h3>
      <p v-if="!loading && !query">{{ t('groups.empty') }}</p>
    </div>
    <ListFooter :count="items.length" :cursor="cursor" :loading="loading" @more="load(true)" />
  </section>
  <Modal v-if="creating" :title="t('groups.create')" :busy="busy" @close="creating = false"
    ><form @submit.prevent="create">
      <label
        >{{ t('common.name') }}<input v-model="name" required :disabled="busy" autofocus /></label
      ><label
        >{{ t('common.code')
        }}<input v-model="code" required :disabled="busy" placeholder="engineering" /></label
      ><label
        >{{ t('common.remark') }}<textarea v-model="remark" rows="3" :disabled="busy"></textarea>
      </label>
      <p v-if="validation || actionError" class="alert error" role="alert">
        {{ validation || actionError }}
      </p>
      <footer class="form-footer">
        <button type="button" class="button" :disabled="busy" @click="creating = false">
          {{ t('common.cancel') }}</button
        ><button class="button primary" :disabled="busy">
          {{ t(busy ? 'common.working' : 'common.create') }}
        </button>
      </footer>
    </form></Modal
  >
  <Modal v-if="selected" :title="selected.name" :busy="busy" wide @close="selected = null"
    ><div class="tabs">
      <button
        v-for="kind in ['members', 'models'] as const"
        :key="kind"
        :class="{ selected: tab === kind }"
        :disabled="busy"
        @click="switchTab(kind)"
      >
        {{ t(`groups.${kind}`) }}
      </button>
    </div>
    <p class="muted">{{ t(tab === 'members' ? 'groups.memberHint' : 'groups.modelHint') }}</p>
    <p v-if="actionError" class="alert error" role="alert">
      {{ actionError
      }}<button class="text-button" :disabled="busy" @click="run(refreshRelations)">
        {{ t('common.retry') }}
      </button>
    </p>
    <template v-if="tab === 'members'">
      <form class="inline-create" @submit.prevent="add">
        <label class="grow"
          >{{ t(tab === 'members' ? 'groups.addMember' : 'groups.addModel')
          }}<select
            v-model="addID"
            required
            :disabled="busy || !relationReady || selected.status !== 'ACTIVE'"
          >
            <option value="">
              {{ t(eligible.length ? 'common.select' : 'groups.noEligible') }}
            </option>
            <option v-for="candidate in eligible" :key="candidate.id" :value="candidate.id">
              {{ candidate.name }} · {{ candidate.id }}
            </option>
          </select></label
        ><button class="button primary" :disabled="busy || !addID || !relationReady">
          {{ t('common.add') }}
        </button>
      </form>
      <ListSearch
        v-model="relationSearch.keyword.value"
        :loading="busy || !relationReady"
        @search="relationSearch.search"
        @reset="relationSearch.reset"
      />
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th>{{ t('common.name') }}</th>
              <th>{{ t('common.status') }}</th>
              <th class="align-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="relation in relationSearch.visible.value" :key="relation.id">
              <td>
                <strong>{{ relation.name }}</strong
                ><small class="subline">{{ relation.id }}</small>
              </td>
              <td><Status :value="relation.status" /></td>
              <td class="align-right">
                <button
                  class="text-button danger"
                  :disabled="busy"
                  @click="removeTarget = relation"
                >
                  {{ t(tab === 'members' ? 'common.remove' : 'groups.revoke') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!relationSearch.visible.value.length" class="empty-compact">
        {{
          t(
            busy
              ? 'common.loading'
              : relationSearch.query.value
                ? 'common.noResults'
                : tab === 'members'
                  ? 'groups.noMembers'
                  : 'groups.noModels',
          )
        }}
      </p>
    </template>
    <template v-else>
      <ListSearch
        v-model="modelSearch.keyword.value"
        :loading="busy || !relationReady"
        @search="modelSearch.search"
        @reset="modelSearch.reset"
      />
      <p v-if="relationReady" class="model-selection-count" role="status">
        {{ t('groups.modelCount', { count: relations.length, total: modelCandidates.length }) }}
      </p>
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th class="model-check-cell">{{ t('groups.selectModel') }}</th>
              <th>{{ t('common.name') }}</th>
              <th>{{ t('common.code') }}</th>
              <th>{{ t('common.status') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="model in modelSearch.visible.value" :key="model.id">
              <td class="model-check-cell">
                <input
                  class="model-checkbox"
                  type="checkbox"
                  :aria-label="t('groups.modelSelection', { name: model.name })"
                  :checked="grantedModelIDs.has(model.id)"
                  :disabled="
                    busy ||
                    !relationReady ||
                    (!grantedModelIDs.has(model.id) &&
                      (model.status !== 'ACTIVE' || selected.status !== 'ACTIVE'))
                  "
                  @change="toggleModel(model, $event)"
                />
              </td>
              <td>
                <strong>{{ model.name }}</strong>
              </td>
              <td>
                <code>{{ model.code }}</code>
              </td>
              <td><Status :value="model.status" /></td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="busy && !relationReady" class="empty-compact">{{ t('common.loading') }}</p>
      <p v-else-if="relationReady && !modelSearch.visible.value.length" class="empty-compact">
        {{ t(modelSearch.query.value ? 'common.noResults' : 'groups.noModelCatalog') }}
      </p>
    </template>
  </Modal>
  <Modal
    v-if="removeTarget"
    :title="t(tab === 'members' ? 'groups.removeTitle' : 'groups.revokeTitle')"
    :busy="busy"
    @close="removeTarget = null"
    ><p>
      {{
        t(tab === 'members' ? 'groups.removeHint' : 'groups.revokeHint', {
          name: removeTarget.name,
        })
      }}
    </p>
    <p v-if="actionError" class="alert error" role="alert">{{ actionError }}</p>
    <footer class="form-footer">
      <button class="button" :disabled="busy" @click="removeTarget = null">
        {{ t('common.cancel') }}</button
      ><button class="button danger-fill" :disabled="busy" @click="remove">
        {{ t('common.confirm') }}
      </button>
    </footer></Modal
  >
</template>
<style scoped>
.model-check-cell {
  width: 60px;
  text-align: center;
}
.model-checkbox {
  width: 18px;
  height: 18px;
  padding: 0;
  margin: 0;
  vertical-align: middle;
  accent-color: var(--blue);
  cursor: pointer;
}
.model-checkbox:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
.model-selection-count {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--muted);
}
</style>
