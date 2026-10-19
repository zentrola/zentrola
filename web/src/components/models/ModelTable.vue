<script setup lang="ts">
import { t } from '../../i18n'
import type { Model } from '../../types'
import RowActionMenu from '../RowActionMenu.vue'
import StatusSwitch from '../StatusSwitch.vue'
import TableScroll from '../TableScroll.vue'
import TechnicalValue from '../TechnicalValue.vue'

defineProps<{
  models: Model[]
  busy: boolean
  loading: boolean
  // 正在切换启用状态的模型。
  statusTargetId?: string
}>()
const emit = defineEmits<{
  priorities: [model: Model]
  changeStatus: [model: Model]
  edit: [model: Model]
  delete: [model: Model]
}>()
</script>

<template>
  <TableScroll has-actions>
    <table class="models-table">
      <colgroup>
        <col class="record-id-column" />
        <col class="model-name-column" />
        <col class="model-publisher-column" />
        <col class="model-status-column" />
        <col class="model-input-column" />
        <col class="model-output-column" />
        <col class="model-remark-column" />
        <col class="model-action-column" />
      </colgroup>
      <thead>
        <tr>
          <th>{{ t('common.id') }}</th>
          <th>{{ t('models.name') }}</th>
          <th>{{ t('models.publisher') }}</th>
          <th>{{ t('common.enableStatus') }}</th>
          <th>{{ t('models.input') }}</th>
          <th>{{ t('models.output') }}</th>
          <th>{{ t('common.remark') }}</th>
          <th class="align-right">{{ t('common.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="model in models" :key="model.id">
          <td class="record-id-cell"><TechnicalValue :value="model.id" :copyable="false" /></td>
          <td>
            <div class="person">
              <span class="avatar">{{ model.name.slice(0, 1) }}</span>
              <div>
                <strong class="model-name-regular">{{ model.name }}</strong>
                <div class="model-code-line">
                  <small>{{ model.code }}</small>
                  <button
                    type="button"
                    class="model-provider-trigger"
                    :disabled="busy"
                    :aria-label="t('models.providerPrioritiesFor', { name: model.name })"
                    @click="emit('priorities', model)"
                  >
                    {{ t('models.providerEntry') }}
                  </button>
                </div>
              </div>
            </div>
          </td>
          <td>{{ model.publisherProviderName || t('common.none') }}</td>
          <td>
            <StatusSwitch
              :value="model.status"
              :name="model.name"
              :disabled="busy || loading"
              :busy="busy && statusTargetId === model.id"
              @change="emit('changeStatus', model)"
            />
          </td>
          <td>
            <div class="modality-tags">
              <span v-for="value in model.inputModalities" :key="value" class="modality-tag">{{
                t(`models.${value}`)
              }}</span>
            </div>
          </td>
          <td>
            <div class="modality-tags">
              <span v-for="value in model.outputModalities" :key="value" class="modality-tag">{{
                t(`models.${value}`)
              }}</span>
            </div>
          </td>
          <td>
            <div class="model-remark">{{ model.remark || t('common.none') }}</div>
          </td>
          <td>
            <div class="row-actions">
              <button class="text-button" :disabled="busy" @click="emit('edit', model)">
                {{ t('models.editAction') }}
              </button>
              <RowActionMenu
                :label="t('common.moreActionsFor', { name: model.name })"
                :title="t('common.moreActions')"
              >
                <button class="text-button danger" :disabled="busy" @click="emit('delete', model)">
                  {{ t('models.delete') }}
                </button>
              </RowActionMenu>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </TableScroll>
</template>

<style scoped>
.models-table {
  min-width: 1180px;
  table-layout: fixed;
}
.model-name-column {
  width: 26%;
}
.model-publisher-column {
  width: 160px;
}
.model-status-column {
  width: 100px;
}
.model-input-column,
.model-output-column {
  width: 120px;
}
.model-remark-column {
  width: calc(74% - 730px);
}
.model-action-column {
  width: 170px;
}
.model-code-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 8px;
}
.models-table .person > div {
  min-width: 0;
}
.model-code-line small {
  min-width: 0;
  margin-top: 0;
}
.model-provider-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  padding: 2px 8px;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text-secondary);
  font: inherit;
  font-size: 11px;
  line-height: 1.5;
  cursor: pointer;
}
.model-provider-trigger:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}
.model-provider-trigger:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.model-provider-trigger:disabled {
  opacity: 0.55;
  cursor: default;
}
.modality-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.modality-tag {
  display: inline-block;
  padding: 3px 7px;
  border-radius: 6px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  font-size: 11px;
  white-space: nowrap;
}
.model-remark {
  min-width: 100px;
  max-width: 240px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
</style>
