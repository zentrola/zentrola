<script setup lang="ts">
import { computed, ref } from 'vue'
import { t } from '../../i18n'
import type { Model } from '../../types'
import Icon from '../Icon.vue'

export type MappingDraft = {
  modelId: string
  upstreamModelCode: string
}

const props = defineProps<{ models: Model[]; busy: boolean }>()
const mappings = defineModel<MappingDraft[]>({ required: true })

const query = ref('')
const mappingByModelID = computed(
  () => new Map(mappings.value.map((mapping) => [mapping.modelId, mapping])),
)
const rows = computed(() =>
  props.models.map((model) => ({ model, mapping: mappingByModelID.value.get(model.id) })),
)
const filteredRows = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  return rows.value.filter(
    (row) =>
      !keyword ||
      [
        row.model.name,
        row.model.code,
        row.model.publisherProviderName || '',
        row.mapping?.upstreamModelCode || '',
      ].some((value) => value.toLocaleLowerCase().includes(keyword)),
  )
})
const filterActive = computed(() => Boolean(query.value.trim()))
const selectedRows = computed(() => rows.value.filter((row) => row.mapping))

function resetFilters() {
  query.value = ''
}

function toggle(model: Model) {
  const index = mappings.value.findIndex((mapping) => mapping.modelId === model.id)
  if (index < 0) mappings.value.push({ modelId: model.id, upstreamModelCode: '' })
  else mappings.value.splice(index, 1)
}

function selectAll() {
  for (const row of filteredRows.value) {
    if (!mappings.value.some((mapping) => mapping.modelId === row.model.id)) {
      mappings.value.push({ modelId: row.model.id, upstreamModelCode: '' })
    }
  }
}

function invertSelection() {
  for (const row of filteredRows.value) toggle(row.model)
}

function updateUpstreamModelCode(modelId: string, event: Event) {
  const mapping = mappings.value.find((candidate) => candidate.modelId === modelId)
  if (mapping) mapping.upstreamModelCode = (event.target as HTMLInputElement).value
}
</script>

<template>
  <header class="mapping-editor-head">
    <p>{{ t('providers.mappingHint') }}</p>
  </header>
  <div v-if="models.length" class="mapping-workspace">
    <section class="mapping-pane mapping-catalog-pane" aria-labelledby="mapping-catalog-title">
      <header class="mapping-pane-head">
        <span class="mapping-pane-title">
          <strong id="mapping-catalog-title">{{ t('providers.mappingListTitle') }}</strong>
          <span class="mapping-count-tag">{{
            filterActive
              ? t('providers.filteredMappingCount', { count: filteredRows.length })
              : models.length
          }}</span>
        </span>
        <span class="mapping-bulk-actions">
          <button
            type="button"
            class="text-button mapping-bulk-button"
            :disabled="busy || !filteredRows.length"
            @click="selectAll"
          >
            {{ t('providers.selectAll') }}
          </button>
          <span class="mapping-action-separator" aria-hidden="true">/</span>
          <button
            type="button"
            class="text-button mapping-bulk-button"
            :disabled="busy || !filteredRows.length"
            @click="invertSelection"
          >
            {{ t('providers.invertSelection') }}
          </button>
        </span>
      </header>
      <div class="mapping-toolbar">
        <div class="mapping-search-box">
          <input
            id="provider-mapping-search"
            v-model="query"
            type="search"
            :aria-label="t('providers.mappingSearch')"
            :placeholder="t('providers.mappingSearchPlaceholder')"
            :disabled="busy"
          />
          <button
            v-if="query"
            type="button"
            class="mapping-search-clear"
            :aria-label="t('providers.clearMappingSearch')"
            :disabled="busy"
            @click="query = ''"
          >
            <Icon name="close" :size="14" />
          </button>
        </div>
      </div>
      <div v-if="filteredRows.length" class="mapping-catalog-list">
        <div
          v-for="row in filteredRows"
          :key="row.model.id"
          class="mapping-catalog-grid mapping-catalog-row"
          :class="{ 'is-selected': row.mapping }"
        >
          <label class="mapping-check">
            <input
              type="checkbox"
              :checked="Boolean(row.mapping)"
              :disabled="busy"
              :aria-label="t('providers.mappingSelectionFor', { name: row.model.name })"
              @change="toggle(row.model)"
            />
          </label>
          <span class="mapping-model-summary">
            <strong class="mapping-model-name model-name-regular">{{ row.model.name }}</strong>
            <span>{{ row.model.code }}</span>
          </span>
        </div>
      </div>
      <p v-else class="mapping-empty mapping-filter-empty">
        {{ t('providers.noMatchingMappings') }}
        <button type="button" class="text-button" @click="resetFilters">
          {{ t('providers.clearMappingFilters') }}
        </button>
      </p>
    </section>
    <section
      class="mapping-pane mapping-selected-pane"
      :aria-label="t('providers.selectedMappingsTitle')"
    >
      <header class="mapping-pane-head mapping-selected-head">
        <strong>{{ t('providers.logicalModel') }}</strong>
        <strong>{{ t('providers.upstreamModelCode') }}</strong>
        <span aria-hidden="true"></span>
      </header>
      <div v-if="selectedRows.length" class="mapping-selected-list">
        <div v-for="row in selectedRows" :key="row.model.id" class="mapping-selected-row">
          <span class="mapping-model-summary mapping-selected-model">
            <strong class="mapping-model-name model-name-regular">{{ row.model.name }}</strong>
            <span>{{ row.model.code }}</span>
          </span>
          <label class="mapping-control">
            <input
              :value="row.mapping?.upstreamModelCode ?? ''"
              spellcheck="false"
              :disabled="busy"
              :placeholder="t('providers.upstreamModelPlaceholder', { code: row.model.code })"
              :aria-label="t('providers.mappingCodeFor', { name: row.model.name })"
              @input="updateUpstreamModelCode(row.model.id, $event)"
            />
          </label>
          <button
            type="button"
            class="mapping-remove"
            :disabled="busy"
            :aria-label="t('providers.removeMappingFor', { name: row.model.name })"
            @click="toggle(row.model)"
          >
            <Icon name="close" :size="15" />
          </button>
        </div>
      </div>
      <p v-else class="mapping-empty mapping-selected-empty">
        {{ t('providers.selectedMappingsEmpty') }}
      </p>
    </section>
  </div>
  <p v-else class="mapping-empty">
    {{ t('providers.noModels') }}
  </p>
</template>

<style scoped>
.mapping-editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 8px;
}
.mapping-editor-head > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}
.mapping-editor-head p {
  margin: 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.mapping-workspace {
  display: grid;
  min-width: 0;
  min-height: 0;
  gap: 14px;
  margin-bottom: 16px;
  grid-template-columns: minmax(280px, 0.9fr) minmax(360px, 1.1fr);
}
.mapping-pane {
  display: grid;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: #fff;
}
.mapping-catalog-pane {
  grid-template-rows: auto auto minmax(0, 1fr);
}
.mapping-selected-pane {
  grid-template-rows: auto minmax(0, 1fr);
}
.mapping-pane-head {
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
.mapping-pane-head strong {
  color: var(--color-text);
  font-size: 12px;
}
.mapping-pane-head .mapping-pane-title {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  align-items: center;
  gap: 8px;
}
.mapping-count-tag {
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
  white-space: nowrap;
}
.mapping-bulk-actions {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  gap: 2px;
}
.mapping-bulk-button {
  min-height: 24px;
  padding: 2px 4px;
  font-size: 11px;
}
.mapping-action-separator {
  color: #94a3b8;
  font-size: 11px;
}
.mapping-selected-head {
  display: grid;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr) 32px;
  gap: 10px;
  padding-right: 22px;
}
.mapping-toolbar {
  min-width: 0;
  padding: 10px 10px 8px;
}
.mapping-search-box {
  position: relative;
  min-width: 0;
  color: #92a0af;
}
.mapping-search-box input[type='search'] {
  min-height: 36px;
  padding: 7px 36px 7px 11px;
  font-size: 12px;
  background: #f8fafc;
}
.mapping-search-box input[type='search']::-webkit-search-cancel-button {
  display: none;
}
.mapping-search-clear {
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
  cursor: pointer;
}
.mapping-search-clear:hover {
  color: var(--color-text);
  background: #e9eff6;
}
.mapping-catalog-list,
.mapping-selected-list {
  min-width: 0;
  min-height: 0;
  overflow-x: clip;
  overflow-y: scroll;
  overscroll-behavior: contain;
  scrollbar-color: #94a3b8 #f1f5f9;
  scrollbar-gutter: stable;
  scrollbar-width: auto;
}
.mapping-catalog-list {
  grid-row: 3;
}
.mapping-selected-list {
  grid-row: 2;
}
.mapping-catalog-list::-webkit-scrollbar,
.mapping-selected-list::-webkit-scrollbar {
  width: 12px;
}
.mapping-catalog-list::-webkit-scrollbar-track,
.mapping-selected-list::-webkit-scrollbar-track {
  background: #f1f5f9;
}
.mapping-catalog-list::-webkit-scrollbar-thumb,
.mapping-selected-list::-webkit-scrollbar-thumb {
  min-height: 40px;
  border: 3px solid #f1f5f9;
  border-radius: 999px;
  background: #94a3b8;
}
.mapping-catalog-list::-webkit-scrollbar-thumb:hover,
.mapping-selected-list::-webkit-scrollbar-thumb:hover {
  background: #64748b;
}
.mapping-catalog-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: 42px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 5px 10px;
  border-top: 1px solid #f1f5f9;
}
.mapping-catalog-row {
  min-height: 52px;
  background: #fff;
}
.mapping-catalog-row.is-selected {
  background: var(--color-primary-soft);
  box-shadow: inset 3px 0 var(--color-primary);
}
.mapping-check {
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0;
}
.mapping-check input {
  width: 18px;
  height: 18px;
  margin: 0;
  padding: 0;
  accent-color: var(--blue);
  cursor: pointer;
}
.mapping-model-name {
  display: block;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mapping-model-summary {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.mapping-model-summary > span {
  min-width: 0;
  overflow: hidden;
  color: var(--muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', monospace;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mapping-selected-row {
  display: grid;
  min-height: 52px;
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr) 32px;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 12px;
  border-top: 1px solid #f1f5f9;
  background: #fff;
}
.mapping-selected-row:first-child {
  border-top: 0;
}
.mapping-control {
  display: block;
  min-width: 0;
  margin: 0;
}
.mapping-control input {
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 5px 10px;
}
.mapping-remove {
  display: grid;
  width: 30px;
  height: 30px;
  padding: 0;
  place-items: center;
  color: var(--muted);
  border: 0;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
}
.mapping-remove:hover:not(:disabled) {
  color: var(--danger);
  background: #fef2f2;
}
.mapping-empty {
  margin: 0;
  padding: 18px 14px;
  color: var(--muted);
  font-size: 12px;
  text-align: center;
}
.mapping-filter-empty {
  grid-row: 4;
  align-self: start;
  border-top: 1px solid #e5ebf1;
}
.mapping-filter-empty .text-button {
  margin-left: 6px;
}
.mapping-selected-empty {
  display: grid;
  min-height: 0;
  place-items: center;
  line-height: 1.6;
}
@media (max-width: 760px) {
  .mapping-editor-head {
    align-items: stretch;
    flex-direction: column;
  }
  .mapping-workspace {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: repeat(2, minmax(260px, 38vh));
  }
  .mapping-pane {
    min-height: 260px;
  }
}
</style>
