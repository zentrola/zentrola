<script setup lang="ts">
import type { ModelPriceForm, PriceCurrency } from '../../credentialPricing'
import { t } from '../../i18n'
import Modal from '../Modal.vue'

// form 由父组件持有，弹窗只负责字段编辑与提交。
defineProps<{
  name: string
  editing: boolean
  form: ModelPriceForm
  error: string
  saving: boolean
}>()
const emit = defineEmits<{ close: []; submit: [] }>()
const currency = defineModel<PriceCurrency>('currency', { required: true })
</script>

<template>
  <Modal
    :title="t(editing ? 'resources.pricing.editTitle' : 'resources.pricing.modelTitle', { name })"
    :busy="saving"
    medium
    @close="emit('close')"
  >
    <form
      id="model-price-form"
      class="price-form price-dialog-form"
      @submit.prevent="emit('submit')"
    >
      <p class="price-note">
        {{ t(editing ? 'resources.pricing.editNote' : 'resources.pricing.versionNote') }}
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
          <input v-model.trim="form.inputPrice" inputmode="decimal" required :disabled="saving" />
        </label>
        <label>
          <span>{{ t('resources.pricing.output') }}</span>
          <input v-model.trim="form.outputPrice" inputmode="decimal" required :disabled="saving" />
        </label>
        <label>
          <span>{{ t('resources.pricing.cachedInput') }}</span>
          <input
            v-model.trim="form.cachedInputPrice"
            inputmode="decimal"
            required
            :disabled="saving"
          />
        </label>
        <label>
          <span>{{ t('resources.pricing.cacheCreationInput') }}</span>
          <input
            v-model.trim="form.cacheCreationInputPrice"
            inputmode="decimal"
            :placeholder="t('resources.pricing.cacheCreationInputPlaceholder')"
            :title="t('resources.pricing.cacheCreationInputHint')"
            :disabled="saving"
          />
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
      <button type="submit" form="model-price-form" class="button primary" :disabled="saving">
        {{ t('common.save') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped src="./priceForm.css"></style>
