<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ApiError, api, errorText } from '../api'
import {
  fillModelPriceForm,
  fillSubscriptionPriceForm,
  mappingLabel,
  modelPriceFormError,
  saveModelPrice,
  saveSubscriptionPrice,
  subscriptionPriceFormError,
  type ModelPriceForm,
  type PriceCurrency,
  type SubscriptionPriceForm,
} from '../credentialPricing'
import { t } from '../i18n'
import { priceHistoryView } from '../priceHistory'
import { showSuccessToast } from '../toast'
import ConfirmDialog from './ConfirmDialog.vue'
import ModelPriceList from './pricing/ModelPriceList.vue'
import ModelPriceModal from './pricing/ModelPriceModal.vue'
import SubscriptionPriceList from './pricing/SubscriptionPriceList.vue'
import SubscriptionPriceModal from './pricing/SubscriptionPriceModal.vue'
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
// busy 同步保存中状态，外层弹窗据此禁止关闭。
const emit = defineEmits<{ close: []; changed: []; busy: [value: boolean] }>()
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
const openMappingID = ref('')
const currency = ref<PriceCurrency>('')
const modelForm = reactive<ModelPriceForm>({
  inputPrice: '',
  outputPrice: '',
  cachedInputPrice: '',
  cacheCreationInputPrice: '',
  effectiveDate: '',
})
const subscriptionForm = reactive<SubscriptionPriceForm>({
  periodAmount: '',
  billingPeriod: '',
  effectiveDate: '',
})
watch(saving, (value) => emit('busy', value))

const mappings = computed(() => detail.value?.mappings ?? [])
const modelNames = computed(
  () => new Map((detail.value?.models ?? []).map((model) => [model.id, model.name])),
)
const selectedMapping = computed(() =>
  mappings.value.find((mapping) => mapping.id === selectedMappingID.value),
)

function modelLabel(mapping: ProviderMapping) {
  return mappingLabel(mapping, modelNames.value)
}

// 当前生效的订阅费用与历史列表是否展开无关。
function currentSubscriptionPrice() {
  return priceHistoryView(prices.value.subscriptionPrices, false).current ?? null
}

function cancelModelEdit() {
  selectedMappingID.value = ''
  editingPrice.value = null
  error.value = ''
}

function editModel(mapping: ProviderMapping, price?: ModelPrice) {
  selectedMappingID.value = mapping.id
  openMappingID.value = mapping.id
  editingPrice.value = price ?? null
  currency.value = price?.currency ?? ''
  fillModelPriceForm(modelForm, price)
  error.value = ''
}

function applySubscription(price: SubscriptionPrice | null) {
  currency.value = price?.currency ?? ''
  fillSubscriptionPriceForm(subscriptionForm, price)
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
  const invalid = modelPriceFormError(modelForm)
  if (invalid) {
    error.value = invalid
    return
  }
  saving.value = true
  error.value = ''
  try {
    const saved = await saveModelPrice(
      props.resource.id,
      selectedMapping.value.id,
      editingPrice.value?.id,
      currency.value,
      modelForm,
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
  const invalid = subscriptionPriceFormError(
    subscriptionForm,
    prices.value.subscriptionPrices,
    editingSubscriptionPrice.value?.id,
  )
  if (invalid) {
    error.value = invalid
    return
  }
  saving.value = true
  error.value = ''
  try {
    const saved = await saveSubscriptionPrice(
      props.resource.id,
      editingSubscriptionPrice.value?.id,
      currency.value,
      subscriptionForm,
    )
    prices.value.subscriptionPrices = [
      saved,
      ...prices.value.subscriptionPrices.filter((price) => price.id !== saved.id),
    ]
    prices.value.subscriptionPrice = currentSubscriptionPrice()
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
    prices.value.subscriptionPrice = currentSubscriptionPrice()
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
        <ModelPriceList
          v-else-if="modelLoaded && mappings.length"
          v-model:open-mapping-id="openMappingID"
          :mappings="mappings"
          :model-names="modelNames"
          :prices="prices.modelPrices"
          :saving="saving"
          :locked="!!priceLoadError"
          @edit="editModel"
          @delete="deleteTarget = $event"
        />
        <p v-else-if="modelLoaded" class="price-feedback">
          {{ t('resources.pricing.noModels') }}
        </p>
      </template>

      <SubscriptionPriceList
        v-else-if="!priceLoadError"
        :prices="prices.subscriptionPrices"
        :saving="saving"
        @edit="editSubscription"
        @delete="subscriptionDeleteTarget = $event"
      />
    </template>
  </div>
  <ModelPriceModal
    v-if="selectedMapping && !priceLoadError"
    v-model:currency="currency"
    :name="modelLabel(selectedMapping)"
    :editing="!!editingPrice"
    :form="modelForm"
    :error="error"
    :saving="saving"
    @close="cancelModelEdit"
    @submit="saveModel"
  />
  <SubscriptionPriceModal
    v-if="subscriptionEditing && !priceLoadError"
    v-model:currency="currency"
    :name="resource.name"
    :editing="!!editingSubscriptionPrice"
    :form="subscriptionForm"
    :error="error"
    :saving="saving"
    @close="cancelSubscriptionEdit"
    @submit="saveSubscription"
  />
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
.price-editor-head {
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
@media (max-width: 640px) {
  .price-editor-head {
    align-items: flex-start;
  }
}
</style>
