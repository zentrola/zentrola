<script setup lang="ts">
import { count } from '../../composables'
import { t } from '../../i18n'
import type { ActiveModel } from '../../types'

defineProps<{ models: ActiveModel[]; error: string }>()
const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div v-if="error" class="alert error active-models-error" role="alert">
    {{ t('home.activeModels') }}: {{ error }}
    <button type="button" class="text-button" @click="emit('retry')">
      {{ t('common.retry') }}
    </button>
  </div>
  <section v-if="models.length" class="active-models-panel" :aria-label="t('home.activeModels')">
    <div class="active-models-head">
      <h2>{{ t('home.activeModels') }}</h2>
      <span>{{ t('home.activeModelsCount', { count: count(models.length) }) }}</span>
    </div>
    <div class="active-models-table-wrap" tabindex="0" :aria-label="t('home.activeModelsList')">
      <table class="active-models-table">
        <thead>
          <tr>
            <th scope="col">{{ t('home.modelIdentity') }}</th>
            <th scope="col">{{ t('home.currentProvider') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="model in models" :key="model.modelCode" class="active-model-row">
            <td>
              <div class="active-model-identity">
                <strong :title="model.modelName || '-'">{{ model.modelName || '-' }}</strong>
                <code class="active-model-code" :title="model.modelCode || '-'">
                  {{ model.modelCode || '-' }}
                </code>
              </div>
            </td>
            <td>
              <span class="active-provider-name" :title="model.providerName || '-'">
                {{ model.providerName || '-' }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.active-models-panel {
  margin-bottom: 18px;
  padding: 22px 24px;
  border: 1px solid rgb(226 232 240 / 80%);
  border-radius: 12px;
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}
.active-models-error {
  margin-bottom: 18px;
}

.active-models-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
}

.active-models-head > span {
  color: var(--muted);
  font-size: 12px;
  white-space: nowrap;
}

.active-models-table-wrap {
  max-height: 280px;
  margin-top: 14px;
  overflow: auto;
  border-top: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
  scrollbar-color: #cbd5e1 transparent;
  scrollbar-width: thin;
}

.active-models-table-wrap:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 3px;
}

.active-models-table {
  width: 100%;
  min-width: 480px;
  border-collapse: collapse;
  table-layout: fixed;
}

.active-models-table th,
.active-models-table td {
  padding: 10px 12px;
  border-bottom: 1px solid #edf1f5;
  text-align: left;
}

.active-models-table th {
  position: sticky;
  z-index: 1;
  top: 0;
  background: var(--color-surface);
  color: var(--muted);
  font-size: 11px;
  font-weight: 600;
}

.active-models-table th:first-child {
  width: 62%;
}

.active-models-table tbody tr:last-child td {
  border-bottom: 0;
}

.active-model-identity strong,
.active-model-code,
.active-provider-name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.active-model-identity {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.active-model-identity strong {
  min-width: 0;
  flex: 0 1 auto;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 600;
}

.active-model-code {
  min-width: 0;
  flex: 0 1 auto;
  padding: 2px 7px;
  border: 1px solid #d9e2ec;
  border-radius: 5px;
  background: #f5f7fa;
  color: #60758a;
  font-size: 11px;
  line-height: 1.4;
}

.active-provider-name {
  max-width: 100%;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 500;
}
</style>
