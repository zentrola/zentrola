<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, all, errorText } from '../api'
import { useCollection, date, count, localTime } from '../composables'
import { t } from '../i18n'
import type { Usage, Member, Model, Resource } from '../types'
import Icon from '../components/Icon.vue'
import Status from '../components/Status.vue'
import Modal from '../components/Modal.vue'
import PageHeader from '../components/PageHeader.vue'
import ListFooter from '../components/ListFooter.vue'
const memberID = ref(''),
  modelID = ref(''),
  resourceID = ref(''),
  from = ref(''),
  to = ref(''),
  query = ref(''),
  validation = ref(''),
  lookupError = ref(''),
  metricsError = ref('')
const members = ref<Member[]>([]),
  models = ref<Model[]>([]),
  resources = ref<Resource[]>([]),
  selected = ref<Usage | null>(null)
const metrics = ref<{ pending: number; failed: number } | null>(null)
const { items, cursor, loading, error, load } = useCollection<Usage>(() => `/usage?${query.value}`)
function resetTimes() {
  const now = new Date()
  to.value = localTime(now)
  from.value = localTime(new Date(now.getTime() - 24 * 60 * 60 * 1000))
}
function search() {
  const start = Date.parse(from.value),
    end = Date.parse(to.value)
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    end <= start ||
    end - start > 366 * 86400000
  ) {
    validation.value = t('usage.invalidRange')
    return
  }
  validation.value = ''
  const params = new URLSearchParams({
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
  })
  for (const [key, value] of [
    ['memberId', memberID.value],
    ['modelId', modelID.value],
    ['resourceId', resourceID.value],
  ])
    if (value) params.set(key, value)
  query.value = params.toString()
  void load()
  void loadMetrics()
}
function reset() {
  memberID.value = ''
  modelID.value = ''
  resourceID.value = ''
  resetTimes()
  search()
}
async function lookups() {
  lookupError.value = ''
  try {
    ;[members.value, models.value, resources.value] = await Promise.all([
      all<Member>('/members'),
      all<Model>('/models'),
      all<Resource>('/resources'),
    ])
  } catch (e) {
    lookupError.value = errorText(e)
  }
}
async function loadMetrics() {
  metricsError.value = ''
  try {
    metrics.value = await api('/usage/writer')
  } catch (e) {
    metrics.value = null
    metricsError.value = errorText(e)
  }
}
function label(list: { id: string; name: string }[], id: string | null) {
  return id === null ? t('common.none') : list.find((x) => x.id === id)?.name || id
}
onMounted(() => {
  void lookups()
  reset()
})
</script>
<template>
  <PageHeader name="usage" />
  <section class="filter-panel">
    <form class="usage-filters" :aria-label="t('usage.filters')" @submit.prevent="search">
      <label
        >{{ t('usage.member')
        }}<select v-model="memberID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="member in members" :key="member.id" :value="member.id">
            {{ member.name }} · {{ member.id }}
          </option>
        </select></label
      ><label
        >{{ t('usage.model')
        }}<select v-model="modelID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="model in models" :key="model.id" :value="model.id">
            {{ model.name }}
          </option>
        </select></label
      ><label
        >{{ t('usage.resource')
        }}<select v-model="resourceID">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="resource in resources" :key="resource.id" :value="resource.id">
            {{ resource.name }}
          </option>
        </select></label
      ><label
        >{{ t('common.from')
        }}<input v-model="from" type="datetime-local" step="1" required /></label
      ><label
        >{{ t('common.to') }}<input v-model="to" type="datetime-local" step="1" required
      /></label>
      <div class="filter-actions">
        <button class="button primary" :disabled="loading">
          <Icon name="search" :size="16" />{{ t('common.searchAction') }}</button
        ><button type="button" class="button" :disabled="loading" @click="reset">
          {{ t('common.reset') }}
        </button>
      </div>
    </form>
    <p class="field-hint">{{ t('usage.range') }}</p>
    <p v-if="validation" class="alert error" role="alert">{{ validation }}</p>
    <p v-if="lookupError" class="alert error" role="alert">
      {{ lookupError }}<button class="text-button" @click="lookups">{{ t('common.retry') }}</button>
    </p>
  </section>
  <div class="usage-context">
    <span><Icon name="usage" :size="17" />{{ t('usage.semantics') }}</span>
    <div v-if="metrics" class="writer-metrics" :title="t('usage.metricsHint')">
      <span
        >{{ t('usage.pending') }} <b>{{ metrics.pending }}</b></span
      ><span :class="{ danger: metrics.failed > 0 }"
        >{{ t('usage.failed') }} <b>{{ metrics.failed }}</b></span
      >
    </div>
  </div>
  <p v-if="metricsError" class="alert error" role="alert">
    {{ t('usage.writer') }}：{{ metricsError
    }}<button class="text-button" @click="loadMetrics">{{ t('common.retry') }}</button>
  </p>
  <section class="panel">
    <p v-if="error" class="alert error" role="alert">
      {{ error }}<button class="text-button" @click="load()">{{ t('common.retry') }}</button>
    </p>
    <div class="table-scroll">
      <table class="usage-table">
        <thead>
          <tr>
            <th>{{ t('usage.time') }}</th>
            <th>{{ t('usage.member') }}</th>
            <th>{{ t('usage.model') }}</th>
            <th>{{ t('usage.protocol') }}</th>
            <th>{{ t('common.status') }}</th>
            <th class="numeric">{{ t('usage.input') }}</th>
            <th class="numeric">{{ t('usage.output') }}</th>
            <th class="numeric">{{ t('usage.cached') }}</th>
            <th class="align-right">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in items" :key="row.id">
            <td>
              {{ date(row.requestAt) }}<small class="subline">{{ row.latencyMs }} ms</small>
            </td>
            <td>{{ label(members, row.principalId) }}</td>
            <td>
              {{ label(models, row.modelId)
              }}<small class="subline">{{ label(resources, row.resourceId) }}</small>
            </td>
            <td>
              <span class="protocol-label">{{
                row.clientProtocol === 'OPENAI'
                  ? 'OpenAI'
                  : row.clientProtocol === 'ANTHROPIC'
                    ? 'Anthropic'
                    : row.clientProtocol
              }}</span>
            </td>
            <td><Status :value="row.status" /></td>
            <td class="numeric">{{ count(row.inputTokens) }}</td>
            <td class="numeric">{{ count(row.outputTokens) }}</td>
            <td class="numeric">{{ count(row.cachedInputTokens) }}</td>
            <td class="align-right">
              <button class="text-button" @click="selected = row">{{ t('common.details') }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="!items.length" class="empty-state">
      <Icon name="usage" :size="32" />
      <h3>{{ t(loading ? 'common.loading' : 'common.empty') }}</h3>
      <p v-if="!loading">{{ t('usage.empty') }}</p>
    </div>
    <ListFooter :count="items.length" :cursor="cursor" :loading="loading" @more="load(true)" />
  </section>
  <Modal v-if="selected" :title="t('common.details')" wide @close="selected = null"
    ><dl class="detail-grid">
      <dt>{{ t('common.requestId') }}</dt>
      <dd>
        <code>{{ selected.requestId }}</code>
      </dd>
      <dt>{{ t('usage.member') }}</dt>
      <dd>
        {{ label(members, selected.principalId)
        }}<small class="subline">{{ selected.principalId }}</small>
      </dd>
      <dt>{{ t('usage.model') }}</dt>
      <dd>{{ label(models, selected.modelId) }}</dd>
      <dt>{{ t('usage.resource') }}</dt>
      <dd>
        {{ label(resources, selected.resourceId)
        }}<small class="subline">{{ selected.resourceId }}</small>
      </dd>
      <dt>{{ t('usage.protocol') }}</dt>
      <dd>{{ selected.clientProtocol }}</dd>
      <dt>{{ t('common.status') }}</dt>
      <dd><Status :value="selected.status" /></dd>
      <dt>{{ t('usage.attempt') }}</dt>
      <dd>
        {{ selected.attemptNo === null ? t('usage.noAttempt') : selected.attemptNo
        }}<small class="subline">{{ selected.attemptStatus }}</small>
      </dd>
      <dt>{{ t('usage.errorType') }}</dt>
      <dd>{{ selected.errorType || selected.attemptErrorType || t('common.none') }}</dd>
      <dt>{{ t('usage.time') }}</dt>
      <dd>{{ date(selected.requestAt) }}</dd>
      <dt>{{ t('usage.latency') }}</dt>
      <dd>{{ selected.latencyMs }} ms</dd>
      <dt>{{ t('usage.input') }}</dt>
      <dd>{{ count(selected.inputTokens) }}</dd>
      <dt>{{ t('usage.output') }}</dt>
      <dd>{{ count(selected.outputTokens) }}</dd>
      <dt>{{ t('usage.cached') }}</dt>
      <dd>{{ count(selected.cachedInputTokens) }}</dd>
    </dl></Modal
  >
</template>
