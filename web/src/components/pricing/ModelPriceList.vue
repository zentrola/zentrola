<script setup lang="ts">
import { computed, ref } from 'vue'
import { utcDateOnly } from '../../composables'
import { groupModelPrices, isScheduledPrice, mappingLabel } from '../../credentialPricing'
import { t } from '../../i18n'
import { currencySymbol } from '../../money'
import { priceHistoryView } from '../../priceHistory'
import type { ModelPrice, ProviderMapping } from '../../types'
import Icon from '../Icon.vue'

const props = defineProps<{
  mappings: ProviderMapping[]
  modelNames: Map<string, string>
  prices: ModelPrice[]
  saving: boolean
  // 价格加载失败时禁止新增价格版本。
  locked: boolean
}>()
const emit = defineEmits<{
  edit: [mapping: ProviderMapping, price?: ModelPrice]
  delete: [price: ModelPrice]
}>()
// 当前展开的模型；编辑价格时由父组件同步展开对应模型。
const openMappingID = defineModel<string>('openMappingId', { required: true })
const expandedHistoryIDs = ref(new Set<string>())

const modelHistories = computed(() => {
  const grouped = groupModelPrices(props.prices)
  return new Map(
    props.mappings.map((mapping) => [
      mapping.id,
      priceHistoryView(grouped.get(mapping.id) ?? [], expandedHistoryIDs.value.has(mapping.id)),
    ]),
  )
})

function modelLabel(mapping: ProviderMapping) {
  return mappingLabel(mapping, props.modelNames)
}

function modelPriceHistory(mappingID: string): ModelPrice[] {
  return modelHistories.value.get(mappingID)?.history ?? []
}

function currentModelPrice(mappingID: string): ModelPrice | undefined {
  return modelHistories.value.get(mappingID)?.current
}

function visibleModelPrices(mappingID: string): ModelPrice[] {
  return modelHistories.value.get(mappingID)?.visible ?? []
}

function toggleModelHistory(mappingID: string) {
  const next = new Set(expandedHistoryIDs.value)
  if (next.has(mappingID)) next.delete(mappingID)
  else next.add(mappingID)
  expandedHistoryIDs.value = next
}

function toggleModel(mappingID: string) {
  openMappingID.value = openMappingID.value === mappingID ? '' : mappingID
}
</script>

<template>
  <div class="price-model-list">
    <div v-for="mapping in mappings" :key="mapping.id" class="price-model-row">
      <div class="price-model-head">
        <div class="price-model-summary">
          <div class="price-model-info">
            <strong>{{ modelLabel(mapping) }}</strong>
            <small>{{ mapping.upstreamModelCode || t('common.none') }}</small>
          </div>
          <span v-if="currentModelPrice(mapping.id)" class="price-model-current">
            {{ currencySymbol(currentModelPrice(mapping.id)!.currency) }}
            {{ currentModelPrice(mapping.id)!.inputPrice }} /
            {{ currentModelPrice(mapping.id)!.outputPrice }} /
            {{ currentModelPrice(mapping.id)!.cachedInputPrice }} /
            {{ currentModelPrice(mapping.id)!.cacheCreationInputPrice ?? '-' }}
          </span>
          <span v-else class="price-model-current is-empty">{{
            t('resources.pricing.notConfigured')
          }}</span>
        </div>
        <div class="price-model-head-actions">
          <button
            type="button"
            class="icon-button price-icon-button"
            :disabled="saving || locked"
            :aria-label="
              currentModelPrice(mapping.id)
                ? t('resources.pricing.addVersion')
                : t('resources.pricing.set')
            "
            :title="
              currentModelPrice(mapping.id)
                ? t('resources.pricing.addVersion')
                : t('resources.pricing.set')
            "
            @click="emit('edit', mapping)"
          >
            <Icon name="plus" :size="18" />
          </button>
          <button
            type="button"
            class="icon-button price-accordion-toggle"
            :aria-expanded="openMappingID === mapping.id"
            :aria-label="
              t(
                openMappingID === mapping.id
                  ? 'resources.pricing.collapseModel'
                  : 'resources.pricing.expandModel',
                { name: modelLabel(mapping) },
              )
            "
            @click="toggleModel(mapping.id)"
          >
            <Icon name="arrow" :size="17" />
          </button>
        </div>
      </div>
      <div
        v-if="openMappingID === mapping.id && modelPriceHistory(mapping.id).length"
        class="price-history"
        :class="{ 'is-expanded': expandedHistoryIDs.has(mapping.id) }"
        :aria-label="t('resources.pricing.history')"
      >
        <div
          v-for="price in visibleModelPrices(mapping.id)"
          :key="price.id"
          class="price-history-item"
          :class="{ 'is-current': price.id === currentModelPrice(mapping.id)?.id }"
        >
          <time :datetime="price.effectiveAt">{{ utcDateOnly(price.effectiveAt) }}</time>
          <span class="price-model-values">
            {{ currencySymbol(price.currency) }} {{ price.inputPrice }} / {{ price.outputPrice }} /
            {{ price.cachedInputPrice }} /
            {{ price.cacheCreationInputPrice ?? '-' }}
          </span>
          <span v-if="isScheduledPrice(price)" class="price-scheduled-badge">{{
            t('resources.pricing.scheduled')
          }}</span>
          <span class="price-history-actions">
            <button
              type="button"
              class="icon-button price-icon-button"
              :disabled="saving"
              :aria-label="t('resources.pricing.modify')"
              :title="t('resources.pricing.modify')"
              @click="emit('edit', mapping, price)"
            >
              <Icon name="edit" :size="17" />
            </button>
            <button
              type="button"
              class="icon-button price-icon-button danger"
              :disabled="saving"
              :aria-label="t('resources.pricing.deleteAction')"
              :title="t('resources.pricing.deleteAction')"
              @click="emit('delete', price)"
            >
              <Icon name="trash" :size="17" />
            </button>
          </span>
        </div>
      </div>
      <button
        v-if="openMappingID === mapping.id && modelPriceHistory(mapping.id).length > 3"
        type="button"
        class="price-history-toggle"
        :aria-expanded="expandedHistoryIDs.has(mapping.id)"
        @click="toggleModelHistory(mapping.id)"
      >
        {{
          expandedHistoryIDs.has(mapping.id)
            ? t('resources.pricing.collapseHistory')
            : t('resources.pricing.showAllHistory', {
                count: modelPriceHistory(mapping.id).length,
              })
        }}
      </button>
    </div>
  </div>
</template>

<style scoped src="./priceHistory.css"></style>
<style scoped>
.price-model-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.price-model-list {
  border: 1px solid var(--color-border);
  border-radius: 8px;
  overflow: hidden;
}
.price-model-row {
  display: grid;
  gap: 9px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--color-border);
}
.price-model-row:last-child {
  border-bottom: 0;
}
.price-model-info {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px 12px;
  min-width: 0;
}
.price-model-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px 16px;
  min-width: 0;
}
.price-model-current {
  color: var(--muted);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.price-model-current.is-empty {
  color: var(--color-text-muted);
}
.price-model-head-actions {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  flex: 0 0 auto;
}
.price-accordion-toggle svg {
  transition: transform 0.15s ease;
}
.price-accordion-toggle[aria-expanded='true'] svg {
  transform: rotate(90deg);
}
.price-model-info strong {
  font-size: 13px;
  color: var(--color-text);
}
.price-model-info small {
  font-size: 12px;
  color: var(--muted);
}
</style>
