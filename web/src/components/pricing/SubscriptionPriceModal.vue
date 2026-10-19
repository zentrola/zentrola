<script setup lang="ts">
import type { PriceCurrency, SubscriptionPriceForm } from '../../credentialPricing'
import { t } from '../../i18n'
import Modal from '../Modal.vue'

// form 由父组件持有，弹窗只负责字段编辑与提交。
defineProps<{
  name: string
  editing: boolean
  form: SubscriptionPriceForm
  error: string
  saving: boolean
}>()
const emit = defineEmits<{ close: []; submit: [] }>()
const currency = defineModel<PriceCurrency>('currency', { required: true })
</script>

<template>
  <Modal
    :title="
      t(
        editing ? 'resources.pricing.subscriptionEditTitle' : 'resources.pricing.subscriptionTitle',
        { name },
      )
    "
    :busy="saving"
    medium
    @close="emit('close')"
  >
    <form
      id="subscription-price-form"
      class="price-form price-dialog-form"
      @submit.prevent="emit('submit')"
    >
      <p class="price-note">
        {{
          t(
            editing
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
          <input v-model.trim="form.periodAmount" inputmode="decimal" required :disabled="saving" />
        </label>
        <label>
          <span>{{ t('resources.pricing.billingPeriod') }}</span>
          <select v-model="form.billingPeriod" required :disabled="saving">
            <option value="" disabled>{{ t('common.select') }}</option>
            <option value="MONTH">{{ t('resources.pricing.periods.MONTH') }}</option>
            <option value="YEAR">{{ t('resources.pricing.periods.YEAR') }}</option>
          </select>
        </label>
        <label>
          <span>{{ t('resources.pricing.effectiveDate') }}</span>
          <input v-model="form.effectiveDate" type="date" required :disabled="saving" />
        </label>
      </div>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="saving" @click="emit('close')">
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
</template>

<style scoped src="./priceForm.css"></style>
