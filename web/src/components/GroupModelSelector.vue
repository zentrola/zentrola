<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { t } from '../i18n'
import type { Model } from '../types'
import Icon from './Icon.vue'

const props = defineProps<{
  models: Model[]
  modelIds: string[]
  selectableModelIds: string[]
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelIds': [value: string[]] }>()

const query = ref('')
const catalogTitleID = useId()
const selectedTitleID = useId()
const selectedIDSet = computed(() => new Set(props.modelIds))
const selectableIDSet = computed(() => new Set(props.selectableModelIds))
const filteredModels = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  if (!keyword) return props.models
  return props.models.filter((model) =>
    [model.name, model.code, model.publisherProviderName || ''].some((value) =>
      value.toLocaleLowerCase().includes(keyword),
    ),
  )
})
const filteredSelectableModels = computed(() =>
  filteredModels.value.filter((model) => selectableIDSet.value.has(model.id)),
)
const selectedModels = computed(() =>
  props.models.filter((model) => selectedIDSet.value.has(model.id)),
)

function updateSelection(change: (nextIDs: Set<string>) => void) {
  const nextIDs = new Set(props.modelIds)
  change(nextIDs)
  const catalogIDs = new Set(props.models.map((model) => model.id))
  const orderedIDs = props.models.filter((model) => nextIDs.has(model.id)).map((model) => model.id)
  for (const id of nextIDs) {
    if (!catalogIDs.has(id)) orderedIDs.push(id)
  }
  emit('update:modelIds', orderedIDs)
}

function toggleModel(model: Model) {
  if (!selectableIDSet.value.has(model.id)) return
  updateSelection((nextIDs) => {
    if (nextIDs.has(model.id)) nextIDs.delete(model.id)
    else nextIDs.add(model.id)
  })
}

function selectAll() {
  updateSelection((nextIDs) => {
    filteredSelectableModels.value.forEach((model) => nextIDs.add(model.id))
  })
}

function invertSelection() {
  updateSelection((nextIDs) => {
    filteredSelectableModels.value.forEach((model) => {
      if (nextIDs.has(model.id)) nextIDs.delete(model.id)
      else nextIDs.add(model.id)
    })
  })
}

function modalityLabel(values: string[]) {
  return values.map((value) => t(`models.${value}`)).join(' / ')
}
</script>

<template>
  <div class="group-model-workspace">
    <section class="group-model-pane group-model-catalog" :aria-labelledby="catalogTitleID">
      <header class="group-model-pane-head">
        <span class="group-model-pane-title">
          <strong :id="catalogTitleID">{{ t('groups.modelListTitle') }}</strong>
          <span class="group-model-count-tag">{{
            query.trim()
              ? t('groups.filteredModelCount', { count: filteredModels.length })
              : models.length
          }}</span>
        </span>
        <span class="group-model-bulk-actions">
          <button
            type="button"
            class="text-button group-model-bulk-button"
            :disabled="disabled || !filteredSelectableModels.length"
            @click="selectAll"
          >
            {{ t('groups.selectAll') }}
          </button>
          <span class="group-model-action-separator" aria-hidden="true">/</span>
          <button
            type="button"
            class="text-button group-model-bulk-button"
            :disabled="disabled || !filteredSelectableModels.length"
            @click="invertSelection"
          >
            {{ t('groups.invertSelection') }}
          </button>
        </span>
      </header>
      <div class="group-model-search-box">
        <input
          v-model="query"
          type="search"
          :aria-label="t('groups.modelSearch')"
          :placeholder="t('groups.modelSearchPlaceholder')"
          :disabled="disabled"
        />
        <button
          v-if="query"
          type="button"
          class="group-model-search-clear"
          :aria-label="t('groups.clearModelSearch')"
          :disabled="disabled"
          @click="query = ''"
        >
          <Icon name="close" :size="14" />
        </button>
      </div>
      <div v-if="filteredModels.length" class="group-model-catalog-list">
        <label
          v-for="model in filteredModels"
          :key="model.id"
          class="group-model-catalog-grid group-model-catalog-row"
          :class="{ 'is-selected': selectedIDSet.has(model.id) }"
        >
          <span class="group-model-check">
            <input
              type="checkbox"
              :checked="selectedIDSet.has(model.id)"
              :disabled="disabled || !selectableIDSet.has(model.id)"
              :aria-label="t('groups.modelSelection', { name: model.name })"
              @change="toggleModel(model)"
            />
          </span>
          <span class="group-model-summary">
            <span class="group-model-identity">
              <strong class="model-name-regular">{{ model.name }}</strong>
              <span class="group-model-code">{{ model.code }}</span>
            </span>
            <span class="group-model-tags">
              <span v-if="model.inputModalities.length" class="modality-tag">{{
                t('groups.inputModalityTag', {
                  type: modalityLabel(model.inputModalities),
                })
              }}</span>
              <span v-if="model.outputModalities.length" class="modality-tag">{{
                t('groups.outputModalityTag', {
                  type: modalityLabel(model.outputModalities),
                })
              }}</span>
            </span>
          </span>
        </label>
      </div>
      <p v-else class="group-model-empty group-model-filter-empty">
        {{ t('groups.noMatchingModels') }}
        <button type="button" class="text-button" @click="query = ''">
          {{ t('groups.clearModelFilters') }}
        </button>
      </p>
    </section>

    <section class="group-model-pane group-model-selected" :aria-labelledby="selectedTitleID">
      <header class="group-model-pane-head">
        <strong :id="selectedTitleID">{{ t('groups.selectedModelsTitle') }}</strong>
        <span class="group-model-pane-count">{{ selectedModels.length }}</span>
      </header>
      <div v-if="selectedModels.length" class="group-model-selected-list">
        <div
          v-for="model in selectedModels"
          :key="model.id"
          class="group-model-selected-grid group-model-selected-row"
        >
          <span class="group-model-summary">
            <span class="group-model-identity">
              <strong class="model-name-regular">{{ model.name }}</strong>
              <span class="group-model-code">{{ model.code }}</span>
            </span>
            <span class="group-model-tags">
              <span v-if="model.inputModalities.length" class="modality-tag">{{
                t('groups.inputModalityTag', {
                  type: modalityLabel(model.inputModalities),
                })
              }}</span>
              <span v-if="model.outputModalities.length" class="modality-tag">{{
                t('groups.outputModalityTag', {
                  type: modalityLabel(model.outputModalities),
                })
              }}</span>
            </span>
          </span>
          <button
            type="button"
            class="group-model-remove"
            :disabled="disabled || !selectableIDSet.has(model.id)"
            :aria-label="t('groups.removeModelSelection', { name: model.name })"
            @click="toggleModel(model)"
          >
            <Icon name="close" :size="15" />
          </button>
        </div>
      </div>
      <p v-else class="group-model-empty group-model-selected-empty">
        {{ t('groups.selectedModelsEmpty') }}
      </p>
    </section>
  </div>
</template>

<style scoped>
.group-model-workspace {
  display: grid;
  width: 100%;
  min-width: 0;
  height: min(42vh, 360px);
  min-height: 280px;
  gap: 14px;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
}
.group-model-pane {
  display: grid;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #fff;
}
.group-model-catalog {
  grid-template-rows: auto auto minmax(0, 1fr);
}
.group-model-selected {
  grid-template-rows: auto minmax(0, 1fr);
}
.group-model-pane-head {
  display: flex;
  min-width: 0;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 12px;
  border-bottom: 1px solid #e5ebf1;
  background: #f8fafc;
}
.group-model-pane-head strong {
  color: var(--color-text);
  font-size: 12px;
}
.group-model-pane-title {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  align-items: center;
  gap: 8px;
}
.group-model-pane-count {
  flex: none;
  color: #60788d;
  font-size: 11px;
}
.group-model-count-tag {
  display: inline-flex;
  flex: none;
  min-height: 20px;
  align-items: center;
  padding: 1px 7px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
  line-height: 18px;
}
.group-model-bulk-actions {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  gap: 2px;
}
.group-model-action-separator {
  color: #94a3b8;
  font-size: 11px;
}
.group-model-search-box {
  position: relative;
  min-width: 0;
  margin: 10px;
}
.group-model-search-box input[type='search'] {
  min-height: 36px;
  padding: 7px 36px 7px 11px;
  font-size: 12px;
  background: #f8fafc;
}
.group-model-search-box input[type='search']::-webkit-search-cancel-button {
  display: none;
}
.group-model-search-clear {
  position: absolute;
  top: 5px;
  right: 5px;
  display: grid;
  width: 26px;
  height: 26px;
  padding: 0;
  place-items: center;
  color: var(--muted);
  background: transparent;
  border: 0;
  border-radius: 6px;
}
.group-model-search-clear:hover:not(:disabled) {
  color: var(--color-text);
  background: #e9eff6;
}
.group-model-bulk-button {
  min-height: 24px;
  padding: 2px 4px;
  font-size: 11px;
}
.group-model-catalog-list,
.group-model-selected-list {
  min-width: 0;
  min-height: 0;
  overflow-x: clip;
  overflow-y: scroll;
  overscroll-behavior: contain;
  scrollbar-color: #94a3b8 #f1f5f9;
  scrollbar-gutter: stable;
}
.group-model-catalog-list::-webkit-scrollbar,
.group-model-selected-list::-webkit-scrollbar {
  width: 12px;
}
.group-model-catalog-list::-webkit-scrollbar-track,
.group-model-selected-list::-webkit-scrollbar-track {
  background: #f1f5f9;
}
.group-model-catalog-list::-webkit-scrollbar-thumb,
.group-model-selected-list::-webkit-scrollbar-thumb {
  min-height: 40px;
  border: 3px solid #f1f5f9;
  border-radius: 999px;
  background: #94a3b8;
}
.group-model-catalog-list::-webkit-scrollbar-thumb:hover,
.group-model-selected-list::-webkit-scrollbar-thumb:hover {
  background: #64748b;
}
.group-model-catalog-grid,
.group-model-selected-grid {
  display: grid;
  min-width: 0;
  align-items: center;
  gap: 6px;
  padding: 7px 8px;
}
.group-model-catalog-grid {
  grid-template-columns: 74px minmax(0, 1fr);
}
.group-model-selected-grid {
  grid-template-columns: minmax(0, 1fr) 30px;
}
.group-model-catalog-row {
  min-height: 62px;
  margin: 0;
  border-bottom: 1px solid #f1f5f9;
  background: #fff;
  cursor: pointer;
}
.group-model-catalog-row.is-selected {
  background: var(--color-primary-soft);
  box-shadow: inset 3px 0 var(--color-primary);
}
.group-model-selected-row {
  min-height: 62px;
  border-bottom: 1px solid #f1f5f9;
  background: #fff;
}
.group-model-check {
  display: grid;
  place-items: center;
}
.group-model-check input {
  width: 18px;
  height: 18px;
  margin: 0;
  padding: 0;
  accent-color: var(--blue);
}
.group-model-summary {
  display: grid;
  min-width: 0;
  gap: 5px;
}
.group-model-identity,
.group-model-identity strong,
.group-model-code {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-model-identity {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.group-model-identity strong {
  flex: 0 1 auto;
  color: var(--color-text);
  font-size: 12px;
}
.group-model-code {
  flex: 1 1 auto;
  color: var(--muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 10px;
}
.group-model-tags {
  display: flex;
  min-width: 0;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}
.modality-tag {
  display: inline-block;
  padding: 2px 5px;
  border-radius: 6px;
  color: var(--color-primary-hover);
  background: var(--color-primary-soft);
  font-size: 9px;
  line-height: 1.4;
  white-space: nowrap;
}
.group-model-remove {
  display: grid;
  width: 30px;
  height: 30px;
  padding: 0;
  place-items: center;
  color: var(--muted);
  border: 0;
  border-radius: 6px;
  background: transparent;
}
.group-model-remove:hover:not(:disabled) {
  color: var(--danger);
  background: #fef2f2;
}
.group-model-empty {
  margin: 0;
  padding: 18px 14px;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}
.group-model-filter-empty {
  align-self: start;
  border-top: 1px solid #e5ebf1;
}
.group-model-filter-empty .text-button {
  margin-left: 6px;
}
.group-model-selected-empty {
  display: grid;
  min-height: 0;
  place-items: center;
  line-height: 1.6;
}
@container group-model-selector (max-width: 620px) {
  .group-model-workspace {
    height: auto;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: repeat(2, minmax(260px, 38vh));
  }
  .group-model-pane {
    min-height: 260px;
  }
}
@media (max-width: 760px) {
  .group-model-workspace {
    height: auto;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: repeat(2, minmax(260px, 38vh));
  }
  .group-model-pane {
    min-height: 260px;
  }
}
</style>
