<script setup lang="ts">
import { computed } from 'vue'
import { compactCount, count } from '../../composables'
import { statisticsRoute } from '../../dashboard'
import { t } from '../../i18n'
import type { Dashboard } from '../../types'
import Icon from '../Icon.vue'

const props = defineProps<{ summary: Dashboard | null; loading: boolean; month: Date }>()

const maxTokens = computed(() =>
  Math.max(1, ...(props.summary?.tokenRanking.map((item) => item.tokens) ?? [])),
)
const maxClientRequests = computed(() =>
  Math.max(1, ...(props.summary?.clientModelRanking.map((item) => item.requests) ?? [])),
)
const maxProviderCalls = computed(() =>
  Math.max(1, ...(props.summary?.providerRanking.map((item) => item.calls) ?? [])),
)
</script>

<template>
  <div class="ranking-grid">
    <section class="ranking-panel">
      <div class="ranking-head">
        <RouterLink
          class="ranking-head-link"
          :to="statisticsRoute(month, 'member')"
          :aria-label="t('home.tokenRanking')"
        >
          <h2>{{ t('home.tokenRanking') }}</h2>
          <span class="ranking-head-meta"><span>Top 10</span><Icon name="arrow" :size="14" /></span>
        </RouterLink>
      </div>
      <ol v-if="summary?.tokenRanking.length" class="ranking-list token-ranking">
        <li v-for="(item, index) in summary.tokenRanking" :key="item.principalId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong>{{ item.name }}</strong>
              <span
                :title="`${count(item.tokens)} Token`"
                :aria-label="`${count(item.tokens)} Token`"
              >
                {{ compactCount(item.tokens) }} Token
              </span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.tokens / maxTokens) * 100}%` }"></i>
            </div>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="usage" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>

    <section class="ranking-panel">
      <div class="ranking-head">
        <RouterLink
          class="ranking-head-link"
          :to="statisticsRoute(month, 'model')"
          :aria-label="t('home.clientModelRanking')"
        >
          <h2>{{ t('home.clientModelRanking') }}</h2>
          <span class="ranking-head-meta"><span>Top 10</span><Icon name="arrow" :size="14" /></span>
        </RouterLink>
      </div>
      <ol v-if="summary?.clientModelRanking.length" class="ranking-list model-ranking">
        <li v-for="(item, index) in summary.clientModelRanking" :key="item.modelId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong :title="item.modelName">{{ item.modelName }}</strong>
              <span
                :title="
                  t('home.modelRankingUsage', {
                    requests: count(item.requests),
                    tokens: count(item.tokens),
                  })
                "
              >
                {{
                  t('home.modelRankingUsage', {
                    requests: count(item.requests),
                    tokens: compactCount(item.tokens),
                  })
                }}
              </span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.requests / maxClientRequests) * 100}%` }"></i>
            </div>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="models" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>

    <section class="ranking-panel">
      <div class="ranking-head">
        <RouterLink
          class="ranking-head-link"
          :to="statisticsRoute(month, 'provider')"
          :aria-label="t('home.providerRanking')"
        >
          <h2>{{ t('home.providerRanking') }}</h2>
          <span class="ranking-head-meta"><span>Top 10</span><Icon name="arrow" :size="14" /></span>
        </RouterLink>
      </div>
      <ol v-if="summary?.providerRanking.length" class="ranking-list provider-ranking">
        <li v-for="(item, index) in summary.providerRanking" :key="item.providerId">
          <span class="rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="rank-content">
            <div class="rank-label">
              <strong :title="item.providerName">{{ item.providerName }}</strong>
              <span
                :title="
                  t('home.providerRankingUsage', {
                    calls: count(item.calls),
                    tokens: count(item.tokens),
                  })
                "
              >
                {{
                  t('home.providerRankingUsage', {
                    calls: count(item.calls),
                    tokens: compactCount(item.tokens),
                  })
                }}
              </span>
            </div>
            <div class="rank-track">
              <i :style="{ width: `${(item.calls / maxProviderCalls) * 100}%` }"></i>
            </div>
          </div>
        </li>
      </ol>
      <div v-else class="ranking-empty">
        <Icon name="models" :size="28" />
        <p>{{ t(loading ? 'common.loading' : 'home.noUsage') }}</p>
      </div>
    </section>
  </div>
</template>
