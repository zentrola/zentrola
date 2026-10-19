<script setup lang="ts">
import { computed, ref } from 'vue'
import { utcDateOnly } from '../../composables'
import { isScheduledPrice } from '../../credentialPricing'
import { t } from '../../i18n'
import { formatMoney } from '../../money'
import { priceHistoryView } from '../../priceHistory'
import type { SubscriptionPrice } from '../../types'
import Icon from '../Icon.vue'

const props = defineProps<{ prices: SubscriptionPrice[]; saving: boolean }>()
const emit = defineEmits<{
  edit: [price?: SubscriptionPrice]
  delete: [price: SubscriptionPrice]
}>()
const expanded = ref(false)
const view = computed(() => priceHistoryView(props.prices, expanded.value))
const history = computed(() => view.value.history)
const current = computed(() => view.value.current)
const visible = computed(() => view.value.visible)
</script>

<template>
  <div class="price-subscription-list">
    <div class="price-subscription-head">
      <strong>{{ t('resources.pricing.list') }}</strong>
      <button
        type="button"
        class="icon-button price-icon-button"
        :disabled="saving"
        :aria-label="
          t(history.length ? 'resources.pricing.addVersion' : 'resources.pricing.setSubscription')
        "
        :title="
          t(history.length ? 'resources.pricing.addVersion' : 'resources.pricing.setSubscription')
        "
        @click="emit('edit')"
      >
        <Icon name="plus" :size="18" />
      </button>
    </div>
    <div
      v-if="history.length"
      class="price-history"
      :class="{ 'is-expanded': expanded }"
      :aria-label="t('resources.pricing.history')"
    >
      <div
        v-for="price in visible"
        :key="price.id"
        class="price-history-item"
        :class="{ 'is-current': price.id === current?.id }"
      >
        <time :datetime="price.effectiveAt">{{ utcDateOnly(price.effectiveAt) }}</time>
        <span class="price-model-values">
          {{ formatMoney(price.periodAmount, price.currency) }} /
          {{ t(`resources.pricing.periods.${price.billingPeriod}`) }}
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
            @click="emit('edit', price)"
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
    <p v-else class="price-subscription-empty">-</p>
    <button
      v-if="history.length > 3"
      type="button"
      class="price-history-toggle"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      {{
        expanded
          ? t('resources.pricing.collapseHistory')
          : t('resources.pricing.showAllHistory', { count: history.length })
      }}
    </button>
  </div>
</template>

<style scoped src="./priceHistory.css"></style>
<style scoped>
.price-subscription-list {
  display: grid;
  gap: 9px;
}
.price-subscription-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 32px;
  padding-left: 8px;
}
.price-subscription-head strong {
  color: var(--color-text);
  font-size: 13px;
}
.price-subscription-empty {
  margin: 0;
  padding: 4px 8px;
  color: var(--muted);
  font-size: 12px;
}
</style>
