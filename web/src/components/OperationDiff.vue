<script setup lang="ts">
import { computed, ref } from 'vue'
import Icon from './Icon.vue'
import { t } from '../i18n'
import { showErrorToast, showSuccessToast } from '../toast'

const props = defineProps<{ before: unknown; after: unknown }>()

type Side = 'before' | 'after'
type ChangeKind = 'added' | 'removed' | 'changed'
type JsonLine = { text: string; kind?: ChangeKind }

const missing = Symbol('missing')
const copyState = ref<Record<Side, 'idle' | 'copied'>>({
  before: 'idle',
  after: 'idle',
})

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stableValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableValue).join(',')}]`
  if (!isRecord(value)) return JSON.stringify(value) ?? 'null'
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${stableValue(value[key])}`)
    .join(',')}}`
}

function changeKind(
  value: unknown,
  other: unknown | typeof missing,
  side: Side,
): ChangeKind | undefined {
  if (other === missing) return side === 'before' ? 'removed' : 'added'
  return stableValue(value) === stableValue(other) ? undefined : 'changed'
}

function scalar(value: unknown) {
  return JSON.stringify(value) ?? 'null'
}

function renderJson(
  value: unknown,
  other: unknown | typeof missing,
  side: Side,
  depth = 0,
  key?: string,
  comma = false,
  inheritedKind?: ChangeKind | null,
): JsonLine[] {
  const indent = '  '.repeat(depth)
  const property = key === undefined ? '' : `${JSON.stringify(key)}: `
  const ownKind =
    inheritedKind === undefined ? changeKind(value, other, side) : (inheritedKind ?? undefined)
  const suffix = comma ? ',' : ''

  if (Array.isArray(value)) {
    const arrayKind = ownKind
    if (!value.length) return [{ text: `${indent}${property}[]${suffix}`, kind: arrayKind }]
    const lines: JsonLine[] = [{ text: `${indent}${property}[`, kind: arrayKind }]
    value.forEach((item, index) => {
      lines.push(
        ...renderJson(
          item,
          missing,
          side,
          depth + 1,
          undefined,
          index < value.length - 1,
          arrayKind ?? null,
        ),
      )
    })
    lines.push({ text: `${indent}]${suffix}`, kind: arrayKind })
    return lines
  }

  if (isRecord(value)) {
    const otherRecord = isRecord(other) ? other : undefined
    const compareChildren = otherRecord !== undefined && inheritedKind === undefined
    const objectKind = compareChildren ? undefined : ownKind
    const entries = Object.entries(value)
    if (!entries.length) return [{ text: `${indent}${property}{}${suffix}`, kind: objectKind }]
    const lines: JsonLine[] = [{ text: `${indent}${property}{`, kind: objectKind }]
    entries.forEach(([childKey, item], index) => {
      const childOther =
        otherRecord && Object.prototype.hasOwnProperty.call(otherRecord, childKey)
          ? otherRecord[childKey]
          : missing
      lines.push(
        ...renderJson(
          item,
          childOther,
          side,
          depth + 1,
          childKey,
          index < entries.length - 1,
          compareChildren ? undefined : (objectKind ?? null),
        ),
      )
    })
    lines.push({ text: `${indent}}${suffix}`, kind: objectKind })
    return lines
  }

  return [{ text: `${indent}${property}${scalar(value)}${suffix}`, kind: ownKind }]
}

const hasBefore = computed(() => props.before !== null && props.before !== undefined)
const hasAfter = computed(() => props.after !== null && props.after !== undefined)
const beforeLines = computed(() =>
  hasBefore.value
    ? renderJson(props.before, hasAfter.value ? props.after : props.before, 'before')
    : [],
)
const afterLines = computed(() =>
  hasAfter.value
    ? renderJson(props.after, hasBefore.value ? props.before : props.after, 'after')
    : [],
)
const beforeTitle = computed(() =>
  t(hasAfter.value ? 'operations.before' : 'operations.logContent'),
)
const afterTitle = computed(() => t(hasBefore.value ? 'operations.after' : 'operations.logContent'))

function formatted(value: unknown) {
  return JSON.stringify(value, null, 2) ?? 'null'
}

async function copy(side: Side) {
  const value = side === 'before' ? props.before : props.after
  try {
    await navigator.clipboard.writeText(formatted(value))
    copyState.value[side] = 'copied'
    showSuccessToast(t('common.copied'))
  } catch {
    copyState.value[side] = 'idle'
    showErrorToast(t('common.copyFailed'))
  }
}

function copyLabel(side: Side) {
  if (copyState.value[side] === 'copied') return t('common.copied')
  if (side === 'before')
    return t(hasAfter.value ? 'operations.copyBefore' : 'operations.copyContent')
  return t(hasBefore.value ? 'operations.copyAfter' : 'operations.copyContent')
}

function lineLabel(line: JsonLine) {
  return line.kind ? `${t(`operations.changes.${line.kind}`)}：${line.text}` : undefined
}
</script>

<template>
  <div
    v-if="hasBefore || hasAfter"
    class="json-diff"
    :class="{ 'json-diff-single': hasBefore !== hasAfter }"
  >
    <section v-if="hasBefore" class="json-snapshot" :aria-label="beforeTitle">
      <header class="json-snapshot-head">
        <h3>{{ beforeTitle }}</h3>
        <button class="json-copy" type="button" @click="copy('before')">
          <Icon name="copy" :size="14" />
          {{ copyLabel('before') }}
        </button>
      </header>
      <pre class="json-code"><code><span
        v-for="(line, index) in beforeLines"
        :key="index"
        class="json-line"
        :class="line.kind"
        :aria-label="lineLabel(line)"
      >{{ line.text }}
</span></code></pre>
    </section>

    <section v-if="hasAfter" class="json-snapshot" :aria-label="afterTitle">
      <header class="json-snapshot-head">
        <h3>{{ afterTitle }}</h3>
        <button class="json-copy" type="button" @click="copy('after')">
          <Icon name="copy" :size="14" />
          {{ copyLabel('after') }}
        </button>
      </header>
      <pre class="json-code"><code><span
        v-for="(line, index) in afterLines"
        :key="index"
        class="json-line"
        :class="line.kind"
        :aria-label="lineLabel(line)"
      >{{ line.text }}
</span></code></pre>
    </section>
  </div>
  <p v-else class="json-diff-empty">{{ t('operations.noSnapshot') }}</p>
</template>

<style scoped>
.json-diff {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.json-diff-single {
  grid-template-columns: minmax(0, 1fr);
}
.json-snapshot {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: #f7f9fc;
}
.json-snapshot-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 44px;
  padding: 8px 12px 8px 14px;
  border-bottom: 1px solid var(--line);
  background: #fff;
}
.json-snapshot-head h3 {
  color: #3e5369;
  font-size: 13px;
  font-weight: 600;
}
.json-copy {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 7px;
  border: 0;
  border-radius: 5px;
  color: var(--blue);
  background: transparent;
  font-size: 12px;
}
.json-copy:hover {
  background: #edf4ff;
}
.json-copy:focus-visible {
  outline: 3px solid #90b7fb;
  outline-offset: 1px;
}
.json-code {
  max-height: 470px;
  margin: 0;
  padding: 12px 0;
  overflow: auto;
  color: #253a4f;
  background: #f7f9fc;
  font-family: Consolas, 'SFMono-Regular', monospace;
  font-size: 12px;
  line-height: 1.75;
  scrollbar-gutter: stable;
}
.json-code code {
  display: block;
  min-width: max-content;
  font: inherit;
}
.json-line {
  display: block;
  min-height: 21px;
  padding: 0 14px 0 17px;
  white-space: pre;
}
.json-line.changed {
  background: #fff3c9;
  box-shadow: inset 3px 0 #d49922;
}
.json-line.added {
  background: #e8f6ed;
  box-shadow: inset 3px 0 #398a59;
}
.json-line.removed {
  background: #fff0f0;
  box-shadow: inset 3px 0 #c55757;
}
.json-diff-empty {
  padding: 28px 16px;
  color: var(--muted);
  text-align: center;
  font-size: 13px;
}
@media (max-width: 700px) {
  .json-diff {
    grid-template-columns: minmax(0, 1fr);
  }
  .json-code {
    max-height: 320px;
  }
}
</style>
