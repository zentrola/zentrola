<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { t } from '../i18n'
import Icon from './Icon.vue'

const emit = defineEmits<{ close: [] }>()

type GuideStep = {
  key: string
  target: string
  fallbackTarget?: string
  icon: string
  titleKey: string
  descriptionKey: string
}

const props = withDefaults(
  defineProps<{
    steps?: readonly GuideStep[]
    controlPrefix?: string
  }>(),
  { controlPrefix: 'home.initialization' },
)

const defaultSteps: readonly GuideStep[] = [
  {
    key: 'provider',
    target: '#nav-providers',
    icon: 'providers',
    titleKey: 'home.initializationSteps.provider.title',
    descriptionKey: 'home.initializationSteps.provider.description',
  },
  {
    key: 'model',
    target: '#nav-models',
    icon: 'models',
    titleKey: 'home.initializationSteps.model.title',
    descriptionKey: 'home.initializationSteps.model.description',
  },
  {
    key: 'group',
    target: '#nav-groups',
    icon: 'groups',
    titleKey: 'home.initializationSteps.group.title',
    descriptionKey: 'home.initializationSteps.group.description',
  },
  {
    key: 'member',
    target: '#nav-members',
    icon: 'members',
    titleKey: 'home.initializationSteps.member.title',
    descriptionKey: 'home.initializationSteps.member.description',
  },
  {
    key: 'usage',
    target: '#nav-usage',
    icon: 'usage',
    titleKey: 'home.initializationSteps.usage.title',
    descriptionKey: 'home.initializationSteps.usage.description',
  },
]

const current = ref(0)
const tooltip = ref<HTMLElement | null>(null)
const highlightStyle = ref<Record<string, string>>({})
const tooltipStyle = ref<Record<string, string>>({})
let activeTarget: HTMLElement | null = null
let openedSidebar: HTMLElement | null = null
let mobilePositionTimer: number | undefined
let positionFrame: number | undefined

const guideSteps = computed<readonly GuideStep[]>(() => props.steps ?? defaultSteps)
const step = computed(() => guideSteps.value[current.value])
const isLast = computed(() => current.value === guideSteps.value.length - 1)

function updatePosition() {
  const target =
    document.querySelector<HTMLElement>(step.value.target) ??
    (step.value.fallbackTarget
      ? document.querySelector<HTMLElement>(step.value.fallbackTarget)
      : null)
  if (!target) return
  const rect = target.getBoundingClientRect()

  if (window.innerWidth <= 800 && rect.right <= 0 && !openedSidebar) {
    const sidebar = target.closest<HTMLElement>('.sidebar')
    if (sidebar) {
      openedSidebar = sidebar
      openedSidebar.classList.add('open')
      mobilePositionTimer = window.setTimeout(updatePosition, 220)
      return
    }
  }

  activeTarget?.classList.remove('initialization-tour-target')
  activeTarget = target
  activeTarget.classList.add('initialization-tour-target')

  const padding = 6
  highlightStyle.value = {
    top: String(rect.top - padding) + 'px',
    left: String(rect.left - padding) + 'px',
    width: String(rect.width + padding * 2) + 'px',
    height: String(rect.height + padding * 2) + 'px',
  }

  const tooltipWidth = Math.min(360, window.innerWidth - 32)
  const tooltipHeight = tooltip.value?.getBoundingClientRect().height ?? 220
  const viewportPadding = 16
  let left = rect.right + 18
  let top = rect.top - 8

  if (window.innerWidth < 700 || left + tooltipWidth > window.innerWidth - viewportPadding) {
    left = Math.max(viewportPadding, window.innerWidth - tooltipWidth - viewportPadding)
    top = rect.bottom + 16
  }

  top = Math.min(
    Math.max(viewportPadding, top),
    Math.max(viewportPadding, window.innerHeight - tooltipHeight - viewportPadding),
  )
  tooltipStyle.value = {
    top: String(top) + 'px',
    left: String(left) + 'px',
    width: String(tooltipWidth) + 'px',
  }
}

function activateTarget() {
  activeTarget?.click()
}

function close() {
  emit('close')
}

function previous() {
  if (current.value > 0) current.value -= 1
}

function next() {
  if (isLast.value) {
    close()
    return
  }
  current.value += 1
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

watch([current, () => guideSteps.value.map((guideStep) => guideStep.key).join('|')], async () => {
  await nextTick()
  updatePosition()
  tooltip.value?.focus()
})

onMounted(async () => {
  window.addEventListener('resize', updatePosition)
  window.addEventListener('scroll', updatePosition, true)
  window.addEventListener('keydown', handleKeydown)
  await nextTick()
  positionFrame = window.requestAnimationFrame(() => {
    updatePosition()
    tooltip.value?.focus()
  })
})

onBeforeUnmount(() => {
  activeTarget?.classList.remove('initialization-tour-target')
  openedSidebar?.classList.remove('open')
  window.clearTimeout(mobilePositionTimer)
  window.cancelAnimationFrame(positionFrame ?? 0)
  window.removeEventListener('resize', updatePosition)
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div
      class="initialization-tour"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="'initialization-step-' + step.key"
      :data-target="step.key"
      @click.self="close"
    >
      <div
        class="initialization-highlight"
        :style="highlightStyle"
        @click.stop="activateTarget"
      ></div>
      <section ref="tooltip" class="initialization-tooltip" :style="tooltipStyle" tabindex="-1">
        <div class="initialization-tooltip-head">
          <span class="initialization-step">
            {{
              t(props.controlPrefix + 'Step', { current: current + 1, total: guideSteps.length })
            }}
          </span>
          <button
            type="button"
            class="icon-button"
            :aria-label="t(props.controlPrefix + 'Exit')"
            @click="close"
          >
            <Icon name="close" :size="17" />
          </button>
        </div>
        <div class="initialization-title">
          <span class="initialization-icon"><Icon :name="step.icon" :size="20" /></span>
          <div>
            <h2 :id="'initialization-step-' + step.key">
              {{ t(step.titleKey) }}
            </h2>
            <p>{{ t(step.descriptionKey) }}</p>
          </div>
        </div>
        <div class="initialization-actions">
          <button v-if="current > 0" type="button" class="button subtle" @click="previous">
            {{ t(props.controlPrefix + 'Previous') }}
          </button>
          <button type="button" class="button primary" @click="next">
            {{ t(props.controlPrefix + (isLast ? 'Finish' : 'Next')) }}
            <Icon v-if="!isLast" name="arrow" :size="14" />
          </button>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.initialization-tour {
  position: fixed;
  inset: 0;
  z-index: 100;
}
.initialization-highlight {
  position: fixed;
  border: 2px solid #60a5fa;
  border-radius: 10px;
  background: rgb(255 255 255 / 8%);
  box-shadow:
    0 0 0 9999px rgb(15 23 42 / 62%),
    0 0 0 4px rgb(96 165 250 / 22%);
  cursor: pointer;
  pointer-events: auto;
  transition:
    top 180ms ease,
    left 180ms ease,
    width 180ms ease,
    height 180ms ease;
}
.initialization-tooltip {
  position: fixed;
  padding: 18px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-card);
  outline: 0;
  background: var(--color-surface);
  box-shadow: 0 22px 56px rgb(15 23 42 / 28%);
}
.initialization-tooltip-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 30px;
  margin-bottom: 13px;
}
.initialization-step {
  color: var(--color-primary);
  font-size: 12px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.initialization-tooltip-head .icon-button {
  width: 30px;
  height: 30px;
  margin: -6px -6px 0 0;
}
.initialization-title {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.initialization-icon {
  display: grid;
  width: 38px;
  height: 38px;
  flex: none;
  place-items: center;
  border-radius: 10px;
  color: var(--color-primary);
  background: var(--color-primary-soft);
}
.initialization-title h2 {
  margin: 0;
  color: var(--color-text);
  font-size: 17px;
  line-height: 1.5;
}
.initialization-title p {
  margin: 5px 0 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.7;
}
.initialization-actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--line);
}
.initialization-actions .button {
  min-height: 36px;
  padding: 7px 13px;
}
:global(.initialization-tour-target) {
  position: relative;
}

@media (max-width: 560px) {
  .initialization-tooltip {
    padding: 16px;
  }
  .initialization-actions .button {
    flex: 1;
  }
}
</style>
