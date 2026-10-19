<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { api, errorText } from '../../api'
import { t } from '../../i18n'
import { providerRoutingAvailable, providerRoutingStatus, validPriority } from '../../modelForm'
import type { Model, ModelProviderMapping } from '../../types'
import Modal from '../Modal.vue'

// 父组件以模型 ID 作为 key 挂载，切换模型时重新加载优先级。
const props = defineProps<{ model: Model }>()
const emit = defineEmits<{ close: [] }>()
const priorityRows = ref<(ModelProviderMapping & { draft: string })[]>([])
const priorityLoading = ref(false)
const prioritySaving = ref(false)
const priorityError = ref('')
// 关闭弹窗或重新加载后，晚到的旧响应不再写入。
let priorityLoadVersion = 0

async function loadPriorities() {
  const version = ++priorityLoadVersion
  const model = props.model
  priorityRows.value = []
  priorityError.value = ''
  priorityLoading.value = true
  try {
    const mappings = await api<ModelProviderMapping[]>(`/models/${model.id}/provider-priorities`)
    if (version !== priorityLoadVersion) return
    priorityRows.value = mappings.map((mapping) => ({
      ...mapping,
      draft: String(mapping.priority),
    }))
  } catch (error) {
    if (version !== priorityLoadVersion) return
    priorityError.value = errorText(error)
  } finally {
    if (version === priorityLoadVersion) priorityLoading.value = false
  }
}

function closePriorities() {
  priorityLoadVersion++
  emit('close')
}

function savePriorities() {
  const model = props.model
  if (priorityRows.value.some((row) => !validPriority(row.draft))) {
    priorityError.value = t('models.priorityInvalid')
    return
  }
  prioritySaving.value = true
  priorityError.value = ''
  void api<ModelProviderMapping[]>(`/models/${model.id}/provider-priorities`, 'PUT', {
    mappings: priorityRows.value.map((row) => ({
      id: row.id,
      priority: Number(row.draft),
      updatedAt: row.updatedAt,
    })),
  })
    .then(() => {
      closePriorities()
    })
    .catch((error) => {
      priorityError.value = errorText(error)
    })
    .finally(() => {
      prioritySaving.value = false
    })
}

// 在首次渲染前进入加载状态，避免闪现“无服务商映射”提示。
void loadPriorities()
onBeforeUnmount(() => {
  priorityLoadVersion++
})
</script>

<template>
  <Modal
    :title="t('models.providerPrioritiesFor', { name: model.name })"
    :busy="prioritySaving"
    @close="closePriorities"
  >
    <p class="model-priority-hint">{{ t('models.priorityHint') }}</p>
    <div v-if="priorityError" class="alert" role="alert">
      {{ priorityError }}
      <button
        type="button"
        class="text-button"
        :disabled="priorityLoading || prioritySaving"
        @click="loadPriorities"
      >
        {{ t('models.reloadPriorities') }}
      </button>
    </div>
    <p v-if="priorityLoading">{{ t('common.loading') }}</p>
    <p v-else-if="!priorityRows.length && !priorityError">
      {{ t('models.noProviderMappings') }}
      <RouterLink to="/providers">{{ t('models.configureProviderMappings') }}</RouterLink>
    </p>
    <table v-else-if="priorityRows.length" class="model-priority-table">
      <colgroup>
        <col class="model-priority-provider-column" />
        <col class="model-priority-status-column" />
        <col class="model-priority-value-column" />
      </colgroup>
      <thead>
        <tr>
          <th scope="col">{{ t('models.provider') }}</th>
          <th scope="col">{{ t('models.routingStatus') }}</th>
          <th scope="col">{{ t('models.priority') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in priorityRows" :key="row.id">
          <td class="model-priority-provider" :title="row.providerName">
            <strong>{{ row.providerName }}</strong>
          </td>
          <td
            class="model-priority-status"
            :class="{ available: providerRoutingAvailable(row) }"
            :title="providerRoutingStatus(row)"
          >
            {{ providerRoutingStatus(row) }}
          </td>
          <td class="model-priority-value">
            <input
              v-model="row.draft"
              type="text"
              inputmode="numeric"
              pattern="[0-9]*"
              :disabled="prioritySaving || row.providerStatus !== 'ACTIVE'"
              :aria-label="t('models.priorityForProvider', { name: row.providerName })"
            />
          </td>
        </tr>
      </tbody>
    </table>
    <template #footer>
      <button type="button" class="button" :disabled="prioritySaving" @click="closePriorities">
        {{ t('common.cancel') }}
      </button>
      <button
        type="button"
        class="button primary"
        :disabled="priorityLoading || prioritySaving || !priorityRows.length"
        @click="savePriorities"
      >
        {{ t(prioritySaving ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.model-priority-hint {
  margin: 0 0 14px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.5;
}
.model-priority-table {
  table-layout: fixed;
}
.model-priority-provider-column {
  width: 44%;
}
.model-priority-status-column {
  width: 32%;
}
.model-priority-value-column {
  width: 24%;
}
.model-priority-table th {
  padding: 8px;
  border-top: 0;
  background: transparent;
}
.model-priority-table td {
  height: 56px;
  padding: 8px;
}
.model-priority-table th:first-child,
.model-priority-table td:first-child {
  padding-left: 0;
}
.model-priority-table th:last-child,
.model-priority-table td:last-child {
  padding-right: 0;
  text-align: right;
}
.model-priority-provider,
.model-priority-status {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.model-priority-provider strong {
  color: var(--color-text);
  font-size: 13px;
}
.model-priority-status {
  color: var(--color-text-secondary);
  font-size: 12px;
}
.model-priority-status.available {
  color: #047857;
}
.model-priority-value input {
  width: 64px;
  min-height: 36px;
  padding: 6px 8px;
  font-size: 14px;
  text-align: center;
}
</style>
