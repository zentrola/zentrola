<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ApiError, api, errorText } from '../api'
import { dateOnly } from '../composables'
import { t } from '../i18n'
import { showSuccessToast } from '../toast'
import ConfirmDialog from './ConfirmDialog.vue'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
import type {
  CredentialPrices,
  ModelPrice,
  Provider,
  ProviderDetail,
  ProviderMapping,
  Resource,
  SubscriptionPrice,
} from '../types'

const props = defineProps<{ provider: Provider; resource: Resource }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const priceLoadError = ref('')
const modelLoadError = ref('')
const modelLoaded = ref(false)
const detail = ref<ProviderDetail | null>(null)
const prices = ref<CredentialPrices>({
  modelPrices: [],
  subscriptionPrices: [],
  subscriptionPrice: null,
})
const selectedMappingID = ref('')
const editingPrice = ref<ModelPrice | null>(null)
const subscriptionEditing = ref(false)
const editingSubscriptionPrice = ref<SubscriptionPrice | null>(null)
const deleteTarget = ref<ModelPrice | null>(null)
const subscriptionDeleteTarget = ref<SubscriptionPrice | null>(null)
const expandedHistoryIDs = ref(new Set<string>())
const subscriptionHistoryExpanded = ref(false)
const openMappingID = ref('')
const currency = ref<'' | 'CNY' | 'USD'>('')
const modelForm = reactive({
  inputPrice: '',
  outputPrice: '',
  cachedInputPrice: '',
  effectiveDate: '',
})
const subscriptionForm = reactive({
  periodAmount: '',
  billingPeriod: '' as '' | 'MONTH' | 'YEAR',
  effectiveDate: '',
})
const pricePattern = /^(0|[1-9][0-9]{0,11})(\.[0-9]{1,8})?$/

const mappings = computed(() => detail.value?.mappings ?? [])
const modelNames = computed(
  () => new Map((detail.value?.models ?? []).map((model) => [model.id, model.name])),
)
const selectedMapping = computed(() =>
  mappings.value.find((mapping) => mapping.id === selectedMappingID.value),
)
const subscriptionPriceHistory = computed(() =>
  [...prices.value.subscriptionPrices].sort((left, right) =>
    right.effectiveAt.localeCompare(left.effectiveAt),
  ),
)
const currentSubscriptionPrice = computed(() => {
  const now = new Date().toISOString()
  return subscriptionPriceHistory.value.find((price) => price.effectiveAt <= now)
})
const visibleSubscriptionPrices = computed(() => {
  const history = subscriptionPriceHistory.value
  if (subscriptionHistoryExpanded.value || history.length <= 3) return history
  const visible = history.slice(0, 3)
  const current = currentSubscriptionPrice.value
  if (current && !visible.some((price) => price.id === current.id)) visible.push(current)
  return visible.sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
})

function modelLabel(mapping: ProviderMapping) {
  return modelNames.value.get(mapping.modelId) || mapping.upstreamModelCode || mapping.modelId
}

function modelPriceHistory(mappingID: string): ModelPrice[] {
  return prices.value.modelPrices
    .filter((price) => price.providerModelId === mappingID)
    .sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
}

function currentModelPrice(mappingID: string): ModelPrice | undefined {
  const now = new Date().toISOString()
  return modelPriceHistory(mappingID).find((price) => price.effectiveAt <= now)
}

function visibleModelPrices(mappingID: string): ModelPrice[] {
  const history = modelPriceHistory(mappingID)
  if (expandedHistoryIDs.value.has(mappingID) || history.length <= 3) return history
  const visible = history.slice(0, 3)
  const current = currentModelPrice(mappingID)
  if (current && !visible.some((price) => price.id === current.id)) visible.push(current)
  return visible.sort((left, right) => right.effectiveAt.localeCompare(left.effectiveAt))
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

function isScheduled(price: ModelPrice | SubscriptionPrice) {
  return price.effectiveAt > new Date().toISOString()
}

function cancelModelEdit() {
  selectedMappingID.value = ''
  editingPrice.value = null
  error.value = ''
}

function currencySymbol(value: string) {
  return value === 'CNY' ? '¥' : '$'
}

function editModel(mapping: ProviderMapping, price?: ModelPrice) {
  selectedMappingID.value = mapping.id
  openMappingID.value = mapping.id
  editingPrice.value = price ?? null
  currency.value = price?.currency ?? ''
  modelForm.inputPrice = price?.inputPrice ?? ''
  modelForm.outputPrice = price?.outputPrice ?? ''
  modelForm.cachedInputPrice = price?.cachedInputPrice ?? ''
  modelForm.effectiveDate = price?.effectiveAt.slice(0, 10) ?? ''
  error.value = ''
}

function applySubscription(price: SubscriptionPrice | null) {
  currency.value = price?.currency ?? ''
  subscriptionForm.periodAmount = price?.periodAmount ?? ''
  subscriptionForm.billingPeriod = price?.billingPeriod ?? ''
  subscriptionForm.effectiveDate = price?.effectiveAt.slice(0, 10) ?? ''
}

function editSubscription(price?: SubscriptionPrice) {
  editingSubscriptionPrice.value = price ?? null
  applySubscription(price ?? null)
  subscriptionEditing.value = true
  error.value = ''
}

function cancelSubscriptionEdit() {
  subscriptionEditing.value = false
  editingSubscriptionPrice.value = null
  error.value = ''
}

async function load() {
  loading.value = true
  error.value = ''
  priceLoadError.value = ''
  modelLoadError.value = ''
  modelLoaded.value = false
  const [priceResult, detailResult] = await Promise.allSettled([
    api<CredentialPrices>(`/resources/${props.resource.id}/prices`),
    props.resource.authType === 'API_KEY'
      ? api<ProviderDetail>(`/providers/${props.provider.id}`)
      : Promise.resolve(null),
  ])
  if (priceResult.status === 'fulfilled') {
    const subscriptionPrices =
      priceResult.value.subscriptionPrices ??
      (priceResult.value.subscriptionPrice ? [priceResult.value.subscriptionPrice] : [])
    prices.value = { ...priceResult.value, subscriptionPrices }
    if (props.resource.authType === 'SUBSCRIPTION') applySubscription(null)
  } else {
    const cause = priceResult.reason
    priceLoadError.value = errorText(cause)
    if (cause instanceof ApiError && cause.code === 'NOT_FOUND') {
      try {
        await api<Resource>(`/resources/${props.resource.id}`)
        priceLoadError.value = t('resources.pricing.backendNotReady')
      } catch {
        // 凭证确实不存在时保留原始错误提示。
      }
    }
  }
  if (detailResult.status === 'fulfilled') {
    detail.value = detailResult.value
    modelLoaded.value = true
    if (selectedMappingID.value) {
      const mapping = detailResult.value?.mappings.find(
        (item) => item.id === selectedMappingID.value,
      )
      if (mapping) editModel(mapping)
    }
  } else {
    detail.value = null
    modelLoadError.value = errorText(detailResult.reason)
  }
  loading.value = false
}

async function saveModel() {
  if (!selectedMapping.value) return
  if (
    ![modelForm.inputPrice, modelForm.outputPrice, modelForm.cachedInputPrice].every((value) =>
      pricePattern.test(value),
    )
  ) {
    error.value = t('resources.pricing.invalidAmount')
    return
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(modelForm.effectiveDate)) {
    error.value = t('resources.pricing.invalidDate')
    return
  }
  saving.value = true
  error.value = ''
  try {
    const path = `/resources/${props.resource.id}/prices/models/${selectedMapping.value.id}`
    const saved = await api<ModelPrice>(
      editingPrice.value ? `${path}/${editingPrice.value.id}` : path,
      'PUT',
      {
        currency: currency.value,
        inputPrice: modelForm.inputPrice,
        outputPrice: modelForm.outputPrice,
        cachedInputPrice: modelForm.cachedInputPrice,
        effectiveAt: `${modelForm.effectiveDate}T00:00:00Z`,
      },
    )
    prices.value.modelPrices = [
      saved,
      ...prices.value.modelPrices.filter((price) => price.id !== saved.id),
    ]
    selectedMappingID.value = ''
    editingPrice.value = null
    showSuccessToast(t('common.saved'))
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    saving.value = false
  }
}

async function deleteModelPrice() {
  if (!deleteTarget.value) return
  saving.value = true
  error.value = ''
  try {
    const target = deleteTarget.value
    await api<{ deleted: boolean }>(
      `/resources/${props.resource.id}/prices/models/${target.providerModelId}/${target.id}`,
      'DELETE',
    )
    prices.value.modelPrices = prices.value.modelPrices.filter((price) => price.id !== target.id)
    if (editingPrice.value?.id === target.id) {
      selectedMappingID.value = ''
      editingPrice.value = null
    }
    deleteTarget.value = null
    showSuccessToast(t('resources.pricing.deleted'))
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    saving.value = false
  }
}

async function saveSubscription() {
  if (!pricePattern.test(subscriptionForm.periodAmount)) {
    error.value = t('resources.pricing.invalidAmount')
    return
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(subscriptionForm.effectiveDate)) {
    error.value = t('resources.pricing.invalidDate')
    return
  }
  if (
    prices.value.subscriptionPrices.some(
      (price) =>
        price.id !== editingSubscriptionPrice.value?.id &&
        price.effectiveAt.slice(0, 10) === subscriptionForm.effectiveDate,
    )
  ) {
    error.value = t('resources.pricing.duplicateEffectiveDate')
    return
  }
  saving.value = true
  error.value = ''
  try {
    const path = `/resources/${props.resource.id}/prices/subscription`
    const saved = await api<SubscriptionPrice>(
      editingSubscriptionPrice.value ? `${path}/${editingSubscriptionPrice.value.id}` : path,
      'PUT',
      {
        currency: currency.value,
        periodAmount: subscriptionForm.periodAmount,
        billingPeriod: subscriptionForm.billingPeriod,
        effectiveAt: `${subscriptionForm.effectiveDate}T00:00:00Z`,
      },
    )
    prices.value.subscriptionPrices = [
      saved,
      ...prices.value.subscriptionPrices.filter((price) => price.id !== saved.id),
    ]
    prices.value.subscriptionPrice = currentSubscriptionPrice.value ?? null
    subscriptionEditing.value = false
    editingSubscriptionPrice.value = null
    emit('changed')
    showSuccessToast(t('common.saved'))
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    saving.value = false
  }
}

async function deleteSubscriptionPrice() {
  if (!subscriptionDeleteTarget.value) return
  saving.value = true
  error.value = ''
  try {
    const target = subscriptionDeleteTarget.value
    await api<{ deleted: boolean }>(
      `/resources/${props.resource.id}/prices/subscription/${target.id}`,
      'DELETE',
    )
    prices.value.subscriptionPrices = prices.value.subscriptionPrices.filter(
      (price) => price.id !== target.id,
    )
    prices.value.subscriptionPrice = currentSubscriptionPrice.value ?? null
    subscriptionDeleteTarget.value = null
    emit('changed')
    showSuccessToast(t('resources.pricing.deleted'))
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="price-editor">
    <div class="price-editor-head">
      <div>
        <h3>{{ resource.name }}</h3>
        <p>{{ t('resources.pricing.hint') }}</p>
      </div>
      <button type="button" class="button" :disabled="saving" @click="emit('close')">
        {{ t('resources.pricing.back') }}
      </button>
    </div>

    <p v-if="loading" class="price-feedback">{{ t('common.loading') }}</p>
    <template v-else>
      <div v-if="priceLoadError" class="price-load-error" role="alert">
        <span>{{ priceLoadError }}</span>
        <button type="button" class="text-button" @click="load">
          {{ t('resources.pricing.retry') }}
        </button>
      </div>
      <p
        v-if="error && !selectedMapping && !subscriptionEditing"
        class="price-feedback is-error"
        role="alert"
      >
        {{ error }}
      </p>

      <template v-if="resource.authType === 'API_KEY'">
        <p class="price-unit">{{ t('resources.pricing.tokenUnit') }}</p>
        <p v-if="modelLoadError" class="price-feedback is-error" role="alert">
          {{ modelLoadError }}
        </p>
        <div v-else-if="modelLoaded && mappings.length" class="price-model-list">
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
                  {{ currentModelPrice(mapping.id)!.cachedInputPrice }}
                </span>
                <span v-else class="price-model-current is-empty">{{
                  t('resources.pricing.notConfigured')
                }}</span>
              </div>
              <div class="price-model-head-actions">
                <button
                  type="button"
                  class="icon-button price-icon-button"
                  :disabled="saving || !!priceLoadError"
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
                  @click="editModel(mapping)"
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
                <time :datetime="price.effectiveAt">{{ dateOnly(price.effectiveAt) }}</time>
                <span class="price-model-values">
                  {{ currencySymbol(price.currency) }} {{ price.inputPrice }} /
                  {{ price.outputPrice }} /
                  {{ price.cachedInputPrice }}
                </span>
                <span v-if="isScheduled(price)" class="price-scheduled-badge">{{
                  t('resources.pricing.scheduled')
                }}</span>
                <span class="price-history-actions">
                  <button
                    type="button"
                    class="icon-button price-icon-button"
                    :disabled="saving"
                    :aria-label="t('resources.pricing.modify')"
                    :title="t('resources.pricing.modify')"
                    @click="editModel(mapping, price)"
                  >
                    <Icon name="edit" :size="17" />
                  </button>
                  <button
                    type="button"
                    class="icon-button price-icon-button danger"
                    :disabled="saving"
                    :aria-label="t('resources.pricing.deleteAction')"
                    :title="t('resources.pricing.deleteAction')"
                    @click="deleteTarget = price"
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
        <p v-else-if="modelLoaded" class="price-feedback">
          {{ t('resources.pricing.noModels') }}
        </p>
      </template>

      <template v-else-if="!priceLoadError">
        <div class="price-subscription-list">
          <div class="price-subscription-head">
            <strong>{{ t('resources.pricing.list') }}</strong>
            <button
              type="button"
              class="icon-button price-icon-button"
              :disabled="saving"
              :aria-label="
                t(
                  subscriptionPriceHistory.length
                    ? 'resources.pricing.addVersion'
                    : 'resources.pricing.setSubscription',
                )
              "
              :title="
                t(
                  subscriptionPriceHistory.length
                    ? 'resources.pricing.addVersion'
                    : 'resources.pricing.setSubscription',
                )
              "
              @click="editSubscription()"
            >
              <Icon name="plus" :size="18" />
            </button>
          </div>
          <div
            v-if="subscriptionPriceHistory.length"
            class="price-history"
            :class="{ 'is-expanded': subscriptionHistoryExpanded }"
            :aria-label="t('resources.pricing.history')"
          >
            <div
              v-for="price in visibleSubscriptionPrices"
              :key="price.id"
              class="price-history-item"
              :class="{ 'is-current': price.id === currentSubscriptionPrice?.id }"
            >
              <time :datetime="price.effectiveAt">{{ dateOnly(price.effectiveAt) }}</time>
              <span class="price-model-values">
                {{ currencySymbol(price.currency) }}{{ price.periodAmount }} /
                {{ t(`resources.pricing.periods.${price.billingPeriod}`) }}
              </span>
              <span v-if="isScheduled(price)" class="price-scheduled-badge">{{
                t('resources.pricing.scheduled')
              }}</span>
              <span class="price-history-actions">
                <button
                  type="button"
                  class="icon-button price-icon-button"
                  :disabled="saving"
                  :aria-label="t('resources.pricing.modify')"
                  :title="t('resources.pricing.modify')"
                  @click="editSubscription(price)"
                >
                  <Icon name="edit" :size="17" />
                </button>
                <button
                  type="button"
                  class="icon-button price-icon-button danger"
                  :disabled="saving"
                  :aria-label="t('resources.pricing.deleteAction')"
                  :title="t('resources.pricing.deleteAction')"
                  @click="subscriptionDeleteTarget = price"
                >
                  <Icon name="trash" :size="17" />
                </button>
              </span>
            </div>
          </div>
          <p v-else class="price-subscription-empty">-</p>
          <button
            v-if="subscriptionPriceHistory.length > 3"
            type="button"
            class="price-history-toggle"
            :aria-expanded="subscriptionHistoryExpanded"
            @click="subscriptionHistoryExpanded = !subscriptionHistoryExpanded"
          >
            {{
              subscriptionHistoryExpanded
                ? t('resources.pricing.collapseHistory')
                : t('resources.pricing.showAllHistory', {
                    count: subscriptionPriceHistory.length,
                  })
            }}
          </button>
        </div>
      </template>
    </template>
  </div>
  <Modal
    v-if="selectedMapping && !priceLoadError"
    :title="
      t(editingPrice ? 'resources.pricing.editTitle' : 'resources.pricing.modelTitle', {
        name: modelLabel(selectedMapping),
      })
    "
    :busy="saving"
    medium
    @close="cancelModelEdit"
  >
    <form id="model-price-form" class="price-form price-dialog-form" @submit.prevent="saveModel">
      <p class="price-note">
        {{ t(editingPrice ? 'resources.pricing.editNote' : 'resources.pricing.versionNote') }}
      </p>
      <p v-if="error" class="price-feedback is-error" role="alert">{{ error }}</p>
      <div class="price-fields">
        <label>
          <span>{{ t('resources.pricing.currency') }}</span>
          <select v-model="currency" required :disabled="saving">
            <option value="" disabled>{{ t('common.select') }}</option>
            <option value="CNY">{{ t('resources.pricing.CNY') }}</option>
            <option value="USD">{{ t('resources.pricing.USD') }}</option>
          </select>
        </label>
        <label>
          <span>{{ t('resources.pricing.input') }}</span>
          <input
            v-model.trim="modelForm.inputPrice"
            inputmode="decimal"
            required
            :disabled="saving"
          />
        </label>
        <label>
          <span>{{ t('resources.pricing.output') }}</span>
          <input
            v-model.trim="modelForm.outputPrice"
            inputmode="decimal"
            required
            :disabled="saving"
          />
        </label>
        <label>
          <span>{{ t('resources.pricing.cachedInput') }}</span>
          <input
            v-model.trim="modelForm.cachedInputPrice"
            inputmode="decimal"
            required
            :disabled="saving"
          />
        </label>
        <label>
          <span>{{ t('resources.pricing.effectiveDate') }}</span>
          <input v-model="modelForm.effectiveDate" type="date" required :disabled="saving" />
        </label>
      </div>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="saving" @click="cancelModelEdit">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" form="model-price-form" class="button primary" :disabled="saving">
        {{ t('common.save') }}
      </button>
    </template>
  </Modal>
  <Modal
    v-if="subscriptionEditing && !priceLoadError"
    :title="
      t(
        editingSubscriptionPrice
          ? 'resources.pricing.subscriptionEditTitle'
          : 'resources.pricing.subscriptionTitle',
        { name: resource.name },
      )
    "
    :busy="saving"
    medium
    @close="cancelSubscriptionEdit"
  >
    <form
      id="subscription-price-form"
      class="price-form price-dialog-form"
      @submit.prevent="saveSubscription"
    >
      <p class="price-note">
        {{
          t(
            editingSubscriptionPrice
              ? 'resources.pricing.subscriptionEditNote'
              : 'resources.pricing.subscriptionVersionNote',
          )
        }}
      </p>
      <p v-if="error" class="price-feedback is-error" role="alert">{{ error }}</p>
      <div class="price-fields">
        <label>
          <span>{{ t('resources.pricing.currency') }}</span>
          <select v-model="currency" required :disabled="saving">
            <option value="" disabled>{{ t('common.select') }}</option>
            <option value="CNY">{{ t('resources.pricing.CNY') }}</option>
            <option value="USD">{{ t('resources.pricing.USD') }}</option>
          </select>
        </label>
        <label>
          <span>{{ t('resources.pricing.periodAmount') }}</span>
          <input
            v-model.trim="subscriptionForm.periodAmount"
            inputmode="decimal"
            required
            :disabled="saving"
          />
        </label>
        <label>
          <span>{{ t('resources.pricing.billingPeriod') }}</span>
          <select v-model="subscriptionForm.billingPeriod" required :disabled="saving">
            <option value="" disabled>{{ t('common.select') }}</option>
            <option value="MONTH">{{ t('resources.pricing.periods.MONTH') }}</option>
            <option value="YEAR">{{ t('resources.pricing.periods.YEAR') }}</option>
          </select>
        </label>
        <label>
          <span>{{ t('resources.pricing.effectiveDate') }}</span>
          <input v-model="subscriptionForm.effectiveDate" type="date" required :disabled="saving" />
        </label>
      </div>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="saving" @click="cancelSubscriptionEdit">
        {{ t('common.cancel') }}
      </button>
      <button
        type="submit"
        form="subscription-price-form"
        class="button primary"
        :disabled="saving"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </Modal>
  <ConfirmDialog
    v-if="deleteTarget"
    :title="t('resources.pricing.deleteTitle')"
    :message="t('resources.pricing.deleteQuestion')"
    :hint="t('resources.pricing.deleteHint')"
    :confirm-label="t('resources.pricing.deleteAction')"
    :busy="saving"
    tone="danger"
    @close="deleteTarget = null"
    @confirm="deleteModelPrice"
  />
  <ConfirmDialog
    v-if="subscriptionDeleteTarget"
    :title="t('resources.pricing.deleteSubscriptionTitle')"
    :message="t('resources.pricing.deleteSubscriptionQuestion')"
    :hint="t('resources.pricing.deleteSubscriptionHint')"
    :confirm-label="t('resources.pricing.deleteAction')"
    :busy="saving"
    tone="danger"
    @close="subscriptionDeleteTarget = null"
    @confirm="deleteSubscriptionPrice"
  />
</template>

<style scoped>
.price-editor-head,
.price-model-head,
.price-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.price-editor-head {
  margin-bottom: 18px;
}
.price-editor-head h3 {
  margin: 0;
  font-size: 17px;
  color: var(--color-text);
}
.price-editor-head p,
.price-note,
.price-unit,
.price-feedback {
  margin: 5px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.55;
}
.price-feedback {
  margin: 14px 0;
}
.price-feedback.is-error {
  color: var(--color-danger);
}
.price-load-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 11px 13px;
  border: 1px solid var(--color-danger);
  border-radius: 8px;
  color: var(--color-danger);
  font-size: 12px;
  line-height: 1.5;
}
.price-unit {
  margin-bottom: 9px;
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
.price-model-info small,
.price-model-values {
  font-size: 12px;
  color: var(--muted);
}
.price-model-values {
  font-variant-numeric: tabular-nums;
}
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
.price-history {
  display: grid;
  gap: 6px;
}
.price-history.is-expanded {
  max-height: 320px;
  padding-right: 4px;
  overflow-y: auto;
  scrollbar-gutter: stable;
}
.price-history-toggle {
  justify-self: start;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--color-primary);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.price-history-toggle:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 3px;
}
.price-history-item {
  display: grid;
  grid-template-columns: minmax(138px, auto) minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  min-height: 28px;
  padding: 4px 8px;
  border-left: 2px solid var(--color-border);
}
.price-history-actions {
  display: inline-flex;
  gap: 2px;
}
.price-icon-button {
  width: 32px;
  height: 32px;
  color: var(--color-primary);
}
.price-icon-button.danger {
  color: var(--danger);
}
.price-icon-button.danger:hover:not(:disabled) {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 9%, transparent);
}
.price-history-item.is-current {
  border-left-color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 5%, transparent);
}
.price-history-item time {
  color: var(--muted);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
.price-scheduled-badge {
  padding: 2px 6px;
  border-radius: 999px;
  background: #fff7ed;
  color: #9a3412;
  font-size: 11px;
}
.price-dialog-form {
  display: grid;
  gap: 16px;
}
.price-dialog-form .price-note,
.price-dialog-form .price-feedback {
  margin: 0;
}
.price-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.price-fields label {
  display: grid;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text);
}
.price-fields input,
.price-fields select {
  width: 100%;
  min-height: 36px;
  padding: 7px 9px;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  background: var(--color-surface);
  color: var(--color-text);
}
.price-fields input:focus-visible,
.price-fields select:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.price-actions {
  justify-content: flex-end;
  margin-top: 18px;
}
@media (max-width: 640px) {
  .price-editor-head {
    align-items: flex-start;
  }
  .price-fields {
    grid-template-columns: 1fr;
  }
  .price-history-item {
    grid-template-columns: 1fr auto;
  }
  .price-history-item .price-model-values {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
