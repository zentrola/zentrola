<script setup lang="ts">
import { t } from '../../i18n'
import { modalities, type ModelForm } from '../../modelForm'
import type { Model, Provider } from '../../types'
import Modal from '../Modal.vue'

// form 由父组件持有，弹窗只负责字段编辑与提交。
defineProps<{
  editTarget: Model | null
  form: ModelForm
  busy: boolean
  providers: Provider[]
  providerLoading: boolean
}>()
const emit = defineEmits<{ close: []; submit: [] }>()
</script>

<template>
  <Modal
    :title="t(editTarget ? 'models.edit' : 'models.create')"
    :busy="busy"
    medium
    @close="emit('close')"
  >
    <p v-if="!editTarget" class="model-form-hint">{{ t('models.createHint') }}</p>
    <form id="model-form" class="model-form" @submit.prevent="emit('submit')">
      <div class="model-form-row">
        <label class="model-form-label required-label" for="model-code-input">{{
          t('models.code')
        }}</label>
        <div class="model-form-control">
          <input
            id="model-code-input"
            v-model="form.code"
            required
            :disabled="busy"
            spellcheck="false"
            autofocus
          />
        </div>
      </div>
      <div class="model-form-row">
        <label class="model-form-label required-label" for="model-name-input">{{
          t('models.name')
        }}</label>
        <div class="model-form-control">
          <input id="model-name-input" v-model="form.name" required :disabled="busy" />
        </div>
      </div>
      <p v-if="editTarget && form.code !== editTarget.code" class="alert" role="status">
        {{ t('models.codeChange') }}
      </p>
      <div class="model-form-row">
        <label class="model-form-label" for="model-publisher-input">{{
          t('models.publisher')
        }}</label>
        <div class="model-form-control">
          <select
            id="model-publisher-input"
            v-model="form.publisherProviderId"
            :disabled="busy || providerLoading"
          >
            <option value="">{{ t('models.publisherPlaceholder') }}</option>
            <option v-for="provider in providers" :key="provider.id" :value="provider.id">
              {{ provider.name }}
            </option>
          </select>
          <small class="model-field-hint">{{ t('models.publisherHint') }}</small>
        </div>
      </div>
      <div class="model-form-row">
        <div id="model-input-title" class="model-form-label required-label">
          {{ t('models.input') }}
        </div>
        <div
          class="model-form-control modality-options"
          role="group"
          aria-labelledby="model-input-title"
        >
          <label v-for="value in modalities" :key="value"
            ><input
              v-model="form.inputModalities"
              type="checkbox"
              :value="value"
              :disabled="busy"
            />{{ t(`models.${value}`) }}</label
          >
        </div>
      </div>
      <div class="model-form-row">
        <div id="model-output-title" class="model-form-label required-label">
          {{ t('models.output') }}
        </div>
        <div
          class="model-form-control modality-options"
          role="group"
          aria-labelledby="model-output-title"
        >
          <label v-for="value in modalities" :key="value"
            ><input
              v-model="form.outputModalities"
              type="checkbox"
              :value="value"
              :disabled="busy"
            />{{ t(`models.${value}`) }}</label
          >
        </div>
      </div>
      <div class="model-form-row">
        <label class="model-form-label" for="model-remark-input">{{ t('common.remark') }}</label>
        <div class="model-form-control">
          <textarea
            id="model-remark-input"
            v-model="form.remark"
            :disabled="busy"
            :placeholder="t('models.remarkPlaceholder')"
            rows="3"
          />
        </div>
      </div>
    </form>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" form="model-form" class="button primary" :disabled="busy">
        {{ t(busy ? 'common.working' : 'common.save') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.model-form {
  display: grid;
  gap: 16px;
}
.model-form label {
  margin-bottom: 0;
}
.model-form-hint {
  margin: 0 0 20px;
  color: var(--muted);
}
.model-form-row {
  display: grid;
  grid-template-columns: 104px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.model-form-label {
  display: block;
  margin: 0;
  padding-top: 10px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
  text-align: right;
}
.model-form-control {
  min-width: 0;
}
.model-field-hint {
  display: block;
  margin-top: 6px;
  color: var(--muted);
  line-height: 1.5;
}
.required-label::after {
  content: '*';
  margin-left: 4px;
  color: var(--danger);
}
.modality-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.modality-options label {
  display: inline-flex;
  flex-direction: row;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  margin: 0;
  font-weight: 400;
  cursor: pointer;
}
.modality-options {
  min-height: 38px;
}
.modality-options input {
  width: 16px;
  height: 16px;
  margin: 0;
  padding: 0;
  accent-color: var(--blue);
}
@media (max-width: 640px) {
  .model-form-row {
    grid-template-columns: 1fr;
    gap: 8px;
  }
  .model-form-label {
    padding-top: 0;
    text-align: left;
  }
}
</style>
