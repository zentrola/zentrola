<script setup lang="ts">
// 字段旁的圆形提示图标，悬停或键盘聚焦时在右侧显示说明文本。
withDefaults(defineProps<{ text: string; symbol?: string }>(), { symbol: 'i' })
</script>

<template>
  <span
    class="field-help"
    role="img"
    tabindex="0"
    :aria-label="text"
    :title="text"
    :data-tooltip="text"
    @click.prevent
    >{{ symbol }}</span
  >
</template>

<style scoped>
.field-help {
  position: relative;
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 15px;
  height: 15px;
  border: 1px solid #8ca0b2;
  border-radius: 50%;
  color: #60788d;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  cursor: help;
}
.field-help::after {
  position: absolute;
  top: 50%;
  left: calc(100% + 8px);
  z-index: 3;
  width: max-content;
  max-width: 220px;
  padding: 6px 8px;
  border-radius: 5px;
  background: var(--color-sidebar);
  box-shadow: 0 4px 12px rgb(15 23 42 / 18%);
  color: #fff;
  content: attr(data-tooltip);
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
  opacity: 0;
  pointer-events: none;
  text-align: left;
  transform: translate(4px, -50%);
  transition:
    opacity 0.15s,
    transform 0.15s;
}
.field-help:hover::after,
.field-help:focus-visible::after {
  opacity: 1;
  transform: translate(0, -50%);
}
.field-help:focus-visible {
  outline: 2px solid #bfdbfe;
  outline-offset: 2px;
}
</style>
