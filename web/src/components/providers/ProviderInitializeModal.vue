<script setup lang="ts">
import { computed } from 'vue'
import { t } from '../../i18n'
import type { ProviderInitializeOption } from '../../types'
import Modal from '../Modal.vue'

const props = defineProps<{ options: ProviderInitializeOption[]; busy: boolean }>()
const selected = defineModel<string[]>({ required: true })
defineEmits<{ close: []; submit: [] }>()

const allSelected = computed(
  () => props.options.length > 0 && selected.value.length === props.options.length,
)

function toggleAll() {
  selected.value = allSelected.value ? [] : props.options.map((option) => option.code)
}
</script>

<template>
  <Modal :title="t('providers.initializeTitle')" :busy="busy" medium @close="$emit('close')">
    <p class="muted provider-initialize-hint">{{ t('providers.initializeHint') }}</p>
    <div class="provider-initialize-toolbar">
      <span>
        {{
          t('providers.initializeSelectionCount', {
            count: selected.length,
            total: options.length,
          })
        }}
      </span>
      <button type="button" class="text-button" :disabled="busy" @click="toggleAll">
        {{ t(allSelected ? 'providers.clearSelection' : 'providers.selectAll') }}
      </button>
    </div>
    <div
      class="provider-initialize-options"
      role="group"
      :aria-label="t('providers.initializeSelection')"
    >
      <label
        v-for="option in options"
        :key="option.code"
        class="provider-initialize-option"
        :class="{ 'is-selected': selected.includes(option.code) }"
      >
        <input
          v-model="selected"
          type="checkbox"
          :value="option.code"
          :disabled="busy"
          :aria-label="t('providers.initializeProviderSelection', { name: option.name })"
        />
        <span>
          <strong>{{ option.name }}</strong>
          <small>{{ option.website }}</small>
        </span>
      </label>
    </div>
    <template #footer>
      <button type="button" class="button" :disabled="busy" @click="$emit('close')">
        {{ t('common.cancel') }}</button
      ><button
        type="button"
        class="button primary"
        :disabled="busy || !selected.length"
        @click="$emit('submit')"
      >
        {{ t(busy ? 'common.working' : 'providers.initializeSelected') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.provider-initialize-hint {
  margin-bottom: 14px;
}
.provider-initialize-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
  color: var(--muted);
  font-size: 12px;
}
.provider-initialize-options {
  display: grid;
  max-height: min(52vh, 520px);
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
}
.provider-initialize-option {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  margin: 0;
  padding: 12px 14px;
  cursor: pointer;
  background: #fff;
}
.provider-initialize-option + .provider-initialize-option {
  border-top: 1px solid #e5ebf1;
}
.provider-initialize-option:hover {
  background: #f8fafc;
}
.provider-initialize-option.is-selected {
  background: var(--color-primary-soft);
}
.provider-initialize-option input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--blue);
}
.provider-initialize-option > span {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.provider-initialize-option strong,
.provider-initialize-option small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.provider-initialize-option small {
  color: var(--muted);
  font-size: 11px;
}
@media (max-width: 760px) {
  .provider-toolbar-actions .button {
    flex: 1;
  }
}
</style>
